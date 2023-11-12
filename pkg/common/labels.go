package common

const (
	LabelSlurmBridgeJobId = "kubecluster.org/jobid"

	LabelLastJobInfo   = "kubecluster.org/jobinfo"
	LabelAgentEndPoint = "kubecluster.org/agent-endpoint"

	LabelsResourceRequestCpusPerTask   = "kubecluster.org/cpus-per-task"
	LabelsResourceRequestNodes         = "kubecluster.org/nodes"
	LabelsResourceRequestMemPerCpu     = "kubecluster.org/mem-per-cpu"
	LabelsResourceRequestNTasksPerNode = "kubecluster.org/ntasks-per-node"
	LabelsResourceRequestArray         = "kubecluster.org/array"
	LabelsResourceRequestNTasks        = "kubecluster.org/ntask"
)
