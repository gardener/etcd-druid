// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package reconciliation

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
	etcd        string
	timeElapsed string
	reconciled  bool
	updated     bool
	done        bool
}

// setupReconcileStatusTable builds a table summarizing reconcile status for each Etcd.
func setupReconcileStatusTable(tasks []task) *table.Table {
	columns := []string{"Etcd", "Reconcile Triggered", "Pods Upto Date", "Time Taken To Reconcile", "Reconciliation Completed"}
	var rows [][]string
	for _, task := range tasks {
		rows = append(rows, []string{
			task.etcd,
			fmt.Sprintf("%t", task.reconciled),
			fmt.Sprintf("%t", task.updated),
			task.timeElapsed,
			fmt.Sprintf("%t", task.done),
		})
	}
	t := table.New().
		Border(lipgloss.NormalBorder()).
		Headers(columns...).
		Rows(rows...).
		StyleFunc(func(_, _ int) lipgloss.Style {
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

func (sm *statusManager) setComplete(key types.NamespacedName, endTime time.Time) {
	sm.Lock()
	defer sm.Unlock()
	if result, exists := sm.data[key]; exists {
		result.endTime = &endTime
	}
}

func (sm *statusManager) getStartTime(key types.NamespacedName) *time.Time {
	sm.RLock()
	defer sm.RUnlock()
	if result, exists := sm.data[key]; exists {
		return result.startTime
	}
	return nil
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
			etcd:        fmt.Sprintf("%s/%s", namespacedName.Namespace, namespacedName.Name),
			reconciled:  result.reconcileTriggered,
			updated:     result.updated,
			timeElapsed: timeElapsed,
			done:        done,
		}
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].etcd < tasks[j].etcd
	})
	table := setupReconcileStatusTable(tasks)
	fmt.Println("Reconciliation Status:")
	fmt.Println(table)
}
