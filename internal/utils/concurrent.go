package utils

import (
	"fmt"
	"runtime/debug"
	"sync"

	"github.com/gardener/etcd-druid/internal/operator/resource"
)

// OperatorTask is a holder for a named function.
type OperatorTask struct {
	// Name is the name of the task
	Name string
	// Fn is the function which accepts an operator context and returns an error if there is one.
	// Implementations of Fn should handle context cancellation properly.
	Fn func(ctx resource.OperatorContext) error
}

// RunConcurrently runs tasks concurrently with number of goroutines bounded by bound.
// If there is a panic executing a single OperatorTask then it will capture the panic and capture it as an error
// which will then subsequently be returned from this function. It will not propagate the panic causing the app to exit.
func RunConcurrently(ctx resource.OperatorContext, tasks []OperatorTask) []error {
	rg := newRunGroup(len(tasks))
	for _, task := range tasks {
		rg.trigger(ctx, task)
	}
	return rg.WaitAndCollectErrors()
}

// runGroup is a runner for concurrently spawning multiple asynchronous tasks. If any task
// errors or panics then these are captured as errors.
type runGroup struct {
	wg    sync.WaitGroup
	errCh chan error
}

// newRunGroup creates a new runGroup.
func newRunGroup(numTasks int) *runGroup {
	return &runGroup{
		wg:    sync.WaitGroup{},
		errCh: make(chan error, numTasks),
	}
}

// trigger executes the task in a go-routine.
func (g *runGroup) trigger(ctx resource.OperatorContext, task OperatorTask) {
	g.wg.Add(1)
	go func(task OperatorTask) {
		defer g.wg.Done()
		defer func() {
			// recovers from a panic if there is one. Creates an error from it which contains the debug stack
			// trace as well and pushes the error to the provided error channel.
			if v := recover(); v != nil {
				stack := debug.Stack()
				panicErr := fmt.Errorf("task: %s execution panicked: %v\n, stack-trace: %s", task.Name, v, stack)
				g.errCh <- panicErr
			}
		}()
		err := task.Fn(ctx)
		if err != nil {
			g.errCh <- err
		}
	}(task)
}

// WaitAndCollectErrors waits for all tasks to finish, collects and returns any errors.
func (g *runGroup) WaitAndCollectErrors() []error {
	g.wg.Wait()
	close(g.errCh)

	var errs []error
	for err := range g.errCh {
		errs = append(errs, err)
	}
	return errs
}
