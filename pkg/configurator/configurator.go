package configurator

import (
	"context"
	"fmt"
	"github.com/chriskery/slurm-bridge-operator/pkg/common/util"
	"github.com/chriskery/slurm-bridge-operator/pkg/workload"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubeclientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sync"
	"time"
)

const (
	ControllerName = "slurm-agent-bridge-configurator"
)

func NewConfiguratorOrDie(mgr manager.Manager, slurmClient workload.WorkloadManagerClient, addr string) *SlurmBridgeConfigurator {
	namespace := os.Getenv("NAMESPACE")
	if len(namespace) == 0 {
		namespace = "default"
	}

	r := &SlurmBridgeConfigurator{
		Client:             mgr.GetClient(),
		Scheme:             mgr.GetScheme(),
		recorder:           mgr.GetEventRecorderFor(ControllerName),
		apiReader:          mgr.GetAPIReader(),
		Log:                log.Log,
		WorkQueue:          &util.FakeWorkQueue{},
		SlurmClient:        slurmClient,
		SlurmAgentEndpoint: addr,
		VKNodeNamespace:    namespace,
		VKKubeletImage:     os.Getenv("KUBELET_IMAGE"),
		VKServiceAccount:   os.Getenv("SERVICE_ACCOUNT"),
		ResultImage:        os.Getenv("RESULTS_IMAGE"),
	}

	if len(r.VKKubeletImage) == 0 {
		logrus.Fatalf("slurm-agent virtual kubelet image can not be empty, please set $KUBELET_IMAGE")
	}
	cfg := mgr.GetConfig()
	kubeClientSet := kubeclientset.NewForConfigOrDie(cfg)
	r.KubeClientSet = kubeClientSet

	return r
}

// SlurmBridgeConfigurator reconciles a for slurm-agent partitions object
type SlurmBridgeConfigurator struct {
	client.Client
	Scheme *runtime.Scheme

	recorder  record.EventRecorder
	apiReader client.Reader
	Log       logr.Logger

	// WorkQueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	WorkQueue workqueue.RateLimitingInterface

	// Recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	Recorder record.EventRecorder

	// KubeClientSet is a standard kubernetes clientset.
	KubeClientSet kubeclientset.Interface

	SlurmClient        workload.WorkloadManagerClient
	SlurmAgentEndpoint string

	VKNodeNamespace  string
	VKServiceAccount string
	VKKubeletImage   string

	ResultImage string
}

func (c *SlurmBridgeConfigurator) WatchPartitions(ctx context.Context, wg *sync.WaitGroup, updateInterval time.Duration) {
	defer wg.Done()

	t := time.NewTicker(updateInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			// getting SLURM partitions
			partitionsResp, err := c.SlurmClient.Partitions(context.Background(), &workload.PartitionsRequest{})
			if err != nil {
				logrus.Printf("Can't get partitions %s", err)
				continue
			}

			err = c.Reconcile(partitionsResp)
			if err != nil {
				logrus.Error(err)
				continue
			}
		}
	}
}

func (c *SlurmBridgeConfigurator) Reconcile(partitionsResp *workload.PartitionsResponse) error {
	// gettings k8s nodes
	nodes, err := c.KubeClientSet.CoreV1().Nodes().List(
		context.Background(),
		metav1.ListOptions{
			LabelSelector: VKNodeLabel,
		})
	if err != nil {
		return errors.Errorf("Can't get virtual nodes %s", err)
	}
	// extract partition names from k8s nodes
	nNames := partitionNames(nodes.Items)

	// check which partitions are not yet represented in k8s
	partitionToCreate := notIn(partitionsResp.Partition, nNames)

	// creating pods for that partitions
	if err = c.ReconcileCreateNodeForPartition(partitionToCreate); err != nil {
		return errors.Errorf("Can't create partitions  %s", err)
	}

	// some partitions can be deleted from SLURM, so we need to delete pods
	// which represent those deleted partitions
	nodesToDelete := notIn(nNames, partitionsResp.Partition)
	// creating pods for that partitions
	if err = c.ReconcileDeleteControllingPod(nodesToDelete); err != nil {
		return errors.Errorf("Can't delete controlling pod %s", err)
	}
	return nil
}

func (c *SlurmBridgeConfigurator) ReconcileCreateNodeForPartition(partitions []string) error {
	for _, p := range partitions {
		_, err := c.KubeClientSet.CoreV1().Pods(c.VKNodeNamespace).Get(context.Background(),
			partitionNodeName(p), metav1.GetOptions{})
		if err == nil {
			continue
		}
		if !k8serrors.IsNotFound(err) {
			return err
		} else {
			logrus.Printf("Creating pod for %s partition in %s namespace", p, c.VKNodeNamespace)
			_, err = c.KubeClientSet.CoreV1().Pods(c.VKNodeNamespace).Create(context.Background(),
				c.virtualKubeletPodTemplate(p), metav1.CreateOptions{})
			if err != nil {
				return errors.Wrapf(err, "could not create pod for %s partition", p)
			}
		}
	}

	return nil
}

func (c *SlurmBridgeConfigurator) ReconcileDeleteControllingPod(nodes []string) error {
	for _, n := range nodes {
		nodeName := partitionNodeName(n)
		logrus.Printf("Deleting pod %s in %s namespace", nodeName, c.VKNodeNamespace)
		err := c.KubeClientSet.CoreV1().Pods(c.VKNodeNamespace).Delete(context.Background(), nodeName,
			metav1.DeleteOptions{})
		if err != nil {
			return errors.Wrapf(err, "could not delete pod %s", nodeName)
		}
	}
	return nil
}

// virtualKubeletPodTemplate returns filled pod model ready to be created in k8s.
// Kubelet pod will create virtual node that will be responsible for handling Slurm jobs.
func (c *SlurmBridgeConfigurator) virtualKubeletPodTemplate(partitionName string) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      partitionNodeName(partitionName),
			Namespace: c.VKNodeNamespace,
		},
		Spec: v1.PodSpec{
			ServiceAccountName: c.VKServiceAccount,
			RestartPolicy:      v1.RestartPolicyAlways,
			Containers: []v1.Container{
				{
					Name:            "slurm-agent-virtual-kubelet",
					Image:           c.VKKubeletImage,
					ImagePullPolicy: v1.PullAlways,
					Args: []string{
						"--nodename", partitionNodeName(partitionName),
						"--partition", partitionName,
						"--endpoint", c.SlurmAgentEndpoint,
					},
					Ports: []v1.ContainerPort{
						{
							Name:          "metrics",
							ContainerPort: 10255,
						},
					},
					Env: []v1.EnvVar{
						{
							Name: "VK_HOST_NAME",
							ValueFrom: &v1.EnvVarSource{
								FieldRef: &v1.ObjectFieldSelector{
									FieldPath: "spec.nodeName",
								},
							},
						},
						{
							Name: "VK_POD_NAME",
							ValueFrom: &v1.EnvVarSource{
								FieldRef: &v1.ObjectFieldSelector{
									FieldPath: "metadata.name",
								},
							},
						},
						{
							Name: "VKUBELET_POD_IP",
							ValueFrom: &v1.EnvVarSource{
								FieldRef: &v1.ObjectFieldSelector{
									FieldPath: "status.podIP",
								},
							},
						},
						{
							Name:  "PARTITION",
							Value: partitionName,
						},
						{
							Name:  "AGNET_ENDPOINT",
							Value: c.SlurmAgentEndpoint,
						},
						{
							Name:  "APISERVER_CERT_LOCATION",
							Value: "/kubelet.crt",
						},
						{
							Name:  "APISERVER_KEY_LOCATION",
							Value: "/kubelet.key",
						},
						//{
						//	Name:  "RESULTS_IMAGE",
						//	Value: resultsImage,
						//},
					},
					VolumeMounts: []v1.VolumeMount{
						//{
						//	Name:      "syslurm-mount",
						//	MountPath: "/syslurm",
						//},
						{
							Name:      "kubelet-crt",
							MountPath: "/kubelet.crt",
						},
						{
							Name:      "kubelet-key",
							MountPath: "/kubelet.key",
						},
					},
				},
			},
			Volumes: []v1.Volume{
				//{
				//	Name: "syslurm-mount", // directory with red-box socket
				//	VolumeSource: v1.VolumeSource{
				//		HostPath: &v1.HostPathVolumeSource{
				//			Path: "/var/run/syslurm",
				//			Type: &[]v1.HostPathType{v1.HostPathDirectory}[0],
				//		},
				//	},
				//},
				{
					Name: "kubelet-crt", // we need certificates for pod rest api, k8s gets pods logruss via rest api
					VolumeSource: v1.VolumeSource{
						HostPath: &v1.HostPathVolumeSource{
							Path: "/var/lib/kubelet/pki/kubelet.crt",
							Type: &[]v1.HostPathType{v1.HostPathFile}[0],
						},
					},
				},
				{
					Name: "kubelet-key",
					VolumeSource: v1.VolumeSource{
						HostPath: &v1.HostPathVolumeSource{
							Path: "/var/lib/kubelet/pki/kubelet.key",
							Type: &[]v1.HostPathType{v1.HostPathFile}[0],
						},
					},
				},
			},
		},
	}
}

func (c *SlurmBridgeConfigurator) InitVKServiceAccount() {

}

// partitionNames extracts slurm-agent partition name from k8s node labels
func partitionNames(nodes []v1.Node) []string {
	names := make([]string, 0)
	for _, n := range nodes {
		if l, ok := n.Labels["kubecluster.org/partition"]; ok {
			names = append(names, l)
		}
	}

	return names
}

// notIn returns elements from s1 which are not in s2
func notIn(s1, s2 []string) []string {
	notIn := make([]string, 0)
	for _, e1 := range s1 {
		if contains(s2, e1) {
			continue
		}
		notIn = append(notIn, e1)
	}

	return notIn
}

// contains checks if elements presents in s
func contains(s []string, e string) bool {
	for _, e1 := range s {
		if e == e1 {
			return true
		}
	}

	return false
}

// partitionNodeName forms partition name that will be used as pod and node name in k8s
func partitionNodeName(partition string) string {
	return fmt.Sprintf("slurm-partition-%s", partition)
}
