package reconcile

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"k8s.io/apimachinery/pkg/types"
)

type task struct {
	Etcd        string
	TimeElapsed string
	Reconciled  bool
	Updated     bool
	Done        bool
}

func setupReconcileStatusTable(tasks []task) *table.Table {
	columns := []string{"Etcd", "Reconcile Triggered", "Pods Upto Date", "Time Taken To Reconcile", "Reconciliation Completed"}
	var rows [][]string
	for _, task := range tasks {
		rows = append(rows, []string{
			task.Etcd,
			fmt.Sprintf("%t", task.Reconciled),
			fmt.Sprintf("%t", task.Updated),
			task.TimeElapsed,
			fmt.Sprintf("%t", task.Done),
		})
	}
	t := table.New().
		Border(lipgloss.NormalBorder()).
		Headers(columns...).
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			return lipgloss.NewStyle()
		})
	return t
}

type statusManager struct {
	sync.RWMutex
	data map[types.NamespacedName]*printResult
}

type printResult struct {
	reconcileTriggered bool
	updated            bool
	startTime          *time.Time
	endTime            *time.Time
}

func newStatusManager() *statusManager {
	return &statusManager{
		data: make(map[types.NamespacedName]*printResult),
	}
}

func (sm *statusManager) initStatus(key types.NamespacedName, startTime time.Time) {
	sm.Lock()
	defer sm.Unlock()
	sm.data[key] = &printResult{
		startTime: &startTime,
	}
}

func (sm *statusManager) setReconcileTriggered(key types.NamespacedName) {
	sm.Lock()
	defer sm.Unlock()
	if result, exists := sm.data[key]; exists {
		result.reconcileTriggered = true
	}
}

func (sm *statusManager) setUpdated(key types.NamespacedName) {
	sm.Lock()
	defer sm.Unlock()
	if result, exists := sm.data[key]; exists {
		result.updated = true
	}
}

func (sm *statusManager) setComplete(key types.NamespacedName) {
	sm.Lock()
	defer sm.Unlock()
	if result, exists := sm.data[key]; exists {
		now := time.Now()
		result.endTime = &now
	}
}

func (sm *statusManager) getStatus() map[types.NamespacedName]*printResult {
	sm.RLock()
	defer sm.RUnlock()

	// Create a deep copy to avoid concurrent access issues
	result := make(map[types.NamespacedName]*printResult)
	for k, v := range sm.data {
		result[k] = &printResult{
			reconcileTriggered: v.reconcileTriggered,
			updated:            v.updated,
			startTime:          v.startTime,
			endTime:            v.endTime,
		}
	}
	return result
}

func printReconcileStatus(sm *statusManager) {
	// Get a safe copy of the current status
	statusMap := sm.getStatus()

	tasks := []task{}
	for namespacedName, result := range statusMap {
		timeElapsed := "N/A"
		done := result.endTime != nil
		if result.startTime != nil {
			if done {
				timeElapsed = result.endTime.Sub(*result.startTime).Round(time.Second).String()
			} else {
				timeElapsed = time.Since(*result.startTime).Round(time.Second).String()
			}
		}

		task := task{
			Etcd:        fmt.Sprintf("%s/%s", namespacedName.Namespace, namespacedName.Name),
			Reconciled:  result.reconcileTriggered,
			Updated:     result.updated,
			TimeElapsed: timeElapsed,
			Done:        done,
		}
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].Etcd < tasks[j].Etcd
	})
	table := setupReconcileStatusTable(tasks)
	fmt.Printf("\nReconciliation Status:\n")
	fmt.Println(table)
	fmt.Printf("\n")
}
