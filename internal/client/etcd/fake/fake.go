// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

// Package fake provides a concurrency-safe test double for clientetcd.MemberClient
// and clientetcd.MemberClientFactory. It is a leaf package so importing it never
// creates an import cycle -- only test code depends on it.
package fake

import (
	"context"
	"fmt"
	"sync"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	clientetcd "github.com/gardener/etcd-druid/internal/client/etcd"
	etcdmember "github.com/gardener/etcd-druid/internal/etcd"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MemberClient is a concurrency-safe test double for clientetcd.MemberClient.
// It records removals in call order and filters removed members from subsequent
// ListMembers responses. All fields are guarded by mu.
type MemberClient struct {
	mu          sync.Mutex
	Members     []etcdmember.Member
	RemoveCalls []uint64
	ListErr     error
	RemoveErr   error
	ListCalls   int
	CloseCalls  int
	removed     map[uint64]struct{}
}

// NewMemberClient returns a MemberClient with total voting members named
// "<etcdName>-<ordinal>", all Healthy, with ordinal 0 as Leader.
func NewMemberClient(etcdName string, total int) *MemberClient {
	members := make([]etcdmember.Member, 0, total)
	for i := range total {
		role := etcdmember.MemberRoleMember
		if i == 0 {
			role = etcdmember.MemberRoleLeader
		}
		members = append(members, etcdmember.Member{
			ID:     uint64(i) + 1, // #nosec G115 -- i is a small non-negative loop index bounded by the member count and never crosses the size of uint64, so the conversion is safe.
			Name:   fmt.Sprintf("%s-%d", etcdName, i),
			Role:   role,
			Health: etcdmember.MemberHealthHealthy,
		})
	}
	return &MemberClient{
		Members: members,
		removed: make(map[uint64]struct{}),
	}
}

// SetHealth sets the Health of the member with the given ID.
func (f *MemberClient) SetHealth(id uint64, health etcdmember.MemberHealth) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.Members {
		if f.Members[i].ID == id {
			f.Members[i].Health = health
			return
		}
	}
}

// GetRemoveCalls returns a copy of RemoveCalls, safe for concurrent reads.
func (f *MemberClient) GetRemoveCalls() []uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]uint64{}, f.RemoveCalls...)
}

// ListMembers returns Members minus removed entries, or ListErr when set.
func (f *MemberClient) ListMembers(_ context.Context) ([]etcdmember.Member, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ListCalls++
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

// RemoveMember records the call and marks the member absent, or returns RemoveErr when set.
func (f *MemberClient) RemoveMember(_ context.Context, id uint64) error {
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

// Close counts the invocation and always succeeds.
func (f *MemberClient) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.CloseCalls++
	return nil
}

// MemberClientFactory is a test double for clientetcd.MemberClientFactory.
type MemberClientFactory struct {
	Client    clientetcd.MemberClient
	CreateErr error
}

// NewMemberClient returns Client, or CreateErr when set.
func (f *MemberClientFactory) NewMemberClient(_ context.Context, _ client.Client, _ *druidv1alpha1.Etcd) (clientetcd.MemberClient, error) {
	if f.CreateErr != nil {
		return nil, f.CreateErr
	}
	return f.Client, nil
}

var (
	_ clientetcd.MemberClient        = (*MemberClient)(nil)
	_ clientetcd.MemberClientFactory = (*MemberClientFactory)(nil)
)
