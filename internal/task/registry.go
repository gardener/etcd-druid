package task

import (
	"fmt"
	"net/http"

	"github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TaskHandlerFactory defines a function signature for creating task handlers.
type TaskHandlerFactory func(k8sclient client.Client, logger logr.Logger, task *v1alpha1.EtcdOpsTask, httpClient *http.Client) (Handler, error)

// TaskHandlerRegistry manages the registration and retrieval of task handlers.
type TaskHandlerRegistry interface {
	// Register registers a task handler factory for a given task type.
	Register(taskType string, factory TaskHandlerFactory)
	// GetHandler creates and returns a task handler for the given task type.
	GetHandler(taskType string, k8sclient client.Client, logger logr.Logger, task *v1alpha1.EtcdOpsTask, httpClient *http.Client) (Handler, error)
}

// taskHandlerRegistry implements TaskHandlerRegistry.
type taskHandlerRegistry struct {
	taskhandlers map[string]TaskHandlerFactory
}

// Register registers a task handler factory for a given task type.
func (t *taskHandlerRegistry) Register(taskType string, factory TaskHandlerFactory) {
	t.taskhandlers[taskType] = factory
}

// GetHandler creates and returns a task handler for the given task type.
func (t *taskHandlerRegistry) GetHandler(taskType string, k8sclient client.Client, logger logr.Logger, task *v1alpha1.EtcdOpsTask, httpClient *http.Client) (Handler, error) {
	taskHandler, exists := t.taskhandlers[taskType]
	if !exists {
		return nil, fmt.Errorf("task type %s not supported", taskType)
	}
	return taskHandler(k8sclient, logger, task, httpClient)
}

// NewTaskHandlerRegistry creates a new task handler registry.
func NewTaskHandlerRegistry() TaskHandlerRegistry {
	return &taskHandlerRegistry{
		taskhandlers: make(map[string]TaskHandlerFactory),
	}
}



