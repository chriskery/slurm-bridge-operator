package slurm_virtual_kubelet

import (
	"context"
	"github.com/chriskery/slurm-bridge-operator/apis/kubecluster.org/v1alpha1"
	"github.com/chriskery/slurm-bridge-operator/pkg/common"
	"github.com/chriskery/slurm-bridge-operator/pkg/slurm-virtual-kubelet/manager"
	"github.com/chriskery/slurm-bridge-operator/pkg/workload"
	"github.com/pkg/errors"
	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/virtual-kubelet/virtual-kubelet/node/api"
	"github.com/virtual-kubelet/virtual-kubelet/node/api/statsv1alpha1"
	"github.com/virtual-kubelet/virtual-kubelet/node/nodeutil"
	"io"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/klog/v2"
	"strconv"
	"strings"
	"sync"
	"time"
)

var _ nodeutil.Provider = &SlurmVirtualKubeletProvider{}

type SlurmVirtualKubeletProvider struct {
	vk              *SlurmVirtualKubelet
	resourceManager *manager.ResourceManager

	knownPods sync.Map
}

func (s *SlurmVirtualKubeletProvider) CreatePod(ctx context.Context, pod *v1.Pod) error {
	klog.Infof("Accept pod schedule for %s/%s", pod.GetName(), pod.GetNamespace())
	//Ignore daemonSet Pod
	if !needReconcile(pod) {
		klog.Infof("Skip to create DaemonSet pod %q", pod.Name)
		return nil
	}

	err := s.validateCreatePod(pod)
	if err != nil {
		return err
	}

	submitRequest := s.newSubmitRequestForPod(pod)
	submitJobResp, err := s.vk.SlurmClient.SubmitJob(ctx, submitRequest)
	if err != nil {
		return err
	}

	s.vk.recorder.Eventf(pod, v1.EventTypeNormal, common.NewReason(v1alpha1.SlurmBridgeJobKind, common.SlurmBridgeJobCreatedReason),
		"SlurmBridgeJob submit to the slurm-agent %s, job id is %d", s.vk.KubeletServer.AgentEndpoint, submitJobResp.JobId)
	s.knownPods.Store(pod.UID, strconv.FormatInt(submitJobResp.GetJobId(), 10))

	s.addAndUpdateSlurmJobInfo(pod, strconv.FormatInt(submitJobResp.GetJobId(), 10))
	return nil
}

func (s *SlurmVirtualKubeletProvider) newSubmitRequestForPod(pod *v1.Pod) *workload.SubmitJobRequest {
	submitRequest := &workload.SubmitJobRequest{
		Uid:       string(pod.GetUID()),
		Partition: s.vk.KubeletServer.SlurmPartition,
	}
	if pod.Spec.Containers[0].SecurityContext != nil && pod.Spec.Containers[0].SecurityContext.RunAsUser != nil {
		submitRequest.RunAsUser = strconv.FormatInt(*pod.Spec.Containers[0].SecurityContext.RunAsUser, 10)
	}
	submitRequest.Script = pod.Spec.Containers[0].Command[0]

	labels := pod.GetLabels()
	nodes, ok := labels[common.LabelsResourceRequestNodes]
	if ok {
		parseInt, err := strconv.ParseInt(nodes, 10, 64)
		if err != nil {
			klog.Errorf("Failed to Parse nodes for %/%s", pod.GetNamespace(), pod.GetName())
		} else {
			submitRequest.Nodes = parseInt
		}
	}
	cpusPerTask, ok := labels[common.LabelsResourceRequestCpusPerTask]
	if ok {
		parseInt, err := strconv.ParseInt(cpusPerTask, 10, 64)
		if err != nil {
			klog.Errorf("Failed to Parse cpusPerTask for %/%s", pod.GetNamespace(), pod.GetName())
		} else {
			submitRequest.CpufsPerTask = parseInt
		}
	}
	memPerCpu, ok := labels[common.LabelsResourceRequestMemPerCpu]
	if ok {
		parseInt, err := strconv.ParseInt(memPerCpu, 10, 64)
		if err != nil {
			klog.Errorf("Failed to Parse memPerCpu for %/%s", pod.GetNamespace(), pod.GetName())
		} else {
			submitRequest.MemPerCpu = parseInt
		}
	}
	ntasksPerNode, ok := labels[common.LabelsResourceRequestNTasksPerNode]
	if ok {
		parseInt, err := strconv.ParseInt(ntasksPerNode, 10, 64)
		if err != nil {
			klog.Errorf("Failed to Parse ntasksPerNode for %/%s", pod.GetNamespace(), pod.GetName())
		} else {
			submitRequest.NtasksPerNode = parseInt
		}
	}
	array, ok := labels[common.LabelsResourceRequestArray]
	if ok {
		submitRequest.Array = array
	}
	ntasks, ok := labels[common.LabelsResourceRequestNTasks]
	if ok {
		parseInt, err := strconv.ParseInt(ntasks, 10, 64)
		if err != nil {
			klog.Errorf("Failed to Parse ntasks for %/%s", pod.GetNamespace(), pod.GetName())
		} else {
			submitRequest.Ntasks = parseInt
		}
	}
	return submitRequest
}

func needReconcile(pod *v1.Pod) bool {
	if pod != nil && pod.OwnerReferences != nil && len(pod.OwnerReferences) != 0 && pod.OwnerReferences[0].Kind == "DaemonSet" {
		return false
	}
	_, ok := pod.Labels[common.LabelSlurmBridgeJobId]
	if ok {
		return false
	}
	return true
}

func (s *SlurmVirtualKubeletProvider) UpdatePod(ctx context.Context, pod *v1.Pod) error {
	klog.Infof("Ignore update pod event for %s/%s, currently not support pod update", pod.GetName(), pod.GetNamespace())
	if pod.Labels != nil {
		jobId, ok := pod.Labels[common.LabelSlurmBridgeJobId]
		if !ok {
			return nil
		}
		s.knownPods.Store(pod.GetUID(), jobId)
	}
	return nil
}

func (s *SlurmVirtualKubeletProvider) DeletePod(ctx context.Context, pod *v1.Pod) error {
	defer s.knownPods.Delete(pod.GetUID())
	jobIds, ok := pod.Labels[common.LabelSlurmBridgeJobId]
	if !ok {
		value, ok := s.knownPods.Load(pod.GetUID())
		if !ok {
			return nil
		}
		jobIds = value.(string)
	}

	jobIdSlice := strings.Split(jobIds, ",")
	for _, jobId := range jobIdSlice {
		atoi, err := strconv.Atoi(jobId)
		if err != nil {
			klog.Error(err)
			continue
		}
		_, err = s.vk.SlurmClient.CancelJob(ctx, &workload.CancelJobRequest{JobId: int64(atoi)})
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *SlurmVirtualKubeletProvider) GetPod(ctx context.Context, namespace, name string) (*v1.Pod, error) {
	pod, err := s.vk.Client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil, err
	}
	jobId := s.getSlurmJobId(pod)
	if jobId != "" {
		return pod, nil
	}
	return nil, nil
}

func (s *SlurmVirtualKubeletProvider) GetPodStatus(ctx context.Context, namespace, name string) (*v1.PodStatus, error) {
	pod, err := s.GetPod(ctx, namespace, name)
	if err != nil {
		return nil, err
	}
	if pod == nil {
		return nil, nil
	}

	jobId := s.getSlurmJobId(pod)
	if jobId == "" {
		return nil, nil
	}

	parseInt, err := strconv.ParseInt(jobId, 10, 64)
	if err != nil {
		return nil, err
	}
	jobInfo, err := s.vk.SlurmClient.JobInfo(ctx, &workload.JobInfoRequest{JobId: parseInt})
	if err != nil {
		return nil, err
	}
	return convertJobInfo2PodStatus(pod, jobInfo)
}

func (s *SlurmVirtualKubeletProvider) getSlurmJobId(pod *v1.Pod) string {
	jobId, ok := pod.Labels[common.LabelSlurmBridgeJobId]
	if ok {
		return jobId
	}

	value, ok := s.knownPods.Load(pod.GetUID())
	if !ok {
		return ""
	}
	return value.(string)
}

func (s *SlurmVirtualKubeletProvider) GetPods(ctx context.Context) ([]*v1.Pod, error) {
	podList, err := s.vk.Client.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	pods := make([]*v1.Pod, 0, len(podList.Items))
	for _, item := range podList.Items {
		pods = append(pods, &item)
	}
	return pods, nil
}

func (s *SlurmVirtualKubeletProvider) GetContainerLogs(
	ctx context.Context,
	namespace, podName, containerName string,
	opts api.ContainerLogOpts,
) (io.ReadCloser, error) {
	pod, err := s.GetPod(ctx, namespace, podName)
	if err != nil {
		return nil, err
	}

	if pod == nil {
		return nil, errors.Errorf("Pod %s/%s not found", namespace, podName)
	}

	jobInfo, err := s.getJobInfo(pod)
	if err != nil {
		return nil, err
	}
	if len(jobInfo.Info) == 0 {
		return nil, errors.Errorf("Invalid jobInfo")
	}

	var logFiles = []string{jobInfo.Info[0].StdOut}
	if jobInfo.Info[0].StdOut != jobInfo.Info[0].StdErr {
		logFiles = append(logFiles, jobInfo.Info[0].StdErr)
	}

	if opts.Follow && !isSlurmJobFinished(jobInfo.Info[0].Status) {
		openReq, err := s.vk.SlurmClient.TailFile(ctx)
		if err != nil {
			return nil, err
		}
		err = openReq.Send(&workload.TailFileRequest{Path: jobInfo.Info[0].StdOut})
		if err != nil {
			return nil, err
		}
		return io.NopCloser(&TailFileReader{WorkloadManager_OpenFileClient: openReq}), nil
	} else {
		var logReq []workload.WorkloadManager_OpenFileClient
		for _, openFile := range logFiles {
			openReq, err := s.vk.SlurmClient.OpenFile(ctx, &workload.OpenFileRequest{Path: openFile})
			if err != nil {
				return nil, err
			}
			logReq = append(logReq, openReq)
		}
		return io.NopCloser(&OpenFileReader{OpenFileClient: logReq}), nil
	}
}

func (s *SlurmVirtualKubeletProvider) getJobInfo(pod *v1.Pod) (*workload.JobInfoResponse, error) {
	if pod.Status.Message == "" {
		return nil, errors.Errorf("Can not found log file path for pod %s/%s", pod.GetNamespace(), pod.GetName())
	}
	info := &workload.JobInfoResponse{}
	err := json.Unmarshal([]byte(pod.Status.Message), info)
	if err != nil {
		return nil, err
	}
	return info, nil
}

func (s *SlurmVirtualKubeletProvider) RunInContainer(ctx context.Context, namespace, podName, containerName string, cmd []string, attach api.AttachIO) error {
	return nil
}

func (s *SlurmVirtualKubeletProvider) AttachToContainer(ctx context.Context, namespace, podName, containerName string, attach api.AttachIO) error {
	return nil
}

func (s *SlurmVirtualKubeletProvider) GetStatsSummary(ctx context.Context) (*statsv1alpha1.Summary, error) {
	//pods, err := s.GetPods(ctx)
	//if err != nil {
	//	return nil, err
	//}
	//
	//var errGroup errgroup.Group
	//
	//end := time.Now()
	//start := end.Add(-1 * time.Minute)
	//chResult := make(chan statsv1alpha1.PodStats, len(pods))
	//sema := make(chan struct{}, 10)
	//for _, pod := range pods {
	//	if pod.Status.Phase != v1.PodRunning {
	//		continue
	//	}
	//	pod := pod
	//	errGroup.Go(func() error {
	//		select {
	//		case <-ctx.Done():
	//			return ctx.Err()
	//		case sema <- struct{}{}:
	//		}
	//		defer func() {
	//			<-sema
	//		}()
	//
	//		klog.Infof("Acquired semaphore")
	//		jobId := s.getSlurmJobId(pod)
	//		if jobId == "" {
	//			klog.Warningf("Failed to get pod %s/%s job id", pod.GetNamespace(), pod.GetName())
	//			return nil
	//		}
	//
	//		jobIdInt, err := strconv.ParseInt(jobId, 10, 64)
	//		if err != nil {
	//			klog.Error(err)
	//			return nil
	//		}
	//		jobStateInfo, err := s.vk.SlurmClient.JobState(ctx, &workload.JobStateRequest{JobId: jobIdInt})
	//		if err != nil {
	//			klog.Error(err)
	//			return nil
	//		}
	//
	//		chResult <- collectMetrics(pod, jobStateInfo)
	//		return nil
	//	})
	//}
	//
	//if err := errGroup.Wait(); err != nil {
	//	span.SetStatus(err)
	//	return nil, errors.Wrap(err, "error in request to fetch container group metrics")
	//}
	//close(chResult)
	//log.G(ctx).Debugf("Collected status from azure for %d pods", len(pods))
	//
	//var s stats.Summary
	//s.Node = stats.NodeStats{
	//	NodeName: p.nodeName,
	//}
	//s.Pods = make([]stats.PodStats, 0, len(chResult))
	//
	//for stat := range chResult {
	//	s.Pods = append(s.Pods, stat)
	//}

	return nil, nil
}

func (s *SlurmVirtualKubeletProvider) GetMetricsResource(ctx context.Context) ([]*io_prometheus_client.MetricFamily, error) {
	return nil, nil
}

func (s *SlurmVirtualKubeletProvider) PortForward(ctx context.Context, namespace, pod string, port int32, stream io.ReadWriteCloser) error {
	return nil
}

func (s *SlurmVirtualKubeletProvider) validateCreatePod(pod *v1.Pod) error {
	if len(pod.Spec.Containers) == 0 || len(pod.Spec.Containers) > 1 {
		return errors.Errorf("Pod container len must be 1")
	}

	if len(pod.Spec.Containers[0].Command) == 0 || len(pod.Spec.Containers[0].Command) > 1 {
		return errors.Errorf("Pod first container command's (will be submitted as sbatch script) len must be 1")
	}

	return nil
}

func (s *SlurmVirtualKubeletProvider) addAndUpdateSlurmJobInfo(pod *v1.Pod, id string) {
	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}
	_, ok := pod.Labels[common.LabelSlurmBridgeJobId]
	if ok {
		return
	}

	pod.Labels[common.LabelSlurmBridgeJobId] = id

	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	pod.Annotations[common.LabelAgentEndPoint] = s.vk.KubeletServer.AgentEndpoint

	_, err := s.vk.Client.CoreV1().Pods(pod.GetNamespace()).UpdateStatus(context.Background(), pod, metav1.UpdateOptions{})
	if err != nil {
		klog.ErrorS(err, "Failed to add slurm job id label for pod %s,%s", pod.GetNamespace()+"/"+pod.GetName())
	}
}

func (s *SlurmVirtualKubeletProvider) addOrUpdateSlurmJobInfoLabels(pod *v1.Pod, info *workload.JobInfoResponse) {
	infoDetail, err := json.Marshal(info)
	if err != nil {
		klog.Error(err)
	}

	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}

	pod.Labels[common.LabelLastJobInfo] = string(infoDetail)
	pod.ResourceVersion = "0"
	_, err = s.vk.Client.CoreV1().Pods(pod.GetNamespace()).Update(context.Background(), pod, metav1.UpdateOptions{})
	if err != nil {
		klog.ErrorS(err, "Failed to add slurm job id label for pod ", pod.GetNamespace()+"/"+pod.GetName())
	}
}

// SlurmVKNaiveNodeProvider is a basic node provider that only uses the passed in context
// on `Ping` to determine if the node is healthy.
type SlurmVKNaiveNodeProvider struct {
	vk *SlurmVirtualKubelet
}

// Ping just implements the NodeProvider interface.
// It returns the error from the passed in context only.
func (SlurmVKNaiveNodeProvider) Ping(ctx context.Context) error {
	return ctx.Err()
}

// NotifyNodeStatus implements the NodeProvider interface.
//
// This NaiveNodeProvider does not support updating node status and so this
// function is a no-op.
func (n SlurmVKNaiveNodeProvider) NotifyNodeStatus(ctx context.Context, nodeFunc func(*v1.Node)) {
	go func() {
		timer := time.NewTimer(time.Minute)
		defer timer.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				node, err := n.vk.NewNodeOrDie()
				if err != nil {
					klog.Error(err)
					continue
				}
				nodeFunc(node)
			}
		}
	}()
}
