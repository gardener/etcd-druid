// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package reconcile

import (
	"context"
	"fmt"
	"strings"
	"sync"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	cmdutils "github.com/gardener/etcd-druid/druidctl/cmd/utils"
)

type resumeReconcileResult struct {
	Etcd  *druidv1alpha1.Etcd
	Error error
}

func (r *resumeReconcileCmdCtx) complete(options *cmdutils.GlobalOptions) error {
	etcdClient, err := options.Clients.EtcdClient()
	if err != nil {
		options.Logger.Error(r.IOStreams.ErrOut, "Unable to create etcd client: ", err)
		return err
	}
	r.etcdClient = etcdClient
	r.etcdRefList = cmdutils.GetEtcdRefList(r.ResourcesRef)
	return nil
}

func (r *resumeReconcileCmdCtx) validate() error {
	if err := cmdutils.ValidateResourceNames(r.ResourcesRef); err != nil {
		return err
	}
	return nil
}

// execute removes the suspend reconcile annotation from the Etcd resource.
func (r *resumeReconcileCmdCtx) execute(ctx context.Context) error {
	etcdList, err := cmdutils.GetEtcdList(ctx, r.etcdClient, r.etcdRefList, r.AllNamespaces)
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
				Etcd:  &etcd,
				Error: err,
			})
		}(etcd)
	}

	wg.Wait()

	failedEtcds := make([]string, 0)
	for _, result := range results {
		if result.Error == nil {
			r.Logger.Success(r.IOStreams.Out, "Resumed reconciliation for etcd", result.Etcd.Name, result.Etcd.Namespace)
		} else {
			r.Logger.Error(r.IOStreams.ErrOut, "Failed to resume reconciliation for etcd", result.Error, result.Etcd.Name, result.Etcd.Namespace)
			failedEtcds = append(failedEtcds, fmt.Sprintf("%s/%s", result.Etcd.Namespace, result.Etcd.Name))
		}
	}
	if len(failedEtcds) > 0 {
		r.Logger.Warning(r.IOStreams.Out, "Failed to resume reconciliation for etcd resources", failedEtcds...)
		return fmt.Errorf("failed to resume reconciliation for etcd resources: %s", strings.Join(failedEtcds, ", "))
	}
	r.Logger.Success(r.IOStreams.Out, "Resumed reconciliation for all etcd resources")
	return nil
}

func (r *resumeReconcileCmdCtx) resumeEtcdReconcile(ctx context.Context, etcd druidv1alpha1.Etcd) error {
	r.Logger.Start(r.IOStreams.Out, "Starting to resume reconciliation for etcd", etcd.Name, etcd.Namespace)

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
