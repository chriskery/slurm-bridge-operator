package slurm_virtual_kubelet

import (
	"context"
	"fmt"
	"github.com/chriskery/slurm-bridge-operator/apis/kubecluster.org/v1alpha1"
	"github.com/chriskery/slurm-bridge-operator/pkg/workload"
	"github.com/docker/distribution/uuid"
	"io"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"net"
	"os/exec"
)

func (vk *SlurmVirtualKubelet) NewNodeOrDie() (*v1.Node, error) {
	taints := getDefaultTaints()
	capacity, err := vk.GetPartitionCapacity(vk.KubeletServer.SlurmPartition)
	if err != nil {
		return nil, err
	}
	systemUUID := uuid.Generate().String()
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   vk.KubeletServer.NodeName,
			Labels: vk.KubeletServer.NodeLabels,
		},
		Spec: v1.NodeSpec{
			Taints: taints,
		},
		Status: v1.NodeStatus{
			NodeInfo: v1.NodeSystemInfo{
				OperatingSystem: vk.KubeletServer.OperatingSystem,
				Architecture:    "amd64",
				KubeletVersion:  vk.KubeletServer.Version,
				MachineID:       systemUUID,
				SystemUUID:      systemUUID,
				BootID:          systemUUID,
				KernelVersion:   getNodeKernelVersion(),
				OSImage:         "CentOS Linux 7 (Core)",
			},
			Capacity:        capacity,
			Allocatable:     capacity,
			Conditions:      nodeConditions(),
			Addresses:       nodeAddresses(),
			DaemonEndpoints: nodeDaemonEndpoints(vk),
		},
	}
	return node, nil
}

func nodeDaemonEndpoints(vk *SlurmVirtualKubelet) v1.NodeDaemonEndpoints {
	return v1.NodeDaemonEndpoints{
		KubeletEndpoint: v1.DaemonEndpoint{
			Port: vk.KubeletServer.Port,
		},
	}
}

func getNodeKernelVersion() string {
	cmd := exec.Command("uname", "-a")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		klog.Error(err)
		return ""
	}
	defer stdout.Close()
	if err = cmd.Start(); err != nil {
		klog.Error(err)
		return ""
	}
	opBytes, err := io.ReadAll(stdout)
	if err != nil {
		klog.Error(err)
		return ""
	}
	return fmt.Sprintf("%s", string(opBytes))
}

// NodeAddresses returns a list of addresses for the node status
// within Kubernetes.
func nodeAddresses() []v1.NodeAddress {
	// TODO: Make these dynamic and augment with custom ECI specific conditions of interest

	return []v1.NodeAddress{
		{
			Type:    "InternalIP",
			Address: getNodeAddress(),
		},
	}
}

func getNodeAddress() string {
	addrList, err := net.InterfaceAddrs()
	if err != nil {
		fmt.Println("get current host ip err: ", err)
		return ""
	}
	var ip string
	for _, address := range addrList {
		if ipNet, ok := address.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				ip = ipNet.IP.String()
				break
			}
		}
	}
	return ip
}

// NodeConditions returns a list of conditions (Ready, OutOfDisk, etc), for updates to the node status
// within Kubernetes.
func nodeConditions() []v1.NodeCondition {
	// TODO: Make these dynamic and augment with custom ECI specific conditions of interest
	return []v1.NodeCondition{
		{
			Type:               "Ready",
			Status:             v1.ConditionTrue,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "KubeletReady",
			Message:            "kubelet is ready.",
		},
		{
			Type:               "OutOfDisk",
			Status:             v1.ConditionFalse,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "KubeletHasSufficientDisk",
			Message:            "kubelet has sufficient disk space available",
		},
		{
			Type:               "MemoryPressure",
			Status:             v1.ConditionFalse,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "KubeletHasSufficientMemory",
			Message:            "kubelet has sufficient memory available",
		},
		{
			Type:               "DiskPressure",
			Status:             v1.ConditionFalse,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "KubeletHasNoDiskPressure",
			Message:            "kubelet has no disk pressure",
		},
		{
			Type:               "NetworkUnavailable",
			Status:             v1.ConditionFalse,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "RouteCreated",
			Message:            "RouteController created a route",
		},
		{
			Type:               "PIDPressure",
			Status:             v1.ConditionFalse,
			LastHeartbeatTime:  metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "KubeletHasSufficientPID",
			Message:            " kubelet has sufficient PID available",
		},
	}
}

func (vk *SlurmVirtualKubelet) GetPartitionCapacity(partition string) (v1.ResourceList, error) {
	partitionResp, err := vk.SlurmClient.Partition(context.TODO(), &workload.PartitionRequest{Partition: partition})
	if err != nil {
		return nil, err
	}

	nodesResp, err := vk.SlurmClient.Nodes(context.TODO(), &workload.NodesRequest{Nodes: partitionResp.Nodes})
	if err != nil {
		return nil, err
	}
	var cpu, allocpu int64
	var mem, allomem int64
	var gpu, allogpu int64

	for _, node := range nodesResp.Nodes {
		cpu += node.Cpus
		allocpu += node.AlloCpus
		mem += node.Memory
		allomem += node.AlloMemory
		gpu += node.Gpus
		allogpu += node.AlloCpus
	}
	rl := v1.ResourceList{}
	rl["cpu"] = *resource.NewQuantity(cpu, resource.DecimalSI)
	rl["memory"] = *resource.NewQuantity(mem*(2<<10), resource.BinarySI)
	if gpu > 0 {
		rl["nvidia.com/gpu"] = *resource.NewQuantity(gpu, resource.DecimalSI)
	}
	rl["pods"] = resource.MustParse(vk.KubeletServer.Pods)
	return rl, nil
}

func getDefaultTaints() []v1.Taint {
	var taints []v1.Taint
	for _, toleration := range v1alpha1.DefaultTolerations {
		taints = append(taints, v1.Taint{Key: toleration.Key, Value: toleration.Value, Effect: toleration.Effect})
	}
	return taints
}
