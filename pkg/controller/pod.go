package controller

import (
	"github.com/chriskery/slurm-bridge-operator/apis/kubecluster.org/v1alpha1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var errAffinityIsNotRequired = errors.New("affinity selectors is not required")

func (r *SlurmBridgeJobReconciler) newPodForSJ(sjb *v1alpha1.SlurmBridgeJob) (*corev1.Pod, error) {
	affinity, err := affinityForSj(sjb)
	if err != nil && !errors.Is(err, errAffinityIsNotRequired) {
		return nil, errors.Wrap(err, "could not form slurm job pod affinity")
	}

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sjb.Name,
			Namespace: sjb.Namespace,
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
					Name:  "jt1",
					Image: "no-image",
				},
			},
			NodeSelector:  nodeSelectorForSj(sjb),
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}, nil
}

func affinityForSj(sjb *v1alpha1.SlurmBridgeJob) (*corev1.Affinity, error) {
	requiredResources, err := extractBatchResources(sjb.Spec.SbatchScript)
	if err != nil {
		return nil, errors.Wrap(err, "could not extract required resources")
	}

	return v1alpha1.AffinityForResources(*requiredResources)
}

func nodeSelectorForSj(sjb *v1alpha1.SlurmBridgeJob) map[string]string {
	nodeSelector := make(map[string]string)

	for k, v := range v1alpha1.DefaultNodeSelectors {
		nodeSelector[k] = v
	}

	nodeSelector[v1alpha1.PartitionLabel] = sjb.Spec.Partition
	return nodeSelector
}
