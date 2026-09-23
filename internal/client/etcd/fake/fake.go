// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

// Package fake provides concurrency-safe test doubles for clientetcd.Client and
// clientetcd.Factory. It is a leaf package so importing it never creates
// an import cycle; only test code depends on it.
package fake

import (
	"context"
	"fmt"
	"sync"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	clientetcd "github.com/gardener/etcd-druid/internal/client/etcd"
	etcdmember "github.com/gardener/etcd-druid/internal/etcd"

	clientv3 "go.etcd.io/etcd/client/v3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Client is a concurrency-safe test double that implements clientetcd.Client.
// All exported fields are guarded by mu.
type Client struct {
	mu          sync.Mutex
	Members     []etcdmember.Member
	RemoveCalls []uint64
	ListErr     error
	RemoveErr   error
	CloseCalls  int
	removed     map[uint64]struct{}
}

// NewClient returns a Client with total voting members named
// "<etcdName>-<ordinal>", all Healthy, with ordinal 0 as Leader.
func NewClient(etcdName string, total int) *Client {
	members := make([]etcdmember.Member, 0, total)
	for i := range total {
		role := etcdmember.MemberRoleMember
		if i == 0 {
			role = etcdmember.MemberRoleLeader
		}
		members = append(members, etcdmember.Member{
			ID:     uint64(i) + 1, // #nosec G115 -- i is a small non-negative loop index bounded by the member count
			Name:   fmt.Sprintf("%s-%d", etcdName, i),
			Role:   role,
			Health: etcdmember.MemberHealthHealthy,
		})
	}
	return &Client{
		Members: members,
		removed: make(map[uint64]struct{}),
	}
}

// SetHealth sets the Health of the member with the given ID.
func (f *Client) SetHealth(id uint64, health etcdmember.MemberHealth) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.Members {
		if f.Members[i].ID == id {
			f.Members[i].Health = health
			return
		}
	}
}

// MemberList implements clientetcd.Cluster.
func (f *Client) MemberList(_ context.Context) ([]etcdmember.Member, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ListErr != nil {
		return nil, f.ListErr
	}
	live := make([]etcdmember.Member, 0, len(f.Members))
	for _, m := range f.Members {
		if _, gone := f.removed[m.ID]; !gone {
			live = append(live, m)
		}
	}
	return live, nil
}

// MemberRemove implements clientetcd.Cluster.
func (f *Client) MemberRemove(_ context.Context, id uint64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.RemoveCalls = append(f.RemoveCalls, id)
	if f.RemoveErr != nil {
		return f.RemoveErr
	}
	if f.removed == nil {
		f.removed = make(map[uint64]struct{})
	}
	f.removed[id] = struct{}{}
	return nil
}

// Status implements clientetcd.Maintenance.
func (f *Client) Status(_ context.Context, _ string) (*clientv3.StatusResponse, error) {
	return &clientv3.StatusResponse{}, nil
}

// Get implements clientetcd.KV.
func (f *Client) Get(_ context.Context, _ string, _ ...clientv3.OpOption) (*clientv3.GetResponse, error) {
	return &clientv3.GetResponse{}, nil
}

// Close counts the invocation and always succeeds.
func (f *Client) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.CloseCalls++
	return nil
}

// Factory is a test double for clientetcd.Factory.
type Factory struct {
	Client    *Client
	CreateErr error
}

// NewClient returns Client as a clientetcd.Client, or CreateErr when set.
func (f *Factory) NewClient(_ context.Context, _ client.Client, _ *druidv1alpha1.Etcd) (clientetcd.Client, error) {
	if f.CreateErr != nil {
		return nil, f.CreateErr
	}
	return f.Client, nil
}

var (
	_ clientetcd.Client  = (*Client)(nil)
	_ clientetcd.Factory = (*Factory)(nil)
)
