package custodian

import (
	"context"
	"fmt"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"

	"github.com/gardener/etcd-druid/pkg/health/status"
	"github.com/gardener/etcd-druid/pkg/utils"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// Reconciler reconciles status of Etcd object
type Reconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Config     *Config
	logger     logr.Logger
	restConfig *rest.Config
}

// NewReconciler creates a new reconciler for Custodian.
func NewReconciler(mgr manager.Manager, config *Config) (*Reconciler, error) {
	return &Reconciler{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		Config:     config,
		restConfig: mgr.GetConfig(),
		logger:     log.Log.WithName(controllerName),
	}, nil
}

// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcds/status,verbs=get;update;patch;create

// Reconcile reconciles the etcd.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.logger.Info("Custodian controller reconciliation started")
	etcd := &druidv1alpha1.Etcd{}
	if err := r.Get(ctx, req.NamespacedName, etcd); err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return. Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	logger := r.logger.WithValues("etcd", kutil.Key(etcd.Namespace, etcd.Name).String())

	if etcd.Status.LastError != nil && *etcd.Status.LastError != "" {
		logger.Info(fmt.Sprintf("Requeue item because of last error: %v", *etcd.Status.LastError))
		return ctrl.Result{
			RequeueAfter: 30 * time.Second,
		}, nil
	}

	statusCheck := status.NewChecker(r.Client, r.Config.EtcdMember.NotReadyThreshold, r.Config.EtcdMember.UnknownThreshold)
	if err := statusCheck.Check(ctx, logger, etcd); err != nil {
		logger.Error(err, "Error executing status checks")
		return ctrl.Result{}, err
	}

	stsList, err := utils.FetchStatefulSets(ctx, r.Client, etcd)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Requeue if we found more than one or no StatefulSet.
	// The Etcd controller needs to decide what to do in such situations.
	if len(stsList.Items) != 1 {
		if err := r.updateEtcdStatus(ctx, logger, etcd, nil); err != nil {
			logger.Error(err, "Error while updating ETCD status when no statefulset found")
		}
		return ctrl.Result{
			RequeueAfter: 5 * time.Second,
		}, nil
	}

	if err := r.updateEtcdStatus(ctx, logger, etcd, &stsList.Items[0]); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: r.Config.SyncPeriod}, nil
}

func (r *Reconciler) updateEtcdStatus(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd, sts *appsv1.StatefulSet) error {
	logger.Info("Updating etcd status with statefulset information")

	// Bootstrap is a special case which is handled by the etcd controller.
	if !inBootstrap(etcd) && len(etcd.Status.Members) != 0 {
		etcd.Status.ClusterSize = pointer.Int32Ptr(int32(len(etcd.Status.Members)))
	}

	if sts != nil {
		etcd.Status.Etcd = &druidv1alpha1.CrossVersionObjectReference{
			APIVersion: sts.APIVersion,
			Kind:       sts.Kind,
			Name:       sts.Name,
		}

		ready := utils.CheckStatefulSet(etcd.Spec.Replicas, sts) == nil

		// To be changed once we have multiple replicas.
		etcd.Status.CurrentReplicas = sts.Status.CurrentReplicas
		etcd.Status.ReadyReplicas = sts.Status.ReadyReplicas
		etcd.Status.UpdatedReplicas = sts.Status.UpdatedReplicas
		etcd.Status.Ready = &ready
		logger.Info(fmt.Sprintf("ETCD status updated for statefulset current replicas: %v, ready replicas: %v, updated replicas: %v", sts.Status.CurrentReplicas, sts.Status.ReadyReplicas, sts.Status.UpdatedReplicas))
	} else {
		etcd.Status.CurrentReplicas = 0
		etcd.Status.ReadyReplicas = 0
		etcd.Status.UpdatedReplicas = 0

		etcd.Status.Ready = pointer.BoolPtr(false)
	}

	return r.Client.Status().Update(ctx, etcd)
}

func inBootstrap(etcd *druidv1alpha1.Etcd) bool {
	if etcd.Status.ClusterSize == nil {
		return true
	}
	return len(etcd.Status.Members) == 0 ||
		(len(etcd.Status.Members) < int(etcd.Spec.Replicas) && etcd.Spec.Replicas == *etcd.Status.ClusterSize)
}
