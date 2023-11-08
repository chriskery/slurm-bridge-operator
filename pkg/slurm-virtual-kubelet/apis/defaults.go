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

package apis

import (
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
)

func addDefaultingFuncs(scheme *kruntime.Scheme) error {
	scheme.AddTypeDefaultingFunc(&SlurmVirtualKubeletConfiguration{}, func(obj interface{}) {
		SetDefaults_SlurmVirtualKubeletConfiguration(obj.(*SlurmVirtualKubeletConfiguration))
	})
	return nil
}

const (
	DefaultListenPort        = 10250
	DefaultTLSCertFile       = "/etc/kubernetes/pki/apiserver-kubelet-client.crt"
	DefaultTLSPrivateKeyFile = "/etc/kubernetes/pki/apiserver-kubelet-client.key"
)

func SetDefaults_SlurmVirtualKubeletConfiguration(obj *SlurmVirtualKubeletConfiguration) {
	if obj.Port == 0 {
		obj.Port = DefaultListenPort
	}
	if obj.Address == "" {
		obj.Address = "0.0.0.0"
	}
	if obj.Pods == "" {
		obj.Pods = "10000"
	}
	if obj.TLSCertFile == "" || obj.TLSPrivateKeyFile == "" {
		klog.Warningf("TLS cert files not supplied, will use default tls cert files")
		obj.TLSCertFile = DefaultTLSCertFile
		obj.TLSPrivateKeyFile = DefaultTLSPrivateKeyFile
	}
}
