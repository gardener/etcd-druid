// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

// Package etcd provides a thin clientv3 wrapper for the membership operations
// required by quorum-safe scale-in. Membership, role, and health are taken
// from etcd RPCs, never from CR status or member leases.
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
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultDialTimeout    = 10 * time.Second
	defaultCommandTimeout = 10 * time.Second
	defaultStatusTimeout  = 10 * time.Second

	// healthProbeKey is the key used in the per-member KV Get probe, mirroring etcdctl endpoint health.
	healthProbeKey = "health"
)

// Cluster is the membership sub-client, mirroring clientv3's Cluster interface.
type Cluster interface {
	// MemberList returns the current cluster members with Role and Health from live RPCs.
	MemberList(ctx context.Context) ([]etcdmember.Member, error)
	// MemberRemove removes the member with the given ID. An already-absent member is treated as success.
	MemberRemove(ctx context.Context, id uint64) error
}

// Maintenance is the maintenance sub-client, mirroring clientv3's Maintenance interface.
type Maintenance interface {
	// Status returns the status of the endpoint.
	Status(ctx context.Context, endpoint string) (*clientv3.StatusResponse, error)
}

// KV is the narrow key-value sub-client used for per-member health probes.
// It mirrors the subset of clientv3.KV needed to confirm a member can serve requests.
type KV interface {
	// Get retrieves keys from etcd.
	Get(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error)
}

// Client is the per-connection etcd API used during a single reconcile.
// It implements Cluster, Maintenance, and KV directly and must be Closed when finished.
type Client interface {
	Cluster
	Maintenance
	KV
	Close() error
}

// Factory builds a Client for a specific Etcd resource.
// Callers inject a fake in unit tests; production uses NewFactory.
type Factory interface {
	NewClient(ctx context.Context, k8sClient client.Client, etcd *druidv1alpha1.Etcd) (Client, error)
}

type clientFactory struct{}

// NewFactory returns the production Factory.
func NewFactory() Factory {
	return &clientFactory{}
}

// NewClient builds a clientv3-backed Client connecting to the Etcd resource's
// client Service. When TLS is enabled, mTLS credentials are resolved from the
// Etcd CR's TLS secret references; it fails closed if they cannot be resolved.
func (f *clientFactory) NewClient(ctx context.Context, k8sClient client.Client, etcd *druidv1alpha1.Etcd) (Client, error) {
	scheme, tlsConfig, err := kutil.GetEtcdClientSchemeAndTLSConfig(ctx, k8sClient, etcd)
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

	// dialFn creates a per-member client for KV health probes, mirroring
	// etcdctl endpoint health. Each probe client uses the same TLS config as
	// the main connection and supports multiple endpoints for fallthrough.
	dialFn := func(endpoints []string) (probeClient, error) {
		return clientv3.New(clientv3.Config{
			Endpoints:   endpoints,
			DialTimeout: defaultDialTimeout,
			TLS:         tlsConfig,
		})
	}
	c := newClient(cli, cli.Close)
	c.(*etcdClient).dialFn = dialFn
	return c, nil
}

// probeClient is the narrow interface returned by dialFn for per-member KV
// health probes. *clientv3.Client satisfies it; tests inject a fakeEndpoint.
type probeClient interface {
	Get(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error)
	Close() error
}

// Compile-time assertion that *clientv3.Client satisfies probeClient.
var _ probeClient = (*clientv3.Client)(nil)

// etcdAPI is the narrow subset of *clientv3.Client that etcdClient uses.
// Keeping it as a seam allows white-box tests to inject a fake without a real
// etcd connection. *clientv3.Client satisfies it; the compile-time assertion
// below catches any signature drift on etcd upgrades.
type etcdAPI interface {
	MemberList(ctx context.Context) (*clientv3.MemberListResponse, error)
	MemberRemove(ctx context.Context, id uint64) (*clientv3.MemberRemoveResponse, error)
	Status(ctx context.Context, endpoint string) (*clientv3.StatusResponse, error)
	Get(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error)
}

// Compile-time assertion that *clientv3.Client satisfies etcdAPI.
// If an etcd version upgrade changes any signature, this fails to build.
var _ etcdAPI = (*clientv3.Client)(nil)

// newClient is used by the production factory and by white-box tests.
func newClient(api etcdAPI, closer func() error) Client {
	return &etcdClient{api: api, closer: closer}
}

// etcdClient is the production Client backed by a *clientv3.Client (or a test fake).
type etcdClient struct {
	api    etcdAPI
	closer func() error
	// dialFn creates a probeClient for per-member KV health probes (mirroring
	// etcdctl endpoint health). Nil in unit tests that skip the KV probe.
	dialFn func([]string) (probeClient, error)
}

// Close releases the underlying connection.
func (c *etcdClient) Close() error {
	if c.closer == nil {
		return nil
	}
	return c.closer()
}

// MemberList returns the current cluster members enriched with live role and
// health from concurrent per-member Status probes. Health is fail-closed:
// Unknown on any probe error. Leader identity is resolved after probing
// (mirrors `etcdctl endpoint status`) using only the healthy members' URLs.
func (c *etcdClient) MemberList(ctx context.Context) ([]etcdmember.Member, error) {
	listCtx, cancel := context.WithTimeout(ctx, defaultCommandTimeout)
	defer cancel()

	resp, err := c.api.MemberList(listCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to list etcd members: %w", err)
	}

	members := buildMembersFromList(resp.Members)
	healthyURLs := c.probeAndSetHealth(ctx, resp.Members, members)
	c.resolveAndSetLeader(ctx, members, healthyURLs)
	return members, nil
}

// MemberRemove removes the member with the given ID. It is idempotent: a
// codes.NotFound status (etcdserver: member not found) is treated as success.
// etcd returns this gRPC status code for absent members (see ErrGRPCMemberNotFound
// in go.etcd.io/etcd/api/v3/v3rpc/rpctypes).
func (c *etcdClient) MemberRemove(ctx context.Context, id uint64) error {
	callCtx, cancel := context.WithTimeout(ctx, defaultCommandTimeout)
	defer cancel()

	if _, err := c.api.MemberRemove(callCtx, id); err != nil {
		if errors.Is(err, rpctypes.ErrMemberNotFound) {
			return nil
		}
		return fmt.Errorf("failed to remove etcd member %x: %w", id, err)
	}
	return nil
}

// Status returns the status of the given etcd endpoint.
func (c *etcdClient) Status(ctx context.Context, endpoint string) (*clientv3.StatusResponse, error) {
	return c.api.Status(ctx, endpoint)
}

// Get retrieves keys from etcd via the main (load-balanced) connection.
func (c *etcdClient) Get(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
	return c.api.Get(ctx, key, opts...)
}

// buildMembersFromList converts raw protobuf members to etcdmember.Member values
// with role set (Learner when IsLearner, Member otherwise) and health Unknown.
func buildMembersFromList(raw []*etcdserverpb.Member) []etcdmember.Member {
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
// Each goroutine writes to a distinct index so no mutex is needed.
func (c *etcdClient) probeAndSetHealth(ctx context.Context, raw []*etcdserverpb.Member, members []etcdmember.Member) [][]string {
	healthyURLs := make([][]string, len(raw))
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
				healthyURLs[idx] = memberURLs
			}
		}(i, urls)
	}
	wg.Wait()
	var result [][]string
	for _, urls := range healthyURLs {
		if urls != nil {
			result = append(result, urls)
		}
	}
	return result
}

// resolveAndSetLeader probes the leader ID from the cluster and marks the
// matching member as Leader in-place. It is a no-op when probeLeaderID returns 0
// (no leader found or all probes failed).
func (c *etcdClient) resolveAndSetLeader(ctx context.Context, members []etcdmember.Member, healthyURLs [][]string) {
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

// probeMemberHealthy verifies that a member is both reachable and can serve
// key-value requests. It uses two probes, mirroring etcdctl endpoint health:
//
//  1. Status RPC: confirms the gRPC connection is alive and provides the
//     follower/leader role used by probeLeaderID. Status(ctx, endpoint) dials
//     directly to the specific member.
//  2. KV Get: a per-endpoint client dials the specific member and issues a
//     linearizable Get to confirm it can serve requests. Status can return
//     successfully even when a member is isolated or lagging; the KV Get is
//     the authoritative health signal. ErrPermissionDenied is treated as
//     healthy (the proposal went through consensus).
//
// When dialFn is nil (unit tests that skip the KV probe), a successful Status
// is sufficient to declare the member healthy. Fail-closed.
func (c *etcdClient) probeMemberHealthy(ctx context.Context, urls []string) bool {
	for _, url := range urls {
		if url == "" {
			continue
		}
		statusCtx, statusCancel := context.WithTimeout(ctx, defaultStatusTimeout)
		_, statusErr := c.Status(statusCtx, url)
		statusCancel()
		if statusErr != nil {
			continue
		}
		if c.dialFn == nil {
			return true
		}
		if c.probeEndpointKV(ctx, urls) {
			return true
		}
	}
	return false
}

// probeEndpointKV dials the given endpoints with a fresh probeClient and issues
// a Get to verify the member can serve requests. The client is closed after the
// probe. ErrPermissionDenied is treated as healthy (the proposal reached
// consensus). Any other error is fail-closed.
func (c *etcdClient) probeEndpointKV(ctx context.Context, endpoints []string) bool {
	kvClient, err := c.dialFn(endpoints)
	if err != nil {
		return false
	}
	defer kvClient.Close() //nolint:errcheck
	kvCtx, kvCancel := context.WithTimeout(ctx, defaultStatusTimeout)
	_, kvErr := kvClient.Get(kvCtx, healthProbeKey)
	kvCancel()
	return kvErr == nil || errors.Is(kvErr, rpctypes.ErrPermissionDenied)
}

// probeLeaderID resolves the cluster leader by querying healthy members and
// returning the first non-zero Leader reported. Returns 0 if no URL answers.
func (c *etcdClient) probeLeaderID(ctx context.Context, memberURLs [][]string) uint64 {
	for _, urls := range memberURLs {
		for _, url := range urls {
			if url == "" {
				continue
			}
			statusCtx, statusCancel := context.WithTimeout(ctx, defaultStatusTimeout)
			st, err := c.Status(statusCtx, url)
			statusCancel()
			if err != nil {
				continue
			}
			if st.Leader != 0 {
				return st.Leader
			}
			break
		}
	}
	return 0
}

// Compile-time assertion that *etcdClient satisfies Client.
var _ Client = (*etcdClient)(nil)

// Compile-time assertion that *clientv3.Client satisfies KV.
var _ KV = (*clientv3.Client)(nil)
