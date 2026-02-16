// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package reconciliation

import (
	"time"

	cmdutils "github.com/gardener/etcd-druid/druidctl/cmd/utils"
	"github.com/gardener/etcd-druid/druidctl/internal/client"

	"k8s.io/apimachinery/pkg/types"
)

// reconcileOptions holds command-specific options for the reconcile trigger command
type reconcileOptions struct {
	*cmdutils.GlobalOptions
	waitTillReady bool
	watch         bool
	timeout       time.Duration
}

// reconcileRuntime holds runtime state for the reconcile trigger command
type reconcileRuntime struct {
	*cmdutils.RuntimeEnv
	etcdRefList []types.NamespacedName
	etcdClient  client.EtcdClientInterface
}

// reconcileCmdCtx composes options and runtime for the reconcile trigger command
type reconcileCmdCtx struct {
	*reconcileOptions
	*reconcileRuntime
}

func newReconcileOptions(options *cmdutils.GlobalOptions, waitTillReady bool, watch bool, timeout time.Duration) *reconcileOptions {
	return &reconcileOptions{
		GlobalOptions: options,
		waitTillReady: waitTillReady,
		watch:         watch,
		timeout:       timeout,
	}
}

func newReconcileRuntime(runtime *cmdutils.RuntimeEnv) *reconcileRuntime {
	return &reconcileRuntime{
		RuntimeEnv: runtime,
	}
}

// suspendReconcileOptions holds command-specific options for the reconcile suspend command
type suspendReconcileOptions struct {
	*cmdutils.GlobalOptions
}

// suspendReconcileRuntime holds runtime state for the reconcile suspend command
type suspendReconcileRuntime struct {
	*cmdutils.RuntimeEnv
	etcdRefList []types.NamespacedName
	etcdClient  client.EtcdClientInterface
}

// suspendReconcileCmdCtx composes options and runtime for the reconcile suspend command
type suspendReconcileCmdCtx struct {
	*suspendReconcileOptions
	*suspendReconcileRuntime
}

func newSuspendReconcileOptions(options *cmdutils.GlobalOptions) *suspendReconcileOptions {
	return &suspendReconcileOptions{
		GlobalOptions: options,
	}
}

func newSuspendReconcileRuntime(runtime *cmdutils.RuntimeEnv) *suspendReconcileRuntime {
	return &suspendReconcileRuntime{
		RuntimeEnv: runtime,
	}
}

// resumeReconcileOptions holds command-specific options for the reconcile resume command
type resumeReconcileOptions struct {
	*cmdutils.GlobalOptions
}

// resumeReconcileRuntime holds runtime state for the reconcile resume command
type resumeReconcileRuntime struct {
	*cmdutils.RuntimeEnv
	etcdRefList []types.NamespacedName
	etcdClient  client.EtcdClientInterface
}

// resumeReconcileCmdCtx composes options and runtime for the reconcile resume command
type resumeReconcileCmdCtx struct {
	*resumeReconcileOptions
	*resumeReconcileRuntime
}

func newResumeReconcileOptions(options *cmdutils.GlobalOptions) *resumeReconcileOptions {
	return &resumeReconcileOptions{
		GlobalOptions: options,
	}
}

func newResumeReconcileRuntime(runtime *cmdutils.RuntimeEnv) *resumeReconcileRuntime {
	return &resumeReconcileRuntime{
		RuntimeEnv: runtime,
	}
}
