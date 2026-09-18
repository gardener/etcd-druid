// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	clientkubernetes "github.com/gardener/etcd-druid/internal/client/kubernetes"
	etcdmember "github.com/gardener/etcd-druid/internal/etcd"

	etcdserverpb "go.etcd.io/etcd/api/v3/etcdserverpb"
	"go.etcd.io/etcd/api/v3/v3rpc/rpctypes"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	. "github.com/onsi/gomega"
)

const (
	memberClientEtcdName  = "etcd-main"
	memberClientNamespace = "test-ns"
)

// fakeMemberAPI is a test double for the memberAPI interface. It returns
// configurable MemberList and Status responses without a real etcd connection.
type fakeMemberAPI struct {
	listResp   *clientv3.MemberListResponse
	listErr    error
	statusResp map[string]*clientv3.StatusResponse // keyed by endpoint URL
	statusErr  map[string]error
	statusHits atomic.Int64 // counts total Status calls across all goroutines
	removeErr  error        // returned by MemberRemove when set
}

func (f *fakeMemberAPI) MemberList(_ context.Context) (*clientv3.MemberListResponse, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.listResp, nil
}

func (f *fakeMemberAPI) MemberRemove(_ context.Context, _ uint64) (*clientv3.MemberRemoveResponse, error) {
	if f.removeErr != nil {
		return nil, f.removeErr
	}
	return &clientv3.MemberRemoveResponse{}, nil
}

func (f *fakeMemberAPI) Status(_ context.Context, endpoint string) (*clientv3.StatusResponse, error) {
	f.statusHits.Add(1)
	if f.statusErr != nil {
		if err, ok := f.statusErr[endpoint]; ok {
			return nil, err
		}
	}
	if f.statusResp != nil {
		if resp, ok := f.statusResp[endpoint]; ok {
			return resp, nil
		}
	}
	return nil, fmt.Errorf("no status configured for %s", endpoint)
}

// pbMember builds an etcdserverpb.Member for use in MemberListResponse.Members.
func pbMember(id uint64, name string, clientURLs []string, isLearner bool) *etcdserverpb.Member {
	return &etcdserverpb.Member{ID: id, Name: name, ClientURLs: clientURLs, IsLearner: isLearner}
}

// TestListMembersHealthProbes verifies the concurrent per-member health probe
// logic: members whose Status RPC succeeds become Healthy; members for which
// every URL errors stay Unknown (fail-closed). The leader is resolved in a
// separate pass only over healthy members.
func TestListMembersHealthProbes(t *testing.T) {
	t.Parallel()

	// Three members: 0 healthy (leader), 1 healthy (voter), 2 all-URL errors (unknown).
	api := &fakeMemberAPI{
		listResp: &clientv3.MemberListResponse{
			Members: []*etcdserverpb.Member{
				pbMember(1, "etcd-main-0", []string{"http://10.0.0.1:2379"}, false),
				pbMember(2, "etcd-main-1", []string{"http://10.0.0.2:2379"}, false),
				pbMember(3, "etcd-main-2", []string{"http://10.0.0.3:2379"}, false),
			},
		},
		statusResp: map[string]*clientv3.StatusResponse{
			"http://10.0.0.1:2379": {Leader: 1, RaftIndex: 10},
			"http://10.0.0.2:2379": {Leader: 1, RaftIndex: 10},
		},
		statusErr: map[string]error{
			"http://10.0.0.3:2379": fmt.Errorf("connection refused"),
		},
	}

	c := &memberClient{api: api}
	members, err := c.ListMembers(context.Background())

	g := NewWithT(t)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(members).To(HaveLen(3))

	byName := map[string]etcdmember.Member{}
	for _, m := range members {
		byName[m.Name] = m
	}

	g.Expect(byName["etcd-main-0"].Health).To(Equal(etcdmember.MemberHealthHealthy))
	g.Expect(byName["etcd-main-0"].Role).To(Equal(etcdmember.MemberRoleLeader))

	g.Expect(byName["etcd-main-1"].Health).To(Equal(etcdmember.MemberHealthHealthy))
	g.Expect(byName["etcd-main-1"].Role).To(Equal(etcdmember.MemberRoleMember))

	// etcd-main-2: all URLs error -> Health stays Unknown (fail-closed), Role stays Member.
	g.Expect(byName["etcd-main-2"].Health).To(Equal(etcdmember.MemberHealthUnknown))
	g.Expect(byName["etcd-main-2"].Role).To(Equal(etcdmember.MemberRoleMember))
}

// TestListMembersLearnerNeverBecomesLeader verifies that a learner member whose
// Status reports it as the Leader is not promoted: learners cannot be leaders.
func TestListMembersLearnerNeverBecomesLeader(t *testing.T) {
	t.Parallel()

	api := &fakeMemberAPI{
		listResp: &clientv3.MemberListResponse{
			Members: []*etcdserverpb.Member{
				pbMember(1, "etcd-main-0", []string{"http://10.0.0.1:2379"}, false), // real voter
				pbMember(9, "learner-0", []string{"http://10.0.0.9:2379"}, true),    // learner
			},
		},
		statusResp: map[string]*clientv3.StatusResponse{
			"http://10.0.0.1:2379": {Leader: 9}, // reports learner as leader (pathological)
			"http://10.0.0.9:2379": {Leader: 9},
		},
	}

	c := &memberClient{api: api}
	members, err := c.ListMembers(context.Background())

	g := NewWithT(t)
	g.Expect(err).NotTo(HaveOccurred())

	byName := map[string]etcdmember.Member{}
	for _, m := range members {
		byName[m.Name] = m
	}
	// The learner's ID matches the reported leader, but learners cannot be leaders:
	// the promotion loop skips members where Role == MemberRoleLearner.
	g.Expect(byName["learner-0"].Role).To(Equal(etcdmember.MemberRoleLearner))
	// The voter is not promoted either, because its ID does not match the reported leader.
	g.Expect(byName["etcd-main-0"].Role).To(Equal(etcdmember.MemberRoleMember))
}

// TestListMembersNoLeaderReported verifies that when all healthy members report
// leader=0 (e.g. during an election), all members keep their default MemberRole.
func TestListMembersNoLeaderReported(t *testing.T) {
	t.Parallel()

	api := &fakeMemberAPI{
		listResp: &clientv3.MemberListResponse{
			Members: []*etcdserverpb.Member{
				pbMember(1, "etcd-main-0", []string{"http://10.0.0.1:2379"}, false),
				pbMember(2, "etcd-main-1", []string{"http://10.0.0.2:2379"}, false),
			},
		},
		statusResp: map[string]*clientv3.StatusResponse{
			"http://10.0.0.1:2379": {Leader: 0}, // no leader yet
			"http://10.0.0.2:2379": {Leader: 0},
		},
	}

	c := &memberClient{api: api}
	members, err := c.ListMembers(context.Background())

	g := NewWithT(t)
	g.Expect(err).NotTo(HaveOccurred())
	for _, m := range members {
		g.Expect(m.Role).To(Equal(etcdmember.MemberRoleMember), "expected no leader when all report leader=0, member=%s", m.Name)
	}
}

// TestListMembersSkipsUnhealthyForLeaderProbe verifies that the leader-ID probe
// only contacts healthy members: the Status call count must equal the number of
// healthy members (one call per health probe) plus at most one call per healthy
// member for the leader probe.
func TestListMembersSkipsUnhealthyForLeaderProbe(t *testing.T) {
	t.Parallel()

	// Member 3 is unreachable; members 1 and 2 are healthy.
	api := &fakeMemberAPI{
		listResp: &clientv3.MemberListResponse{
			Members: []*etcdserverpb.Member{
				pbMember(1, "etcd-main-0", []string{"http://10.0.0.1:2379"}, false),
				pbMember(2, "etcd-main-1", []string{"http://10.0.0.2:2379"}, false),
				pbMember(3, "etcd-main-2", []string{"http://10.0.0.3:2379"}, false),
			},
		},
		statusResp: map[string]*clientv3.StatusResponse{
			"http://10.0.0.1:2379": {Leader: 1},
			"http://10.0.0.2:2379": {Leader: 1},
		},
		statusErr: map[string]error{
			"http://10.0.0.3:2379": fmt.Errorf("timeout"),
		},
	}

	c := &memberClient{api: api}
	_, err := c.ListMembers(context.Background())

	g := NewWithT(t)
	g.Expect(err).NotTo(HaveOccurred())
	// 3 health probes (one per member) + 1 leader probe (first healthy member
	// answers with a non-zero leader so the loop stops) = 4 total Status calls.
	// The unhealthy member (10.0.0.3) is never queried for leader info.
	g.Expect(api.statusHits.Load()).To(Equal(int64(4)))
}

// TestListMembersNoClientURLs verifies that members without client URLs are
// returned with Health Unknown (no probe possible) and are excluded from the
// leader-resolution pass.
func TestListMembersNoClientURLs(t *testing.T) {
	t.Parallel()

	api := &fakeMemberAPI{
		listResp: &clientv3.MemberListResponse{
			Members: []*etcdserverpb.Member{
				pbMember(1, "etcd-main-0", []string{"http://10.0.0.1:2379"}, false),
				{ID: 2, Name: "etcd-main-1"}, // no client URLs
			},
		},
		statusResp: map[string]*clientv3.StatusResponse{
			"http://10.0.0.1:2379": {Leader: 1},
		},
	}

	c := &memberClient{api: api}
	members, err := c.ListMembers(context.Background())

	g := NewWithT(t)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(members).To(HaveLen(2))

	byName := map[string]etcdmember.Member{}
	for _, m := range members {
		byName[m.Name] = m
	}
	g.Expect(byName["etcd-main-0"].Role).To(Equal(etcdmember.MemberRoleLeader))
	g.Expect(byName["etcd-main-0"].Health).To(Equal(etcdmember.MemberHealthHealthy))
	g.Expect(byName["etcd-main-1"].Health).To(Equal(etcdmember.MemberHealthUnknown))
}

// TestNewMemberClientFailsClosedWhenTLSExpectedButUnresolved verifies that
// NewMemberClient fails closed when TLS is enabled but the CA cannot be resolved
// from either the StatefulSet volumes or the spec secret (both absent).
func TestNewMemberClientFailsClosedWhenTLSExpectedButUnresolved(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	etcd := &druidv1alpha1.Etcd{
		ObjectMeta: metav1.ObjectMeta{Name: memberClientEtcdName, Namespace: memberClientNamespace},
	}
	etcd.Spec.Etcd.ClientUrlTLS = &druidv1alpha1.TLSConfig{
		TLSCASecretRef: druidv1alpha1.SecretReference{
			SecretReference: corev1.SecretReference{Name: "ca-etcd", Namespace: memberClientNamespace},
		},
	}

	// No StatefulSet object exists, so the CA cannot be resolved from a volume.
	cl := fakeclient.NewClientBuilder().WithScheme(clientkubernetes.Scheme).Build()

	factory := NewMemberClientFactory()
	_, err := factory.NewMemberClient(context.Background(), cl, etcd)
	g.Expect(err).To(HaveOccurred())
}

// TestMemberClientCloseNilClient verifies that Close is a no-op when closer is nil.
func TestMemberClientCloseNilClient(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	c := &memberClient{api: nil, closer: nil}
	g.Expect(c.Close()).To(Succeed())
}

// TestRemoveMemberIdempotency verifies that RemoveMember treats an already-absent
// member as success (both the typed rpctypes.ErrMemberNotFound and a
// codes.NotFound gRPC status), and surfaces any other error wrapped.
func TestRemoveMemberIdempotency(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		removeErr error
		wantErr   bool
	}{
		{
			name:      "success",
			removeErr: nil,
			wantErr:   false,
		},
		{
			name:      "typed member-not-found is treated as success",
			removeErr: rpctypes.ErrMemberNotFound,
			wantErr:   false,
		},
		{
			name:      "gRPC NotFound status is treated as success",
			removeErr: grpcstatus.Error(codes.NotFound, "etcdserver: member not found"),
			wantErr:   false,
		},
		{
			name:      "other error is surfaced wrapped",
			removeErr: errors.New("connection refused"),
			wantErr:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)
			c := &memberClient{api: &fakeMemberAPI{removeErr: tc.removeErr}}
			err := c.RemoveMember(context.Background(), 0x1)
			if tc.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}

// TestListMembersMultiURLFallthrough verifies that when a member advertises
// multiple client URLs and the first errors, the probe falls through to the next
// URL and the member is still marked Healthy.
func TestListMembersMultiURLFallthrough(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	api := &fakeMemberAPI{
		listResp: &clientv3.MemberListResponse{
			Members: []*etcdserverpb.Member{
				pbMember(1, "etcd-main-0", []string{"http://10.0.0.1:2379", "http://10.0.0.11:2379"}, false),
			},
		},
		statusResp: map[string]*clientv3.StatusResponse{
			"http://10.0.0.11:2379": {Leader: 1},
		},
		statusErr: map[string]error{
			"http://10.0.0.1:2379": fmt.Errorf("connection refused"),
		},
	}

	c := &memberClient{api: api}
	members, err := c.ListMembers(context.Background())

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(members).To(HaveLen(1))
	g.Expect(members[0].Health).To(Equal(etcdmember.MemberHealthHealthy))
	g.Expect(members[0].Role).To(Equal(etcdmember.MemberRoleLeader))
}
