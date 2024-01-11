package memberlease

import (
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/operator/resource"
	"github.com/gardener/etcd-druid/internal/utils"
	"github.com/hashicorp/go-multierror"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/controllerutils"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ErrListMemberLease   druidv1alpha1.ErrorCode = "ERR_LIST_MEMBER_LEASE"
	ErrDeleteMemberLease druidv1alpha1.ErrorCode = "ERR_DELETE_MEMBER_LEASE"
	ErrSyncMemberLease   druidv1alpha1.ErrorCode = "ERR_SYNC_MEMBER_LEASE"
)

const purpose = "etcd-member-lease"

type _resource struct {
	client client.Client
}

func (r _resource) GetExistingResourceNames(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) ([]string, error) {
	resourceNames := make([]string, 0, 1)
	leaseList := &coordinationv1.LeaseList{}
	err := r.client.List(ctx,
		leaseList,
		client.InNamespace(etcd.Namespace),
		client.MatchingLabels(getLabels(etcd)))
	if err != nil {
		return resourceNames, druiderr.WrapError(err,
			ErrListMemberLease,
			"GetExistingResourceNames",
			fmt.Sprintf("Error listing member leases for etcd: %v", etcd.GetNamespaceName()))
	}
	for _, lease := range leaseList.Items {
		resourceNames = append(resourceNames, lease.Name)
	}
	return resourceNames, nil
}

func (r _resource) Sync(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	objectKeys := getObjectKeys(etcd)
	createTasks := make([]utils.OperatorTask, len(objectKeys))
	var errs error

	for i, objKey := range objectKeys {
		objKey := objKey // capture the range variable
		createTasks[i] = utils.OperatorTask{
			Name: "CreateOrUpdate-" + objKey.String(),
			Fn: func(ctx resource.OperatorContext) error {
				return r.doCreateOrUpdate(ctx, etcd, objKey)
			},
		}
	}
	if errorList := utils.RunConcurrently(ctx, createTasks); len(errorList) > 0 {
		for _, err := range errorList {
			errs = multierror.Append(errs, err)
		}
	}
	return errs
}

func (r _resource) doCreateOrUpdate(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd, objKey client.ObjectKey) error {
	lease := emptyMemberLease(objKey)
	opResult, err := controllerutils.GetAndCreateOrMergePatch(ctx, r.client, lease, func() error {
		lease.Labels = utils.MergeMaps[string](utils.GetMemberLeaseLabels(etcd.Name), etcd.GetDefaultLabels())
		lease.OwnerReferences = []metav1.OwnerReference{etcd.GetAsOwnerReference()}
		return nil
	})
	if err != nil {
		return druiderr.WrapError(err,
			ErrSyncMemberLease,
			"Sync",
			fmt.Sprintf("Error syncing member lease: %s for etcd: %v", objKey.Name, etcd.GetNamespaceName()))
	}
	ctx.Logger.Info("triggered create or update of member lease", "lease", objKey, "operationResult", opResult)
	return nil
}

func (r _resource) TriggerDelete(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	ctx.Logger.Info("Triggering delete of member leases")
	if err := r.client.DeleteAllOf(ctx,
		&coordinationv1.Lease{},
		client.InNamespace(etcd.Namespace),
		client.MatchingLabels(getLabels(etcd))); err != nil {
		return druiderr.WrapError(err,
			ErrDeleteMemberLease,
			"TriggerDelete",
			fmt.Sprintf("Failed to delete member leases for etcd: %v", etcd.GetNamespaceName()))
	}
	ctx.Logger.Info("deleted", "resource", "member-leases")
	return nil
}

func New(client client.Client) resource.Operator {
	return &_resource{
		client: client,
	}
}

func getObjectKeys(etcd *druidv1alpha1.Etcd) []client.ObjectKey {
	leaseNames := etcd.GetMemberLeaseNames()
	objectKeys := make([]client.ObjectKey, 0, len(leaseNames))
	for _, leaseName := range leaseNames {
		objectKeys = append(objectKeys, client.ObjectKey{Name: leaseName, Namespace: etcd.Namespace})
	}
	return objectKeys
}

func getLabels(etcd *druidv1alpha1.Etcd) map[string]string {
	labels := make(map[string]string)
	labels[common.GardenerOwnedBy] = etcd.Name
	labels[v1beta1constants.GardenerPurpose] = purpose
	return utils.MergeMaps[string, string](labels, etcd.GetDefaultLabels())
}

func emptyMemberLease(objectKey client.ObjectKey) *coordinationv1.Lease {
	return &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objectKey.Name,
			Namespace: objectKey.Namespace,
		},
	}
}
