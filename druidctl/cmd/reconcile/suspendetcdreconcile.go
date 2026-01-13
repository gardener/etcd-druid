// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package reconcile

import (
	"context"
	"fmt"
	"sync"

	cmdutils "github.com/gardener/etcd-druid/druidctl/cmd/utils"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
)

type suspendReconcileResult struct {
	Etcd  *druidv1alpha1.Etcd
	Error error
}

func (s *suspendReconcileCmdCtx) complete(options *cmdutils.GlobalOptions) error {
	etcdClient, err := options.Clients.EtcdClient()
	if err != nil {
		options.Logger.Error(s.IOStreams.ErrOut, "Unable to create etcd client: ", err)
		return err
	}
	s.etcdClient = etcdClient
	s.etcdRefList = options.BuildEtcdRefList()
	return nil
}

func (s *suspendReconcileCmdCtx) validate() error {
	return s.GlobalOptions.ValidateResourceSelection()
}

// execute adds the suspend reconcile annotation to the Etcd resource.
func (s *suspendReconcileCmdCtx) execute(ctx context.Context) error {
	// Prompt for confirmation when operating on all namespaces
	if s.AllNamespaces {
		confirmed, err := cmdutils.ConfirmAllNamespaces(s.IOStreams.Out, s.IOStreams.In, "suspend reconciliation for")
		if err != nil {
			return fmt.Errorf("confirmation failed: %w", err)
		}
		if !confirmed {
			return cmdutils.ErrConfirmationDeclined
		}
	}

	etcdList, err := cmdutils.GetEtcdList(ctx, s.etcdClient, s.etcdRefList, s.AllNamespaces, s.GetNamespace(), s.LabelSelector)
	if err != nil {
		return err
	}

	if s.Verbose {
		s.Logger.Info(s.IOStreams.Out, "Fetched etcd resources for SuspendEtcdReconcile", fmt.Sprintf("%d", len(etcdList.Items)))
	}

	results := make([]*suspendReconcileResult, 0, len(etcdList.Items))
	var wg sync.WaitGroup

	for _, etcd := range etcdList.Items {
		if s.Verbose {
			s.Logger.Info(s.IOStreams.Out, "Processing suspend reconcile for etcd", etcd.Name, etcd.Namespace)
		}

		wg.Add(1)
		go func(etcd druidv1alpha1.Etcd) {
			defer wg.Done()
			err := s.suspendEtcdReconcile(ctx, etcd)
			results = append(results, &suspendReconcileResult{
				Etcd:  &etcd,
				Error: err,
			})
		}(etcd)
	}

	wg.Wait()

	failedEtcds := make([]string, 0)
	for _, result := range results {
		if result.Error == nil {
			s.Logger.Success(s.IOStreams.Out, "Suspended reconciliation for etcd", result.Etcd.Name, result.Etcd.Namespace)
		} else {
			s.Logger.Error(s.IOStreams.ErrOut, "Failed to suspend reconciliation for etcd", result.Error, result.Etcd.Name, result.Etcd.Namespace)
			failedEtcds = append(failedEtcds, fmt.Sprintf("%s/%s", result.Etcd.Namespace, result.Etcd.Name))
		}
	}
	if len(failedEtcds) > 0 {
		s.Logger.Warning(s.IOStreams.Out, "Failed to suspend reconciliation for etcd resources", failedEtcds...)
		return fmt.Errorf("suspending reconciliation failed for etcd resources: %v", failedEtcds)
	}
	s.Logger.Success(s.IOStreams.Out, "Suspended reconciliation for all etcd resources")
	return nil
}

func (s *suspendReconcileCmdCtx) suspendEtcdReconcile(ctx context.Context, etcd druidv1alpha1.Etcd) error {
	s.Logger.Start(s.IOStreams.Out, "Starting to suspend reconciliation for etcd", etcd.Name, etcd.Namespace)

	etcdModifier := func(e *druidv1alpha1.Etcd) {
		if e.Annotations == nil {
			e.Annotations = make(map[string]string)
		}
		e.Annotations[druidv1alpha1.SuspendEtcdSpecReconcileAnnotation] = "true"
	}
	if err := s.etcdClient.UpdateEtcd(ctx, &etcd, etcdModifier); err != nil {
		return fmt.Errorf("unable to update etcd object: %w", err)
	}
	return nil
}
