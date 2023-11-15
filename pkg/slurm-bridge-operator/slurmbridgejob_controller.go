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

package slurm_bridge_operator

import (
	"context"
	"fmt"
	"github.com/chriskery/slurm-bridge-operator/apis/kubecluster.org/v1alpha1"
	"github.com/chriskery/slurm-bridge-operator/pkg/common"
	"github.com/chriskery/slurm-bridge-operator/pkg/common/util"
	"github.com/chriskery/slurm-bridge-operator/pkg/workload"
	"github.com/go-logr/logr"
	"github.com/sirupsen/logrus"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	kubeclientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"strconv"
	"time"
)

const (
	ControllerName = "slurm-bridge-operator"
)

func NewReconciler(mgr manager.Manager) *SlurmBridgeJobReconciler {
	r := &SlurmBridgeJobReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		recorder:  mgr.GetEventRecorderFor(ControllerName),
		apiReader: mgr.GetAPIReader(),
		Log:       log.Log,
		WorkQueue: &util.FakeWorkQueue{},
	}

	cfg := mgr.GetConfig()
	kubeClientSet := kubeclientset.NewForConfigOrDie(cfg)
	r.KubeClientSet = kubeClientSet

	return r
}

// SlurmBridgeJobReconciler reconciles a SlurmBridgeJob object
type SlurmBridgeJobReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	recorder  record.EventRecorder
	apiReader client.Reader
	Log       logr.Logger

	// WorkQueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	WorkQueue workqueue.RateLimitingInterface

	// KubeClientSet is a standard kubernetes clientset.
	KubeClientSet kubeclientset.Interface
}

//+kubebuilder:rbac:groups=kubecluster.org,resources=slurmbridgejobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubecluster.org,resources=slurmbridgejobs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubecluster.org,resources=slurmbridgejobs/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;update;list;watch;create;delete
//+kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="batch",resources=jobs,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *SlurmBridgeJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	logger := r.Log.WithValues(v1alpha1.SlurmBridgeJobSingular, req.NamespacedName)

	sjb := &v1alpha1.SlurmBridgeJob{}
	err := r.Get(ctx, req.NamespacedName, sjb)
	if err != nil {
		logger.Info(err.Error(), "unable to fetch slurm-agent bridge job", req.NamespacedName.String())
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if err = v1alpha1.ValidateV1alphaSlurmBridgeJob(sjb); err != nil {
		r.recorder.Eventf(sjb, corev1.EventTypeWarning, common.NewReason(v1alpha1.SlurmBridgeJobKind, common.SlurmBridgeJobFailedValidationReason),
			"SlurmBridgeJob failed validation because %s", err)
		return ctrl.Result{}, err
	}

	if sjb.GetDeletionTimestamp() != nil {
		logger.Info("reconcile cancelled, SlurmBridgeJob has been deleted", "deleted", sjb.GetDeletionTimestamp() != nil)
		return ctrl.Result{}, nil
	}

	sjb = sjb.DeepCopy()
	// Set default priorities to kubecluster
	r.Scheme.Default(sjb)

	needReconcile := isSBJFinished(sjb)
	if !needReconcile && sjb.Spec.Result == nil {
		return ctrl.Result{}, nil
	}

	oldStatus := sjb.Status.DeepCopy()
	if !needReconcile {
		if sjb.Status.FetchResult && isFinishedFetchResult(sjb.Status.FetchResultStatus) {
			return ctrl.Result{}, nil
		}
		if err = r.ReconcileSlurmBridgeJobResult(sjb); err != nil {
			return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 30}, err
		}
	} else {
		if err = r.ReconcileSlurmBridgeJob(sjb); err != nil {
			logrus.Warnf("Reconcile Kube Cluster error %v", err)
			return ctrl.Result{}, err
		}
	}

	// No need to update the cluster status if the status hasn't changed since last time.
	if !reflect.DeepEqual(*oldStatus, &sjb.Status) {
		logrus.Infof("Updating slurm-agent job %q", sjb.Name)
		if err = r.Client.Status().Update(context.Background(), sjb); err != nil {
			logrus.Errorf("Could not update slurm-agent job: %v", err)
			return ctrl.Result{}, nil
		}
	}
	return ctrl.Result{}, nil
}

func isSBJFinished(sjb *v1alpha1.SlurmBridgeJob) bool {
	return !(sjb.Status.State == string(corev1.PodSucceeded) || sjb.Status.State == string(corev1.PodFailed))
}

// onOwnerCreateFunc modify creation condition.
func (r *SlurmBridgeJobReconciler) onOwnerCreateFunc() func(event.CreateEvent) bool {
	return func(e event.CreateEvent) bool {
		sbj, ok := e.Object.(*v1alpha1.SlurmBridgeJob)
		if !ok {
			return true
		}

		r.Scheme.Default(sbj)
		msg := fmt.Sprintf("Kubecluster %s is created.", e.Object.GetName())
		logrus.Info(msg)
		if len(sbj.Status.State) == 0 {
			sbj.Status.State = common.SlurmBridgeJobSubmitting
		}
		return true
	}
}

// SetupWithManager sets up the slurm-bridge-operator with the Manager.
func (r *SlurmBridgeJobReconciler) SetupWithManager(mgr ctrl.Manager, controllerThreads int) error {
	c, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler:              r,
		MaxConcurrentReconciles: controllerThreads,
	})
	if err != nil {
		return err
	}

	// using onOwnerCreateFunc is easier to set defaults
	if err = c.Watch(source.Kind(mgr.GetCache(), &v1alpha1.SlurmBridgeJob{}), &handler.EnqueueRequestForObject{},
		predicate.Funcs{CreateFunc: r.onOwnerCreateFunc()},
	); err != nil {
		return err
	}

	// eventHandler for owned objects
	eventHandler := handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &v1alpha1.SlurmBridgeJob{}, handler.OnlyControllerOwner())

	// inject watching for cluster related pod
	if err = c.Watch(source.Kind(mgr.GetCache(), &corev1.Pod{}), eventHandler); err != nil {
		return err
	}

	return nil
}

func (r *SlurmBridgeJobReconciler) ReconcileSlurmBridgeJob(sjb *v1alpha1.SlurmBridgeJob) error {
	// Check if this Pod already exists
	sizeCardNamespaceName := types.NamespacedName{Name: genPodNameByPodRole(sjb, v1alpha1.SlurmBridgeJobPodRoleSizeCar), Namespace: sjb.Namespace}
	// Fetch the SlurmJob instance
	sjSizeCarPod := &corev1.Pod{}
	err := r.Client.Get(context.Background(), sizeCardNamespaceName, sjSizeCarPod)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		sjSizeCarPod, err = r.ReconcileSizeCarPods(sjb)
		if err != nil {
			return err
		}
	}

	if err = r.UpdateSBJStatus(sjb, sjSizeCarPod); err != nil {
		return err
	}

	// Check if this Pod already exists
	workerNamespaceName := types.NamespacedName{Name: genPodNameByPodRole(sjb, v1alpha1.SlurmBridgeJobPodRoleSizeWorker), Namespace: sjb.Namespace}
	// Fetch the SlurmJob instance
	err = r.Client.Get(context.Background(), workerNamespaceName, &corev1.Pod{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		if err = r.ReconcileWorkerPods(sjSizeCarPod, sjb); err != nil {
			return err
		}
	}
	return nil
}

func (r *SlurmBridgeJobReconciler) UpdateSBJStatus(sjb *v1alpha1.SlurmBridgeJob, sjCurrentPod *corev1.Pod) error {
	if sjb.Status.State != string(sjCurrentPod.Status.Phase) {
		r.recorder.Eventf(sjb, corev1.EventTypeNormal, common.NewReason(v1alpha1.SlurmBridgeJobKind, common.SlurmBridgeJobStatusChangeReason),
			"SlurmBridgeJob status change to %s", sjCurrentPod.Status.Phase)
		sjb.Status.State = string(sjCurrentPod.Status.Phase)
	}

	if sjCurrentPod.Annotations != nil {
		endpoint, ok := sjCurrentPod.Annotations[common.LabelAgentEndPoint]
		if ok {
			sjb.Status.ClusterEndPoint = endpoint
		}
	}

	// Otherwise smth has changed, need to update things
	if sjCurrentPod.Status.Message != "" {
		info := &workload.JobInfoResponse{}
		err := json.Unmarshal([]byte(sjCurrentPod.Status.Message), info)
		if err != nil {
			return err
		}

		if sjb.Status.SubjobStatus == nil {
			sjb.Status.SubjobStatus = make(map[v1alpha1.SlurmJobId]*v1alpha1.SlurmSubjobStatus)
		}
		for _, jobInfo := range info.Info {
			sjb.Status.SubjobStatus[v1alpha1.SlurmJobId(jobInfo.Id)] = &v1alpha1.SlurmSubjobStatus{
				ID:         jobInfo.Id,
				UserID:     jobInfo.UserId,
				ArrayJobID: jobInfo.ArrayId,
				Name:       jobInfo.Name,
				ExitCode:   jobInfo.ExitCode,
				State:      jobInfo.Status.String(),
				SubmitTime: util.GetMetaTimePointer(jobInfo.SubmitTime.AsTime()),
				StartTime:  util.GetMetaTimePointer(jobInfo.StartTime.AsTime()),
				RunTime:    jobInfo.RunTime.String(),
				TimeLimit:  jobInfo.TimeLimit.String(),
				WorkDir:    jobInfo.WorkingDir,
				StdOut:     jobInfo.StdErr,
				StdErr:     jobInfo.StdOut,
				Partition:  jobInfo.Partition,
				NodeList:   jobInfo.NodeList,
				BatchHost:  jobInfo.BatchHost,
				NumNodes:   jobInfo.NumNodes,
			}
		}
	}
	return nil
}

func (r *SlurmBridgeJobReconciler) ReconcileSizeCarPods(sjb *v1alpha1.SlurmBridgeJob) (*corev1.Pod, error) {
	if len(sjb.Status.SubjobStatus) != 0 {
		logrus.Info("Pod will not be created, it was already created once")
		r.recorder.Eventf(sjb, corev1.EventTypeWarning, common.NewReason(v1alpha1.SlurmBridgeJobKind, common.SlurmBridgeJobFailedReason),
			"SlurmBridgeJob status change to failed, because pod not found but sub job status not empty")
		sjb.Status.State = string(corev1.PodFailed)
		return nil, nil
	}

	// Translate SlurmJob to Pod
	sjPod, err := r.newSizeCarPodForSJ(sjb)
	if err != nil {
		logrus.Errorf("Could not translate slurm-agent job into pod: %v", err)
		return nil, err
	}

	logrus.Infof("Creating new pod %q for slurm-agent job %q", sjPod.Name, sjb.Name)
	err = r.Create(context.Background(), sjPod)
	if err != nil {
		logrus.Errorf("Could not create new pod: %v", err)
		return nil, err
	}
	return sjPod, nil
}

func (r *SlurmBridgeJobReconciler) ReconcileSlurmBridgeJobResult(sjb *v1alpha1.SlurmBridgeJob) error {
	if len(sjb.Status.SubjobStatus) == 0 {
		r.recorder.Eventf(sjb, corev1.EventTypeWarning, common.NewReason(v1alpha1.SlurmBridgeJobKind, common.SlurmBridgeJobFailedValidationReason),
			"SlurmBridgeJob had not sub jobs found, skip fetch result")
		return nil
	}

	fetchResultJob := &batchv1.Job{}
	err := r.Get(
		context.Background(),
		types.NamespacedName{Name: fmt.Sprintf("%s-result-fetcher", sjb.Name), Namespace: sjb.Namespace},
		fetchResultJob,
	)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		if sjb.Status.FetchResult {
			return nil
		}
		job := r.newJobForSJResult(sjb)
		if err = r.Create(context.Background(), job); err != nil {
			return err
		}
		r.recorder.Eventf(sjb, corev1.EventTypeNormal, common.NewReason(v1alpha1.SlurmBridgeJobKind, common.SlurmBridgeJobResultFetchCreatedReason),
			"SlurmBridgeJob begin to fetch result")
		sjb.Status.FetchResult = true
	}
	if fetchResultJob.Status.Succeeded > 0 {
		sjb.Status.FetchResultStatus = string(corev1.PodSucceeded)
		r.recorder.Eventf(sjb, corev1.EventTypeNormal, common.NewReason(v1alpha1.SlurmBridgeJobKind, common.SlurmBridgeJobResultFetchSucceedReason),
			"SlurmBridgeJob fetch result status change to %s", common.SlurmBridgeJobResultFetchSucceedReason)
	} else if fetchResultJob.Status.Failed > 0 {
		sjb.Status.FetchResultStatus = string(corev1.PodFailed)
		r.recorder.Eventf(sjb, corev1.EventTypeNormal, common.NewReason(v1alpha1.SlurmBridgeJobKind, common.SlurmBridgeJobResultFetchFailReason),
			"SlurmBridgeJob fetch result status change to %s", common.SlurmBridgeJobFailedReason)
	} else {
		sjb.Status.FetchResultStatus = string(corev1.PodRunning)
		r.recorder.Eventf(sjb, corev1.EventTypeNormal, common.NewReason(v1alpha1.SlurmBridgeJobKind, common.SlurmBridgeJobResultFetchRunningReason),
			"SlurmBridgeJob fetch result status change to %s", common.SlurmBridgeJobResultFetchRunningReason)
	}
	return nil
}

func (r *SlurmBridgeJobReconciler) ReconcileWorkerPods(sizecarPod *corev1.Pod, sjb *v1alpha1.SlurmBridgeJob) error {
	labels := sizecarPod.GetLabels()
	if labels == nil {
		r.recorder.Event(sjb, corev1.EventTypeNormal, common.NewReason(v1alpha1.SlurmBridgeJobKind, common.SlurmBridgeJobResultFetchRunningReason),
			"Waiting size car pod to be submitted")
		return nil
	}
	_, ok := labels[common.LabelSlurmBridgeJobId]
	if !ok {
		r.recorder.Event(sjb, corev1.EventTypeNormal, common.NewReason(v1alpha1.SlurmBridgeJobKind, common.SlurmBridgeJobResultFetchRunningReason),
			"Waiting size car pod to be submitted")
		return nil
	}
	info := &workload.JobInfoResponse{}
	err := json.Unmarshal([]byte(sizecarPod.Status.Message), info)
	if err != nil {
		return err
	}
	if len(info.Info) == 0 {
		r.recorder.Event(sjb, corev1.EventTypeNormal, common.NewReason(v1alpha1.SlurmBridgeJobKind, common.SlurmBridgeJobResultFetchRunningReason),
			"Waiting size car pod to be submitted")
		return nil
	}
	// Translate SlurmJob to Pod
	sjPod, err := r.newWorkerPodForSJ(sizecarPod, sjb, info)
	if err != nil {
		logrus.Errorf("Could not translate slurm-agent job into pod: %v", err)
		return err
	}

	logrus.Infof("Creating new pod %q for slurm-agent job %q", sjPod.Name, sjb.Name)
	err = r.Create(context.Background(), sjPod)
	if err != nil {
		logrus.Errorf("Could not create new pod: %v", err)
		return err
	}
	return nil
}

func (r *SlurmBridgeJobReconciler) newWorkerPodForSJ(sizecarPod *corev1.Pod, sjb *v1alpha1.SlurmBridgeJob, info *workload.JobInfoResponse) (*corev1.Pod, error) {
	labels := sizecarPod.GetLabels()
	labels[common.LabelsRole] = v1alpha1.SlurmBridgeJobPodRoleSizeWorker

	var containers []corev1.Container
	for _, jobInfo := range info.Info {
		sc := r.getWorkerPodContainerSecurityContext(jobInfo)
		containers = append(containers, corev1.Container{
			Name:            jobInfo.Id,
			SecurityContext: sc,
			WorkingDir:      jobInfo.WorkingDir,
		})
	}

	sjPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      genPodNameByPodRole(sjb, v1alpha1.SlurmBridgeJobPodRoleSizeWorker),
			Namespace: sjb.Namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			//skipped scheduling
			NodeName: sizecarPod.Spec.NodeName,
			SecurityContext: &corev1.PodSecurityContext{
				RunAsUser:  sjb.Spec.RunAsUser,
				RunAsGroup: sjb.Spec.RunAsGroup,
			},
			Tolerations:   v1alpha1.DefaultTolerations,
			Containers:    containers,
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}

	// Set SlurmJob instance as the owner and slurm-bridge-operator
	err := controllerutil.SetControllerReference(sjb, sjPod, r.Scheme)
	if err != nil {
		logrus.Errorf("Could not set slurm-bridge-operator reference for pod: %v", err)
		return nil, err
	}
	return sjPod, nil
}

func (r *SlurmBridgeJobReconciler) getWorkerPodContainerSecurityContext(info *workload.JobInfo) *corev1.SecurityContext {
	userId := info.GetUserId()
	parseInt, _ := strconv.ParseInt(userId, 10, 64)
	return &corev1.SecurityContext{RunAsUser: &parseInt}
}
