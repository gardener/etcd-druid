// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

// Package etcd provides a thin clientv3 wrapper for the membership operations
// required by quorum-safe scale-in. Membership, role, and health are taken only
// from etcd RPCs -- never from CR status or member leases.
package etcd

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	etcdmember "github.com/gardener/etcd-druid/internal/etcd"
	kutil "github.com/gardener/etcd-druid/internal/utils/kubernetes"

	etcdserverpb "go.etcd.io/etcd/api/v3/etcdserverpb"
	"go.etcd.io/etcd/api/v3/v3rpc/rpctypes"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultDialTimeout    = 10 * time.Second
	defaultCommandTimeout = 10 * time.Second
	defaultStatusTimeout  = 10 * time.Second
)

// MemberClient is the per-connection API used during a single reconcile.
// Construct it via MemberClientFactory and always Close it when finished.
type MemberClient interface {
	// ListMembers returns the current cluster members with Role and Health from live RPCs.
	ListMembers(ctx context.Context) ([]etcdmember.Member, error)
	// RemoveMember removes the member with the given ID. An already-absent member is treated as success.
	RemoveMember(ctx context.Context, id uint64) error
	// Close releases the underlying client connection.
	Close() error
}

// MemberClientFactory builds a MemberClient for a specific Etcd resource.
// Callers inject a fake in unit tests; production uses NewMemberClientFactory.
type MemberClientFactory interface {
	NewMemberClient(ctx context.Context, cl client.Client, etcd *druidv1alpha1.Etcd) (MemberClient, error)
}

type memberClientFactory struct{}

// NewMemberClientFactory returns the production MemberClientFactory.
func NewMemberClientFactory() MemberClientFactory {
	return &memberClientFactory{}
}

// NewMemberClient builds a clientv3-backed MemberClient connecting to the Etcd
// resource's client Service. When TLS is enabled, mTLS credentials are resolved
// from the StatefulSet's TLS volumes; it fails closed if they cannot be resolved.
func (f *memberClientFactory) NewMemberClient(ctx context.Context, cl client.Client, etcd *druidv1alpha1.Etcd) (MemberClient, error) {
	scheme, tlsConfig, err := kutil.GetEtcdClientSchemeAndTLSConfig(ctx, cl, etcd)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s://%s.%s.svc:%d",
		scheme,
		druidv1alpha1.GetClientServiceName(etcd.ObjectMeta),
		etcd.Namespace,
		druidv1alpha1.GetClientPort(etcd),
	)

	cli, err := clientv3.New(clientv3.Config{
		Context:     ctx,
		Endpoints:   []string{endpoint},
		DialTimeout: defaultDialTimeout,
		TLS:         tlsConfig,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd client for %s/%s: %w", etcd.Namespace, etcd.Name, err)
	}
	return &memberClient{api: cli, closer: cli.Close}, nil
}

// memberAPI is the narrow subset of clientv3 that memberClient uses.
// *clientv3.Client satisfies this interface.
type memberAPI interface {
	MemberList(ctx context.Context) (*clientv3.MemberListResponse, error)
	MemberRemove(ctx context.Context, id uint64) (*clientv3.MemberRemoveResponse, error)
	Status(ctx context.Context, endpoint string) (*clientv3.StatusResponse, error)
}

// memberClient is the production MemberClient backed by clientv3.
type memberClient struct {
	api    memberAPI
	closer func() error // separate from api; memberAPI intentionally omits Close
}

// ListMembers returns the current cluster members enriched with live role and
// health from concurrent per-member Status probes. Health is fail-closed:
// Unknown on any probe error. All probe goroutines are joined before returning.
// Leader identity is resolved after probing (mirrors `etcdctl endpoint status`)
// using only the healthy members' URLs to avoid waiting out a full timeout on
// members already observed to be down.
func (c *memberClient) ListMembers(ctx context.Context) ([]etcdmember.Member, error) {
	listCtx, cancel := context.WithTimeout(ctx, defaultCommandTimeout)
	defer cancel()

	resp, err := c.api.MemberList(listCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to list etcd members: %w", err)
	}

	members := c.buildMembersFromList(resp.Members)
	healthyURLs := c.probeAndSetHealth(ctx, resp.Members, members)
	c.resolveAndSetLeader(ctx, members, healthyURLs)
	return members, nil
}

// buildMembersFromList converts the raw MemberList response into etcdmember.Member
// values with Role set (Learner or Member) and Health initialised to Unknown.
func (c *memberClient) buildMembersFromList(raw []*etcdserverpb.Member) []etcdmember.Member {
	members := make([]etcdmember.Member, len(raw))
	for i, m := range raw {
		role := etcdmember.MemberRoleMember
		if m.GetIsLearner() {
			role = etcdmember.MemberRoleLearner
		}
		members[i] = etcdmember.Member{
			ID:     m.GetID(),
			Name:   m.GetName(),
			Role:   role,
			Health: etcdmember.MemberHealthUnknown,
		}
	}
	return members
}

// probeAndSetHealth fires concurrent Status probes for each member that has
// client URLs, updates members[i].Health in place, and returns the client-URL
// sets for members confirmed healthy (for use by resolveAndSetLeader).
// All goroutines are joined before returning.
func (c *memberClient) probeAndSetHealth(ctx context.Context, raw []*etcdserverpb.Member, members []etcdmember.Member) [][]string {
	var mu sync.Mutex
	var healthyURLs [][]string
	var wg sync.WaitGroup
	for i, m := range raw {
		urls := m.GetClientURLs()
		if len(urls) == 0 {
			continue
		}
		wg.Add(1)
		go func(idx int, memberURLs []string) {
			defer wg.Done()
			if c.probeMemberHealthy(ctx, memberURLs) {
				members[idx].Health = etcdmember.MemberHealthHealthy
				mu.Lock()
				healthyURLs = append(healthyURLs, memberURLs)
				mu.Unlock()
			}
		}(i, urls)
	}
	wg.Wait()
	return healthyURLs
}

// resolveAndSetLeader queries healthy members to find the current leader and
// promotes that member's Role to Leader. healthyURLs is the list of client-URL
// sets for members already confirmed healthy by probeAndSetHealth.
func (c *memberClient) resolveAndSetLeader(ctx context.Context, members []etcdmember.Member, healthyURLs [][]string) {
	leaderID := c.probeLeaderID(ctx, healthyURLs)
	if leaderID == 0 {
		return
	}
	for i := range members {
		if members[i].ID == leaderID && members[i].Role != etcdmember.MemberRoleLearner {
			members[i].Role = etcdmember.MemberRoleLeader
			break
		}
	}
}

// probeMemberHealthy issues a Status RPC against the member's client URLs,
// trying each in order until one succeeds. Each attempt is bounded by
// defaultStatusTimeout, derived from the caller's ctx so cancellation still
// propagates. Returns true on the first success; false if all URLs error
// (fail-closed).
func (c *memberClient) probeMemberHealthy(ctx context.Context, urls []string) bool {
	for _, url := range urls {
		if url == "" {
			continue
		}
		statusCtx, statusCancel := context.WithTimeout(ctx, defaultStatusTimeout)
		_, err := c.api.Status(statusCtx, url)
		statusCancel()
		if err == nil {
			return true
		}
	}
	return false
}

// probeLeaderID resolves the cluster leader by trying each member's client URLs
// (a member's URLs are tried in order until one answers, mirroring how
// `etcdctl endpoint status` probes) and returning the first non-zero Leader
// reported. Returns 0 if no URL reports a leader.
func (c *memberClient) probeLeaderID(ctx context.Context, memberURLs [][]string) uint64 {
	for _, urls := range memberURLs {
		for _, url := range urls {
			if url == "" {
				continue
			}
			statusCtx, statusCancel := context.WithTimeout(ctx, defaultStatusTimeout)
			st, err := c.api.Status(statusCtx, url)
			statusCancel()
			if err != nil {
				continue
			}
			if st.Leader != 0 {
				return st.Leader
			}
			// This URL answered but reported no leader; no point trying the
			// member's other URLs -- move on to the next member.
			break
		}
	}
	return 0
}

// RemoveMember removes the member with the given ID. It is idempotent: a
// codes.NotFound status or the typed rpctypes.ErrMemberNotFound (an already-absent
// member) is treated as success.
func (c *memberClient) RemoveMember(ctx context.Context, id uint64) error {
	callCtx, cancel := context.WithTimeout(ctx, defaultCommandTimeout)
	defer cancel()

	if _, err := c.api.MemberRemove(callCtx, id); err != nil {
		if status.Code(err) == codes.NotFound || errors.Is(err, rpctypes.ErrMemberNotFound) {
			return nil
		}
		return fmt.Errorf("failed to remove etcd member %x: %w", id, err)
	}
	return nil
}

// Close releases the underlying client connection.
func (c *memberClient) Close() error {
	if c.closer == nil {
		return nil
	}
	return c.closer()
}
