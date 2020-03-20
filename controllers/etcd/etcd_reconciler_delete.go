// Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package etcd

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *Reconciler) delete(ctx context.Context, etcd *druidv1alpha1.Etcd) (ctrl.Result, error) {
	logger.Infof("Deletion timestamp set for etcd: %s", etcd.GetName())
	if err := r.removeDependantStatefulset(ctx, etcd); err != nil {
		if err := r.updateEtcdErrorStatus(etcd, nil, err); err != nil {
			return ctrl.Result{
				Requeue: true,
			}, err
		}
		return ctrl.Result{
			Requeue: true,
		}, err
	}

	if err := r.removeFinalizersToDependantSecrets(ctx, etcd); err != nil {
		if err := r.updateEtcdErrorStatus(etcd, nil, err); err != nil {
			return ctrl.Result{
				Requeue: true,
			}, err
		}
		return ctrl.Result{
			Requeue: true,
		}, err
	}

	if sets.NewString(etcd.Finalizers...).Has(FinalizerName) {
		logger.Infof("Removing finalizer (%s) from etcd %s", FinalizerName, etcd.GetName())
		finalizers := sets.NewString(etcd.Finalizers...)
		finalizers.Delete(FinalizerName)
		etcd.Finalizers = finalizers.UnsortedList()
		if err := r.Update(ctx, etcd); err != nil && !errors.IsConflict(err) {
			return ctrl.Result{
				Requeue: true,
			}, err
		}
	}
	logger.Infof("Deleted etcd %s successfully.", etcd.GetName())
	return ctrl.Result{}, nil
}

func (r *Reconciler) removeDependantStatefulset(ctx context.Context, etcd *druidv1alpha1.Etcd) error {
	logger.Infof("Deleting etcd statefulset for etcd:%s in namespace:%s", etcd.Name, etcd.Namespace)
	selector, err := metav1.LabelSelectorAsSelector(etcd.Spec.Selector)
	if err != nil {
		return err
	}

	statefulSets := &appsv1.StatefulSetList{}
	if err = r.List(ctx, statefulSets, client.InNamespace(etcd.Namespace), client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return err
	}
	for _, sts := range statefulSets.Items {
		if canDeleteStatefulset(&sts, etcd) {
			logger.Infof("Etcd statefulset can be deleted. Deleting statefulset: %s/%s", sts.GetNamespace(), sts.GetName())
			if err := r.Delete(ctx, &sts); err != nil {
				return err
			}
		}
	}
	return nil
}

func canDeleteStatefulset(sts *appsv1.StatefulSet, etcd *druidv1alpha1.Etcd) bool {
	// Adding check for ownerReference to have the same delete path for statefulset.
	// The statefulset with ownerReference will be deleted automatically when etcd is
	// delete but we would like to explicitly delete it to maintain uniformity in the
	// delete path.
	return checkEtcdOwnerReference(sts.GetOwnerReferences(), etcd) ||
		checkEtcdAnnotations(sts.GetAnnotations(), etcd)
}

func (r *Reconciler) removeFinalizersToDependantSecrets(ctx context.Context, etcd *druidv1alpha1.Etcd) error {
	secrets := []*corev1.SecretReference{}
	if etcd.Spec.Etcd.TLS != nil {
		secrets = append(secrets,
			&etcd.Spec.Etcd.TLS.ClientTLSSecretRef,
			&etcd.Spec.Etcd.TLS.ServerTLSSecretRef,
			&etcd.Spec.Etcd.TLS.TLSCASecretRef,
		)
	}
	if etcd.Spec.Backup.Store != nil && etcd.Spec.Backup.Store.SecretRef != nil {
		secrets = append(secrets, etcd.Spec.Backup.Store.SecretRef)
	}

	for _, secretRef := range secrets {
		secret := &corev1.Secret{}
		if err := r.Client.Get(ctx, types.NamespacedName{
			Name:      secretRef.Name,
			Namespace: etcd.Namespace,
		}, secret); err != nil {
			if !errors.IsNotFound(err) {
				return err
			}
		} else if finalizers := sets.NewString(secret.Finalizers...); finalizers.Has(FinalizerName) {
			logger.Infof("Removing finalizer (%s) from secret %s", FinalizerName, secret.GetName())
			finalizers.Delete(FinalizerName)
			secret.Finalizers = finalizers.UnsortedList()
			if err := r.Update(ctx, secret); client.IgnoreNotFound(err) != nil {
				return err
			}
		}
	}
	return nil
}
