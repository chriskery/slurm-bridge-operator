package v1alpha1

import v1 "k8s.io/api/core/v1"

// JobResults is a schema for result-fetcher collection.
// +k8s:openapi-gen=true
type JobResults struct {
	// Mount is a directory where job result-fetcher will be stored.
	// After result-fetcher collection all job generated files can be found in Mount/<SlurmJob.Name> directory.
	Volume v1.Volume `json:"mount,omitempty"`
}
