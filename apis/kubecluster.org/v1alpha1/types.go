package v1alpha1

import v1 "k8s.io/api/core/v1"

// JobResult is a schema for result-fetcher collection.
type JobResult struct {
	// Mount is a directory where job result-fetcher will be stored.
	// After result-fetcher collection all job generated files can be found in Mount/<SlurmJob.Name> directory.
	Volume v1.Volume `json:"volume,omitempty"`
}

type SlurmBridgeJobPodRole string

const (
	SlurmBridgeJobPodRoleSizeCar    = "sizecar"
	SlurmBridgeJobPodRoleSizeWorker = "worker"
)
