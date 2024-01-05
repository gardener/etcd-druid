package statefulset

import (
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/features"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/component-base/featuregate"

	"github.com/gardener/etcd-druid/internal/operator/resource"
	"github.com/gardener/etcd-druid/internal/utils"
	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type _resource struct {
	client         client.Client
	imageVector    imagevector.ImageVector
	useEtcdWrapper bool
}

func New(client client.Client, imageVector imagevector.ImageVector, featureGates map[featuregate.Feature]bool) resource.Operator {
	return &_resource{
		client:         client,
		imageVector:    imageVector,
		useEtcdWrapper: featureGates[features.UseEtcdWrapper],
	}
}

func (r _resource) GetExistingResourceNames(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) ([]string, error) {
	sts, err := r.getExistingStatefulSet(ctx, etcd)
	return []string{sts.Name}, err
}

func (r _resource) Sync(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	existingSts, err := r.getExistingStatefulSet(ctx, etcd)
	if err != nil {
		return fmt.Errorf("error getting existing StatefulSet: %w", err)
	}
	if existingSts == nil {
		return r.createOrPatch(ctx, etcd, etcd.Spec.Replicas)
	}
	if err = r.handlePeerTLSEnabled(ctx, etcd, existingSts); err != nil {
		return fmt.Errorf("error handling peer TLS: %w", err)
	}
	if err = r.handleImmutableFieldUpdates(ctx, etcd, existingSts); err != nil {
		return fmt.Errorf("error handling immutable field updates: %w", err)
	}
	return r.createOrPatch(ctx, etcd, etcd.Spec.Replicas)
}

func (r _resource) TriggerDelete(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	return client.IgnoreNotFound(r.client.Delete(ctx, emptyStatefulSet(getObjectKey(etcd))))
}

func (r _resource) getExistingStatefulSet(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) (*appsv1.StatefulSet, error) {
	sts := emptyStatefulSet(getObjectKey(etcd))
	if err := r.client.Get(ctx, getObjectKey(etcd), sts); err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return sts, nil
}

func (r _resource) createOrPatch(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd, replicas int32) error {
	desiredStatefulSet := emptyStatefulSet(getObjectKey(etcd))
	mutatingFn := func() error {
		if builder, err := newStsBuilder(r.client, ctx.Logger, etcd, replicas, r.useEtcdWrapper, r.imageVector, desiredStatefulSet); err != nil {
			return err
		} else {
			return builder.Build(ctx)
		}
	}
	opResult, err := controllerutils.GetAndCreateOrStrategicMergePatch(ctx, r.client, desiredStatefulSet, mutatingFn)
	if err != nil {
		return err
	}

	ctx.Logger.Info("triggered creation of statefulSet", "statefulSet", getObjectKey(etcd), "operationResult", opResult)
	return nil
}

func (r _resource) handleImmutableFieldUpdates(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd, existingSts *appsv1.StatefulSet) error {
	if existingSts.Generation > 1 && hasImmutableFieldChanged(existingSts, etcd) {
		ctx.Logger.Info("Immutable fields have been updated, need to recreate StatefulSet", "etcd")

		if err := r.TriggerDelete(ctx, etcd); err != nil {
			return fmt.Errorf("error deleting StatefulSet with immutable field updates: %v", err)
		}
	}
	return nil
}

// hasImmutableFieldChanged checks if any immutable fields have changed in the StatefulSet
// specification compared to the Etcd object. It returns true if there are changes in the immutable fields.
// Currently, it checks for changes in the ServiceName and PodManagementPolicy.
func hasImmutableFieldChanged(sts *appsv1.StatefulSet, etcd *druidv1alpha1.Etcd) bool {
	return sts.Spec.ServiceName != etcd.GetPeerServiceName() || sts.Spec.PodManagementPolicy != appsv1.ParallelPodManagement
}

func (r _resource) handlePeerTLSEnabled(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd, existingSts *appsv1.StatefulSet) error {
	peerTLSEnabled, err := utils.IsPeerURLTLSEnabled(ctx, r.client, etcd.Namespace, etcd.Name, ctx.Logger)
	if err != nil {
		return fmt.Errorf("error checking if peer URL TLS is enabled: %v", err)
	}

	if isPeerTLSChangedToEnabled(peerTLSEnabled, etcd) {
		ctx.Logger.Info("Enabling TLS for etcd peers", "namespace", etcd.Namespace, "name", etcd.Name)

		if err := r.createOrPatch(ctx, etcd, *existingSts.Spec.Replicas); err != nil {
			return fmt.Errorf("error creating or patching StatefulSet with TLS enabled: %v", err)
		}

		tlsEnabled, err := utils.IsPeerURLTLSEnabled(ctx, r.client, etcd.Namespace, etcd.Name, ctx.Logger)
		if err != nil {
			return fmt.Errorf("error verifying if TLS is enabled post-patch: %v", err)
		}
		if !tlsEnabled {
			return fmt.Errorf("failed to enable TLS for etcd [name: %s, namespace: %s]", etcd.Name, etcd.Namespace)
		}

		if err := deleteAllStsPods(ctx, r.client, "Deleting all StatefulSet pods due to TLS enablement", existingSts); err != nil {
			return fmt.Errorf("error deleting StatefulSet pods after enabling TLS: %v", err)
		}

		ctx.Logger.Info("TLS enabled for etcd peers", "namespace", etcd.Namespace, "name", etcd.Name)
	}

	return nil
}

// isPeerTLSChangedToEnabled checks if the Peer TLS setting has changed to enabled
func isPeerTLSChangedToEnabled(peerTLSEnabledStatusFromMembers bool, etcd *druidv1alpha1.Etcd) bool {
	return !peerTLSEnabledStatusFromMembers && etcd.Spec.Etcd.PeerUrlTLS != nil
}

func deleteAllStsPods(ctx resource.OperatorContext, cl client.Client, opName string, sts *appsv1.StatefulSet) error {
	// Get all Pods belonging to the StatefulSet
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(sts.Namespace),
		client.MatchingLabels(sts.Spec.Selector.MatchLabels),
	}

	if err := cl.List(ctx, podList, listOpts...); err != nil {
		ctx.Logger.Error(err, "Failed to list pods for StatefulSet", "StatefulSet", client.ObjectKeyFromObject(sts))
		return err
	}

	for _, pod := range podList.Items {
		if err := cl.Delete(ctx, &pod); err != nil {
			ctx.Logger.Error(err, "Failed to delete pod", "Pod", pod.Name, "Namespace", pod.Namespace)
			return err
		}
	}

	return nil
}

func emptyStatefulSet(objectKey client.ObjectKey) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objectKey.Name,
			Namespace: objectKey.Namespace,
		},
	}
}

func getObjectKey(etcd *druidv1alpha1.Etcd) client.ObjectKey {
	return client.ObjectKey{
		Name:      etcd.Name,
		Namespace: etcd.Namespace,
	}
}
