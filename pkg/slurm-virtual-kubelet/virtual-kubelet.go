package slurm_virtual_kubelet

import (
	"context"
	"crypto/tls"
	"github.com/chriskery/slurm-bridge-operator/apis/kubecluster.org/v1alpha1"
	"github.com/chriskery/slurm-bridge-operator/cmd/slurm-virtual-kubelet/app/options"
	"github.com/chriskery/slurm-bridge-operator/pkg/slurm-virtual-kubelet/events"
	"github.com/chriskery/slurm-bridge-operator/pkg/slurm-virtual-kubelet/manager"
	"github.com/chriskery/slurm-bridge-operator/pkg/workload"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	"github.com/virtual-kubelet/virtual-kubelet/node"
	"github.com/virtual-kubelet/virtual-kubelet/node/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	netutils "k8s.io/utils/net"
	"net"
	"net/http"
	"strconv"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
	kubeinformers "k8s.io/client-go/informers"
	kubeclientset "k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"os"
	"strings"
)

// PreInitRuntimeService will init runtime service before RunKubelet.
func PreInitRuntimeService(_ *v1alpha1.SlurmVirtualKubeletConfiguration) error {
	return nil
}

func newClient(configPath string) (*kubeclientset.Clientset, error) {
	var config *rest.Config

	// Check if the kubeConfig file exists.
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		// Get the kubeconfig from the filepath.
		config, err = clientcmd.BuildConfigFromFlags("", configPath)
		if err != nil {
			return nil, errors.Wrap(err, "error building client config")
		}
	} else {
		// Set to in-cluster config.
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, errors.Wrap(err, "error building in cluster config")
		}
	}

	if masterURI := os.Getenv("MASTER_URI"); masterURI != "" {
		config.Host = masterURI
	}

	return kubeclientset.NewForConfig(config)
}

func NewMainSlurmVirtualKubelet(kubeletServer *options.SlurmVirtualKubeletServer) (*SlurmVirtualKubelet, error) {
	client, err := newClient(kubeletServer.KubeConfig)
	if err != nil {
		return nil, err
	}

	p, err := NewSlurmVirtualKubelet(
		kubeletServer, client,
	)
	if err != nil {
		return nil, err
	}

	return p, nil
}

func NewSlurmVirtualKubelet(kubeletServer *options.SlurmVirtualKubeletServer, client *kubeclientset.Clientset) (*SlurmVirtualKubelet, error) {
	if kubeletServer == nil {
		return nil, errors.New("kubelet server can not be empty")
	}
	if len(kubeletServer.AgentEndpoint) == 0 {
		return nil, errors.New("slurm-agent agent endpoint can not be empty")
	}
	svk := &SlurmVirtualKubelet{KubeletServer: *kubeletServer, Client: client}

	// construct a node reference used for events
	nodeRef := &v1.ObjectReference{
		Kind:      "Node",
		Name:      svk.KubeletServer.NodeName,
		UID:       types.UID(svk.KubeletServer.NodeName),
		Namespace: "",
	}
	svk.nodeRef = nodeRef

	eb := record.NewBroadcaster()
	eb.StartLogging(log.G(context.Background()).Infof)
	eb.StartRecordingToSink(&corev1client.EventSinkImpl{Interface: client.CoreV1().Events(kubeletServer.KubeNamespace)})
	svk.recorder = eb.NewRecorder(scheme.Scheme, v1.EventSource{Component: "slurm-virtual-node"})

	addr := kubeletServer.AgentEndpoint
	if strings.HasSuffix(addr, "sock") {
		addr = "unix://" + addr
	}
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		logrus.Fatalf("can't connect to %s %s", addr, err)
	}
	slurmC := workload.NewWorkloadManagerClient(conn)
	svk.SlurmClient = slurmC

	return svk, nil
}

// SlurmVirtualKubelet is the main kubelet implementation.
type SlurmVirtualKubelet struct {
	KubeletServer options.SlurmVirtualKubeletServer

	// The EventRecorder to use
	recorder record.EventRecorder

	// Reference to this node.
	nodeRef *v1.ObjectReference

	Client *kubeclientset.Clientset

	Provider    SlurmVirtualKubeletProvider
	SlurmClient workload.WorkloadManagerClient
}

func (vk *SlurmVirtualKubelet) ListenAndServeSlurmVirtualKubeletServer() {
	address := netutils.ParseIPSloppy(vk.KubeletServer.Address)
	port := uint(vk.KubeletServer.Port)
	klog.InfoS("Starting to listen", "address", address, "port", port)
	handler := http.NewServeMux()

	podRoutes := api.PodHandlerConfig{
		RunInContainer:   vk.Provider.RunInContainer,
		GetContainerLogs: vk.Provider.GetContainerLogs,
		GetPods:          vk.Provider.GetPods,
	}
	api.AttachPodRoutes(podRoutes, handler, true)
	s := &http.Server{
		Addr:           net.JoinHostPort(address.String(), strconv.FormatUint(uint64(port), 10)),
		Handler:        handler,
		IdleTimeout:    90 * time.Second, // matches http.DefaultTransport keep-alive timeout
		ReadTimeout:    4 * 60 * time.Minute,
		WriteTimeout:   4 * 60 * time.Minute,
		MaxHeaderBytes: 1 << 20,
	}
	if vk.KubeletServer.TLSCertFile != "" && vk.KubeletServer.TLSPrivateKeyFile != "" {
		tlsCfg, err := loadTLSConfig(vk.KubeletServer.TLSCertFile, vk.KubeletServer.TLSPrivateKeyFile)
		if err != nil {
			klog.ErrorS(err, "Failed to load tls config")
			os.Exit(1)
		}
		s.TLSConfig = tlsCfg
		// Passing empty strings as the cert and key files means no
		// cert/keys are specified and GetCertificate in the TLSConfig
		// should be called instead.
		if err = s.ListenAndServeTLS(vk.KubeletServer.TLSCertFile, vk.KubeletServer.TLSPrivateKeyFile); err != nil {
			klog.ErrorS(err, "Failed to listen and serve")
			os.Exit(1)
		}
	} else if err := s.ListenAndServe(); err != nil {
		klog.ErrorS(err, "Failed to listen and serve")
		os.Exit(1)
	}

}

// AcceptedCiphers is the list of accepted TLS ciphers, with known weak ciphers elided
// Note this list should be a moving target.
var AcceptedCiphers = []uint16{
	tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
	tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
	tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,

	tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
	tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
}

func loadTLSConfig(certPath, keyPath string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, errors.Wrap(err, "error loading tls certs")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		CipherSuites: AcceptedCiphers,
	}, nil
}

// BirthCry sends an event that the kubelet has started up.
func (vk *SlurmVirtualKubelet) BirthCry() {
	// Make an event that kubelet restarted.
	vk.recorder.Eventf(vk.nodeRef, v1.EventTypeNormal, events.StartingKubelet, "Starting kubelet.")
}

func (vk *SlurmVirtualKubelet) Run() {
	// Create a shared informer factory for Kubernetes pods in the current namespace (if specified) and scheduled to the current node.
	podInformerFactory := kubeinformers.NewSharedInformerFactoryWithOptions(
		vk.Client,
		vk.KubeletServer.InformerResyncPeriod,
		kubeinformers.WithNamespace(vk.KubeletServer.KubeNamespace),
		kubeinformers.WithTweakListOptions(func(options *metav1.ListOptions) {
			options.FieldSelector = fields.OneTermEqualSelector("spec.nodeName", vk.KubeletServer.NodeName).String()
		}))
	podInformer := podInformerFactory.Core().V1().Pods()

	// Create another shared informer factory for Kubernetes secrets and configmaps (not subject to any selectors).
	scmInformerFactory := kubeinformers.NewSharedInformerFactoryWithOptions(vk.Client, vk.KubeletServer.InformerResyncPeriod)
	// Create a secret informer and a config map informer so we can pass their listers to the resource manager.
	secretInformer := scmInformerFactory.Core().V1().Secrets()
	configMapInformer := scmInformerFactory.Core().V1().ConfigMaps()
	serviceInformer := scmInformerFactory.Core().V1().Services()

	go podInformerFactory.Start(wait.NeverStop)
	go scmInformerFactory.Start(wait.NeverStop)

	rm, err := manager.NewResourceManager(podInformer.Lister(), secretInformer.Lister(), configMapInformer.Lister(), serviceInformer.Lister())
	if err != nil {
		klog.ErrorS(err, "could not create resource manager")
		os.Exit(1)
	}
	vk.Provider = NewSlurmVirtualKubeletProvider(rm, vk)

	if err = options.SetupTracing(&vk.KubeletServer); err != nil {
		klog.ErrorS(err, "could not setup tracing")
		os.Exit(1)
	}

	pNode, err := vk.NewNodeOrDie()
	if err != nil {
		klog.Fatal(err)
	}
	pc, err := node.NewPodController(node.PodControllerConfig{
		PodClient:         vk.Client.CoreV1(),
		PodInformer:       podInformer,
		EventRecorder:     vk.recorder,
		Provider:          &vk.Provider,
		SecretInformer:    secretInformer,
		ConfigMapInformer: configMapInformer,
		ServiceInformer:   serviceInformer,
	})
	if err != nil {
		klog.ErrorS(err, "error setting up pod slurm-bridge-operator")
		os.Exit(1)
	}

	if vk.KubeletServer.StartupTimeout > 0 {
		// If there is a startup timeout, it does two things:
		// 1. It causes the VK to shutdown if we haven't gotten into an operational state in a time period
		// 2. It prevents node advertisement from happening until we're in an operational state
		err = waitFor(vk.KubeletServer.StartupTimeout, pc.Ready())
		if err != nil {
			klog.Fatal(err)
		}
	}
	svkProvider := &SlurmVKNaiveNodeProvider{vk}
	nodeRunner, err := node.NewNodeController(
		svkProvider,
		pNode,
		vk.Client.CoreV1().Nodes(),
		node.WithNodeStatusUpdateErrorHandler(func(ctx context.Context, err error) error {
			if !k8serrors.IsNotFound(err) {
				return err
			}
			newNode := pNode.DeepCopy()
			newNode.ResourceVersion = ""
			_, err = vk.Client.CoreV1().Nodes().Create(context.Background(), newNode, metav1.CreateOptions{})
			if err != nil {
				return err
			}
			return nil
		}),
	)
	if err != nil {
		klog.Fatal(err)
	}

	go func() {
		if err = nodeRunner.Run(context.Background()); err != nil {
			klog.Fatal(err)
		}
	}()
	<-nodeRunner.Ready()

	go func() {
		if err := pc.Run(context.Background(), vk.KubeletServer.PodSyncWorkers); err != nil && !errors.Is(errors.Cause(err), context.Canceled) {
			klog.Fatal(err)
		}
	}()
	<-pc.Ready()
}

func waitFor(time time.Duration, ready <-chan struct{}) error {
	ctx, cancel := context.WithTimeout(context.TODO(), time)
	defer cancel()

	// Wait for the VK / PC close the the ready channel, or time out and return
	log.G(ctx).Info("Waiting for pod slurm-bridge-operator / VK to be ready")

	select {
	case <-ready:
		return nil
	case <-ctx.Done():
		return errors.Wrap(ctx.Err(), "Error while starting up VK")
	}
}

func NewSlurmVirtualKubeletProvider(rm *manager.ResourceManager, vk *SlurmVirtualKubelet) SlurmVirtualKubeletProvider {
	return SlurmVirtualKubeletProvider{resourceManager: rm, vk: vk}
}
