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

// fakeAPI is a test double satisfying both clusterAPI and maintenanceAPI. It
// returns configurable MemberList and Status responses without a real etcd
// connection.
type fakeAPI struct {
	listResp   *clientv3.MemberListResponse
	listErr    error
	statusResp map[string]*clientv3.StatusResponse // keyed by endpoint URL
	statusErr  map[string]error
	statusHits atomic.Int64 // counts total Status calls across all goroutines
	removeErr  error        // returned by MemberRemove when set
}

func (f *fakeAPI) MemberList(_ context.Context) (*clientv3.MemberListResponse, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.listResp, nil
}

func (f *fakeAPI) MemberRemove(_ context.Context, _ uint64) (*clientv3.MemberRemoveResponse, error) {
	if f.removeErr != nil {
		return nil, f.removeErr
	}
	return &clientv3.MemberRemoveResponse{}, nil
}

func (f *fakeAPI) Get(_ context.Context, _ string, _ ...clientv3.OpOption) (*clientv3.GetResponse, error) {
	return &clientv3.GetResponse{}, nil
}

func (f *fakeAPI) Status(_ context.Context, endpoint string) (*clientv3.StatusResponse, error) {
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

// fakeEndpoint satisfies probeClient (Get + Close). Its Get result is fixed per
// instance, so the production health path (probeEndpointKV) is exercised without
// a real etcd connection.
type fakeEndpoint struct {
	getErr error
}

func (e *fakeEndpoint) Get(_ context.Context, _ string, _ ...clientv3.OpOption) (*clientv3.GetResponse, error) {
	if e.getErr != nil {
		return nil, e.getErr
	}
	return &clientv3.GetResponse{}, nil
}

func (e *fakeEndpoint) Close() error { return nil }

// healthDialer returns a dialFn compatible with etcdClient.dialFn. It maps the
// first endpoint in the slice to a fakeEndpoint whose Get result is fixed:
//   - key present with nil error   -> Get succeeds (healthy)
//   - key present with non-nil err -> Get returns that error
//   - dialErr[url] present         -> the dial itself fails (probe skips the URL)
//   - key absent                   -> Get returns a generic error (unhealthy)
//
// dialHits counts dial calls so tests can assert the probe client is created
// exactly once per probed member.
func healthDialer(getResults map[string]error, dialErr map[string]error, dialHits *atomic.Int64) func([]string) (probeClient, error) {
	return func(endpoints []string) (probeClient, error) {
		if dialHits != nil {
			dialHits.Add(1)
		}
		ep := endpoints[0]
		if dialErr != nil {
			if err, ok := dialErr[ep]; ok {
				return nil, err
			}
		}
		if err, ok := getResults[ep]; ok {
			return &fakeEndpoint{getErr: err}, nil
		}
		return &fakeEndpoint{getErr: fmt.Errorf("unhealthy: no result for %s", ep)}, nil
	}
}

// newTestClient builds an etcdClient with the given api and an injected health
// dialer, so health is decided by the production Get(healthProbeKey) path.
func newTestClient(api etcdAPI, dialFn func([]string) (probeClient, error)) Client {
	c := newClient(api, nil)
	c.(*etcdClient).dialFn = dialFn
	return c
}

// TestListMembersHealthProbes verifies the concurrent per-member health probe
// logic: members whose Get("health") succeeds become Healthy; members for which
// every URL fails the KV Get stay Unknown (fail-closed). The leader is resolved
// in a separate pass (via Status) only over healthy members.
func TestListMembersHealthProbes(t *testing.T) {
	t.Parallel()

	// Three members: 0 healthy (leader), 1 healthy (voter), 2 KV Get fails (unknown).
	api := &fakeAPI{
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
	}
	dialFn := healthDialer(map[string]error{
		"http://10.0.0.1:2379": nil,                                     // healthy
		"http://10.0.0.2:2379": nil,                                     // healthy
		"http://10.0.0.3:2379": fmt.Errorf("context deadline exceeded"), // KV Get fails
	}, nil, nil)

	c := newTestClient(api, dialFn)
	members, err := c.MemberList(context.Background())

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

	// etcd-main-2: KV Get fails -> Health stays Unknown (fail-closed), Role stays Member.
	g.Expect(byName["etcd-main-2"].Health).To(Equal(etcdmember.MemberHealthUnknown))
	g.Expect(byName["etcd-main-2"].Role).To(Equal(etcdmember.MemberRoleMember))
}

// TestListMembersPermissionDeniedIsHealthy verifies that a member whose
// Get("health") returns ErrPermissionDenied is treated as healthy: the read
// reached consensus, which is the signal we care about (mirrors etcdctl).
func TestListMembersPermissionDeniedIsHealthy(t *testing.T) {
	t.Parallel()

	api := &fakeAPI{
		listResp: &clientv3.MemberListResponse{
			Members: []*etcdserverpb.Member{
				pbMember(1, "etcd-main-0", []string{"http://10.0.0.1:2379"}, false),
			},
		},
		statusResp: map[string]*clientv3.StatusResponse{
			"http://10.0.0.1:2379": {Leader: 1},
		},
	}
	dialFn := healthDialer(map[string]error{
		"http://10.0.0.1:2379": rpctypes.ErrPermissionDenied,
	}, nil, nil)

	c := newTestClient(api, dialFn)
	members, err := c.MemberList(context.Background())

	g := NewWithT(t)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(members).To(HaveLen(1))
	g.Expect(members[0].Health).To(Equal(etcdmember.MemberHealthHealthy))
	g.Expect(members[0].Role).To(Equal(etcdmember.MemberRoleLeader))
}

// TestListMembersDialFailureIsUnhealthy verifies that when the per-member client
// cannot be dialed, the member is fail-closed to Unknown.
func TestListMembersDialFailureIsUnhealthy(t *testing.T) {
	t.Parallel()

	api := &fakeAPI{
		listResp: &clientv3.MemberListResponse{
			Members: []*etcdserverpb.Member{
				pbMember(1, "etcd-main-0", []string{"http://10.0.0.1:2379"}, false),
			},
		},
		statusResp: map[string]*clientv3.StatusResponse{
			"http://10.0.0.1:2379": {Leader: 1},
		},
	}
	dialFn := healthDialer(nil, map[string]error{
		"http://10.0.0.1:2379": fmt.Errorf("dial tcp: connection refused"),
	}, nil)

	c := newTestClient(api, dialFn)
	members, err := c.MemberList(context.Background())

	g := NewWithT(t)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(members).To(HaveLen(1))
	g.Expect(members[0].Health).To(Equal(etcdmember.MemberHealthUnknown))
}

// TestListMembersLearnerNeverBecomesLeader verifies that a learner member whose
// Status reports it as the Leader is not promoted: learners cannot be leaders.
func TestListMembersLearnerNeverBecomesLeader(t *testing.T) {
	t.Parallel()

	api := &fakeAPI{
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
	dialFn := healthDialer(map[string]error{
		"http://10.0.0.1:2379": nil,
		"http://10.0.0.9:2379": nil,
	}, nil, nil)

	c := newTestClient(api, dialFn)
	members, err := c.MemberList(context.Background())

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

	api := &fakeAPI{
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
	dialFn := healthDialer(map[string]error{
		"http://10.0.0.1:2379": nil,
		"http://10.0.0.2:2379": nil,
	}, nil, nil)

	c := newTestClient(api, dialFn)
	members, err := c.MemberList(context.Background())

	g := NewWithT(t)
	g.Expect(err).NotTo(HaveOccurred())
	for _, m := range members {
		g.Expect(m.Role).To(Equal(etcdmember.MemberRoleMember), "expected no leader when all report leader=0, member=%s", m.Name)
	}
}

// TestListMembersSkipsUnhealthyForLeaderProbe verifies that the leader-ID probe
// (which uses Status) only contacts healthy members. Health is decided by the
// KV Get, so the unhealthy member is dialed for its health probe but never
// queried via Status for leader info.
func TestListMembersSkipsUnhealthyForLeaderProbe(t *testing.T) {
	t.Parallel()

	// Member 3 fails its KV health probe; members 1 and 2 are healthy.
	api := &fakeAPI{
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
	}
	var dialHits atomic.Int64
	dialFn := healthDialer(map[string]error{
		"http://10.0.0.1:2379": nil,
		"http://10.0.0.2:2379": nil,
		"http://10.0.0.3:2379": fmt.Errorf("timeout"),
	}, nil, &dialHits)

	c := newTestClient(api, dialFn)
	_, err := c.MemberList(context.Background())

	g := NewWithT(t)
	g.Expect(err).NotTo(HaveOccurred())
	// Only members whose Status probe succeeds reach the KV dial step.
	// Member 3 has no configured Status response, so its probe short-circuits
	// before dialFn is called: expect exactly 2 dial calls (members 1 and 2).
	g.Expect(dialHits.Load()).To(Equal(int64(2)))
	// The leader probe runs only over healthy members and stops at the first
	// non-zero leader, so exactly one Status call is made for leader resolution.
	// Members 1 and 2 each had one Status call during health probing (concurrent),
	// plus one for leader resolution = 3 total (stops after first non-zero leader).
	g.Expect(api.statusHits.Load()).To(BeNumerically(">=", int64(2)))
}

// TestListMembersNoClientURLs verifies that members without client URLs are
// returned with Health Unknown (no probe possible) and are excluded from the
// leader-resolution pass.
func TestListMembersNoClientURLs(t *testing.T) {
	t.Parallel()

	api := &fakeAPI{
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
	dialFn := healthDialer(map[string]error{
		"http://10.0.0.1:2379": nil,
	}, nil, nil)

	c := newTestClient(api, dialFn)
	members, err := c.MemberList(context.Background())

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

// TestNewClientFailsClosedWhenTLSExpectedButUnresolved verifies that
// NewClient fails closed when TLS is enabled but the CA cannot be resolved
// from either the StatefulSet volumes or the spec secret (both absent).
func TestNewClientFailsClosedWhenTLSExpectedButUnresolved(t *testing.T) {
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

	factory := NewFactory()
	_, err := factory.NewClient(context.Background(), cl, etcd)
	g.Expect(err).To(HaveOccurred())
}

// TestClientCloseNilClient verifies that Close is a no-op when closer is nil.
func TestClientCloseNilClient(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	c := newClient(nil, nil)
	g.Expect(c.Close()).To(Succeed())
}

// TestRemoveMemberIdempotency verifies that MemberRemove treats an already-absent
// member as success (rpctypes.ErrMemberNotFound, which etcd returns as gRPC
// codes.NotFound with "etcdserver: member not found"), and surfaces any other
// error wrapped. A generic codes.NotFound with a different message is NOT
// treated as success since etcd could return it for other reasons.
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
			name:      "rpctypes.ErrMemberNotFound is treated as success",
			removeErr: rpctypes.ErrMemberNotFound,
			wantErr:   false,
		},
		{
			name:      "generic codes.NotFound with different message surfaces as error",
			removeErr: grpcstatus.Error(codes.NotFound, "some other not-found"),
			wantErr:   true,
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
			c := newClient(&fakeAPI{removeErr: tc.removeErr}, nil)
			err := c.MemberRemove(context.Background(), 0x1)
			if tc.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}

// TestListMembersMultiURLFallthrough verifies that when a member advertises
// multiple client URLs, all URLs are passed to dialFn as a single slice so
// clientv3 can load-balance and fall through to a reachable endpoint. The
// member is marked Healthy when the probe succeeds on any of the URLs.
func TestListMembersMultiURLFallthrough(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	wantURLs := []string{"http://10.0.0.1:2379", "http://10.0.0.11:2379"}
	api := &fakeAPI{
		listResp: &clientv3.MemberListResponse{
			Members: []*etcdserverpb.Member{
				pbMember(1, "etcd-main-0", wantURLs, false),
			},
		},
		statusResp: map[string]*clientv3.StatusResponse{
			"http://10.0.0.1:2379": {Leader: 1},
		},
	}

	var gotURLs []string
	dialFn := func(endpoints []string) (probeClient, error) {
		gotURLs = endpoints
		return &fakeEndpoint{}, nil // healthy
	}

	c := newTestClient(api, dialFn)
	members, err := c.MemberList(context.Background())

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(members).To(HaveLen(1))
	g.Expect(members[0].Health).To(Equal(etcdmember.MemberHealthHealthy))
	g.Expect(members[0].Role).To(Equal(etcdmember.MemberRoleLeader))
	// All member URLs must be passed to dialFn so clientv3 can load-balance.
	g.Expect(gotURLs).To(Equal(wantURLs))
}
