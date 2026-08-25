// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Package etcd provides a thin wrapper around the etcd v3 client, exposing only
// the membership operations required to orchestrate a quorum-safe scale-in.
// Only member discovery and removal are exposed; member removal is the only
// cluster-wide mutation the controller performs directly against etcd.
package etcd

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	etcdutil "github.com/gardener/etcd-druid/internal/utils/etcd"
	kutil "github.com/gardener/etcd-druid/internal/utils/kubernetes"

	clientv3 "go.etcd.io/etcd/client/v3"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// defaultDialTimeout bounds how long we wait to establish a connection to
	// the etcd client service before giving up on a membership call.
	defaultDialTimeout = 10 * time.Second
	// defaultCommandTimeout bounds a single MemberList/MemberRemove call.
	defaultCommandTimeout = 10 * time.Second
)

// Member is a minimal view of a single etcd cluster member, containing its
// numeric ID, name, and learner status.
type Member = etcdutil.Member

// MemberClient exposes the etcd membership operations required for a quorum-safe scale-in.
type MemberClient interface {
	// ListMembers returns the current members of the etcd cluster.
	ListMembers(ctx context.Context) ([]Member, error)
	// RemoveMember removes the member with the given ID from the cluster. etcd
	// applies its own quorum-safety admission check on removal.
	RemoveMember(ctx context.Context, id uint64) error
	// Close releases the underlying client connection.
	Close() error
}

// MemberClientFactory constructs a MemberClient for a given Etcd resource.
// The interface allows callers to substitute a test double in unit tests
// without requiring a live etcd cluster or Kubernetes secrets.
type MemberClientFactory interface {
	NewMemberClient(ctx context.Context, cl client.Client, etcd *druidv1alpha1.Etcd) (MemberClient, error)
}

// clientFactory is the production MemberClientFactory backed by clientv3.
type clientFactory struct{}

// NewMemberClientFactory returns the production MemberClientFactory.
func NewMemberClientFactory() MemberClientFactory {
	return &clientFactory{}
}

// NewMemberClient builds a clientv3-backed MemberClient that connects to the
// Etcd resource's client Service. When client TLS is enabled, the mTLS
// credentials are resolved from the running StatefulSet's TLS volumes (see
// kutil.BuildEtcdClientTLSConfig); it fails closed if TLS is enabled but the
// credentials cannot be resolved.
func (f *clientFactory) NewMemberClient(ctx context.Context, cl client.Client, etcd *druidv1alpha1.Etcd) (MemberClient, error) {
	scheme := "http"
	var tlsConfig *tls.Config

	if etcd.Spec.Etcd.ClientUrlTLS != nil {
		scheme = "https"
		sts, err := kutil.GetStatefulSet(ctx, cl, etcd)
		if err != nil {
			return nil, fmt.Errorf("failed to get StatefulSet for etcd %s/%s to resolve client TLS credentials: %w", etcd.Namespace, etcd.Name, err)
		}
		caDataKey := ptr.Deref(etcd.Spec.Etcd.ClientUrlTLS.TLSCASecretRef.DataKey, "")
		tlsConfig, err = kutil.BuildEtcdClientTLSConfig(ctx, cl, sts, etcd.Namespace, caDataKey)
		if err != nil {
			return nil, err
		}
		// Client TLS is configured on the Etcd spec, but the credentials could not be
		// resolved from the running StatefulSet (StatefulSet or CA volume absent). A
		// scale-in must not silently fall back to a plaintext dial, so fail closed and
		// let the reconcile requeue once the StatefulSet is available.
		if tlsConfig == nil {
			return nil, fmt.Errorf("client TLS is enabled for etcd %s/%s but the CA could not be resolved from the StatefulSet volumes", etcd.Namespace, etcd.Name)
		}
	}

	clientPort := ptr.Deref(etcd.Spec.Etcd.ClientPort, common.DefaultPortEtcdClient)
	endpoint := fmt.Sprintf("%s://%s.%s.svc:%d",
		scheme,
		druidv1alpha1.GetClientServiceName(etcd.ObjectMeta),
		etcd.Namespace,
		clientPort,
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
	return &v3Client{cli: cli}, nil
}

// v3Client is the production implementation of MemberClient, wrapping clientv3
// and exposing only the membership operations required for scale-in.
type v3Client struct {
	cli *clientv3.Client
}

// ListMembers returns the current members of the etcd cluster.
func (c *v3Client) ListMembers(ctx context.Context) ([]Member, error) {
	callCtx, cancel := context.WithTimeout(ctx, defaultCommandTimeout)
	defer cancel()

	resp, err := c.cli.MemberList(callCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to list etcd members: %w", err)
	}
	members := make([]Member, 0, len(resp.Members))
	for _, m := range resp.Members {
		members = append(members, Member{
			ID:        m.GetID(),
			Name:      m.GetName(),
			IsLearner: m.GetIsLearner(),
		})
	}
	return members, nil
}

// RemoveMember removes the member with the given ID from the cluster.
func (c *v3Client) RemoveMember(ctx context.Context, id uint64) error {
	callCtx, cancel := context.WithTimeout(ctx, defaultCommandTimeout)
	defer cancel()

	if _, err := c.cli.MemberRemove(callCtx, id); err != nil {
		return fmt.Errorf("failed to remove etcd member %x: %w", id, err)
	}
	return nil
}

// Close releases the underlying client connection.
func (c *v3Client) Close() error {
	if c.cli == nil {
		return nil
	}
	return c.cli.Close()
}
