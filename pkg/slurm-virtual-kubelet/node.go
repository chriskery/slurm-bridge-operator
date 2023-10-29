package slurm_virtual_kubelet

import (
	"context"
	"github.com/chriskery/slurm-bridge-operator/apis/kubecluster.org/v1alpha1"
	"github.com/chriskery/slurm-bridge-operator/pkg/workload"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

func (vk *SlurmVirtualKubelet) NewNodeOrDie() *v1.Node {
	taints := getDefaultTaints()
	capacity, err := vk.GetPartitionCapacity(vk.KubeletServer.SlurmPartition)
	if err != nil {
		klog.Fatal(err)
	}
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
			},
			Capacity:    capacity,
			Allocatable: capacity,
			//Conditions:      p.NodeConditions(ctx),
			//Addresses:       p.NodeAddresses(ctx),
			//DaemonEndpoints: *p.NodeDaemonEndpoints(ctx),
		},
	}
	return node
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
	rl["nvidia.com/gpu"] = *resource.NewQuantity(gpu, resource.DecimalSI)
	return rl, nil
}

func getDefaultTaints() []v1.Taint {
	var taints []v1.Taint
	for _, toleration := range v1alpha1.DefaultTolerations {
		taints = append(taints, v1.Taint{Key: toleration.Key, Value: toleration.Value, Effect: toleration.Effect})
	}
	return taints
}
