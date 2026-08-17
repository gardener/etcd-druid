// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package statefulset

import (
	"context"
	"fmt"
	"testing"

	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	etcdclient "github.com/gardener/etcd-druid/internal/client/etcd"
	clientkubernetes "github.com/gardener/etcd-druid/internal/client/kubernetes"
	"github.com/gardener/etcd-druid/internal/component"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	etcdutils "github.com/gardener/etcd-druid/internal/utils/etcd"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	. "github.com/onsi/gomega"
)

const (
	removalEtcdName   = "etcd-main"
	removalNamespace  = "test-ns"
	removalEtcdUID    = types.UID("etcd-main-uid")
	removalStsReplica = int32(5)
)

// fakeMemberClient is a test double for etcdclient.MemberClient that
// records removals and returns a canned member list.
type fakeMemberClient struct {
	members    []etcdutils.Member
	removed    []uint64
	listErr    error
	removeErr  error
	closeCalls int
}

func (f *fakeMemberClient) ListMembers(_ context.Context) ([]etcdutils.Member, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.members, nil
}

func (f *fakeMemberClient) RemoveMember(_ context.Context, id uint64) error {
	if f.removeErr != nil {
		return f.removeErr
	}
	f.removed = append(f.removed, id)
	return nil
}

func (f *fakeMemberClient) Close() error {
	f.closeCalls++
	return nil
}

// fakeMemberClientFactory hands out a preconfigured fakeMemberClient.
type fakeMemberClientFactory struct {
	client    *fakeMemberClient
	createErr error
}

func (f *fakeMemberClientFactory) NewMemberClient(_ context.Context, _ client.Client, _ *druidv1alpha1.Etcd) (etcdclient.MemberClient, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f.client, nil
}

// TestEnsureMemberRemoval exercises the pre-sync member removal: it must
// remove exactly one surplus member per reconcile (learners first, leader last),
// requeue after a successful removal, hold back when the removal would break
// quorum, and be a no-op when there is no scale-in in progress.
func TestEnsureMemberRemoval(t *testing.T) {
	// member returns an EtcdMemberStatus with the given hex ID, role and status.
	member := func(name, hexID string, ready bool, leader bool) druidv1alpha1.EtcdMemberStatus {
		status := druidv1alpha1.EtcdMemberStatusNotReady
		if ready {
			status = druidv1alpha1.EtcdMemberStatusReady
		}
		m := druidv1alpha1.EtcdMemberStatus{
			Name:   name,
			ID:     ptr.To(hexID),
			Status: status,
		}
		if leader {
			m.Role = ptr.To(druidv1alpha1.EtcdRoleLeader)
		} else {
			m.Role = ptr.To(druidv1alpha1.EtcdRoleMember)
		}
		return m
	}

	fiveMembers := []etcdutils.Member{
		{ID: 0x1, Name: "etcd-main-0"},
		{ID: 0x2, Name: "etcd-main-1"},
		{ID: 0x3, Name: "etcd-main-2"},
		{ID: 0x4, Name: "etcd-main-3"},
		{ID: 0x5, Name: "etcd-main-4"},
	}
	fiveStatusHealthy := []druidv1alpha1.EtcdMemberStatus{
		member("etcd-main-0", "1", true, true),
		member("etcd-main-1", "2", true, false),
		member("etcd-main-2", "3", true, false),
		member("etcd-main-3", "4", true, false),
		member("etcd-main-4", "5", true, false),
	}

	tests := []struct {
		name string
		// specReplicas is the desired replica count (StatefulSet observed is 5).
		specReplicas int32
		// noStatefulSet omits the StatefulSet entirely (fresh cluster).
		noStatefulSet bool
		// nilFactory wires a nil MemberClientFactory (removal not enabled).
		nilFactory     bool
		liveMembers    []etcdutils.Member
		statusMembers  []druidv1alpha1.EtcdMemberStatus
		listErr        error
		removeErr      error
		wantRemovedIDs []uint64
		wantRequeue    bool
		wantErrCode    druidapicommon.ErrorCode
		// wantCloseCalls is the expected number of times MemberClient.Close
		// should be called; 0 for early-exit paths that never reach NewMemberClient.
		wantCloseCalls int
	}{
		{
			name:           "no scale-in in progress -> no-op",
			specReplicas:   5,
			liveMembers:    fiveMembers,
			statusMembers:  fiveStatusHealthy,
			wantCloseCalls: 0,
		},
		{
			name:           "nil factory -> no-op even with a scale-in",
			specReplicas:   3,
			nilFactory:     true,
			liveMembers:    fiveMembers,
			statusMembers:  fiveStatusHealthy,
			wantCloseCalls: 0,
		},
		{
			name:           "no StatefulSet yet -> no-op",
			specReplicas:   3,
			noStatefulSet:  true,
			liveMembers:    fiveMembers,
			statusMembers:  fiveStatusHealthy,
			wantCloseCalls: 0,
		},
		{
			name:           "scale-in 5->3 removes highest surplus ordinal first and requeues",
			specReplicas:   3,
			liveMembers:    fiveMembers,
			statusMembers:  fiveStatusHealthy,
			wantRemovedIDs: []uint64{0x5}, // etcd-main-4 (highest surplus ordinal)
			wantRequeue:    true,
			wantCloseCalls: 1,
		},
		{
			name:         "scale-in but surplus member already gone from etcd -> no-op",
			specReplicas: 4,
			liveMembers: []etcdutils.Member{
				{ID: 0x1, Name: "etcd-main-0"},
				{ID: 0x2, Name: "etcd-main-1"},
				{ID: 0x3, Name: "etcd-main-2"},
				{ID: 0x4, Name: "etcd-main-3"},
			},
			statusMembers:  fiveStatusHealthy,
			wantCloseCalls: 1,
		},
		{
			name:         "quorum-unsafe removal is held back",
			specReplicas: 1,
			// etcd-main-0 (the keeper) is down; only the surplus etcd-main-1
			// responds to ListMembers. Removing it would leave votersAfter=0
			// -> unsafe (last voter).
			liveMembers: []etcdutils.Member{
				{ID: 0x2, Name: "etcd-main-1"},
			},
			statusMembers: []druidv1alpha1.EtcdMemberStatus{
				member("etcd-main-0", "1", false, false),
				member("etcd-main-1", "2", false, false),
			},
			wantRequeue:    true,
			wantCloseCalls: 1,
		},
		{
			name:           "ListMembers error surfaces as ErrRemoveEtcdMember",
			specReplicas:   3,
			liveMembers:    fiveMembers,
			statusMembers:  fiveStatusHealthy,
			listErr:        fmt.Errorf("boom"),
			wantErrCode:    ErrRemoveEtcdMember,
			wantCloseCalls: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			etcd := testutils.EtcdBuilderWithoutDefaults(removalEtcdName, removalNamespace).
				WithReplicas(tc.specReplicas).
				Build()
			etcd.UID = removalEtcdUID
			etcd.Status.Members = tc.statusMembers

			builder := fakeclient.NewClientBuilder().WithScheme(clientkubernetes.Scheme)
			if !tc.noStatefulSet {
				sts := testutils.CreateStatefulSet(druidv1alpha1.GetStatefulSetName(etcd.ObjectMeta), removalNamespace, removalEtcdUID, removalStsReplica)
				sts.Spec.Replicas = ptr.To(removalStsReplica)
				builder = builder.WithObjects(sts)
			}
			cl := builder.Build()

			fakeClient := &fakeMemberClient{
				members:   tc.liveMembers,
				listErr:   tc.listErr,
				removeErr: tc.removeErr,
			}
			var factory etcdclient.MemberClientFactory
			if !tc.nilFactory {
				factory = &fakeMemberClientFactory{client: fakeClient}
			}

			r := _resource{client: cl, memberClientFactory: factory}
			opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), "test-run")

			err := r.ensureMemberRemoval(opCtx, etcd)

			if tc.wantErrCode != "" {
				g.Expect(err).To(HaveOccurred())
				derr := druiderr.AsDruidError(err)
				g.Expect(derr).NotTo(BeNil())
				g.Expect(derr.Code).To(Equal(tc.wantErrCode))
			} else if tc.wantRequeue {
				g.Expect(err).To(HaveOccurred())
				derr := druiderr.AsDruidError(err)
				g.Expect(derr).NotTo(BeNil())
				g.Expect(derr.Code).To(Equal(druidapicommon.ErrorCode(druiderr.ErrRequeueAfter)))
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
			g.Expect(fakeClient.removed).To(Equal(tc.wantRemovedIDs))
			g.Expect(fakeClient.closeCalls).To(Equal(tc.wantCloseCalls), "MemberClient.Close must be called exactly once per factory creation")
		})
	}
}

// TestParseMemberID verifies that etcd member IDs are parsed as hex only, since
// they originate from the member lease holder identity that etcd-backup-restore
// writes as the hex form of the ID. A non-hex string is treated as unknown
// rather than silently reinterpreted in another base.
func TestParseMemberID(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		wantID uint64
		wantOK bool
	}{
		{name: "lowercase hex", in: "ff", wantID: 0xff, wantOK: true},
		{name: "uppercase hex", in: "FF", wantID: 0xff, wantOK: true},
		{name: "multi-digit hex", in: "a1b2", wantID: 0xa1b2, wantOK: true},
		{name: "single digit", in: "3", wantID: 0x3, wantOK: true},
		{name: "empty string is unknown", in: "", wantOK: false},
		{name: "non-hex characters are unknown", in: "zzz", wantOK: false},
		// "10" is a valid hex value (16); it must NOT be reinterpreted as decimal 10.
		{name: "decimal-looking value parses as hex", in: "10", wantID: 0x10, wantOK: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			id, ok := parseMemberID(tc.in)
			g.Expect(ok).To(Equal(tc.wantOK))
			if tc.wantOK {
				g.Expect(id).To(Equal(tc.wantID))
			}
		})
	}
}

// TestSurplusMemberNamesBootstrapRemoval verifies that joined bootstrap members
// no longer present in spec are selected for removal even without a replica
// change.
func TestSurplusMemberNamesBootstrapRemoval(t *testing.T) {
	g := NewWithT(t)

	etcd := testutils.EtcdBuilderWithoutDefaults(removalEtcdName, removalNamespace).
		WithReplicas(3).
		Build()
	etcd.UID = removalEtcdUID
	etcd.Spec.Etcd.BootstrapWithExistingCluster = &druidv1alpha1.BootstrapWithExistingCluster{
		Members: []druidv1alpha1.BootstrapExistingMember{{Name: "etcd-source-0"}},
	}
	etcd.Status.BootstrapWithExistingCluster = &druidv1alpha1.BootstrapWithExistingClusterStatus{
		Members: []druidv1alpha1.BootstrapJoinedMember{
			{Name: "etcd-source-0"},
			{Name: "etcd-source-1"},
		},
	}

	// StatefulSet at the desired size, so no replica-driven surplus.
	sts := testutils.CreateStatefulSet(druidv1alpha1.GetStatefulSetName(etcd.ObjectMeta), removalNamespace, removalEtcdUID, 3)
	sts.Spec.Replicas = ptr.To(int32(3))
	cl := fakeclient.NewClientBuilder().WithScheme(clientkubernetes.Scheme).WithObjects(sts).Build()

	r := _resource{client: cl}
	opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), "test-run")

	surplus, err := r.surplusMemberNames(opCtx, etcd)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(surplus).To(HaveKey("etcd-source-1"))
	g.Expect(surplus).NotTo(HaveKey("etcd-source-0"))
}

// TestDeleteSurplusPVCs verifies that scale-in deletes the PVCs of removed pod
// ordinals (>= spec.replicas) and leaves the retained ones intact, while
// leaving all PVCs intact on a transition to zero replicas (hibernation).
func TestDeleteSurplusPVCs(t *testing.T) {
	tests := []struct {
		name         string
		specReplicas int32
		stsReplicas  int32
		// extraPVCs are ordinals whose PVCs exist beyond the StatefulSet's current
		// size, modelling leaked PVCs after Sync has already shrunk the STS or PVCs
		// retained across hibernation.
		extraPVCs []int
		// scaleInInProgress seeds ScaleOperationComplete=False/ScalingIn.
		scaleInInProgress bool
		// deleteIntercept, when non-nil, replaces the fake client's Delete verb so
		// individual cases can inject transport-level errors or NotFound responses.
		deleteIntercept func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.DeleteOption) error
		// wantErrCode, when set, asserts the function returns a DruidError with this
		// code instead of succeeding.
		wantErrCode druidapicommon.ErrorCode
		// wantDeleted / wantKept are pod ordinals whose PVCs must be gone / present.
		wantDeleted []int
		wantKept    []int
	}{
		{
			name:              "scale-in 5->3 deletes ordinals 3 and 4, keeps 0-2",
			specReplicas:      3,
			stsReplicas:       5,
			scaleInInProgress: true,
			wantDeleted:       []int{3, 4},
			wantKept:          []int{0, 1, 2},
		},
		{
			// Regression: Sync patches the StatefulSet down to the
			// desired size before deleteSurplusPVCs runs, so the observed STS is
			// already at 3 while ordinals 3 and 4 still have leaked PVCs. Deletion
			// must be driven by the PVCs that exist, not by the (already shrunk)
			// StatefulSet replica count.
			name:              "STS already shrunk to 3 still deletes leaked ordinals 3 and 4",
			specReplicas:      3,
			stsReplicas:       3,
			extraPVCs:         []int{3, 4},
			scaleInInProgress: true,
			wantDeleted:       []int{3, 4},
			wantKept:          []int{0, 1, 2},
		},
		{
			name:         "no scale-in keeps all PVCs",
			specReplicas: 5,
			stsReplicas:  5,
			wantKept:     []int{0, 1, 2, 3, 4},
		},
		{
			name:         "hibernation (spec 0) keeps all PVCs",
			specReplicas: 0,
			stsReplicas:  5,
			wantKept:     []int{0, 1, 2, 3, 4},
		},
		{
			name:         "wake-up from hibernation keeps PVCs above desired replicas when no scale-in is recorded",
			specReplicas: 3,
			stsReplicas:  0,
			extraPVCs:    []int{0, 1, 2, 3, 4},
			wantKept:     []int{0, 1, 2, 3, 4},
		},
		{
			name:              "Delete returns server error -> surfaces as ErrDeletePVC",
			specReplicas:      3,
			stsReplicas:       5,
			scaleInInProgress: true,
			deleteIntercept: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.DeleteOption) error {
				if _, ok := obj.(*corev1.PersistentVolumeClaim); ok {
					return fmt.Errorf("server error")
				}
				return nil
			},
			wantErrCode: ErrDeletePVC,
		},
		{
			// NotFound on Delete simulates the PVC being removed between the List and
			// the Delete call; the function must treat that as already-done and succeed.
			name:              "surplus PVC absent from store (deleted between List and Delete) -> succeeds",
			specReplicas:      3,
			stsReplicas:       5,
			scaleInInProgress: true,
			deleteIntercept: func(_ context.Context, cl client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				if _, ok := obj.(*corev1.PersistentVolumeClaim); ok {
					return apierrors.NewNotFound(corev1.Resource("persistentvolumeclaims"), obj.GetName())
				}
				return cl.Delete(context.Background(), obj, opts...)
			},
			wantKept: []int{0, 1, 2},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			etcd := testutils.EtcdBuilderWithoutDefaults(removalEtcdName, removalNamespace).
				WithReplicas(tc.specReplicas).
				Build()
			etcd.UID = removalEtcdUID
			if tc.scaleInInProgress {
				etcd.Status.Conditions = []druidv1alpha1.Condition{{
					Type:   druidv1alpha1.ConditionTypeScaleOperationComplete,
					Status: druidv1alpha1.ConditionFalse,
					Reason: druidv1alpha1.ScaleOperationReasonScalingIn,
				}}
			}

			sts := testutils.CreateStatefulSet(druidv1alpha1.GetStatefulSetName(etcd.ObjectMeta), removalNamespace, removalEtcdUID, tc.stsReplicas)
			sts.Spec.Replicas = ptr.To(tc.stsReplicas)

			// PVC name is "<vctName>-<stsName>-<ordinal>"; vctName defaults to the
			// etcd name. CreatePVC uses the VCT name + pod name = "<vctName>-<stsName>-<ordinal>".
			objs := []client.Object{sts}
			for i := int32(0); i < tc.stsReplicas; i++ {
				podName := druidv1alpha1.GetOrdinalPodName(etcd.ObjectMeta, int(i))
				objs = append(objs, testutils.CreatePVC(sts, podName, corev1.ClaimBound))
			}
			// extraPVCs model leaked PVCs whose ordinals are beyond the STS's
			// current (already shrunk) replica count.
			for _, ordinal := range tc.extraPVCs {
				podName := druidv1alpha1.GetOrdinalPodName(etcd.ObjectMeta, ordinal)
				objs = append(objs, testutils.CreatePVC(sts, podName, corev1.ClaimBound))
			}
			clientBuilder := fakeclient.NewClientBuilder().WithScheme(clientkubernetes.Scheme).WithObjects(objs...)
			if tc.deleteIntercept != nil {
				clientBuilder = clientBuilder.WithInterceptorFuncs(interceptor.Funcs{Delete: tc.deleteIntercept})
			}
			cl := clientBuilder.Build()

			r := _resource{client: cl}
			opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), "test-run")

			err := r.deleteSurplusPVCs(opCtx, etcd)
			if tc.wantErrCode != "" {
				g.Expect(err).To(HaveOccurred())
				derr := druiderr.AsDruidError(err)
				g.Expect(derr).NotTo(BeNil())
				g.Expect(derr.Code).To(Equal(tc.wantErrCode))
				return
			}
			g.Expect(err).NotTo(HaveOccurred())

			vctName := etcd.Name
			stsName := druidv1alpha1.GetStatefulSetName(etcd.ObjectMeta)
			pvcExists := func(ordinal int) bool {
				pvcName := fmt.Sprintf("%s-%s-%d", vctName, stsName, ordinal)
				getErr := cl.Get(context.Background(), types.NamespacedName{Namespace: removalNamespace, Name: pvcName}, &corev1.PersistentVolumeClaim{})
				return getErr == nil
			}
			for _, ordinal := range tc.wantDeleted {
				g.Expect(pvcExists(ordinal)).To(BeFalse(), "PVC for ordinal %d should be deleted", ordinal)
			}
			for _, ordinal := range tc.wantKept {
				g.Expect(pvcExists(ordinal)).To(BeTrue(), "PVC for ordinal %d should be retained", ordinal)
			}
		})
	}
}
