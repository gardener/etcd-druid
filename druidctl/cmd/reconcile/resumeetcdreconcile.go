// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package reconcile

import (
	"context"
	"errors"
	"fmt"
	"sync"

	cmdutils "github.com/gardener/etcd-druid/druidctl/cmd/utils"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
)

type resumeReconcileResult struct {
	etcd *druidv1alpha1.Etcd
	err  error
}

func (r *resumeReconcileCmdCtx) complete(options *cmdutils.GlobalOptions) error {
	etcdClient, err := options.Clients.EtcdClient()
	if err != nil {
		return fmt.Errorf("unable to create etcd client: %w", err)
	}
	r.etcdClient = etcdClient
	r.etcdRefList = options.BuildEtcdRefList()
	return nil
}

func (r *resumeReconcileCmdCtx) validate() error {
	return r.GlobalOptions.ValidateResourceSelection()
}

// execute removes the suspend reconcile annotation from the Etcd resource.
func (r *resumeReconcileCmdCtx) execute(ctx context.Context) error {
	// Prompt for confirmation when operating on all namespaces
	if r.AllNamespaces {
		confirmed, err := cmdutils.ConfirmAllNamespaces(r.IOStreams.Out, r.IOStreams.In, "resume reconciliation for")
		if err != nil {
			return fmt.Errorf("confirmation failed: %w", err)
		}
		if !confirmed {
			return cmdutils.ErrConfirmationDeclined
		}
	}

	etcdList, err := cmdutils.GetEtcdList(ctx, r.etcdClient, r.etcdRefList, r.AllNamespaces, r.GetNamespace(), r.LabelSelector)
	if err != nil {
		return err
	}

	if r.Verbose {
		r.Logger.Info(r.IOStreams.Out, "Fetched etcd resources for ResumeEtcdReconcile", fmt.Sprintf("%d", len(etcdList.Items)))
	}

	results := make([]*resumeReconcileResult, 0, len(etcdList.Items))
	var wg sync.WaitGroup

	for _, etcd := range etcdList.Items {
		if r.Verbose {
			r.Logger.Info(r.IOStreams.Out, "Processing resume reconcile for etcd", etcd.Name, etcd.Namespace)
		}

		wg.Add(1)
		go func(etcd druidv1alpha1.Etcd) {
			defer wg.Done()
			err := r.resumeEtcdReconcile(ctx, etcd)
			results = append(results, &resumeReconcileResult{
				etcd: &etcd,
				err:  err,
			})
		}(etcd)
	}

	wg.Wait()

	var errs []error
	for _, result := range results {
		if result.err == nil {
			if r.Verbose {
				r.Logger.Success(r.IOStreams.Out, "Resumed reconciliation for etcd", result.etcd.Name, result.etcd.Namespace)
			}
		} else {
			errs = append(errs, fmt.Errorf("failed to resume reconciliation for etcd %s/%s: %w", result.etcd.Namespace, result.etcd.Name, result.err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to resume reconciliation for some etcd resources: %w", errors.Join(errs...))
	}
	if r.Verbose {
		r.Logger.Success(r.IOStreams.Out, "Resumed reconciliation for all etcd resources")
	}
	return nil
}

func (r *resumeReconcileCmdCtx) resumeEtcdReconcile(ctx context.Context, etcd druidv1alpha1.Etcd) error {
	if r.Verbose {
		r.Logger.Start(r.IOStreams.Out, "Starting to resume reconciliation for etcd", etcd.Name, etcd.Namespace)
	}

	etcdModifier := func(e *druidv1alpha1.Etcd) {
		if e.Annotations != nil {
			delete(e.Annotations, druidv1alpha1.SuspendEtcdSpecReconcileAnnotation)
		}
	}
	if err := r.etcdClient.UpdateEtcd(ctx, &etcd, etcdModifier); err != nil {
		return fmt.Errorf("unable to update etcd object: %w", err)
	}
	return nil
}
