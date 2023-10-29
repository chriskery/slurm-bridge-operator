/*
Copyright 2021 The Kubernetes Authors.

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

// Package app makes it easy to create a kubelet server for various contexts.
package app

import (
	"context"
	"fmt"
	"github.com/chriskery/slurm-bridge-operator/cmd/slurm-virtual-kubelet/app/options"
	utilfs "github.com/chriskery/slurm-bridge-operator/pkg/filesystem"
	"github.com/chriskery/slurm-bridge-operator/pkg/slurm-virtual-kubelet"
	"github.com/chriskery/slurm-bridge-operator/pkg/slurm-virtual-kubelet/apis"
	"github.com/chriskery/slurm-bridge-operator/pkg/slurm-virtual-kubelet/configfiles"
	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"io"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	genericapiserver "k8s.io/apiserver/pkg/server"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	cliflag "k8s.io/component-base/cli/flag"
	logsapi "k8s.io/component-base/logs/api/v1"
	"k8s.io/component-base/version/verflag"
	nodeutil "k8s.io/component-helpers/node/util"
	"k8s.io/klog/v2"
	"os"
	"path/filepath"
)

func init() {
	utilruntime.Must(logsapi.AddFeatureGates(utilfeature.DefaultMutableFeatureGate))
}

const (
	componentSlurmVirtualKubelet = "slurm-virtual-kubelet"
)

// NewSlurmVirtualKubeltCommand creates a *cobra.Command object with default parameters
func NewSlurmVirtualKubeltCommand() *cobra.Command {
	cleanFlagSet := pflag.NewFlagSet(componentSlurmVirtualKubelet, pflag.ContinueOnError)
	cleanFlagSet.SetNormalizeFunc(cliflag.WordSepNormalizeFunc)
	slurmVKFlags := options.NewSlurmVirtualKubeletFlags()

	kubeletConfig, err := options.NewSlurmVirtualKubeletConfiguration()
	// programmer error
	if err != nil {
		klog.ErrorS(err, "Failed to create a new slurm virtual kubelet configuration")
		os.Exit(1)
	}

	cmd := &cobra.Command{
		Use: componentSlurmVirtualKubelet,
		Long: `The kubelet is the primary "node agent" that runs on each
node. It can register the node with the apiserver using one of: the hostname; a flag to
override the hostname; or specific logic for a cloud provider.

The kubelet works in terms of a PodSpec. A PodSpec is a YAML or JSON object
that describes a pod. The kubelet takes a set of PodSpecs that are provided through
various mechanisms (primarily through the apiserver) and ensures that the containers
described in those PodSpecs are running and healthy. The kubelet doesn't manage
containers which were not created by Kubernetes.

Other than from an PodSpec from the apiserver, there are two ways that a container
manifest can be provided to the Kubelet.

File: Path passed as a flag on the command line. Files under this path will be monitored
periodically for updates. The monitoring period is 20s by default and is configurable
via a flag.

HTTP endpoint: HTTP endpoint passed as a parameter on the command line. This endpoint
is checked every 20 seconds (also configurable with a flag).`,
		// The Kubelet has special flag parsing requirements to enforce flag precedence rules,
		// so we do all our parsing manually in Run, below.
		// DisableFlagParsing=true provides the full set of flags passed to the kubelet in the
		// `args` arg to Run, without Cobra's interference.
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// initial flag parse, since we disable cobra's flag parsing
			if err := cleanFlagSet.Parse(args); err != nil {
				return fmt.Errorf("failed to parse kubelet flag: %w", err)
			}

			// check if there are non-flag arguments in the command line
			cmds := cleanFlagSet.Args()
			if len(cmds) > 0 {
				return fmt.Errorf("unknown command %+s", cmds[0])
			}

			// short-circuit on help
			help, err := cleanFlagSet.GetBool("help")
			if err != nil {
				return errors.New(`"help" flag is non-bool, programmer error, please correct`)
			}
			if help {
				return cmd.Help()
			}

			// short-circuit on verflag
			verflag.PrintAndExitIfRequested()

			// validate the initial SlurmVirtualKubeletFlags
			if err := options.ValidateKubeletFlags(slurmVKFlags); err != nil {
				return fmt.Errorf("failed to validate kubelet flags: %w", err)
			}

			// load kubelet config file, if provided
			if len(slurmVKFlags.VirtualKubeletConfigFile) > 0 {
				kubeletConfig, err = loadConfigFile(slurmVKFlags.VirtualKubeletConfigFile)
				if err != nil {
					return fmt.Errorf("failed to load kubelet config file, path: %s, error: %w", slurmVKFlags.VirtualKubeletConfigFile, err)
				}
			}

			if len(slurmVKFlags.VirtualKubeletConfigFile) > 0 {
				// We must enforce flag precedence by re-parsing the command line into the new object.
				// This is necessary to preserve backwards-compatibility across binary upgrades.
				// See issue #56171 for more details.
				if err := kubeletConfigFlagPrecedence(kubeletConfig, args); err != nil {
					return fmt.Errorf("failed to precedence kubeletConfigFlag: %w", err)
				}
			}

			cliflag.PrintFlags(cleanFlagSet)

			// construct a SlurmVirtualKubeletServer from slurmVKFlags and kubeletConfig
			kubeletServer := &options.SlurmVirtualKubeletServer{
				SlurmVirtualKubeletFlags:         *slurmVKFlags,
				SlurmVirtualKubeletConfiguration: *kubeletConfig,
			}

			if err := checkPermissions(); err != nil {
				klog.ErrorS(err, "kubelet running with insufficient permissions")
			}

			// log the kubelet's config for inspection
			klog.V(5).InfoS("KubeletConfiguration", "configuration", klog.Format(kubeletServer.SlurmVirtualKubeletConfiguration))

			// set up signal context for kubelet shutdown
			ctx := genericapiserver.SetupSignalContext()
			// run the kubelet
			return Run(ctx, kubeletServer)
		},
	}

	// keep cleanFlagSet separate, so Cobra doesn't pollute it with the global flags
	slurmVKFlags.AddFlags(cleanFlagSet)
	options.AddKubeletConfigFlags(cleanFlagSet, kubeletConfig)
	options.AddGlobalFlags(cleanFlagSet)
	cleanFlagSet.BoolP("help", "h", false, fmt.Sprintf("help for %s", cmd.Name()))

	// ugly, but necessary, because Cobra's default UsageFunc and HelpFunc pollute the flagset with global flags
	const usageFmt = "Usage:\n  %s\n\nFlags:\n%s"
	cmd.SetUsageFunc(func(cmd *cobra.Command) error {
		fmt.Fprintf(cmd.OutOrStderr(), usageFmt, cmd.UseLine(), cleanFlagSet.FlagUsagesWrapped(2))
		return nil
	})
	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		fmt.Fprintf(cmd.OutOrStdout(), "%s\n\n"+usageFmt, cmd.Long, cmd.UseLine(), cleanFlagSet.FlagUsagesWrapped(2))
	})

	return cmd
}

// newFlagSetWithGlobals constructs a new pflag.FlagSet with global flags registered
// on it.
func newFlagSetWithGlobals() *pflag.FlagSet {
	fs := pflag.NewFlagSet("", pflag.ExitOnError)
	// set the normalize func, similar to k8s.io/component-base/cli//flags.go:InitFlags
	fs.SetNormalizeFunc(cliflag.WordSepNormalizeFunc)
	// explicitly add flags from libs that register global flags
	options.AddGlobalFlags(fs)
	return fs
}

// newFakeFlagSet constructs a pflag.FlagSet with the same flags as fs, but where
// all values have noop Set implementations
func newFakeFlagSet(fs *pflag.FlagSet) *pflag.FlagSet {
	ret := pflag.NewFlagSet("", pflag.ExitOnError)
	ret.SetNormalizeFunc(fs.GetNormalizeFunc())
	fs.VisitAll(func(f *pflag.Flag) {
		ret.VarP(cliflag.NoOp{}, f.Name, f.Shorthand, f.Usage)
	})
	return ret
}

// kubeletConfigFlagPrecedence re-parses flags over the KubeletConfiguration object.
// We must enforce flag precedence by re-parsing the command line into the new object.
// This is necessary to preserve backwards-compatibility across binary upgrades.
// See issue #56171 for more details.
func kubeletConfigFlagPrecedence(kc *apis.SlurmVirtualKubeletConfiguration, args []string) error {
	// We use a throwaway kubeletFlags and a fake global flagset to avoid double-parses,
	// as some Set implementations accumulate values from multiple flag invocations.
	fs := newFakeFlagSet(newFlagSetWithGlobals())
	// register throwaway SlurmVirtualKubeletFlags
	options.NewSlurmVirtualKubeletFlags().AddFlags(fs)
	// register new KubeletConfiguration
	options.AddKubeletConfigFlags(fs, kc)
	// avoid duplicate printing the flag deprecation warnings during re-parsing
	fs.SetOutput(io.Discard)
	// re-parse flags
	if err := fs.Parse(args); err != nil {
		return err
	}
	return nil
}

func loadConfigFile(name string) (*apis.SlurmVirtualKubeletConfiguration, error) {
	const errFmt = "failed to load Kubelet config file %s, error %v"
	// compute absolute path based on current working dir
	kubeletConfigFile, err := filepath.Abs(name)
	if err != nil {
		return nil, fmt.Errorf(errFmt, name, err)
	}
	loader, err := configfiles.NewFsLoader(&utilfs.DefaultFs{}, kubeletConfigFile)
	if err != nil {
		return nil, fmt.Errorf(errFmt, name, err)
	}
	kc, err := loader.Load()
	if err != nil {
		return nil, fmt.Errorf(errFmt, name, err)
	}

	return kc, err
}

// Run runs the specified SlurmVirtualKubeletServer with the given Dependencies. This should never exit.
// The kubeDeps argument may be nil - if so, it is initialized from the settings on SlurmVirtualKubeletServer.
// Otherwise, the caller is assumed to have set up the Dependencies object and a default one will
// not be generated.
func Run(ctx context.Context, s *options.SlurmVirtualKubeletServer) error {
	klog.InfoS("Golang settings", "GOGC", os.Getenv("GOGC"), "GOMAXPROCS", os.Getenv("GOMAXPROCS"), "GOTRACEBACK", os.Getenv("GOTRACEBACK"))
	if err := run(ctx, s); err != nil {
		return fmt.Errorf("failed to run Kubelet: %w", err)
	}
	return nil
}

func run(ctx context.Context, s *options.SlurmVirtualKubeletServer) (err error) {
	// validate the initial SlurmVirtualKubeletServer (we set feature gates first, because this validation depends on feature gates)
	if err = options.ValidateKubeletServer(s); err != nil {
		return err
	}

	// About to get clients and such, detect standaloneMode
	standaloneMode := true
	if len(s.KubeConfig) > 0 {
		standaloneMode = false
	}

	hostName, err := nodeutil.GetHostname(s.NodeName)
	if err != nil {
		return err
	}
	s.NodeName = hostName

	// if in standalone mode, indicate as much by setting all clients to nil
	if standaloneMode {
		klog.InfoS("Standalone mode, no API client")
	}
	// if in standalone mode, indicate as much by setting all clients to nil

	if err := slurm_virtual_kubelet.PreInitRuntimeService(&s.SlurmVirtualKubeletConfiguration); err != nil {
		return err
	}

	if err := RunVirtualKubelet(s); err != nil {
		return err
	}
	// If systemd is used, notify it that we have started
	go daemon.SdNotify(false, "READY=1")

	select {
	case <-ctx.Done():
		klog.V(5).InfoS("shutting down for slurm virtual kubelet: ", s.NodeName)
		break
	}

	return nil
}

// RunVirtualKubelet is responsible for setting up and running a kubelet.  It is used in three different applications:
//
//	1 Integration tests
//	2 Kubelet binary
//	3 Standalone 'kubernetes' binary
//
// Eventually, #2 will be replaced with instances of #3
func RunVirtualKubelet(kubeServer *options.SlurmVirtualKubeletServer) error {
	vk, err := createAndInitVirtualKubelet(kubeServer)
	if err != nil {
		return err
	}

	startVirtualKubelet(vk)
	klog.InfoS("Started slurm virtual kubelet")
	return nil
}

func startVirtualKubelet(vk *slurm_virtual_kubelet.SlurmVirtualKubelet) {
	// start the kubelet
	go vk.Run()

	go vk.ListenAndServeSlurmVirtualKubeletServer()
}

func createAndInitVirtualKubelet(kubeServer *options.SlurmVirtualKubeletServer) (*slurm_virtual_kubelet.SlurmVirtualKubelet, error) {
	vk, err := slurm_virtual_kubelet.NewMainSlurmVirtualKubelet(kubeServer)
	if err != nil {
		return nil, err
	}
	vk.BirthCry()
	return vk, nil
}
