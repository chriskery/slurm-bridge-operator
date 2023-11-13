//go:build !ignore_autogenerated
// +build !ignore_autogenerated

/*
Copyright 2023.

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
// Code generated by openapi-gen. DO NOT EDIT.

// This file was autogenerated by openapi-gen. Do not edit it manually!

package v1alpha1

import (
	common "k8s.io/kube-openapi/pkg/common"
	spec "k8s.io/kube-openapi/pkg/validation/spec"
)

func GetOpenAPIDefinitions(ref common.ReferenceCallback) map[string]common.OpenAPIDefinition {
	return map[string]common.OpenAPIDefinition{
		"github.com/chriskery/slurm-bridge-operator/apis/kubecluster.org/v1alpha1.SlurmBridgeJob":                   schema_slurm_bridge_operator_apis_kubeclusterorg_v1alpha1_SlurmBridgeJob(ref),
		"github.com/chriskery/slurm-bridge-operator/apis/kubecluster.org/v1alpha1.SlurmVirtualKubeletConfiguration": schema_slurm_bridge_operator_apis_kubeclusterorg_v1alpha1_SlurmVirtualKubeletConfiguration(ref),
	}
}

func schema_slurm_bridge_operator_apis_kubeclusterorg_v1alpha1_SlurmBridgeJob(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "SlurmBridgeJob is the Schema for the slurmbridgejobs API",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"kind": {
						SchemaProps: spec.SchemaProps{
							Description: "Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"apiVersion": {
						SchemaProps: spec.SchemaProps{
							Description: "APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"metadata": {
						SchemaProps: spec.SchemaProps{
							Default: map[string]interface{}{},
							Ref:     ref("k8s.io/apimachinery/pkg/apis/meta/v1.ObjectMeta"),
						},
					},
					"spec": {
						SchemaProps: spec.SchemaProps{
							Default: map[string]interface{}{},
							Ref:     ref("github.com/chriskery/slurm-bridge-operator/apis/kubecluster.org/v1alpha1.SlurmBridgeJobSpec"),
						},
					},
					"status": {
						SchemaProps: spec.SchemaProps{
							Default: map[string]interface{}{},
							Ref:     ref("github.com/chriskery/slurm-bridge-operator/apis/kubecluster.org/v1alpha1.SlurmBridgeJobStatus"),
						},
					},
				},
			},
		},
		Dependencies: []string{
			"github.com/chriskery/slurm-bridge-operator/apis/kubecluster.org/v1alpha1.SlurmBridgeJobSpec", "github.com/chriskery/slurm-bridge-operator/apis/kubecluster.org/v1alpha1.SlurmBridgeJobStatus", "k8s.io/apimachinery/pkg/apis/meta/v1.ObjectMeta"},
	}
}

func schema_slurm_bridge_operator_apis_kubeclusterorg_v1alpha1_SlurmVirtualKubeletConfiguration(ref common.ReferenceCallback) common.OpenAPIDefinition {
	return common.OpenAPIDefinition{
		Schema: spec.Schema{
			SchemaProps: spec.SchemaProps{
				Description: "SlurmVirtualKubeletConfiguration contains the configuration for the Kubelet",
				Type:        []string{"object"},
				Properties: map[string]spec.Schema{
					"kind": {
						SchemaProps: spec.SchemaProps{
							Description: "Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"apiVersion": {
						SchemaProps: spec.SchemaProps{
							Description: "APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"metadata": {
						SchemaProps: spec.SchemaProps{
							Default: map[string]interface{}{},
							Ref:     ref("k8s.io/apimachinery/pkg/apis/meta/v1.ListMeta"),
						},
					},
					"Pods": {
						SchemaProps: spec.SchemaProps{
							Default: "",
							Type:    []string{"string"},
							Format:  "",
						},
					},
					"EnableServer": {
						SchemaProps: spec.SchemaProps{
							Description: "enableServer enables Kubelet's secured server. Note: Kubelet's insecure port is controlled by the readOnlyPort option.",
							Default:     false,
							Type:        []string{"boolean"},
							Format:      "",
						},
					},
					"StaticPodPath": {
						SchemaProps: spec.SchemaProps{
							Description: "staticPodPath is the path to the directory containing local (static) pods to run, or the path to a single static pod file.",
							Default:     "",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"SyncFrequency": {
						SchemaProps: spec.SchemaProps{
							Description: "syncFrequency is the max period between synchronizing running containers and config",
							Default:     0,
							Ref:         ref("k8s.io/apimachinery/pkg/apis/meta/v1.Duration"),
						},
					},
					"FileCheckFrequency": {
						SchemaProps: spec.SchemaProps{
							Description: "fileCheckFrequency is the duration between checking config files for new data",
							Default:     0,
							Ref:         ref("k8s.io/apimachinery/pkg/apis/meta/v1.Duration"),
						},
					},
					"HTTPCheckFrequency": {
						SchemaProps: spec.SchemaProps{
							Description: "httpCheckFrequency is the duration between checking http for new data",
							Default:     0,
							Ref:         ref("k8s.io/apimachinery/pkg/apis/meta/v1.Duration"),
						},
					},
					"StaticPodURL": {
						SchemaProps: spec.SchemaProps{
							Description: "staticPodURL is the URL for accessing static pods to run",
							Default:     "",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"StaticPodURLHeader": {
						SchemaProps: spec.SchemaProps{
							Description: "staticPodURLHeader is a map of slices with HTTP headers to use when accessing the podURL",
							Type:        []string{"object"},
							AdditionalProperties: &spec.SchemaOrBool{
								Allows: true,
								Schema: &spec.Schema{
									SchemaProps: spec.SchemaProps{
										Type: []string{"array"},
										Items: &spec.SchemaOrArray{
											Schema: &spec.Schema{
												SchemaProps: spec.SchemaProps{
													Default: "",
													Type:    []string{"string"},
													Format:  "",
												},
											},
										},
									},
								},
							},
						},
					},
					"Address": {
						SchemaProps: spec.SchemaProps{
							Description: "address is the IP address for the Kubelet to serve on (set to 0.0.0.0 for all interfaces)",
							Default:     "",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"Port": {
						SchemaProps: spec.SchemaProps{
							Description: "port is the port for the Kubelet to serve on.",
							Default:     0,
							Type:        []string{"integer"},
							Format:      "int32",
						},
					},
					"ReadOnlyPort": {
						SchemaProps: spec.SchemaProps{
							Description: "readOnlyPort is the read-only port for the Kubelet to serve on with no authentication/authorization (set to 0 to disable)",
							Default:     0,
							Type:        []string{"integer"},
							Format:      "int32",
						},
					},
					"VolumePluginDir": {
						SchemaProps: spec.SchemaProps{
							Description: "volumePluginDir is the full path of the directory in which to search for additional third party volume plugins.",
							Default:     "",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"ProviderID": {
						SchemaProps: spec.SchemaProps{
							Description: "providerID, if set, sets the unique id of the instance that an external provider (i.e. cloudprovider) can use to identify a specific node",
							Default:     "",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"TLSCertFile": {
						SchemaProps: spec.SchemaProps{
							Description: "tlsCertFile is the file containing x509 Certificate for HTTPS.  (CA cert, if any, concatenated after server cert). If tlsCertFile and tlsPrivateKeyFile are not provided, a self-signed certificate and key are generated for the public address and saved to the directory passed to the Kubelet's --cert-dir flag.",
							Default:     "",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"TLSPrivateKeyFile": {
						SchemaProps: spec.SchemaProps{
							Description: "tlsPrivateKeyFile is the file containing x509 private key matching tlsCertFile",
							Default:     "",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"TLSCipherSuites": {
						SchemaProps: spec.SchemaProps{
							Description: "TLSCipherSuites is the list of allowed cipher suites for the server. Note that TLS 1.3 ciphersuites are not configurable. Values are from tls package constants (https://golang.org/pkg/crypto/tls/#pkg-constants).",
							Type:        []string{"array"},
							Items: &spec.SchemaOrArray{
								Schema: &spec.Schema{
									SchemaProps: spec.SchemaProps{
										Default: "",
										Type:    []string{"string"},
										Format:  "",
									},
								},
							},
						},
					},
					"TLSMinVersion": {
						SchemaProps: spec.SchemaProps{
							Description: "TLSMinVersion is the minimum TLS version supported. Values are from tls package constants (https://golang.org/pkg/crypto/tls/#pkg-constants).",
							Default:     "",
							Type:        []string{"string"},
							Format:      "",
						},
					},
					"RotateCertificates": {
						SchemaProps: spec.SchemaProps{
							Description: "rotateCertificates enables client certificate rotation. The Kubelet will request a new certificate from the certificates.k8s.io API. This requires an approver to approve the certificate signing requests.",
							Default:     false,
							Type:        []string{"boolean"},
							Format:      "",
						},
					},
					"ServerTLSBootstrap": {
						SchemaProps: spec.SchemaProps{
							Description: "serverTLSBootstrap enables server certificate bootstrap. Instead of self signing a serving certificate, the Kubelet will request a certificate from the certificates.k8s.io API. This requires an approver to approve the certificate signing requests. The RotateKubeletServerCertificate feature must be enabled.",
							Default:     false,
							Type:        []string{"boolean"},
							Format:      "",
						},
					},
				},
				Required: []string{"Pods", "EnableServer", "StaticPodPath", "SyncFrequency", "FileCheckFrequency", "HTTPCheckFrequency", "StaticPodURL", "StaticPodURLHeader", "Address", "Port", "ReadOnlyPort", "VolumePluginDir", "ProviderID", "TLSCertFile", "TLSPrivateKeyFile", "TLSCipherSuites", "TLSMinVersion", "RotateCertificates", "ServerTLSBootstrap"},
			},
		},
		Dependencies: []string{
			"k8s.io/apimachinery/pkg/apis/meta/v1.Duration", "k8s.io/apimachinery/pkg/apis/meta/v1.ListMeta"},
	}
}
