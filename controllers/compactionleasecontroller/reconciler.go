package compactionleasecontroller

import (
	"context"

	"github.com/gardener/etcd-druid/controllers/utils"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type Reconciler struct {
	client.Client
	config      *Config
	ImageVector imagevector.ImageVector
	logger      logr.Logger
}

// NewReconciler creates a new reconciler for compaction lease.
func NewReconciler(mgr manager.Manager, config *Config, withImageVector bool) (*Reconciler, error) {
	var (
		imageVector imagevector.ImageVector
		err         error
	)
	if withImageVector {
		if imageVector, err = utils.CreateDefaultImageVector(); err != nil {
			return nil, err
		}
	}
	return &Reconciler{
		Client:      mgr.GetClient(),
		config:      config,
		ImageVector: imageVector,
		logger:      log.Log.WithName("compaction-lease-controller"),
	}, nil
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	panic("implement me")
}
