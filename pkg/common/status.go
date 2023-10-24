package common

import (
	"fmt"
)

const (
	// SlurmBridgeJobSubmitting means the SlurmBridgeJob has been accepted by the system,
	// but one or more of the pods/services has not been started.
	// This includes time before pods being scheduled and launched.
	SlurmBridgeJobSubmitting string = "SUBMITTING"
)

const (
	// SlurmBridgeJobCreatedReason is added in a KubeSlurmBridgeJob when it is created.
	SlurmBridgeJobCreatedReason = "Created"
	// SlurmBridgeJobRunningReason is added in a KubeSlurmBridgeJob when it is running.
	SlurmBridgeJobRunningReason = "Running"
	// SlurmBridgeJobFailedReason is added in a KubeSlurmBridgeJob when it is failed.
	SlurmBridgeJobFailedReason = "Failed"
	// SlurmBridgeJobRestartingReason is added in a KubeSlurmBridgeJob when it is restarting.
	SlurmBridgeJobRestartingReason = "Restarting"
	// SlurmBridgeJobFailedValidationReason is added in a KubeSlurmBridgeJob when it failed validation
	SlurmBridgeJobFailedValidationReason = "FailedValidation"
	// SlurmBridgeJobSuspendedReason is added in a KubeSlurmBridgeJob when it is suspended.
	SlurmBridgeJobSuspendedReason = "Suspended"
	// SlurmBridgeJobResumedReason is added in a KubeSlurmBridgeJob when it is unsuspended.
	SlurmBridgeJobResumedReason = "Resumed"
)

func NewReason(kind, reason string) string {
	return fmt.Sprintf("%s%s", kind, reason)
}

const (
	SlurmBridgeJobIdAnnotation = "kubecluster.org/jobid"
)
