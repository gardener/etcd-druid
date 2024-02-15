package etcd

import (
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	testutils "github.com/gardener/etcd-druid/test/utils"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/gomega"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	mockmanager "github.com/gardener/etcd-druid/internal/mock/controller-runtime/manager"
)

type preCondition struct {
	enableEtcdSpecAutoReconcile bool
	ignoreOperationAnnotation   bool
	reconcileAnnotationPresent  bool
	etcdSpecChanged             bool
	etcdStatusChanged           bool
}

func TestBuildPredicate(t *testing.T) {
	testCases := []struct {
		name         string
		preCondition preCondition
		// expected behavior for different event types
		shouldAllowCreateEvent  bool
		shouldAllowDeleteEvent  bool
		shouldAllowGenericEvent bool
		shouldAllowUpdateEvent  bool
	}{
		// TODO (madhav): remove this test case once ignoreOperationAnnotation has been removed. It has already been marked as deprecated.
		{
			name:                    "when ignoreOperationAnnotation is true and only spec has changed",
			preCondition:            preCondition{ignoreOperationAnnotation: true, reconcileAnnotationPresent: false, etcdSpecChanged: true},
			shouldAllowCreateEvent:  true,
			shouldAllowDeleteEvent:  true,
			shouldAllowGenericEvent: true,
			shouldAllowUpdateEvent:  true,
		},
		// TODO (madhav): remove this test case once ignoreOperationAnnotation has been removed. It has already been marked as deprecated.
		{
			name:                    "when ignoreOperationAnnotation is true and there is no change to spec but only to status",
			preCondition:            preCondition{ignoreOperationAnnotation: true, reconcileAnnotationPresent: false, etcdStatusChanged: true},
			shouldAllowCreateEvent:  true,
			shouldAllowDeleteEvent:  true,
			shouldAllowGenericEvent: true,
			shouldAllowUpdateEvent:  false,
		},
		{
			name:                    "when enableEtcdSpecAutoReconcile is true and only spec has changed",
			preCondition:            preCondition{enableEtcdSpecAutoReconcile: true, reconcileAnnotationPresent: false, etcdSpecChanged: true},
			shouldAllowCreateEvent:  true,
			shouldAllowDeleteEvent:  true,
			shouldAllowGenericEvent: true,
			shouldAllowUpdateEvent:  true,
		},
		{
			name:                    "when enableEtcdSpecAutoReconcile is true and there is no change to spec but only to status",
			preCondition:            preCondition{enableEtcdSpecAutoReconcile: true, reconcileAnnotationPresent: false, etcdStatusChanged: true},
			shouldAllowCreateEvent:  true,
			shouldAllowDeleteEvent:  true,
			shouldAllowGenericEvent: true,
			shouldAllowUpdateEvent:  false,
		},
		{
			name:                    "when reconcile annotation is present and no change to spec and status",
			preCondition:            preCondition{reconcileAnnotationPresent: true},
			shouldAllowCreateEvent:  true,
			shouldAllowDeleteEvent:  true,
			shouldAllowGenericEvent: true,
			shouldAllowUpdateEvent:  true,
		},
	}
	g := NewWithT(t)
	etcd := createEtcd()
	etcd.Status.Replicas = 1
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := createReconciler(t, tc.preCondition.enableEtcdSpecAutoReconcile, tc.preCondition.ignoreOperationAnnotation)
			predicate := r.buildPredicate()
			g.Expect(predicate.Create(event.CreateEvent{Object: etcd})).To(Equal(tc.shouldAllowCreateEvent))
			g.Expect(predicate.Delete(event.DeleteEvent{Object: etcd})).To(Equal(tc.shouldAllowDeleteEvent))
			g.Expect(predicate.Generic(event.GenericEvent{Object: etcd})).To(Equal(tc.shouldAllowGenericEvent))
			updatedEtcd := updateEtcd(tc.preCondition, etcd)
			g.Expect(predicate.Update(event.UpdateEvent{ObjectOld: etcd, ObjectNew: updatedEtcd})).To(Equal(tc.shouldAllowUpdateEvent))
		})
	}
}

func createEtcd() *druidv1alpha1.Etcd {
	etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).WithReplicas(3).Build()
	etcd.Status = druidv1alpha1.EtcdStatus{
		ObservedGeneration: pointer.Int64(0),
		Etcd: &druidv1alpha1.CrossVersionObjectReference{
			Kind:       "StatefulSet",
			Name:       testutils.TestEtcdName,
			APIVersion: "apps/v1",
		},
		CurrentReplicas: 3,
		Replicas:        3,
		ReadyReplicas:   3,
		Ready:           pointer.Bool(true),
	}
	return etcd
}

func updateEtcd(preCondition preCondition, originalEtcd *druidv1alpha1.Etcd) *druidv1alpha1.Etcd {
	newEtcd := originalEtcd.DeepCopy()
	annotations := make(map[string]string)
	if preCondition.reconcileAnnotationPresent {
		annotations[v1beta1constants.GardenerOperation] = v1beta1constants.GardenerOperationReconcile
		newEtcd.SetAnnotations(annotations)
	}
	if preCondition.etcdSpecChanged {
		// made a single change to the spec
		newEtcd.Spec.Backup.Image = pointer.String("eu.gcr.io/gardener-project/gardener/etcdbrctl-distroless:v1.0.0")
		newEtcd.Generation++
	}
	if preCondition.etcdStatusChanged {
		// made a single change to the status
		newEtcd.Status.ReadyReplicas = 2
		newEtcd.Status.Ready = pointer.Bool(false)
	}
	return newEtcd
}

func createReconciler(t *testing.T, enableEtcdSpecAutoReconcile, ignoreOperationAnnotation bool) *Reconciler {
	g := NewWithT(t)
	mockCtrl := gomock.NewController(t)
	mgr := mockmanager.NewMockManager(mockCtrl)
	fakeClient := fake.NewClientBuilder().Build()
	mgr.EXPECT().GetClient().AnyTimes().Return(testutils.NewTestClientBuilder().WithClient(fakeClient).Build())
	mgr.EXPECT().GetEventRecorderFor(gomock.Any()).AnyTimes().Return(nil)
	etcdConfig := Config{
		IgnoreOperationAnnotation:   ignoreOperationAnnotation,
		EnableEtcdSpecAutoReconcile: enableEtcdSpecAutoReconcile,
	}
	r, err := NewReconcilerWithImageVector(mgr, &etcdConfig, nil)
	g.Expect(err).NotTo(HaveOccurred())
	return r
}
