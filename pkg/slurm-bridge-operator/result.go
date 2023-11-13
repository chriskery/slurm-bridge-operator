package slurm_bridge_operator

import (
	"fmt"
	"github.com/chriskery/slurm-bridge-operator/apis/kubecluster.org/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *SlurmBridgeJobReconciler) newJobForSJResult(sjb *v1alpha1.SlurmBridgeJob) *batchv1.Job {
	backOffLimit := int32(0)
	containers := r.getJobResultContainers(sjb)
	boolPtr := func(b bool) *bool { return &b }
	jobSpec := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-result-fetcher", sjb.Name),
			Namespace: sjb.Namespace,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion:         v1alpha1.GroupVersion.String(),
				Kind:               v1alpha1.GroupVersion.WithKind(v1alpha1.SlurmBridgeJobKind).Kind,
				Name:               sjb.GetName(),
				UID:                sjb.GetUID(),
				BlockOwnerDeletion: boolPtr(true),
				Controller:         boolPtr(true),
			}},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: containers,
					//https://kubernetes.io/zh/docs/concepts/workloads/pods/pod-lifecycle/#restart-policy
					RestartPolicy: corev1.RestartPolicyNever,
					Volumes:       []corev1.Volume{sjb.Spec.Result.Volume},
				},
			},
			//Specifies the number of retries before marking this job failed. Defaults to 6 +optional
			//指定标记此任务失败之前的重试次数为0次，创建失败就不再创建。默认值为6
			BackoffLimit: &backOffLimit,
		},
	}
	return jobSpec
}

func (r *SlurmBridgeJobReconciler) getJobResultContainers(sjb *v1alpha1.SlurmBridgeJob) []corev1.Container {
	var containers []corev1.Container
	to := "/result"
	for _, jobStatus := range sjb.Status.SubjobStatus {
		fetcherCMD := fmt.Sprintf("/result-fetcher --from %s --to %s --endpoint %s ",
			jobStatus.StdOut, fmt.Sprintf("%s/%s", to, sjb.Name), sjb.Status.ClusterEndPoint)
		containers = append(containers, corev1.Container{
			Name:    fmt.Sprintf("%s-%s", jobStatus.Name, jobStatus.ID),
			Image:   "docker.io/chriskery/result-fetcher:latest",
			Command: []string{"sh", "-c", fetcherCMD},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      sjb.Spec.Result.Volume.Name,
					ReadOnly:  false,
					MountPath: to,
				},
			},
		})
	}
	return containers
}

func isFinishedFetchResult(status string) bool {
	return status == string(corev1.PodFailed) || status == string(corev1.PodSucceeded)
}
