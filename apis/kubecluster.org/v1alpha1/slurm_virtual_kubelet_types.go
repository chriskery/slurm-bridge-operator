package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SlurmVirtualKubeletConfiguration contains the configuration for the Kubelet
// +k8s:openapi-gen=true
type SlurmVirtualKubeletConfiguration struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Pods string
	// enableServer enables Kubelet's secured server.
	// Note: Kubelet's insecure port is controlled by the readOnlyPort option.
	EnableServer bool
	// staticPodPath is the path to the directory containing local (static) pods to
	// run, or the path to a single static pod file.
	StaticPodPath string
	// syncFrequency is the max period between synchronizing running
	// containers and config
	SyncFrequency metav1.Duration
	// fileCheckFrequency is the duration between checking config files for
	// new data
	FileCheckFrequency metav1.Duration
	// httpCheckFrequency is the duration between checking http for new data
	HTTPCheckFrequency metav1.Duration
	// staticPodURL is the URL for accessing static pods to run
	StaticPodURL string
	// staticPodURLHeader is a map of slices with HTTP headers to use when accessing the podURL
	StaticPodURLHeader map[string][]string `datapolicy:"token"`
	// address is the IP address for the Kubelet to serve on (set to 0.0.0.0
	// for all interfaces)
	Address string
	// port is the port for the Kubelet to serve on.
	Port int32
	// readOnlyPort is the read-only port for the Kubelet to serve on with
	// no authentication/authorization (set to 0 to disable)
	ReadOnlyPort int32
	// volumePluginDir is the full path of the directory in which to search
	// for additional third party volume plugins.
	VolumePluginDir string
	// providerID, if set, sets the unique id of the instance that an external provider (i.e. cloudprovider)
	// can use to identify a specific node
	ProviderID string
	// tlsCertFile is the file containing x509 Certificate for HTTPS.  (CA cert,
	// if any, concatenated after server cert). If tlsCertFile and
	// tlsPrivateKeyFile are not provided, a self-signed certificate
	// and key are generated for the public address and saved to the directory
	// passed to the Kubelet's --cert-dir flag.
	TLSCertFile string
	// tlsPrivateKeyFile is the file containing x509 private key matching tlsCertFile
	TLSPrivateKeyFile string
	// TLSCipherSuites is the list of allowed cipher suites for the server.
	// Note that TLS 1.3 ciphersuites are not configurable.
	// Values are from tls package constants (https://golang.org/pkg/crypto/tls/#pkg-constants).
	TLSCipherSuites []string
	// TLSMinVersion is the minimum TLS version supported.
	// Values are from tls package constants (https://golang.org/pkg/crypto/tls/#pkg-constants).
	TLSMinVersion string
	// rotateCertificates enables client certificate rotation. The Kubelet will request a
	// new certificate from the certificates.k8s.io API. This requires an approver to approve the
	// certificate signing requests.
	RotateCertificates bool
	// serverTLSBootstrap enables server certificate bootstrap. Instead of self
	// signing a serving certificate, the Kubelet will request a certificate from
	// the certificates.k8s.io API. This requires an approver to approve the
	// certificate signing requests. The RotateKubeletServerCertificate feature
	// must be enabled.
	ServerTLSBootstrap bool
}

func init() {
	// addKnownTypes registers known types to the given scheme
	SchemeBuilder.Register(&SlurmVirtualKubeletConfiguration{})
	SchemeBuilder.SchemeBuilder.Register(addDefaultingFuncs)
}
