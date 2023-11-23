package memberlease

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/operator/resource"
	"github.com/gardener/etcd-druid/internal/utils"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/gardener/gardener/pkg/utils/flow"
	"github.com/go-logr/logr"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const purpose = "etcd-member-lease"

type _resource struct {
	client client.Client
	logger logr.Logger
}

func (r _resource) GetExistingResourceNames(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) ([]string, error) {
	resourceNames := make([]string, 0, 1)
	leaseList := &coordinationv1.LeaseList{}
	err := r.client.List(ctx,
		leaseList,
		client.InNamespace(etcd.Namespace),
		client.MatchingLabels(getLabels(etcd)))
	if err != nil {
		return resourceNames, err
	}
	for _, lease := range leaseList.Items {
		resourceNames = append(resourceNames, lease.Name)
	}
	return resourceNames, nil
}

func (r _resource) Sync(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	objectKeys := getObjectKeys(etcd)
	createFns := make([]flow.TaskFn, len(objectKeys))
	for _, objKey := range objectKeys {
		objKey := objKey
		createFns = append(createFns, func(ctx context.Context) error {
			return r.doCreateOrUpdate(ctx, etcd, objKey)
		})
	}
	return flow.Parallel(createFns...)(ctx)
}

func (r _resource) doCreateOrUpdate(ctx context.Context, etcd *druidv1alpha1.Etcd, objKey client.ObjectKey) error {
	lease := emptyMemberLease(objKey)
	opResult, err := controllerutils.GetAndCreateOrMergePatch(ctx, r.client, lease, func() error {
		lease.Labels = etcd.GetDefaultLabels()
		lease.OwnerReferences = []metav1.OwnerReference{etcd.GetAsOwnerReference()}
		return nil
	})
	if err != nil {
		return err
	}
	r.logger.Info("triggered creation of member lease", "lease", objKey, "operationResult", opResult)
	return nil
}

func (r _resource) TriggerDelete(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	return r.client.DeleteAllOf(ctx,
		&coordinationv1.Lease{},
		client.InNamespace(etcd.Namespace),
		client.MatchingLabels(etcd.GetDefaultLabels()))
}

func New(client client.Client, logger logr.Logger) resource.Operator {
	return &_resource{
		client: client,
		logger: logger,
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
