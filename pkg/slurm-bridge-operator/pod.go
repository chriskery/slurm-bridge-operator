package slurm_bridge_operator

import (
	"github.com/chriskery/slurm-bridge-operator/apis/kubecluster.org/v1alpha1"
	"github.com/chriskery/slurm-bridge-operator/pkg/common"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"strconv"
)

var errAffinityIsNotRequired = errors.New("affinity selectors is not required")

func (r *SlurmBridgeJobReconciler) newPodForSJ(sjb *v1alpha1.SlurmBridgeJob) (*corev1.Pod, error) {
	requiredResources, err := extractBatchResourcesFromScript(sjb.Spec.SbatchScript)
	if err != nil {
		return nil, errors.Wrap(err, "could not extract required resources")
	}
	setRequireResourceBySpec(sjb.Spec, requiredResources)
	setDefaultRequireResource(requiredResources)
	resourceList := r.genResourceListForPod(requiredResources)

	affinity, err := affinityForSj(sjb, *requiredResources)
	if err != nil && !errors.Is(err, errAffinityIsNotRequired) {
		return nil, errors.Wrap(err, "could not form slurm-agent job pod affinity")
	}

	labels := r.getResourceRequestLabelsForPod(sjb.Spec)
	sjPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sjb.Name,
			Namespace: sjb.Namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			Affinity: affinity,
			SecurityContext: &corev1.PodSecurityContext{
				RunAsUser:  sjb.Spec.RunAsUser,
				RunAsGroup: sjb.Spec.RunAsGroup,
			},
			Tolerations: v1alpha1.DefaultTolerations,
			Containers: []corev1.Container{
				{
					Name:            sjb.Name,
					Image:           "no-image",
					Resources:       corev1.ResourceRequirements{Requests: resourceList, Limits: resourceList},
					Command:         []string{sjb.Spec.SbatchScript},
					SecurityContext: &corev1.SecurityContext{RunAsUser: sjb.Spec.RunAsUser},
					WorkingDir:      sjb.Spec.WorkingDir,
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}

	// Set SlurmJob instance as the owner and slurm-bridge-operator
	err = controllerutil.SetControllerReference(sjb, sjPod, r.Scheme)
	if err != nil {
		logrus.Errorf("Could not set slurm-bridge-operator reference for pod: %v", err)
		return nil, err
	}
	return sjPod, nil
}

func setRequireResourceBySpec(spec v1alpha1.SlurmBridgeJobSpec, resources *v1alpha1.Resources) {
	if spec.Nodes > 0 {
		resources.Nodes = spec.Nodes
	}
	if spec.CpusPerTask > 0 {
		resources.CpusPerTask = spec.CpusPerTask
	}
	if spec.MemPerCpu > 0 {
		resources.MemPerCpu = spec.MemPerCpu
	}
	if spec.NtasksPerNode > 0 {
		resources.NtasksPerNode = spec.NtasksPerNode
	}
	if len(spec.Array) > 0 {
		resources.Array = spec.Array
	}
	if spec.Ntasks > 0 {
		resources.Ntasks = spec.Ntasks
	}
}

const (
	DefaultRequireNodes       = 1
	DefaultRequireCpusPerTask = 1
	DefaultRequireMemPerCpu   = 1024
)

func setDefaultRequireResource(requiredResources *v1alpha1.Resources) {
	if requiredResources.Nodes == 0 {
		requiredResources.Nodes = DefaultRequireNodes
	}
	if requiredResources.CpusPerTask == 0 {
		requiredResources.CpusPerTask = DefaultRequireCpusPerTask
	}
	if requiredResources.MemPerCpu == 0 {
		requiredResources.MemPerCpu = DefaultRequireMemPerCpu
	}
}

func affinityForSj(sjb *v1alpha1.SlurmBridgeJob, requiredResources v1alpha1.Resources) (*corev1.Affinity, error) {

	nodeMatch, err := v1alpha1.AffinityForResources(requiredResources)
	if err != nil {
		return nil, err
	}

	for key, value := range v1alpha1.DefaultNodeSelectors {
		nodeMatch = append(nodeMatch, corev1.NodeSelectorRequirement{
			Key:      key,
			Operator: corev1.NodeSelectorOpIn,
			Values:   []string{value},
		})
	}

	nodeMatch = append(nodeMatch, corev1.NodeSelectorRequirement{
		Key:      v1alpha1.PartitionLabel,
		Operator: corev1.NodeSelectorOpIn,
		Values:   []string{sjb.Spec.Partition},
	})

	if len(nodeMatch) == 0 {
		return nil, v1alpha1.ErrAffinityIsNotRequired
	}

	return &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: nodeMatch}},
			},
		},
	}, nil
}

func (r *SlurmBridgeJobReconciler) genResourceListForPod(resources *v1alpha1.Resources) corev1.ResourceList {
	var cpuCount int64
	if resources.Ntasks > 0 {
		cpuCount = resources.CpusPerTask * resources.Ntasks
	} else if resources.NtasksPerNode > 0 && resources.Nodes > 0 {
		cpuCount = resources.CpusPerTask * resources.NtasksPerNode * resources.Nodes
	} else {
		cpuCount = resources.CpusPerTask
	}

	if len(resources.Array) > 0 {
		arrayLen := parseArrayLen(resources.Array)
		cpuCount *= arrayLen
	}

	resourceList := corev1.ResourceList{}
	resourceList[corev1.ResourceCPU] = resource.MustParse(strconv.Itoa(int(cpuCount)))
	resourceList[corev1.ResourceMemory] = resource.MustParse(strconv.Itoa(int(cpuCount * resources.MemPerCpu * 1024)))
	return resourceList
}

func (r *SlurmBridgeJobReconciler) getResourceRequestLabelsForPod(spec v1alpha1.SlurmBridgeJobSpec) map[string]string {
	labels := make(map[string]string)

	if spec.Nodes > 0 {
		labels[common.LabelsResourceRequestNodes] = strconv.Itoa(int(spec.Nodes))
	}
	if spec.CpusPerTask > 0 {
		labels[common.LabelsResourceRequestCpusPerTask] = strconv.Itoa(int(spec.CpusPerTask))
	}
	if spec.MemPerCpu > 0 {
		labels[common.LabelsResourceRequestMemPerCpu] = strconv.Itoa(int(spec.MemPerCpu))
	}
	if spec.NtasksPerNode > 0 {
		labels[common.LabelsResourceRequestNTasksPerNode] = strconv.Itoa(int(spec.NtasksPerNode))
	}
	if len(spec.Array) > 0 {
		labels[common.LabelsResourceRequestArray] = spec.Array
	}
	if spec.Ntasks > 0 {
		labels[common.LabelsResourceRequestNTasks] = strconv.Itoa(int(spec.Ntasks))
	}

	return labels
}
