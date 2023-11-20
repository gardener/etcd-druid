package configmap

import (
	"encoding/json"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/registry/resource"
	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/gardener/gardener/pkg/utils"
	"github.com/go-logr/logr"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type _resource struct {
	client client.Client
	logger logr.Logger
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
	cfg := createEtcdConfig(etcd)
	cfgYaml, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	cm := emptyConfigMap(getObjectKey(etcd))
	_, err = controllerutils.GetAndCreateOrMergePatch(ctx, r.client, cm, func() error {
		cm.Name = etcd.GetConfigMapName()
		cm.Namespace = etcd.Namespace
		cm.Labels = etcd.GetDefaultLabels()
		cm.OwnerReferences = []metav1.OwnerReference{etcd.GetAsOwnerReference()}
		cm.Data = map[string]string{"etcd.conf.yaml": string(cfgYaml)}
		return nil
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
	r.logger.Info("Triggering delete of ConfigMap", "objectKey", objectKey)
	return client.IgnoreNotFound(r.client.Delete(ctx, emptyConfigMap(objectKey)))
}

func New(client client.Client, logger logr.Logger) resource.Operator {
	return &_resource{
		client: client,
		logger: logger,
	}
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
	return utils.ComputeSHA256Hex(jsonData), nil
}
