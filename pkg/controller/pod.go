package controller

import (
	"github.com/chriskery/slurm-bridge-operator/apis/kubecluster.org/v1alpha1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strconv"
)

var errAffinityIsNotRequired = errors.New("affinity selectors is not required")

func (r *SlurmBridgeJobReconciler) newPodForSJ(sjb *v1alpha1.SlurmBridgeJob) (*corev1.Pod, error) {
	requiredResources, err := extractBatchResources(sjb.Spec.SbatchScript)
	if err != nil {
		return nil, errors.Wrap(err, "could not extract required resources")
	}
	setDefaultRequireResource(requiredResources)

	affinity, err := affinityForSj(sjb, *requiredResources)
	if err != nil && !errors.Is(err, errAffinityIsNotRequired) {
		return nil, errors.Wrap(err, "could not form slurm-agent job pod affinity")
	}

	resourceList := corev1.ResourceList{}
	resourceList[corev1.ResourceCPU] = resource.MustParse(strconv.Itoa(int(requiredResources.Nodes * requiredResources.CPUPerNode)))
	resourceList[corev1.ResourceMemory] = resource.MustParse(strconv.Itoa(int(requiredResources.Nodes * requiredResources.MemPerNode)))

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sjb.Name,
			Namespace: sjb.Namespace,
		},
		Spec: corev1.PodSpec{
			Affinity: affinity,
			SecurityContext: &corev1.PodSecurityContext{
				RunAsUser: sjb.Spec.RunAsUser,
			},
			Tolerations: v1alpha1.DefaultTolerations,
			Containers: []corev1.Container{
				{
					Name:            "slurm-job",
					Image:           "no-image",
					Resources:       corev1.ResourceRequirements{Requests: resourceList, Limits: resourceList},
					Command:         []string{sjb.Spec.SbatchScript},
					SecurityContext: &corev1.SecurityContext{RunAsUser: sjb.Spec.RunAsUser},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}, nil
}

func setDefaultRequireResource(requiredResources *v1alpha1.Resources) {
	if requiredResources.Nodes == 0 {
		requiredResources.Nodes = 1
	}
	if requiredResources.CPUPerNode == 0 {
		requiredResources.CPUPerNode = 1
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
