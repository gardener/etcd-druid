// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package resourceprotection

import (
	"context"
	"errors"
	"fmt"

	cmdutils "github.com/gardener/etcd-druid/druidctl/cmd/utils"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
)

func (r *resourceProtectionCmdCtx) complete() error {
	etcdClient, err := r.Clients.EtcdClient()
	if err != nil {
		return fmt.Errorf("unable to create etcd client: %w", err)
	}
	r.etcdClient = etcdClient
	r.etcdRefList = r.GlobalOptions.BuildEtcdRefList()
	return nil
}

func (r *resourceProtectionCmdCtx) validate() error {
	return r.GlobalOptions.ValidateResourceSelection()
}

// addDisableProtectionAnnotation adds the disable protection annotation to the Etcd resource. It makes the resources vulnerable
func (r *resourceProtectionCmdCtx) addDisableProtectionAnnotation(ctx context.Context) error {
	// Prompt for confirmation when operating on all namespaces
	if r.AllNamespaces {
		confirmed, err := cmdutils.ConfirmAllNamespaces(r.IOStreams.Out, r.IOStreams.In, "remove protection from")
		if err != nil {
			return fmt.Errorf("confirmation failed: %w", err)
		}
		if !confirmed {
			r.Logger.Info(r.IOStreams.Out, "Operation cancelled by user")
			return nil
		}
		if r.Verbose {
			r.Logger.Info(r.IOStreams.Out, "Removing component protection from Etcds across all namespaces")
		}
	} else if r.Verbose {
		r.Logger.Info(r.IOStreams.Out, "Removing component protection from selected Etcds")
	}
	etcdList, err := cmdutils.GetEtcdList(ctx, r.etcdClient, r.etcdRefList, r.AllNamespaces, r.GetNamespace(), r.LabelSelector)
	if err != nil {
		return err
	}

	if r.Verbose {
		r.Logger.Info(r.IOStreams.Out, fmt.Sprintf("Fetched %d etcd resources for AddDisableProtectionAnnotation", len(etcdList.Items)))
	}

	var errs []error
	for _, etcd := range etcdList.Items {
		if r.Verbose {
			r.Logger.Info(r.IOStreams.Out, "Processing set disable protection annotation for etcd", etcd.Name, etcd.Namespace)
		}
		etcdModifier := func(e *druidv1alpha1.Etcd) {
			if e.Annotations == nil {
				e.Annotations = map[string]string{}
			}
			e.Annotations[druidv1alpha1.DisableEtcdComponentProtectionAnnotation] = ""
		}
		if err := r.etcdClient.UpdateEtcd(ctx, &etcd, etcdModifier); err != nil {
			errs = append(errs, fmt.Errorf("unable to update etcd object %s/%s: %w", etcd.Namespace, etcd.Name, err))
			continue
		}
		if r.Verbose {
			r.Logger.Success(r.IOStreams.Out, "Added protection annotation to etcd", etcd.Name, etcd.Namespace)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to add protection annotation to some etcd objects: %w", errors.Join(errs...))
	}
	return nil
}

// removeDisableProtectionAnnotation removes the disable protection annotation from the Etcd resource. It protects the resources
func (r *resourceProtectionCmdCtx) removeDisableProtectionAnnotation(ctx context.Context) error {
	// Prompt for confirmation when operating on all namespaces
	if r.AllNamespaces {
		confirmed, err := cmdutils.ConfirmAllNamespaces(r.IOStreams.Out, r.IOStreams.In, "add protection to")
		if err != nil {
			return fmt.Errorf("confirmation failed: %w", err)
		}
		if !confirmed {
			r.Logger.Info(r.IOStreams.Out, "Operation cancelled by user")
			return nil
		}
		if r.Verbose {
			r.Logger.Info(r.IOStreams.Out, "Adding component protection to all namespaces")
		}
	} else if r.Verbose {
		r.Logger.Info(r.IOStreams.Out, "Adding component protection to selected Etcds")
	}
	etcdList, err := cmdutils.GetEtcdList(ctx, r.etcdClient, r.etcdRefList, r.AllNamespaces, r.GetNamespace(), r.LabelSelector)
	if err != nil {
		return err
	}

	if r.Verbose {
		r.Logger.Info(r.IOStreams.Out, fmt.Sprintf("Fetched %d etcd resources for RemoveDisableProtectionAnnotation", len(etcdList.Items)))
	}

	var errs []error
	for _, etcd := range etcdList.Items {
		if r.Verbose {
			r.Logger.Info(r.IOStreams.Out, "Processing remove disable protection annotation for etcd", etcd.Name, etcd.Namespace)
		}
		if etcd.Annotations == nil {
			if r.Verbose {
				r.Logger.Info(r.IOStreams.Out, "No annotation found to remove in ns/etcd: %s/%s", etcd.Namespace, etcd.Name)
			}
			continue
		}
		etcdModifier := func(e *druidv1alpha1.Etcd) {
			if e.Annotations != nil {
				delete(e.Annotations, druidv1alpha1.DisableEtcdComponentProtectionAnnotation)
			}
		}
		if err := r.etcdClient.UpdateEtcd(ctx, &etcd, etcdModifier); err != nil {
			errs = append(errs, fmt.Errorf("unable to update etcd object %s/%s: %w", etcd.Namespace, etcd.Name, err))
			continue
		}
		if r.Verbose {
			r.Logger.Success(r.IOStreams.Out, "Removed protection annotation from etcd", etcd.Name, etcd.Namespace)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to remove protection annotation from some etcd objects: %w", errors.Join(errs...))
	}
	return nil
}
