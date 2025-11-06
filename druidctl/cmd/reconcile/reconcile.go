// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package reconcile

import (
	"context"
	"fmt"
	"sync"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	cmdutils "github.com/gardener/etcd-druid/druidctl/cmd/utils"
	"k8s.io/apimachinery/pkg/types"
)

type reconcileResult struct {
	Etcd     *druidv1alpha1.Etcd
	Error    error
	Duration time.Duration
}

func (r *reconcileCmdCtx) complete(options *cmdutils.GlobalOptions) error {
	etcdClient, err := options.Clients.EtcdClient()
	if err != nil {
		options.Logger.Error(r.IOStreams.ErrOut, "Unable to create etcd client: ", err)
		return err
	}
	r.etcdClient = etcdClient
	r.etcdRefList = cmdutils.GetEtcdRefList(r.ResourcesRef)
	return nil
}

func (r *reconcileCmdCtx) validate() error {
	if err := cmdutils.ValidateResourceNames(r.ResourcesRef); err != nil {
		return err
	}
	// timeout is only valid if wait-till-ready is set
	if !r.waitTillReady && r.timeout != defaultTimeout {
		return fmt.Errorf("cannot specify --timeout/-t without --wait-till-ready/-w")
	}
	// --watch already waits indefinitely, so --wait-till-ready/-w or --timeout/-t are redundant
	if r.watch && (r.waitTillReady || r.timeout != defaultTimeout) {
		return fmt.Errorf("cannot specify --watch/-W with --wait-till-ready/-w or --timeout/-t")
	}
	return nil
}

// There are two types of reconciles, one where you add the reconcile annotation and exit.
// Another where you wait till all the changes done to the Etcd resource have successfully reconciled and post reconciliation
// all the etcd cluster members are Ready
func (r *reconcileCmdCtx) execute(ctx context.Context) error {
	var ctxWithTimeout context.Context
	var cancel context.CancelFunc
	if r.watch {
		ctxWithTimeout = ctx
		cancel = func() {}
	} else {
		ctxWithTimeout, cancel = context.WithTimeout(ctx, r.timeout)
	}
	defer cancel()

	etcdList, err := cmdutils.GetEtcdList(ctxWithTimeout, r.etcdClient, r.etcdRefList, r.AllNamespaces)
	if err != nil {
		return err
	}

	resultChan := make(chan *reconcileResult, len(etcdList.Items))
	statusMgr := newStatusManager()
	wg := sync.WaitGroup{}

	// Reconcile each Etcd resource
	for _, etcd := range etcdList.Items {
		wg.Add(1)
		go func(etcd druidv1alpha1.Etcd) {
			defer wg.Done()
			startTime := time.Now()
			key := types.NamespacedName{Namespace: etcd.Namespace, Name: etcd.Name}
			statusMgr.initStatus(key, startTime)
			err := r.processReconcile(ctxWithTimeout, &etcd, statusMgr)
			resultChan <- &reconcileResult{
				Etcd:     &etcd,
				Error:    err,
				Duration: time.Since(startTime),
			}
		}(etcd)
	}

	printTicker := time.NewTicker(10 * time.Second)

	go func() {
		wg.Wait()
		close(resultChan)
		printTicker.Stop()
	}()

	if r.waitTillReady || r.watch {
		go func() {
			for range printTicker.C {
				printReconcileStatus(statusMgr)
			}
		}()
	}

	failedResults := make([]*reconcileResult, 0)
	for result := range resultChan {
		if result.Error != nil {
			failedResults = append(failedResults, result)
		}
	}
	if r.waitTillReady || r.watch {
		printReconcileStatus(statusMgr)
	}

	if len(failedResults) > 0 {
		r.Logger.Info(r.IOStreams.Out, "Reconciliation failed for the following etcd resources:")
		for _, result := range failedResults {
			r.Logger.Error(r.IOStreams.ErrOut, "Reconciliation failed", result.Error, result.Etcd.Name, result.Etcd.Namespace)
		}
		return fmt.Errorf("one or more reconciliations failed")
	}
	return nil
}

func (r *reconcileCmdCtx) processReconcile(ctx context.Context, etcd *druidv1alpha1.Etcd, sm *statusManager) error {
	if r.Verbose {
		r.Logger.Start(r.IOStreams.Out, "Starting reconciliation for etcd", etcd.Name, etcd.Namespace)
	}

	// first reconcile the Etcd resource
	if err := r.reconcileEtcdResource(ctx, etcd); err != nil {
		return err
	}
	sm.setReconcileTriggered(types.NamespacedName{Namespace: etcd.Namespace, Name: etcd.Name})

	//  check if the reconciliation is suspended, if yes, then return error as we cannot proceed
	if _, suspended := etcd.Annotations[druidv1alpha1.SuspendEtcdSpecReconcileAnnotation]; suspended {
		return fmt.Errorf("reconciliation triggered, but is suspended for Etcd, cannot proceed")
	}

	if r.waitTillReady || r.watch {
		if err := r.waitForEtcdReady(ctx, etcd, sm); err != nil {
			return fmt.Errorf("error waiting for Etcd to be ready: %w", err)
		}
	}
	return nil
}

func (r *reconcileCmdCtx) reconcileEtcdResource(ctx context.Context, etcd *druidv1alpha1.Etcd) error {
	etcdModifier := func(e *druidv1alpha1.Etcd) {
		if e.Annotations == nil {
			e.Annotations = make(map[string]string)
		}
		e.Annotations[druidv1alpha1.DruidOperationAnnotation] = druidv1alpha1.DruidOperationReconcile
	}
	if err := r.etcdClient.UpdateEtcd(ctx, etcd, etcdModifier); err != nil {
		return fmt.Errorf("unable to update etcd object '%s/%s': %w", etcd.Namespace, etcd.Name, err)
	}
	if r.Verbose {
		r.Logger.Info(r.IOStreams.Out, "Triggered reconciliation for etcd", etcd.Name, etcd.Namespace)
	}
	return nil
}

func (r *reconcileCmdCtx) waitForEtcdReady(ctx context.Context, etcd *druidv1alpha1.Etcd, sm *statusManager) error {
	if r.Verbose {
		r.Logger.Progress(r.IOStreams.Out, "Waiting for etcd to be ready...", etcd.Name, etcd.Namespace)
	}

	// For the Etcd to be considered ready, the conditions in the conditions slice must all be set to true
	conditions := []druidv1alpha1.ConditionType{
		druidv1alpha1.ConditionTypeAllMembersUpdated,
		druidv1alpha1.ConditionTypeAllMembersReady,
	}

	// use a progressTicker to periodically check & update the progress of the status conditions
	progressTicker := time.NewTicker(10 * time.Second)
	defer progressTicker.Stop()

	for {
		select {
		case <-progressTicker.C:
			// Check if all conditions are met
			ready, err := r.checkEtcdConditions(ctx, etcd, conditions, sm)
			if err != nil {
				if r.Verbose {
					r.Logger.Warning(r.IOStreams.Out, "Warning : failed checking conditions for Etcd", err.Error(), etcd.Name, etcd.Namespace)
				}
			}
			if ready {
				key := types.NamespacedName{Namespace: etcd.Namespace, Name: etcd.Name}
				sm.setComplete(key)
				if r.Verbose {
					r.Logger.Success(r.IOStreams.Out, "Etcd is now ready", etcd.Name, etcd.Namespace)
				}
				return nil
			}
		case <-ctx.Done():
			return fmt.Errorf("context canceled while waiting for Etcd to be ready: %w", ctx.Err())
		}
	}
}

func (r *reconcileCmdCtx) checkEtcdConditions(ctx context.Context, etcd *druidv1alpha1.Etcd, conditions []druidv1alpha1.ConditionType, sm *statusManager) (bool, error) {
	latestEtcd, err := r.etcdClient.GetEtcd(ctx, etcd.Namespace, etcd.Name)
	if err != nil {
		return false, fmt.Errorf("failed to get latest Etcd: %w", err)
	}

	failingConditions := []druidv1alpha1.ConditionType{}
	for _, condition := range conditions {
		if isEtcdConditionTrue(latestEtcd, condition) {
			if condition == druidv1alpha1.ConditionTypeAllMembersUpdated {
				key := types.NamespacedName{Namespace: etcd.Namespace, Name: etcd.Name}
				sm.setUpdated(key)
			}
		} else {
			failingConditions = append(failingConditions, condition)
		}
	}
	if len(failingConditions) > 0 {
		if r.Verbose {
			r.Logger.Warning(r.IOStreams.Out, fmt.Sprintf("Warning : Etcd is not ready. Failing conditions: %v", failingConditions), latestEtcd.Name, latestEtcd.Namespace)
		}
		return false, nil
	}
	return true, nil
}

func isEtcdConditionTrue(etcd *druidv1alpha1.Etcd, condition druidv1alpha1.ConditionType) bool {
	for _, cond := range etcd.Status.Conditions {
		if cond.Type == condition && cond.Status == druidv1alpha1.ConditionTrue {
			return true
		}
	}
	return false
}
