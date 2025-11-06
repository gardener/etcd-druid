package resourceprotection

import (
	"context"
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	cmdutils "github.com/gardener/etcd-druid/druidctl/cmd/utils"
	"github.com/gardener/etcd-druid/druidctl/internal/utils"
)

func (r *resourceProtectionCmdCtx) complete(options *cmdutils.GlobalOptions) error {
	etcdClient, err := options.Clients.EtcdClient()
	if err != nil {
		options.Logger.Error(r.IOStreams.ErrOut, "Unable to create etcd client: ", err)
		return err
	}
	r.etcdClient = etcdClient
	r.etcdRefList = cmdutils.GetEtcdRefList(r.ResourcesRef)
	return nil
}

func (r *resourceProtectionCmdCtx) validate() error {
	if err := cmdutils.ValidateResourceNames(r.ResourcesRef); err != nil {
		return err
	}
	return nil
}

// addDisableProtectionAnnotation adds the disable protection annotation to the Etcd resource. It makes the resources vulnerable
func (r *resourceProtectionCmdCtx) addDisableProtectionAnnotation(ctx context.Context) error {
	if r.AllNamespaces {
		r.Logger.Info(r.IOStreams.Out, "Removing component protection from Etcds across all namespaces")
	} else {
		r.Logger.Info(r.IOStreams.Out, "Removing component protection from selected Etcds", r.ResourcesRef)
	}
	etcdList, err := utils.GetEtcdList(ctx, r.etcdClient, r.etcdRefList, r.AllNamespaces)
	if err != nil {
		return err
	}

	if r.Verbose {
		r.Logger.Info(r.IOStreams.Out, fmt.Sprintf("Fetched %d etcd resources for AddDisableProtectionAnnotation", len(etcdList.Items)))
	}

	var errList []error
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
			r.Logger.Error(r.IOStreams.ErrOut, "Failed to update etcd object", err, etcd.Name, etcd.Namespace)
			errList = append(errList, fmt.Errorf("unable to update etcd object %s/%s: %w", etcd.Namespace, etcd.Name, err))
			continue
		}
		r.Logger.Success(r.IOStreams.Out, "Added protection annotation to etcd", etcd.Name, etcd.Namespace)
	}
	if len(errList) > 0 {
		return fmt.Errorf("failed to add protection annotation to some etcd objects: %v", errList)
	}
	return nil
}

// removeDisableProtectionAnnotation removes the disable protection annotation from the Etcd resource. It protects the resources
func (r *resourceProtectionCmdCtx) removeDisableProtectionAnnotation(ctx context.Context) error {
	if r.AllNamespaces {
		r.Logger.Info(r.IOStreams.Out, "Adding component protection to all namespaces")
	} else {
		r.Logger.Info(r.IOStreams.Out, "Adding component protection from selected Etcds", r.ResourcesRef)
	}
	etcdList, err := utils.GetEtcdList(ctx, r.etcdClient, r.etcdRefList, r.AllNamespaces)
	if err != nil {
		return err
	}

	if r.Verbose {
		r.Logger.Info(r.IOStreams.Out, fmt.Sprintf("Fetched %d etcd resources for RemoveDisableProtectionAnnotation", len(etcdList.Items)))
	}

	var errList []error
	for _, etcd := range etcdList.Items {
		if r.Verbose {
			r.Logger.Info(r.IOStreams.Out, "Processing remove disable protection annotation for etcd", etcd.Name, etcd.Namespace)
		}
		if etcd.Annotations == nil {
			r.Logger.Info(r.IOStreams.Out, "No annotation found to remove in ns/etcd: %s/%s", etcd.Namespace, etcd.Name)
			continue
		}
		etcdModifier := func(e *druidv1alpha1.Etcd) {
			if e.Annotations != nil {
				delete(e.Annotations, druidv1alpha1.DisableEtcdComponentProtectionAnnotation)
			}
		}
		if err := r.etcdClient.UpdateEtcd(ctx, &etcd, etcdModifier); err != nil {
			r.Logger.Error(r.IOStreams.ErrOut, "Failed to update etcd object", err, etcd.Name, etcd.Namespace)
			errList = append(errList, fmt.Errorf("unable to update etcd object %s/%s: %w", etcd.Namespace, etcd.Name, err))
			continue
		}
		r.Logger.Success(r.IOStreams.Out, "Removed protection annotation from etcd", etcd.Name, etcd.Namespace)
	}
	if len(errList) > 0 {
		return fmt.Errorf("failed to remove protection annotation from some etcd objects: %v", errList)
	}
	return nil
}
