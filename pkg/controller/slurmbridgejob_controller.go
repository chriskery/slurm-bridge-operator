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

package controller

import (
	"context"
	"fmt"
	"github.com/chriskery/slurm-bridge-operator/apis/kubecluster.org/v1alpha1"
	"github.com/chriskery/slurm-bridge-operator/pkg/common"
	"github.com/chriskery/slurm-bridge-operator/pkg/common/util"
	"github.com/go-logr/logr"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubeclientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
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
)

const (
	ControllerName = "slurm-agent-bridge-operator"
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

	// Recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	Recorder record.EventRecorder

	// KubeClientSet is a standard kubernetes clientset.
	KubeClientSet kubeclientset.Interface
}

//+kubebuilder:rbac:groups=kubecluster.org.kubecluster.org,resources=slurmbridgejobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubecluster.org.kubecluster.org,resources=slurmbridgejobs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubecluster.org.kubecluster.org,resources=slurmbridgejobs/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;update;list;watch;create;delete
//+kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the SlurmBridgeJob object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.0/pkg/reconcile
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
		r.Recorder.Eventf(sjb, corev1.EventTypeWarning, common.NewReason(v1alpha1.SlurmBridgeJobKind, common.SlurmBridgeJobFailedValidationReason),
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

	if err = r.ReconcileSlurmBridgeJob(sjb); err != nil {
		logrus.Warnf("Reconcile Kube Cluster error %v", err)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
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

// SetupWithManager sets up the controller with the Manager.
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
	// inject watching for cluster related service
	if err = c.Watch(source.Kind(mgr.GetCache(), &corev1.ConfigMap{}), eventHandler); err != nil {
		return err
	}

	return nil
}

func (r *SlurmBridgeJobReconciler) ReconcileSlurmBridgeJob(sjb *v1alpha1.SlurmBridgeJob) error {
	// Check if this Pod already exists
	namespaceName := types.NamespacedName{Name: sjb.Name, Namespace: sjb.Namespace}
	// Fetch the SlurmJob instance
	sjCurrentPod := &corev1.Pod{}
	err := r.Client.Get(context.Background(), namespaceName, sjCurrentPod)
	if err != nil && errors.IsNotFound(err) {
		if len(sjb.Status.SubjobStatus) != 0 {
			logrus.Info("Pod will not be created, it was already created once")
			return err
		}

		// Translate SlurmJob to Pod
		sjPod, err := r.newPodForSJ(sjb)
		if err != nil {
			logrus.Errorf("Could not translate slurm-agent job into pod: %v", err)
			return err
		}

		// Set SlurmJob instance as the owner and controller
		err = controllerutil.SetControllerReference(sjb, sjPod, r.Scheme)
		if err != nil {
			logrus.Errorf("Could not set controller reference for pod: %v", err)
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

	logrus.Infof("Updating slurm-agent job %q", sjb.Name)
	// Otherwise smth has changed, need to update things
	annotations := sjCurrentPod.GetAnnotations()
	jobId, exists := annotations[common.SlurmBridgeJobIdAnnotation]
	if exists {
		subJobStatus, subJobExist := sjb.Status.SubjobStatus[v1alpha1.SlurmJobId(jobId)]
		if !subJobExist {
			subJobStatus = &v1alpha1.SlurmSubjobStatus{}
		}
		subJobStatus.ID = jobId
	}

	err = r.Client.Status().Update(context.Background(), sjb)
	if err != nil {
		logrus.Errorf("Could not update slurm-agent job: %v", err)
		return err
	}
	return nil
}
