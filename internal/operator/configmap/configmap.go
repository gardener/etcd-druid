package configmap

import (
	"encoding/json"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/operator/resource"
	"github.com/gardener/etcd-druid/internal/utils"
	"github.com/gardener/gardener/pkg/controllerutils"
	gardenerutils "github.com/gardener/gardener/pkg/utils"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type _resource struct {
	client client.Client
}

func (r _resource) GetExistingResourceNames(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) ([]string, error) {
	resourceNames := make([]string, 0, 1)
	cm := &corev1.ConfigMap{}
	if err := r.client.Get(ctx, getObjectKey(etcd), cm); err != nil {
		if errors.IsNotFound(err) {
			return resourceNames, nil
		}
		return resourceNames, err
	}
	resourceNames = append(resourceNames, cm.Name)
	return resourceNames, nil
}

func (r _resource) Sync(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	cm := emptyConfigMap(getObjectKey(etcd))
	_, err := controllerutils.GetAndCreateOrMergePatch(ctx, r.client, cm, func() error {
		return buildResource(etcd, cm)
	})
	if err != nil {
		return err
	}
	checkSum, err := computeCheckSum(cm)
	if err != nil {
		return err
	}
	ctx.Data[resource.ConfigMapCheckSumKey] = checkSum
	return nil
}

func (r _resource) TriggerDelete(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	objectKey := getObjectKey(etcd)
	ctx.Logger.Info("Triggering delete of ConfigMap", "objectKey", objectKey)
	return client.IgnoreNotFound(r.client.Delete(ctx, emptyConfigMap(objectKey)))
}

func New(client client.Client) resource.Operator {
	return &_resource{
		client: client,
	}
}

func buildResource(etcd *druidv1alpha1.Etcd, cm *corev1.ConfigMap) error {
	cfg := createEtcdConfig(etcd)
	cfgYaml, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	cm.Name = etcd.GetConfigMapName()
	cm.Namespace = etcd.Namespace
	cm.Labels = getLabels(etcd)
	cm.OwnerReferences = []metav1.OwnerReference{etcd.GetAsOwnerReference()}
	cm.Data = map[string]string{"etcd.conf.yaml": string(cfgYaml)}
	return nil
}

func getLabels(etcd *druidv1alpha1.Etcd) map[string]string {
	cmLabels := map[string]string{
		druidv1alpha1.LabelComponentKey: common.ConfigMapComponentName,
		druidv1alpha1.LabelAppNameKey:   etcd.GetConfigMapName(),
	}
	return utils.MergeMaps[string, string](etcd.GetDefaultLabels(), cmLabels)
}

func getObjectKey(etcd *druidv1alpha1.Etcd) client.ObjectKey {
	return client.ObjectKey{
		Name:      etcd.GetConfigMapName(),
		Namespace: etcd.Namespace,
	}
}

func emptyConfigMap(objectKey client.ObjectKey) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objectKey.Name,
			Namespace: objectKey.Namespace,
		},
	}
}

func computeCheckSum(cm *corev1.ConfigMap) (string, error) {
	jsonData, err := json.Marshal(cm.Data)
	if err != nil {
		return "", nil
	}
	return gardenerutils.ComputeSHA256Hex(jsonData), nil
}
