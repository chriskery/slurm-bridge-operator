/*
Copyright 2015 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package options contains all of the primary arguments for a kubelet.
package options

import (
	"fmt"
	"github.com/chriskery/slurm-bridge-operator/pkg/slurm-virtual-kubelet/apis"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/virtual-kubelet/virtual-kubelet/errdefs"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	_ "net/http/pprof" // Enable pprof HTTP handlers.
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/pflag"

	vklabels "github.com/chriskery/slurm-bridge-operator/pkg/slurm-virtual-kubelet/apis/labels"
	vkvalidation "github.com/chriskery/slurm-bridge-operator/pkg/slurm-virtual-kubelet/apis/validation"

	utilflag "github.com/chriskery/slurm-bridge-operator/pkg/common/flag"
	"k8s.io/apimachinery/pkg/util/sets"
	cliflag "k8s.io/component-base/cli/flag"
)

type SlurmVirtualKubeletFlags struct {
	KubeConfig string

	// NodeName is the hostname used to identify the kubelet instead
	// of the actual hostname.
	NodeName string
	// NodeIP is IP address of the node.
	// If set, kubelet will use this IP address for the node.
	NodeIP string

	// Node Labels are the node labels to add when registering the node in the cluster
	NodeLabels map[string]string

	// The Kubelet will load its initial configuration from this file.
	// The path may be absolute or relative; relative paths are under the Kubelet's current working directory.
	// Omit this flag to use the combination of built-in default configuration values and flags.
	VirtualKubeletConfigFile string

	// Namespace to watch for pods and other resources
	KubeNamespace string

	// Operating system to run pods for
	OperatingSystem string

	Provider           string
	ProviderConfigPath string

	AgentEndpoint  string
	SlurmPartition string

	TaintKey     string
	TaintEffect  string
	DisableTaint bool

	MetricsAddr string

	// Number of workers to use to handle pod notifications
	PodSyncWorkers       int
	InformerResyncPeriod time.Duration

	// Use node leases when supported by Kubernetes (instead of node status updates)
	EnableNodeLease bool

	TraceExporters  []string
	TraceSampleRate string
	TraceConfig     TracingExporterOptions

	// Startup Timeout is how long to wait for the kubelet to start
	StartupTimeout time.Duration

	Version string
}

// NewSlurmVirtualKubeletFlags will create a new SlurmVirtualKubeletFlags with default values
func NewSlurmVirtualKubeletFlags() *SlurmVirtualKubeletFlags {
	slurmVirtualKubeletFlags := &SlurmVirtualKubeletFlags{
		NodeLabels: map[string]string{},
	}
	setDefaultSlurmVirtualKubeletFlags(slurmVirtualKubeletFlags)
	return slurmVirtualKubeletFlags
}

// Defaults for root command options
const (
	DefaultNodeName             = "virtual-kubelet"
	DefaultOperatingSystem      = "Linux"
	DefaultInformerResyncPeriod = 1 * time.Minute
	DefaultMetricsAddr          = ":10255"
	DefaultListenPort           = 10250
	DefaultPodSyncWorkers       = 10
	DefaultKubeNamespace        = corev1.NamespaceAll

	DefaultTaintEffect = string(corev1.TaintEffectNoSchedule)
	DefaultTaintKey    = "virtual-kubelet.io/provider"
)

func setDefaultSlurmVirtualKubeletFlags(c *SlurmVirtualKubeletFlags) {
	if c.OperatingSystem == "" {
		c.OperatingSystem = DefaultOperatingSystem
	}

	if c.NodeName == "" {
		c.NodeName = getEnvOrDefault("DEFAULT_NODE_NAME", DefaultNodeName)
	}

	if c.InformerResyncPeriod == 0 {
		c.InformerResyncPeriod = DefaultInformerResyncPeriod
	}

	if c.MetricsAddr == "" {
		c.MetricsAddr = DefaultMetricsAddr
	}

	if c.PodSyncWorkers == 0 {
		c.PodSyncWorkers = DefaultPodSyncWorkers
	}

	if c.KubeNamespace == "" {
		c.KubeNamespace = DefaultKubeNamespace
	}

	if c.TaintKey == "" {
		c.TaintKey = DefaultTaintKey
	}
	if c.TaintEffect == "" {
		c.TaintEffect = DefaultTaintEffect
	}

	if c.KubeConfig == "" {
		c.KubeConfig = os.Getenv("KUBECONFIG")
		if c.KubeConfig == "" {
			home, _ := homedir.Dir()
			if home != "" {
				c.KubeConfig = filepath.Join(home, ".kube", "config")
			}
		}
	}

}

func getEnvOrDefault(key, defaultValue string) string {
	value, found := os.LookupEnv(key)
	if found {
		return value
	}
	return defaultValue
}

// ValidateKubeletFlags validates Kubelet's configuration flags and returns an error if they are invalid.
func ValidateKubeletFlags(f *SlurmVirtualKubeletFlags) error {
	unknownLabels := sets.NewString()
	invalidLabelErrs := make(map[string][]string)
	for k, v := range f.NodeLabels {
		if isKubernetesLabel(k) && !vklabels.IsKubeletLabel(k) {
			unknownLabels.Insert(k)
		}

		if errs := validation.IsQualifiedName(k); len(errs) > 0 {
			invalidLabelErrs[k] = append(invalidLabelErrs[k], errs...)
		}
		if errs := validation.IsValidLabelValue(v); len(errs) > 0 {
			invalidLabelErrs[v] = append(invalidLabelErrs[v], errs...)
		}
	}
	if len(unknownLabels) > 0 {
		return fmt.Errorf("unknown 'kubernetes.io' or 'k8s.io' labels specified with --node-labels: %v\n--node-labels in the 'kubernetes.io' namespace must begin with an allowed prefix (%s) or be in the specifically allowed set (%s)",
			unknownLabels.List(), strings.Join(vklabels.KubeletLabelNamespaces(), ", "), strings.Join(vklabels.KubeletLabels(), ", "))
	}
	if len(invalidLabelErrs) > 0 {
		var labelErrs []string
		for k, v := range invalidLabelErrs {
			labelErrs = append(labelErrs, fmt.Sprintf("'%s' - %s", k, strings.Join(v, ", ")))
		}
		return fmt.Errorf("invalid node labels: %s", strings.Join(labelErrs, "; "))
	}
	if f.PodSyncWorkers == 0 {
		return errdefs.InvalidInput("pod sync workers must be greater than 0")
	}
	return nil
}

func isKubernetesLabel(key string) bool {
	namespace := getLabelNamespace(key)
	if namespace == "kubernetes.io" || strings.HasSuffix(namespace, ".kubernetes.io") {
		return true
	}
	if namespace == "k8s.io" || strings.HasSuffix(namespace, ".k8s.io") {
		return true
	}
	return false
}

func getLabelNamespace(key string) string {
	if parts := strings.SplitN(key, "/", 2); len(parts) == 2 {
		return parts[0]
	}
	return ""
}

// NewSlurmVirtualKubeletConfiguration will create a new KubeletConfiguration with default values
func NewSlurmVirtualKubeletConfiguration() (*apis.SlurmVirtualKubeletConfiguration, error) {
	config := &apis.SlurmVirtualKubeletConfiguration{}
	err := setDefaultSlurmVirtualKubeletConfiguration(config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func setDefaultSlurmVirtualKubeletConfiguration(c *apis.SlurmVirtualKubeletConfiguration) error {
	if c.Port == 0 {
		if kp := os.Getenv("KUBELET_PORT"); kp != "" {
			p, err := strconv.Atoi(kp)
			if err != nil {
				return errors.Wrap(err, "error parsing KUBELET_PORT environment variable")
			}
			c.Port = int32(p)
		} else {
			c.Port = DefaultListenPort
		}
	}
	return nil
}

// SlurmVirtualKubeletServer encapsulates all of the parameters necessary for starting up
// a kubelet. These can either be set via command line or directly.
type SlurmVirtualKubeletServer struct {
	SlurmVirtualKubeletFlags
	apis.SlurmVirtualKubeletConfiguration
}

// ValidateKubeletServer validates configuration of SlurmVirtualKubeletServer and returns an error if the input configuration is invalid.
func ValidateKubeletServer(s *SlurmVirtualKubeletServer) error {
	// please add any KubeletConfiguration validation to the kubeletconfigvalidation.ValidateKubeletConfiguration function
	if err := vkvalidation.ValidateSlurmVirtualKubeletConfiguration(&s.SlurmVirtualKubeletConfiguration); err != nil {
		return err
	}
	if err := ValidateKubeletFlags(&s.SlurmVirtualKubeletFlags); err != nil {
		return err
	}
	return nil
}

// AddFlags adds flags for a specific SlurmVirtualKubeletServer to the specified FlagSet
func (s *SlurmVirtualKubeletServer) AddFlags(fs *pflag.FlagSet) {
	s.SlurmVirtualKubeletFlags.AddFlags(fs)
	AddKubeletConfigFlags(fs, &s.SlurmVirtualKubeletConfiguration)
}

// AddFlags adds flags for a specific SlurmVirtualKubeletFlags to the specified FlagSet
func (f *SlurmVirtualKubeletFlags) AddFlags(mainfs *pflag.FlagSet) {
	fs := pflag.NewFlagSet("", pflag.ExitOnError)
	defer func() {
		// Unhide deprecated flags. We want deprecated flags to show in Kubelet help.
		// We have some hidden flags, but we might as well unhide these when they are deprecated,
		// as silently deprecating and removing (even hidden) things is unkind to people who use them.
		fs.VisitAll(func(f *pflag.Flag) {
			if len(f.Deprecated) > 0 {
				f.Hidden = false
			}
		})
		mainfs.AddFlagSet(fs)
	}()

	fs.StringVar(&f.VirtualKubeletConfigFile, "config", f.VirtualKubeletConfigFile, "The Kubelet will load its initial configuration from this file. The path may be absolute or relative; relative paths start at the Kubelet's current working directory. Omit this flag to use the built-in default configuration values. Command-line flags override configuration from this file.")
	fs.StringVar(&f.KubeConfig, "kubeconfig", f.KubeConfig, "Path to a kubeconfig file, specifying how to connect to the API server. Providing --kubeconfig enables API server mode, omitting --kubeconfig enables standalone mode.")
	fs.StringVar(&f.NodeName, "hostname-override", f.NodeName, "If non-empty, will use this string as identification instead of the actual hostname. If --cloud-provider is set, the cloud provider determines the name of the node (consult cloud provider documentation to determine if and how the hostname is used).")
	fs.StringVar(&f.NodeIP, "node-ip", f.NodeIP, "IP address (or comma-separated dual-stack IP addresses) of the node. If unset, kubelet will use the node's default IPv4 address, if any, or its default IPv6 address if it has no IPv4 addresses. You can pass '::' to make it prefer the default IPv6 address rather than the default IPv4 address.")
	fs.StringVar(&f.AgentEndpoint, "agent-endpoint", f.AgentEndpoint, "slurm-agent agent endpoint addr.")
}

// AddKubeletConfigFlags adds flags for a specific kubeletconfig.KubeletConfiguration to the specified FlagSet
func AddKubeletConfigFlags(mainfs *pflag.FlagSet, c *apis.SlurmVirtualKubeletConfiguration) {
	fs := pflag.NewFlagSet("", pflag.ExitOnError)
	defer func() {
		// All KubeletConfiguration flags are now deprecated, and any new flags that point to
		// KubeletConfiguration fields are deprecated-on-creation. When removing flags at the end
		// of their deprecation period, be careful to check that they have *actually* been deprecated
		// members of the KubeletConfiguration for the entire deprecation period:
		// e.g. if a flag was added after this deprecation function, it may not be at the end
		// of its lifetime yet, even if the rest are.
		deprecated := "This parameter should be set via the config file specified by the Kubelet's --config flag. See https://kubernetes.io/docs/tasks/administer-cluster/kubelet-config-file/ for more information."
		// Some flags are permanently (?) meant to be available. In
		// Kubernetes 1.23, the following options were added to
		// LoggingConfiguration (long-term goal, more complete
		// configuration file) but deprecating the flags seemed
		// premature.
		notDeprecated := map[string]bool{
			"v":                   true,
			"vmodule":             true,
			"log-flush-frequency": true,
			"provider-id":         true,
		}
		fs.VisitAll(func(f *pflag.Flag) {
			if notDeprecated[f.Name] {
				return
			}
			f.Deprecated = deprecated
		})
		mainfs.AddFlagSet(fs)
	}()

	fs.BoolVar(&c.EnableServer, "enable-server", c.EnableServer, "Enable the Kubelet's server")

	fs.StringVar(&c.StaticPodPath, "pod-manifest-path", c.StaticPodPath, "Path to the directory containing static pod files to run, or the path to a single static pod file. Files starting with dots will be ignored.")
	fs.DurationVar(&c.SyncFrequency.Duration, "sync-frequency", c.SyncFrequency.Duration, "Max period between synchronizing running containers and config")
	fs.DurationVar(&c.FileCheckFrequency.Duration, "file-check-frequency", c.FileCheckFrequency.Duration, "Duration between checking config files for new data")
	fs.DurationVar(&c.HTTPCheckFrequency.Duration, "http-check-frequency", c.HTTPCheckFrequency.Duration, "Duration between checking http for new data")
	fs.StringVar(&c.StaticPodURL, "manifest-url", c.StaticPodURL, "URL for accessing additional Pod specifications to run")
	fs.Var(cliflag.NewColonSeparatedMultimapStringString(&c.StaticPodURLHeader), "manifest-url-header", "Comma-separated list of HTTP headers to use when accessing the url provided to --manifest-url. Multiple headers with the same name will be added in the same order provided. This flag can be repeatedly invoked. For example: --manifest-url-header 'a:hello,b:again,c:world' --manifest-url-header 'b:beautiful'")
	fs.Int32Var(&c.Port, "port", c.Port, "The port for the Kubelet to serve on.")
	fs.Int32Var(&c.ReadOnlyPort, "read-only-port", c.ReadOnlyPort, "The read-only port for the Kubelet to serve on with no authentication/authorization (set to 0 to disable)")
	fs.Var(&utilflag.IPVar{Val: &c.Address}, "address", "The IP address for the Kubelet to serve on (set to '0.0.0.0' or '::' for listening on all interfaces and IP address families)")

}
