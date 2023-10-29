package slurm_virtual_kubelet

import (
	"context"
	"github.com/chriskery/slurm-bridge-operator/pkg/slurm-virtual-kubelet/manager"
	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/virtual-kubelet/virtual-kubelet/node/api"
	"github.com/virtual-kubelet/virtual-kubelet/node/api/statsv1alpha1"
	"github.com/virtual-kubelet/virtual-kubelet/node/nodeutil"
	"io"
	v1 "k8s.io/api/core/v1"
)

var _ nodeutil.Provider = (SlurmVirtualKubeletProvider{})

type SlurmVirtualKubeletProvider struct {
	resourceManager    *manager.ResourceManager
	resourceGroup      string
	region             string
	nodeName           string
	operatingSystem    string
	clusterName        string
	cpu                string
	memory             string
	pods               string
	internalIP         string
	daemonEndpointPort int32
	secureGroup        string
	vSwitch            string
}

func (s SlurmVirtualKubeletProvider) CreatePod(ctx context.Context, pod *v1.Pod) error {
	//TODO implement me
	panic("implement me")
}

func (s SlurmVirtualKubeletProvider) UpdatePod(ctx context.Context, pod *v1.Pod) error {
	//TODO implement me
	panic("implement me")
}

func (s SlurmVirtualKubeletProvider) DeletePod(ctx context.Context, pod *v1.Pod) error {
	//TODO implement me
	panic("implement me")
}

func (s SlurmVirtualKubeletProvider) GetPod(ctx context.Context, namespace, name string) (*v1.Pod, error) {
	//TODO implement me
	panic("implement me")
}

func (s SlurmVirtualKubeletProvider) GetPodStatus(ctx context.Context, namespace, name string) (*v1.PodStatus, error) {
	//TODO implement me
	panic("implement me")
}

func (s SlurmVirtualKubeletProvider) GetPods(ctx context.Context) ([]*v1.Pod, error) {
	//TODO implement me
	panic("implement me")
}

func (s SlurmVirtualKubeletProvider) GetContainerLogs(ctx context.Context, namespace, podName, containerName string, opts api.ContainerLogOpts) (io.ReadCloser, error) {
	//TODO implement me
	panic("implement me")
}

func (s SlurmVirtualKubeletProvider) RunInContainer(ctx context.Context, namespace, podName, containerName string, cmd []string, attach api.AttachIO) error {
	//TODO implement me
	panic("implement me")
}

func (s SlurmVirtualKubeletProvider) AttachToContainer(ctx context.Context, namespace, podName, containerName string, attach api.AttachIO) error {
	//TODO implement me
	panic("implement me")
}

func (s SlurmVirtualKubeletProvider) GetStatsSummary(ctx context.Context) (*statsv1alpha1.Summary, error) {
	//TODO implement me
	panic("implement me")
}

func (s SlurmVirtualKubeletProvider) GetMetricsResource(ctx context.Context) ([]*io_prometheus_client.MetricFamily, error) {
	//TODO implement me
	panic("implement me")
}

func (s SlurmVirtualKubeletProvider) PortForward(ctx context.Context, namespace, pod string, port int32, stream io.ReadWriteCloser) error {
	//TODO implement me
	panic("implement me")
}
