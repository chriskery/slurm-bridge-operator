/*
Copyright 2017 The Kubernetes Authors.

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

package configfiles

import (
	"fmt"
	"github.com/chriskery/slurm-bridge-operator/apis/kubecluster.org/v1alpha1"
	"github.com/chriskery/slurm-bridge-operator/pkg/slurm-virtual-kubelet/scheme"
	"path/filepath"

	utilfs "github.com/chriskery/slurm-bridge-operator/pkg/filesystem"
	utilcodec "github.com/chriskery/slurm-bridge-operator/pkg/slurm-virtual-kubelet/util/codec"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

// Loader loads configuration from a storage layer
type Loader interface {
	// Load loads and returns the KubeletConfiguration from the storage layer, or an error if a configuration could not be loaded
	Load() (*v1alpha1.SlurmVirtualKubeletConfiguration, error)
}

// fsLoader loads configuration from `configDir`
type fsLoader struct {
	// fs is the filesystem where the config files exist; can be mocked for testing
	fs utilfs.Filesystem
	// kubeletCodecs is the scheme used to decode config files
	kubeletCodecs *serializer.CodecFactory
	// kubeletFile is an absolute path to the file containing a serialized KubeletConfiguration
	kubeletFile string
}

// NewFsLoader returns a Loader that loads a KubeletConfiguration from the `kubeletFile`
func NewFsLoader(fs utilfs.Filesystem, kubeletFile string) (Loader, error) {
	_, kubeletCodecs, err := scheme.NewSchemeAndCodecs(serializer.EnableStrict)
	if err != nil {
		return nil, err
	}

	return &fsLoader{
		fs:            fs,
		kubeletCodecs: kubeletCodecs,
		kubeletFile:   kubeletFile,
	}, nil
}

func (loader *fsLoader) Load() (*v1alpha1.SlurmVirtualKubeletConfiguration, error) {
	data, err := loader.fs.ReadFile(loader.kubeletFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read kubelet config file %q, error: %v", loader.kubeletFile, err)
	}

	// no configuration is an error, some parameters are required
	if len(data) == 0 {
		return nil, fmt.Errorf("kubelet config file %q was empty", loader.kubeletFile)
	}

	kc, err := utilcodec.DecodeKubeletConfiguration(loader.kubeletCodecs, data)
	if err != nil {
		return nil, err
	}

	// make all paths absolute
	resolveRelativePaths(v1alpha1.SlurmVirtualKubeletConfigurationPathRefs(kc), filepath.Dir(loader.kubeletFile))
	return kc, nil
}

// resolveRelativePaths makes relative paths absolute by resolving them against `root`
func resolveRelativePaths(paths []*string, root string) {
	for _, path := range paths {
		// leave empty paths alone, "no path" is a valid input
		// do not attempt to resolve paths that are already absolute
		if len(*path) > 0 && !filepath.IsAbs(*path) {
			*path = filepath.Join(root, *path)
		}
	}
}
