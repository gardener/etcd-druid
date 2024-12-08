// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/controller/etcd"
	"github.com/gardener/etcd-druid/internal/utils"
	"github.com/gardener/etcd-druid/test/it/controller/assets"
	"github.com/gardener/etcd-druid/test/it/setup"
	testutils "github.com/gardener/etcd-druid/test/utils"

	appsv1 "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/component-base/featuregate"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	. "github.com/onsi/gomega"
)

const testNamespacePrefix = "etcd-reconciler-test-"

func createTestNamespaceName(t *testing.T) string {
	g := NewWithT(t)
	namespaceSuffix := testutils.GenerateRandomAlphanumericString(g, 4)
	return fmt.Sprintf("%s-%s", testNamespacePrefix, namespaceSuffix)
}

func initializeEtcdReconcilerTestEnv(t *testing.T, controllerName string, itTestEnv setup.DruidTestEnvironment, autoReconcile bool, clientBuilder *testutils.TestClientBuilder) ReconcilerTestEnv {
	g := NewWithT(t)
	var (
		reconciler *etcd.Reconciler
		err        error
	)
	g.Expect(itTestEnv.CreateManager(clientBuilder)).To(Succeed())
	itTestEnv.RegisterReconciler(func(mgr manager.Manager) {
		reconciler, err = etcd.NewReconcilerWithImageVector(mgr, controllerName,
			&etcd.Config{
				Workers:                            5,
				EnableEtcdSpecAutoReconcile:        autoReconcile,
				DisableEtcdServiceAccountAutomount: false,
				EtcdStatusSyncPeriod:               2 * time.Second,
				FeatureGates:                       map[featuregate.Feature]bool{},
				EtcdMember: etcd.MemberConfig{
					NotReadyThreshold: 5 * time.Minute,
					UnknownThreshold:  1 * time.Minute,
				},
			}, assets.CreateImageVector(g))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(reconciler.RegisterWithManager(mgr, controllerName)).To(Succeed())
	})
	g.Expect(itTestEnv.StartManager()).To(Succeed())
	t.Log("successfully registered etcd reconciler with manager and started manager")
	return ReconcilerTestEnv{
		itTestEnv:  itTestEnv,
		reconciler: reconciler,
	}
}

func createAndAssertEtcdAndAllManagedResources(ctx context.Context, t *testing.T, reconcilerTestEnv ReconcilerTestEnv, etcdInstance *druidv1alpha1.Etcd) {
	g := NewWithT(t)
	cl := reconcilerTestEnv.itTestEnv.GetClient()
	// create etcdInstance resource
	g.Expect(cl.Create(ctx, etcdInstance)).To(Succeed())
	t.Logf("trigggered creation of etcd instance: {name: %s, namespace: %s}, waiting for resources to be created...", etcdInstance.Name, etcdInstance.Namespace)
	// ascertain that all etcd resources are created
	assertAllComponentsExists(ctx, t, reconcilerTestEnv, etcdInstance, 3*time.Minute, 2*time.Second)
	t.Logf("successfully created all resources for etcd instance: {name: %s, namespace: %s}", etcdInstance.Name, etcdInstance.Namespace)

	// In envtest KCM does not run. As a consequence StatefulSet controller also does not run which results in no pods being created.
	// Status of StatefulSet also does not get updated. During reconciliation in StatefulSet component preSync method both StatefulSet
	// and Pod status are checked. In order for the IT tests to function correctly both the StatefulSet status needs to be updated
	// and pods needs to be explicitly created.
	stsUpdateRevision := updateAndGetStsRevision(ctx, t, cl, etcdInstance)
	t.Logf("successfully updated sts revision for etcd instance: {name: %s, namespace: %s}", etcdInstance.Name, etcdInstance.Namespace)
	createStsPods(ctx, t, cl, etcdInstance.ObjectMeta, stsUpdateRevision)
	t.Logf("successfully created pods for statefulset of etcd instance: {name: %s, namespace: %s}", etcdInstance.Name, etcdInstance.Namespace)
}

func updateAndGetStsRevision(ctx context.Context, t *testing.T, cl client.Client, etcdInstance *druidv1alpha1.Etcd) string {
	g := NewWithT(t)
	sts := &appsv1.StatefulSet{}
	g.Expect(cl.Get(ctx, client.ObjectKey{Name: druidv1alpha1.GetStatefulSetName(etcdInstance.ObjectMeta), Namespace: etcdInstance.Namespace}, sts)).To(Succeed())
	originalSts := sts.DeepCopy()
	sts.Status.ObservedGeneration = sts.Generation
	sts.Status.Replicas = etcdInstance.Spec.Replicas
	sts.Status.ReadyReplicas = etcdInstance.Spec.Replicas
	sts.Status.UpdatedReplicas = etcdInstance.Spec.Replicas
	revision := fmt.Sprintf("%s-%s", sts.Name, testutils.GenerateRandomAlphanumericString(g, 4))
	sts.Status.CurrentRevision = revision
	sts.Status.UpdateRevision = revision
	g.Expect(cl.Status().Patch(ctx, sts, client.MergeFrom(originalSts))).To(Succeed())
	return sts.Status.UpdateRevision
}

func createStsPods(ctx context.Context, t *testing.T, cl client.Client, etcdObjectMeta metav1.ObjectMeta, stsUpdateRevision string) {
	g := NewWithT(t)
	sts := &appsv1.StatefulSet{}
	g.Expect(cl.Get(ctx, client.ObjectKey{Name: druidv1alpha1.GetStatefulSetName(etcdObjectMeta), Namespace: etcdObjectMeta.Namespace}, sts)).To(Succeed())
	stsReplicas := *sts.Spec.Replicas
	for i := 0; i < int(stsReplicas); i++ {
		podName := fmt.Sprintf("%s-%d", sts.Name, i)
		podLabels := utils.MergeMaps(sts.Spec.Template.Labels, etcdObjectMeta.Labels, map[string]string{
			appsv1.PodIndexLabel:            strconv.Itoa(i),
			appsv1.StatefulSetPodNameLabel:  podName,
			appsv1.StatefulSetRevisionLabel: stsUpdateRevision,
		})
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podName,
				Namespace: etcdObjectMeta.Namespace,
				Labels:    podLabels,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion:         sts.APIVersion,
						Kind:               sts.Kind,
						BlockOwnerDeletion: ptr.To(true),
						Name:               sts.Name,
						UID:                sts.UID,
					},
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "etcd",
						Image: "etcd-wrapper:latest",
					},
				},
			},
		}
		g.Expect(cl.Create(ctx, pod)).To(Succeed())
		t.Logf("successfully created pod: %s", pod.Name)
		// update pod status and set ready condition to true
		pod.Status.Conditions = []corev1.PodCondition{
			{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			},
		}
		g.Expect(cl.Status().Update(ctx, pod)).To(Succeed())
		t.Logf("successfully updated status of pod: %s with ready-condition set to true", pod.Name)
	}
}

func createAndAssertEtcdReconciliation(ctx context.Context, t *testing.T, reconcilerTestEnv ReconcilerTestEnv, etcdInstance *druidv1alpha1.Etcd) {
	// create etcd and assert etcd spec reconciliation
	createAndAssertEtcdAndAllManagedResources(ctx, t, reconcilerTestEnv, etcdInstance)

	// assert etcd status reconciliation
	assertETCDObservedGeneration(t, reconcilerTestEnv.itTestEnv.GetClient(), client.ObjectKeyFromObject(etcdInstance), ptr.To[int64](1), 5*time.Second, 1*time.Second)
	expectedLastOperation := &druidv1alpha1.LastOperation{
		Type:  druidv1alpha1.LastOperationTypeReconcile,
		State: druidv1alpha1.LastOperationStateSucceeded,
	}
	assertETCDLastOperationAndLastErrorsUpdatedSuccessfully(t, reconcilerTestEnv.itTestEnv.GetClient(), client.ObjectKeyFromObject(etcdInstance), expectedLastOperation, nil, 5*time.Second, 1*time.Second)
	assertETCDOperationAnnotation(t, reconcilerTestEnv.itTestEnv.GetClient(), client.ObjectKeyFromObject(etcdInstance), false, 5*time.Second, 1*time.Second)
	t.Logf("successfully reconciled status of etcd instance: {name: %s, namespace: %s}", etcdInstance.Name, etcdInstance.Namespace)
}

type etcdMemberLeaseConfig struct {
	name        string
	memberID    string
	role        druidv1alpha1.EtcdRole
	renewTime   *metav1.MicroTime
	annotations map[string]string
}

func updateMemberLeases(ctx context.Context, t *testing.T, cl client.Client, namespace string, memberLeaseConfigs []etcdMemberLeaseConfig) {
	g := NewWithT(t)
	for _, config := range memberLeaseConfigs {
		lease := &coordinationv1.Lease{}
		g.Expect(cl.Get(ctx, client.ObjectKey{Name: config.name, Namespace: namespace}, lease)).To(Succeed())
		updatedLease := lease.DeepCopy()
		for key, value := range config.annotations {
			metav1.SetMetaDataAnnotation(&updatedLease.ObjectMeta, key, value)
		}
		if config.memberID != "" && config.role != "" {
			updatedLease.Spec.HolderIdentity = ptr.To(fmt.Sprintf("%s:%s", config.memberID, config.role))
		}
		if config.renewTime != nil {
			updatedLease.Spec.RenewTime = config.renewTime
		}
		g.Expect(cl.Update(ctx, updatedLease)).To(Succeed())
		t.Logf("successfully updated member lease %s with config %+v", config.name, config)
	}
}

func createPVCs(ctx context.Context, t *testing.T, cl client.Client, sts *appsv1.StatefulSet) []*corev1.PersistentVolumeClaim {
	g := NewWithT(t)
	pvcs := make([]*corev1.PersistentVolumeClaim, 0, int(*sts.Spec.Replicas))
	volClaimName := sts.Spec.VolumeClaimTemplates[0].Name
	for i := 0; i < int(*sts.Spec.Replicas); i++ {
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%s-%d", volClaimName, sts.Name, i),
				Namespace: sts.Namespace,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("1Gi"),
					},
				},
			},
		}
		g.Expect(cl.Create(ctx, pvc)).To(Succeed())
		pvcs = append(pvcs, pvc)
		t.Logf("successfully created pvc: %s", pvc.Name)
	}
	return pvcs
}

func createPVCWarningEvent(ctx context.Context, t *testing.T, cl client.Client, namespace, pvcName, reason, message string) {
	g := NewWithT(t)
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-event-", pvcName),
			Namespace:    namespace,
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:       "PersistentVolumeClaim",
			Name:       pvcName,
			Namespace:  namespace,
			APIVersion: "v1",
		},
		Reason:  reason,
		Message: message,
		Type:    corev1.EventTypeWarning,
	}
	g.Expect(cl.Create(ctx, event)).To(Succeed())
	t.Logf("successfully created warning event for pvc: %s", pvcName)
}
