// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ondelete

import (
	"context"
	"maps"
	"testing"

	druidconfigv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/client/kubernetes"
	mockmanager "github.com/gardener/etcd-druid/internal/mock/controller-runtime/manager"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr"
	"go.uber.org/mock/gomock"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

func TestNewReconciler_DefaultsNilConcurrentSyncs(t *testing.T) {
	tests := []struct {
		name          string
		input         druidconfigv1alpha1.OnDeleteControllerConfiguration
		expectedSyncs int
	}{
		{
			name:          "nil ConcurrentSyncs is defaulted",
			input:         druidconfigv1alpha1.OnDeleteControllerConfiguration{},
			expectedSyncs: druidconfigv1alpha1.DefaultOnDeleteControllerConcurrentSyncs,
		},
		{
			name:          "explicit ConcurrentSyncs is preserved",
			input:         druidconfigv1alpha1.OnDeleteControllerConfiguration{ConcurrentSyncs: ptr.To(7)},
			expectedSyncs: 7,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			mockCtrl := gomock.NewController(t)
			mgr := mockmanager.NewMockManager(mockCtrl)
			mgr.EXPECT().GetClient().AnyTimes().Return(testutils.NewTestClientBuilder().Build())

			r := NewReconciler(mgr, tc.input)
			g.Expect(r).ToNot(BeNil())
			g.Expect(r.config.ConcurrentSyncs).ToNot(BeNil())
			g.Expect(*r.config.ConcurrentSyncs).To(Equal(tc.expectedSyncs))
		})
	}
}

func TestReconcile_BailsOut(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name         string
		buildObjects func() []client.Object
	}{
		{
			name: "StatefulSet not found -> no-op",
			buildObjects: func() []client.Object {
				return nil
			},
		},
		{
			name: "StatefulSet on RollingUpdate -> no-op",
			buildObjects: func() []client.Object {
				sts := stsFixtureWithOwner(testStsName, testStsNamespace, testRevNew, 3)
				sts.Spec.UpdateStrategy.Type = appsv1.RollingUpdateStatefulSetStrategyType
				return []client.Object{sts, etcdFixture()}
			},
		},
		{
			name: "StatefulSet with empty updateRevision -> no-op",
			buildObjects: func() []client.Object {
				sts := stsFixtureWithOwner(testStsName, testStsNamespace, "", 3)
				return []client.Object{sts, etcdFixture()}
			},
		},
		{
			name: "StatefulSet with no owning Etcd -> no-op",
			buildObjects: func() []client.Object {
				sts := stsFixtureWithOwner(testStsName, testStsNamespace, testRevNew, 3)
				sts.OwnerReferences = nil
				return []client.Object{sts}
			},
		},
		{
			name: "StatefulSet whose owning Etcd is being deleted -> no-op",
			buildObjects: func() []client.Object {
				sts := stsFixtureWithOwner(testStsName, testStsNamespace, testRevNew, 3)
				etcd := etcdFixture()
				now := metav1.Now()
				etcd.DeletionTimestamp = &now
				etcd.Finalizers = []string{"kubernetes"}
				return []client.Object{sts, etcd}
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			objs := tc.buildObjects()
			cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(objs...).Build()
			r := &Reconciler{client: cl, logger: logr.Discard()}
			result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: testStsName, Namespace: testStsNamespace}})
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(result.Requeue).To(BeFalse())
			g.Expect(result.RequeueAfter).To(BeZero())
		})
	}
}

func TestReconcile_DrivesProcedureWhenEligible(t *testing.T) {
	ctx := context.Background()
	g := NewWithT(t)

	sts := stsFixtureWithOwner(testStsName, testStsNamespace, testRevNew, 3)
	etcd := etcdFixture()

	// All three replicas present: p0 is outdated and non-participating (Step 2
	// target), p1 and p2 are current-revision and participating.
	p0 := makeStsPod(testStsName+"-0", testStsNamespace, testRevOld, false)
	p1 := makeStsPod(testStsName+"-1", testStsNamespace, testRevNew, true)
	p2 := makeStsPod(testStsName+"-2", testStsNamespace, testRevNew, true)
	for _, p := range []*corev1.Pod{p0, p1, p2} {
		maps.Copy(p.Labels, sts.Spec.Selector.MatchLabels)
	}
	cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(sts, etcd, p0, p1, p2).Build()
	r := &Reconciler{client: cl, logger: logr.Discard()}

	result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: sts.Name, Namespace: sts.Namespace}})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Requeue || result.RequeueAfter > 0).To(BeTrue())

	// p0 deleted from the fake client.
	err = cl.Get(ctx, client.ObjectKeyFromObject(p0), &corev1.Pod{})
	g.Expect(err).To(HaveOccurred())
}

func TestGetOwningEtcd(t *testing.T) {
	ctx := context.Background()
	g := NewWithT(t)

	t.Run("returns nil when no Etcd owner reference is set", func(t *testing.T) {
		sts := stsFixtureWithOwner(testStsName, testStsNamespace, testRevNew, 3)
		sts.OwnerReferences = nil
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(sts).Build()
		r := &Reconciler{client: cl, logger: logr.Discard()}
		etcd, err := r.getOwningEtcd(ctx, sts)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(etcd).To(BeNil())
	})

	t.Run("returns nil when the Etcd owner exists in OwnerReferences but the resource is missing", func(t *testing.T) {
		sts := stsFixtureWithOwner(testStsName, testStsNamespace, testRevNew, 3)
		// Only the STS is present. The Etcd referenced in ownerReferences doesn't exist.
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(sts).Build()
		r := &Reconciler{client: cl, logger: logr.Discard()}
		etcd, err := r.getOwningEtcd(ctx, sts)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(etcd).To(BeNil())
	})

	t.Run("returns the Etcd when the owner exists", func(t *testing.T) {
		sts := stsFixtureWithOwner(testStsName, testStsNamespace, testRevNew, 3)
		etcd := etcdFixture()
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(sts, etcd).Build()
		r := &Reconciler{client: cl, logger: logr.Discard()}
		got, err := r.getOwningEtcd(ctx, sts)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.Name).To(Equal(etcd.Name))
	})
}

func TestListStsPods(t *testing.T) {
	ctx := context.Background()
	g := NewWithT(t)

	t.Run("returns error when selector is nil", func(t *testing.T) {
		sts := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: testStsName, Namespace: testStsNamespace}}
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).Build()
		r := &Reconciler{client: cl, logger: logr.Discard()}
		_, err := r.listStsPods(ctx, sts)
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("returns only the pods matching the selector", func(t *testing.T) {
		sts := stsFixtureWithOwner(testStsName, testStsNamespace, testRevNew, 3)
		match1 := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "match-1", Namespace: testStsNamespace, Labels: sts.Spec.Selector.MatchLabels}}
		match2 := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "match-2", Namespace: testStsNamespace, Labels: sts.Spec.Selector.MatchLabels}}
		other := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: testStsNamespace, Labels: map[string]string{"unrelated": "yes"}}}
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(match1, match2, other).Build()
		r := &Reconciler{client: cl, logger: logr.Discard()}

		got, err := r.listStsPods(ctx, sts)
		g.Expect(err).ToNot(HaveOccurred())
		gotNames := []string{}
		for _, p := range got {
			gotNames = append(gotNames, p.Name)
		}
		g.Expect(gotNames).To(ConsistOf("match-1", "match-2"))
	})
}

// stsFixtureWithOwner builds an OnDelete StatefulSet with the standard test
// selector and an OwnerReference pointing at etcdFixture().
func stsFixtureWithOwner(name, namespace, updateRevision string, replicas int32) *appsv1.StatefulSet {
	sts := stsFixture(name, namespace, updateRevision, replicas)
	sts.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: druidv1alpha1.SchemeGroupVersion.String(),
		Kind:       "Etcd",
		Name:       name,
		UID:        "some-uid",
		Controller: ptr.To(true),
	}}
	return sts
}

// etcdFixture builds a minimal Etcd resource matching stsFixtureWithOwner.
func etcdFixture() *druidv1alpha1.Etcd {
	return &druidv1alpha1.Etcd{
		ObjectMeta: metav1.ObjectMeta{Name: testStsName, Namespace: testStsNamespace, UID: "some-uid"},
		Spec:       druidv1alpha1.EtcdSpec{Replicas: 3},
	}
}
