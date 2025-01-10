package secret

import (
	"context"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// mapEtcdToSecret checks if the object is an Etcd resource and maps it to reconcile.Request objects
// for every secret that Etcd resource references.
func mapEtcdToSecret(_ context.Context, obj client.Object) []reconcile.Request {
	etcd, ok := obj.(*druidv1alpha1.Etcd)
	if !ok {
		return nil
	}

	var requests []reconcile.Request

	if etcd.Spec.Etcd.ClientUrlTLS != nil {
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: etcd.Namespace,
			Name:      etcd.Spec.Etcd.ClientUrlTLS.TLSCASecretRef.Name,
		}})

		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: etcd.Namespace,
			Name:      etcd.Spec.Etcd.ClientUrlTLS.ServerTLSSecretRef.Name,
		}})

		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: etcd.Namespace,
			Name:      etcd.Spec.Etcd.ClientUrlTLS.ClientTLSSecretRef.Name,
		}})
	}

	if etcd.Spec.Etcd.PeerUrlTLS != nil {
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: etcd.Namespace,
			Name:      etcd.Spec.Etcd.PeerUrlTLS.TLSCASecretRef.Name,
		}})

		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: etcd.Namespace,
			Name:      etcd.Spec.Etcd.PeerUrlTLS.ServerTLSSecretRef.Name,
		}})
	}

	if etcd.Spec.Backup.Store != nil && etcd.Spec.Backup.Store.SecretRef != nil {
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: etcd.Namespace,
			Name:      etcd.Spec.Backup.Store.SecretRef.Name,
		}})
	}

	return requests
}
