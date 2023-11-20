package poddistruptionbudget

import (
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/registry/resource"
	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/go-logr/logr"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type _resource struct {
	client client.Client
	logger logr.Logger
}

func (r _resource) GetExistingResourceNames(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) ([]string, error) {
	resourceNames := make([]string, 0, 1)
	pdb := &policyv1.PodDisruptionBudget{}
	if err := r.client.Get(ctx, getObjectKey(etcd), pdb); err != nil {
		if errors.IsNotFound(err) {
			return resourceNames, nil
		}
		return resourceNames, err
	}
	resourceNames = append(resourceNames, pdb.Name)
	return resourceNames, nil
}

func (r _resource) Sync(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	pdb := emptyPodDisruptionBudget(getObjectKey(etcd))
	_, err := controllerutils.GetAndCreateOrStrategicMergePatch(ctx, r.client, pdb, func() error {
		pdb.Labels = etcd.GetDefaultLabels()
		pdb.Annotations = getAnnotations(etcd)
		pdb.OwnerReferences = []metav1.OwnerReference{etcd.GetAsOwnerReference()}
		pdb.Spec.MinAvailable = &intstr.IntOrString{
			IntVal: computePDBMinAvailable(int(etcd.Spec.Replicas)),
			Type:   intstr.Int,
		}
		pdb.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: etcd.GetDefaultLabels(),
		}
		return nil
	})
	return err
}

func (r _resource) TriggerDelete(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	return r.client.Delete(ctx, emptyPodDisruptionBudget(getObjectKey(etcd)))
}

func New(client client.Client, logger logr.Logger) resource.Operator {
	return &_resource{
		client: client,
		logger: logger,
	}
}

func getObjectKey(etcd *druidv1alpha1.Etcd) client.ObjectKey {
	return client.ObjectKey{Name: etcd.Name, Namespace: etcd.Namespace}
}

func emptyPodDisruptionBudget(objectKey client.ObjectKey) *policyv1.PodDisruptionBudget {
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objectKey.Name,
			Namespace: objectKey.Namespace,
		},
	}
}

func getAnnotations(etcd *druidv1alpha1.Etcd) map[string]string {
	return map[string]string{
		common.GardenerOwnedBy:   fmt.Sprintf("%s/%s", etcd.Namespace, etcd.Name),
		common.GardenerOwnerType: "etcd",
	}
}

func computePDBMinAvailable(etcdReplicas int) int32 {
	// do not enable PDB for single node cluster
	if etcdReplicas <= 1 {
		return 0
	}
	return int32(etcdReplicas/2 + 1)
}
