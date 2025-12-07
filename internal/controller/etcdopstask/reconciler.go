// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdopstask

import (
	"context"
	"fmt"

	druidconfigv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/controller/etcdopstask/handler"
	"github.com/gardener/etcd-druid/internal/controller/etcdopstask/handler/ondemandsnapshot"
	ctrlutils "github.com/gardener/etcd-druid/internal/controller/utils"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// reconcileFn defines a function signature for a reconciliation step.
type reconcileFn func(ctx context.Context, logger logr.Logger, task *druidv1alpha1.EtcdOpsTask, taskHandler handler.Handler) ctrlutils.ReconcileStepResult

// Reconciler reconciles EtcdOpsTask resources.
type Reconciler struct {
	client              client.Client
	logger              logr.Logger
	config              *druidconfigv1alpha1.EtcdOpsTaskControllerConfiguration
	taskHandlerRegistry handler.TaskHandlerRegistry
}

// NewReconciler returns a new Reconciler for EtcdOpsTask resources.
func NewReconciler(mgr manager.Manager, cfg *druidconfigv1alpha1.EtcdOpsTaskControllerConfiguration) *Reconciler {
	taskHandlerRegistry := DefaultTaskHandlerRegistry()
	return NewReconcilerWithTaskHandlerRegistry(mgr, cfg, taskHandlerRegistry)
}

// NewReconcilerWithTaskHandlerRegistry returns a new Reconciler with a custom task handler registry.
func NewReconcilerWithTaskHandlerRegistry(mgr manager.Manager, cfg *druidconfigv1alpha1.EtcdOpsTaskControllerConfiguration, taskHandlerRegistry handler.TaskHandlerRegistry) *Reconciler {
	logger := log.Log.WithName(controllerName)
	return &Reconciler{
		client:              mgr.GetClient(),
		logger:              logger,
		config:              cfg,
		taskHandlerRegistry: taskHandlerRegistry,
	}
}

// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcdopstasks,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcdopstasks/status,verbs=get;create;update;patch

// Reconcile is the main reconciliation loop for EtcdOpsTask resources.
func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := r.logger.WithValues(
		"runID", string(controller.ReconcileIDFromContext(ctx)),
		"namespace", req.Namespace,
		"name", req.Name,
	)
	logger.Info("Reconciling EtcdOpsTask")

	task, err := r.getTask(ctx, req.NamespacedName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(5).Info("Task not found, skipping reconciliation")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	taskHandlerInstance, err := r.getTaskHandler(task)
	if err != nil {
		logger.Error(err, "Failed to create task handler instance")
		return reconcile.Result{}, err
	}

	if r.shouldDeleteTask(task) {
		return r.triggerDeletionFlow(ctx, logger, task, taskHandlerInstance).ReconcileResult()
	}

	result := r.reconcileTask(ctx, logger, task, taskHandlerInstance)
	return result.ReconcileResult()

}

// getTaskHandler instantiates the appropriate TaskHandler for the given task.
func (r *Reconciler) getTaskHandler(task *druidv1alpha1.EtcdOpsTask) (handler.Handler, error) {
	config := task.Spec.Config

	switch {
	case config.OnDemandSnapshot != nil:
		return r.taskHandlerRegistry.GetHandler("OnDemandSnapshot", r.client, task, nil)
	default:
		return nil, fmt.Errorf("unsupported task configuration: no valid task type found")
	}
}

func (r *Reconciler) shouldDeleteTask(task *druidv1alpha1.EtcdOpsTask) bool {
	return task.IsCompleted() || druidv1alpha1.IsResourceMarkedForDeletion(task.ObjectMeta)
}

// DefaultTaskHandlerRegistry creates and initializes the task handler registry with default handlers.
func DefaultTaskHandlerRegistry() handler.TaskHandlerRegistry {
	registry := handler.NewTaskHandlerRegistry()

	// Register OnDemandSnapshot handler
	registry.Register("OnDemandSnapshot", ondemandsnapshot.New)
	return registry
}

// GetTaskHandlerRegistry returns the task handler registry for testing purposes.
func (r *Reconciler) GetTaskHandlerRegistry() handler.TaskHandlerRegistry {
	return r.taskHandlerRegistry
}
