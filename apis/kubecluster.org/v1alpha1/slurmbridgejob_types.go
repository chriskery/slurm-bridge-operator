/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// SlurmBridgeJobKind is the kind name.
	SlurmBridgeJobKind = "SlurmBridgeJob"
	// SlurmBridgeJobPlural is the TensorflowPlural for SlurmBridgeJob.
	SlurmBridgeJobPlural = "SlurmBridgeJobs"
	// SlurmBridgeJobSingular is the singular for SlurmBridgeJob.
	SlurmBridgeJobSingular = "SlurmBridgeJob"

	// ControllerNameLabel represents the label key for the operator name, e.g. tf-operator, mpi-operator, etc.
	ControllerNameLabel = "kubeclusetr.org/controller-name"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// SlurmBridgeJobSpec defines the desired state of SlurmBridgeJob
type SlurmBridgeJobSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Partition    string `json:"partition"`
	SbatchScript string `json:"sbatchScript"`
	RunAsUser    *int64 `json:"runAsUser,omitempty"`
	RunAsGroup   *int64 `json:"runAsGroup,omitempty"`

	Array         string `json:"array,omitempty"`
	CpusPerTask   int64  `json:"cpusPerTask,omitempty"`
	Ntasks        int64  `json:"ntasks,omitempty"`
	NtasksPerNode int64  `json:"ntasksPerNode,omitempty"`
	Nodes         int64  `json:"nodes,omitempty"`
	WorkingDir    string `json:"workingDir,omitempty"`
	MemPerCpu     int64  `json:"memPerCpu,omitempty"`
	Gres          string `json:"gres,omitempty"`
	Licenses      string `json:"licenses,omitempty"`
	// Result may be specified for an optional result-fetcher collection step.
	// When specified, after job is completed all result-fetcher will be downloaded from Slurm
	// cluster with respect to this configuration.
	Result *JobResult `json:"result,omitempty"`
}

type SlurmJobId string

type SlurmSubjobStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	ID         string       `json:"id"`
	UserID     string       `json:"userId" `
	ArrayJobID string       `json:"arrayJobID"`
	Name       string       `json:"name"`
	ExitCode   string       `json:"exitCode"`
	State      string       `json:"state"`
	SubmitTime *metav1.Time `json:"submitTime"`
	StartTime  *metav1.Time `json:"startTime"`
	RunTime    string       `json:"runTime"`
	TimeLimit  string       `json:"timeLimit"`
	WorkDir    string       `json:"WorkDir"`
	StdOut     string       `json:"stdOut"`
	StdErr     string       `json:"stdErr"`
	Partition  string       `json:"partition"`
	NodeList   string       `json:"nodeList"`
	BatchHost  string       `json:"batchHost"`
	NumNodes   string       `json:"numNodes"`
}

// SlurmBridgeJobStatus defines the observed state of SlurmBridgeJob
type SlurmBridgeJobStatus struct {
	State             string                            `json:"state"`
	SubjobStatus      map[SlurmJobId]*SlurmSubjobStatus `json:"subjobStatus,omitempty"`
	FetchResult       bool                              `json:"fetchResult,omitempty"`
	FetchResultStatus string                            `json:"fetchResultStatus,omitempty"`
	ClusterEndPoint   string                            `json:"clusterEndPoint,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +resource:path=slurmbridgejobs
//+kubebuilder:resource:scope=Namespaced,path=slurmbridgejobs,shortName={"slurmbridgejob","sbj"}
//+kubebuilder:object:root=true
//+kubebuilder:printcolumn:JSONPath=`.metadata.creationTimestamp`,name="Age",type=date
//+kubebuilder:printcolumn:JSONPath=`.status.state`,name="State",type=string
//+kubebuilder:subresource:status

// SlurmBridgeJob is the Schema for the slurmbridgejobs API
type SlurmBridgeJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SlurmBridgeJobSpec   `json:"spec,omitempty"`
	Status SlurmBridgeJobStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SlurmBridgeJobList contains a list of SlurmBridgeJob
type SlurmBridgeJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SlurmBridgeJob `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SlurmBridgeJob{}, &SlurmBridgeJobList{})
}
