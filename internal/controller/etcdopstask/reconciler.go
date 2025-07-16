package etcdopstask

import (
	"context"
	"fmt"
	"net/http"
	"reflect"

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
	client              client.Client
	recorder            record.EventRecorder
	logger              logr.Logger
	config              *druidconfigv1alpha1.EtcdOpsTaskControllerConfiguration
	taskHandlerRegistry task.TaskHandlerRegistry
}

// New returns a new Reconciler for EtcdOperatorTask resources.
func New(mgr manager.Manager, cfg *druidconfigv1alpha1.EtcdOpsTaskControllerConfiguration) *Reconciler {
	logger := log.Log.WithName(ControllerName)
	taskHandlerRegistry := createAndInitializeTaskHandlerRegistry()
	return &Reconciler{
		client:              mgr.GetClient(),
		recorder:            mgr.GetEventRecorderFor(ControllerName),
		logger:              logger,
		config:              cfg,
		taskHandlerRegistry: taskHandlerRegistry,
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
		if apierrors.IsNotFound(err) {
			logger.Info("Task not found, skipping reconciliation",
				"namespace", req.Namespace,
				"name", req.Name)
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	if task == nil {
		// NotFound: resource deleted or not created yet, do not requeue
		logger.Info("Task not found, skipping reconciliation",
			"namespace", req.Namespace,
			"name", req.Name)
		return reconcile.Result{}, nil
	}

	taskHandlerInstance, err := r.createTaskHandlerInstance(task, logger)
	if err != nil {
		logger.Error(err, "Failed to create task handler instance")
		return reconcile.Result{}, nil
	}
	logger.Info(fmt.Sprintf("Task handler created successfully: %T", taskHandlerInstance))

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
func (r *Reconciler) createTaskHandlerInstance(task *v1alpha1.EtcdOpsTask, logger logr.Logger) (task.Handler, error) {
	configValue := reflect.ValueOf(task.Spec.Config)
	configType := reflect.TypeOf(task.Spec.Config)
	taskType := ""

	for i := 0; i < configValue.NumField(); i++ {
		field := configValue.Field(i)
		fieldType := configType.Field(i)

		if field.Kind() == reflect.Ptr && !field.IsNil() {
			taskType = fieldType.Name
			break
		}
	}
	return r.taskHandlerRegistry.GetHandler(taskType, r.client, logger, task, nil)
}

func (r *Reconciler) shouldDeleteTask(task *v1alpha1.EtcdOpsTask) bool {
	return task.IsCompleted() || task.IsMarkedForDeletion()
}

// createAndInitializeTaskHandlerRegistry creates and initializes the task handler registry with default handlers.
func createAndInitializeTaskHandlerRegistry() task.TaskHandlerRegistry {
	registry := task.NewTaskHandlerRegistry()

	// Register OnDemandSnapshot handler
	registry.Register("OnDemandSnapshot", func(k8sclient client.Client, logger logr.Logger, task *v1alpha1.EtcdOpsTask, httpClient *http.Client) (task.Handler, error) {
		return ondemandsnapshot.NewWithHTTPClient(k8sclient, logger, task, httpClient)
	})

	return registry
}

// GetTaskHandlerRegistry returns the task handler registry for testing purposes.
func (r *Reconciler) GetTaskHandlerRegistry() task.TaskHandlerRegistry {
	return r.taskHandlerRegistry
}
