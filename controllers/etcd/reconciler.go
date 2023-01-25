package etcd

import (
	"github.com/gardener/gardener/pkg/chartrenderer"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// Reconciler reconciles an Etcd object
type Reconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Config        *Config
	recorder      record.EventRecorder
	chartRenderer chartrenderer.Interface
	RestConfig    *rest.Config
	ImageVector   imagevector.ImageVector
	logger        logr.Logger
}

func NewReconciler(mgr manager.Manager, config *Config) (*Reconciler, error) {
	panic("implement me")
}
