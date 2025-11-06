package reconcile

import (
	"context"
	"time"

	cmdutils "github.com/gardener/etcd-druid/druidctl/cmd/utils"
	"github.com/gardener/etcd-druid/druidctl/internal/client"
	"k8s.io/apimachinery/pkg/types"
)

type reconcileCmdCtxInterface interface {
	complete(*cmdutils.GlobalOptions) error
	validate() error
	execute(context.Context) error
}

// reconcileOptions holds state and functionality specific to the reconcile command
type reconcileOptions struct {
	*cmdutils.GlobalOptions
	waitTillReady bool
	watch         bool
	timeout       time.Duration
}

type reconcileCmdCtx struct {
	*reconcileOptions
	etcdRefList []types.NamespacedName
	etcdClient  client.EtcdClientInterface
}

func newReconcileOptions(options *cmdutils.GlobalOptions, waitTillReady bool, watch bool, timeout time.Duration) *reconcileOptions {
	return &reconcileOptions{
		GlobalOptions: options,
		waitTillReady: waitTillReady,
		watch:         watch,
		timeout:       timeout,
	}
}

// suspendReconcileOptions holds state and functionality specific to the suspend-reconcile command
type suspendReconcileOptions struct {
	*cmdutils.GlobalOptions
}

type suspendReconcileCmdCtx struct {
	*suspendReconcileOptions
	etcdRefList []types.NamespacedName
	etcdClient  client.EtcdClientInterface
}

func newSuspendReconcileOptions(options *cmdutils.GlobalOptions) *suspendReconcileOptions {
	return &suspendReconcileOptions{
		GlobalOptions: options,
	}
}

// resumeReconcileOptions holds state and functionality specific to the resume-reconcile command
type resumeReconcileOptions struct {
	*cmdutils.GlobalOptions
}

type resumeReconcileCmdCtx struct {
	*resumeReconcileOptions
	etcdRefList []types.NamespacedName
	etcdClient  client.EtcdClientInterface
}

func newResumeReconcileOptions(options *cmdutils.GlobalOptions) *resumeReconcileOptions {
	return &resumeReconcileOptions{
		GlobalOptions: options,
	}
}
