package etcdopstask

import (
	"context"
	"fmt"

	druidconfigv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"
	"github.com/gardener/etcd-druid/api/core/v1alpha1"
	ctrlutils "github.com/gardener/etcd-druid/internal/controller/utils"
	"github.com/gardener/etcd-druid/internal/task"
	"github.com/gardener/etcd-druid/internal/task/ondemandsnapshot"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Defines the Controller name and finalizer for EtcdOperatorTask resources.
const (
	ControllerName = "etcdopstask-controller"
	FinalizerName  = "etcd-druid.gardener.cloud/etcd-operator-task"
)

// reconcileFn defines a function signature for a reconciliation step.
type reconcileFn func(ctx context.Context, taskObjKey client.ObjectKey, taskHandler task.Handler) ctrlutils.ReconcileStepResult

// StepFunction represents a single step in the reconciliation process.
type StepFunction struct {
	StepName string
	StepFunc reconcileFn
}

// Reconciler reconciles EtcdOperatorTask resources.
type Reconciler struct {
	client   client.Client
	recorder record.EventRecorder
	logger   logr.Logger
	config   *druidconfigv1alpha1.EtcdOpsTaskControllerConfiguration
}

// New returns a new Reconciler for EtcdOperatorTask resources.
func New(mgr manager.Manager, cfg *druidconfigv1alpha1.EtcdOpsTaskControllerConfiguration) *Reconciler {
	logger := log.Log.WithName(ControllerName)
	return &Reconciler{
		client:   mgr.GetClient(),
		recorder: mgr.GetEventRecorderFor(ControllerName),
		logger:   logger,
		config:   cfg,
	}
}

// Reconcile is the main reconciliation loop for EtcdOperatorTask resources.
func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := r.logger.WithValues(
		"runId", string(controller.ReconcileIDFromContext(ctx)),
		"namespace", req.Namespace,
		"name", req.Name,
	)
	logger.Info("Reconciling EtcdOperatorTask")

	task, err := r.getTask(ctx, req.NamespacedName)
	if err != nil {
		// Per k8s standards, return error to trigger requeue with backoff
		if apierrors.IsNotFound(err) {
			logger.Info("Task not found, skipping reconciliation")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	if task == nil {
		// NotFound: resource deleted or not created yet, do not requeue
		logger.Info("Task not found, skipping reconciliation")
		return reconcile.Result{}, nil
	}

	taskHandlerInstance, err := r.createTaskHandlerInstance(task, logger)
	if err != nil {
		// Permanent error: do not requeue, but log for visibility
		logger.Error(err, "Failed to create task handler instance")
		return reconcile.Result{}, nil
	}

	if r.shouldDeleteTask(task) {
		return r.triggerDeletionFlow(ctx, taskHandlerInstance, task).ReconcileResult()
	}

	result := r.reconcileTask(ctx, client.ObjectKeyFromObject(task), taskHandlerInstance)
	if result.HasErrors() || result.NeedsRequeue() {
		return result.ReconcileResult()
	}

	task, err = r.getTask(ctx, req.NamespacedName)
	if err != nil {
		return reconcile.Result{}, err
	}

	if task.IsCompleted() {
		return reconcile.Result{RequeueAfter: task.GetTimeToExpiry()}, nil
	}
	return reconcile.Result{}, nil
}

// createTaskHandlerInstance instantiates the appropriate TaskHandler for the given task.
// Returns (nil, nil) if the task type is not supported (should not happen if webhook validation is correct).
func (r *Reconciler) createTaskHandlerInstance(task *v1alpha1.EtcdOpsTask, logger logr.Logger) (task.Handler, error) {
	if task.Spec.Config.OnDemandSnapshot != nil {
		return ondemandsnapshot.New(r.client, logger, task)
	}
	// Should never happen if webhook validation is correct
	logger.Info("Task type not supported", "taskType", task.Spec.Config)
	return nil, fmt.Errorf("task type not supported")
}

func (r *Reconciler) shouldDeleteTask(task *v1alpha1.EtcdOpsTask) bool {
	return task.IsCompleted() || task.IsMarkedForDeletion()
}
