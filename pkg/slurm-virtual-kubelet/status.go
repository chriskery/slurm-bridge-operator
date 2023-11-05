package slurm_virtual_kubelet

import (
	"fmt"
	"github.com/chriskery/slurm-bridge-operator/pkg/workload"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/klog/v2"
	"strconv"
	"strings"
)

const (
	SlurmJobSubmitReason = "Slurm sbatch script submitted"
)

func slurmState2PodPhase(status workload.JobStatus) v1.PodPhase {
	switch status {
	case workload.JobStatus_UNKNOWN:
		return v1.PodUnknown
	case workload.JobStatus_PENDING:
		return v1.PodPending
	case workload.JobStatus_COMPLETED:
		return v1.PodSucceeded
	case workload.JobStatus_FAILED, workload.JobStatus_CANCELLED, workload.JobStatus_TIMEOUT:
		return v1.PodFailed
	default:
		return v1.PodRunning
	}
}

func unknowPodStatus() *v1.PodStatus {
	return &v1.PodStatus{
		Phase:   v1.PodUnknown,
		Message: fmt.Sprintf("can not find pod relative slurm job id, mybe not handled"),
	}
}

func convertJobInfo2PodStatus(pod *v1.Pod, jobInfo *workload.JobInfoResponse) (*v1.PodStatus, error) {
	if len(jobInfo.Info) == 0 {
		return unknowPodStatus(), nil
	}
	infoDetail, err := json.Marshal(jobInfo)
	if err != nil {
		klog.Error(err)
	}

	info := jobInfo.Info[0]
	startTime := metav1.NewTime(info.StartTime.AsTime())
	podStatus := &v1.PodStatus{
		Phase:             slurmState2PodPhase(info.GetStatus()),
		Conditions:        slurmStateToPodConditions(info),
		Message:           string(infoDetail),
		StartTime:         &startTime,
		ContainerStatuses: slurmState2ContainerStatuses(pod, jobInfo),
	}
	return podStatus, nil
}

func slurmState2ContainerStatuses(pod *v1.Pod, info *workload.JobInfoResponse) []v1.ContainerStatus {
	containerStatuses := make([]v1.ContainerStatus, 0, len(pod.Spec.Containers))
	for i, c := range pod.Spec.Containers {
		containerStatus := v1.ContainerStatus{
			Name:        c.Name,
			State:       slurmState2ContainerState(info.Info[i]),
			Ready:       slurmState2PodPhase(info.Info[i].Status) == v1.PodRunning,
			Image:       c.Image,
			ContainerID: info.Info[i].ArrayId,
		}
		// Add to containerStatuses
		containerStatuses = append(containerStatuses, containerStatus)
	}
	return containerStatuses
}

func slurmState2ContainerState(info *workload.JobInfo) v1.ContainerState {
	infoDetail, err := json.Marshal(info)
	if err != nil {
		klog.Error(err)
	}
	startTime := metav1.NewTime(info.StartTime.AsTime())
	switch info.Status {
	case workload.JobStatus_RUNNING, workload.JobStatus_COMPLETED:
		return v1.ContainerState{
			Running: &v1.ContainerStateRunning{
				StartedAt: startTime,
			},
		}
	case workload.JobStatus_FAILED, workload.JobStatus_CANCELLED:
		finishTime := metav1.NewTime(info.GetEndTime().AsTime())
		exitCode, _ := strconv.ParseInt(strings.Split(info.GetExitCode(), ":")[0], 10, 32)
		// Handle the case where the container failed.
		return v1.ContainerState{
			Terminated: &v1.ContainerStateTerminated{
				ExitCode:   int32(exitCode),
				Reason:     info.GetReason(),
				Message:    fmt.Sprintf("slurm job detail: 【%s】", infoDetail),
				StartedAt:  startTime,
				FinishedAt: finishTime,
			},
		}
	}

	// Handle the case where the container is pending.
	// Which should be all other eci states.
	return v1.ContainerState{
		Waiting: &v1.ContainerStateWaiting{
			Reason:  info.GetReason(),
			Message: fmt.Sprintf("slurm job detail: 【%s】", infoDetail),
		},
	}
}

func slurmStateToPodConditions(info *workload.JobInfo) []v1.PodCondition {
	switch info.Status {
	case workload.JobStatus_RUNNING:
		return []v1.PodCondition{
			{
				Type:   v1.PodReady,
				Status: v1.ConditionTrue,
			},
			{
				Type:   v1.PodInitialized,
				Status: v1.ConditionTrue,
			},
			{
				Type:   v1.PodScheduled,
				Status: v1.ConditionTrue,
			},
		}
	}
	return []v1.PodCondition{}
}

func isSlurmJobFinished(status workload.JobStatus) bool {
	return !(status == workload.JobStatus_RUNNING || status == workload.JobStatus_PENDING)
}