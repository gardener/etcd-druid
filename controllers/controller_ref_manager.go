// Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

// Package controllers is used to provide the core functionalities of hvpa-controller
package controllers

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"k8s.io/client-go/tools/cache"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	rbacv1 "k8s.io/api/rbac/v1"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/common"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// BaseControllerRefManager is the struct is used to identify the base controller of the object
type BaseControllerRefManager struct {
	Controller metav1.Object
	Selector   labels.Selector

	canAdoptErr  error
	canAdoptOnce sync.Once
	CanAdoptFunc func() error
}

// CanAdopt is used to identify if the object can be adopted by the controller
func (m *BaseControllerRefManager) CanAdopt() error {
	m.canAdoptOnce.Do(func() {
		if m.CanAdoptFunc != nil {
			m.canAdoptErr = m.CanAdoptFunc()
		}
	})
	return m.canAdoptErr
}

// claimObject tries to take ownership of an object for this controller.
//
// It will reconcile the following:
//   - Adopt orphans if the match function returns true.
//   - Release owned objects if the match function returns false.
//
// A non-nil error is returned if some form of reconciliation was attempted and
// failed. Usually, controllers should try again later in case reconciliation
// is still needed.
//
// If the error is nil, either the reconciliation succeeded, or no
// reconciliation was necessary. The returned boolean indicates whether you now
// own the object.
//
// No reconciliation will be attempted if the controller is being deleted.
func (m *BaseControllerRefManager) claimObject(ctx context.Context, obj client.Object, match func(metav1.Object) bool, adopt, release func(context.Context, client.Object) error) (bool, error) {
	controllerRef := metav1.GetControllerOf(obj)
	if controllerRef != nil {
		if controllerRef.UID != m.Controller.GetUID() {
			// Owned by someone else. Ignore.
			return false, nil
		}
		if match(obj) {
			// We already own it and the selector matches.
			// However, if the ownerReference is set for adopting the statefulset, druid
			// needs to inject the annotations and remove the ownerReference.
			// Return true (successfully claimed) before checking deletion timestamp.
			if err := adopt(ctx, obj); err != nil {
				// If the object no longer exists, ignore the error.
				if errors.IsNotFound(err) {
					return false, nil
				}

				// Either someone else claimed it first, or there was a transient error.
				// The controller should requeue and try again if it's still orphaned.
				return false, err
			}
			return true, nil
		}
		// Owned by us but selector doesn't match.
		// Try to release, unless we're being deleted.
		if m.Controller.GetDeletionTimestamp() != nil {
			return false, nil
		}
		if err := release(ctx, obj); err != nil {
			// If the object no longer exists, ignore the error.
			if errors.IsNotFound(err) {
				return false, nil
			}
			// Either someone else released it, or there was a transient error.
			// The controller should requeue and try again if it's still stale.
			return false, err
		}
		// Successfully released.
		return false, nil
	}

	// It's an orphan.
	if m.Controller.GetDeletionTimestamp() != nil || !match(obj) {
		// Ignore if we're being deleted or selector doesn't match.
		return false, nil
	}

	if obj.GetDeletionTimestamp() != nil {
		// Ignore if the object is being deleted
		return false, nil
	}

	// OwnerReference is not set here. Adopt the resource by adding
	// annotations if the annotations are not present. Resource other
	// than sts will have ownerReference set as well.
	if !checkEtcdAnnotations(obj.GetAnnotations(), m.Controller) {
		// Selector matches. Try to adopt.
		if err := adopt(ctx, obj); err != nil {
			// If the object no longer exists, ignore the error.
			if errors.IsNotFound(err) {
				return false, nil
			}

			// Either someone else claimed it first, or there was a transient error.
			// The controller should requeue and try again if it's still orphaned.
			return false, err
		}
	}
	// Successfully adopted.
	return true, nil
}

// EtcdDruidRefManager is the struct used to manage its child objects
type EtcdDruidRefManager struct {
	BaseControllerRefManager
	controllerKind schema.GroupVersionKind
	cl             client.Client
	scheme         *runtime.Scheme
}

// NewEtcdDruidRefManager returns a EtcdDruidRefManager that exposes
// methods to manage the controllerRef of its child objects.
//
// The CanAdopt() function can be used to perform a potentially expensive check
// (such as a live GET from the API server) prior to the first adoption.
// It will only be called (at most once) if an adoption is actually attempted.
// If CanAdopt() returns a non-nil error, all adoptions will fail.
func NewEtcdDruidRefManager(
	cl client.Client,
	scheme *runtime.Scheme,
	controller metav1.Object,
	selector labels.Selector,
	controllerKind schema.GroupVersionKind,
	canAdopt func() error,
) *EtcdDruidRefManager {
	return &EtcdDruidRefManager{
		BaseControllerRefManager: BaseControllerRefManager{
			Controller:   controller,
			Selector:     selector,
			CanAdoptFunc: canAdopt,
		},
		controllerKind: controllerKind,
		cl:             cl,
		scheme:         scheme,
	}
}

// FetchStatefulSet fetches statefulset based on ETCD resource
func (m *EtcdDruidRefManager) FetchStatefulSet(ctx context.Context, etcd *druidv1alpha1.Etcd) (*appsv1.StatefulSetList, error) {
	selector, err := metav1.LabelSelectorAsSelector(etcd.Spec.Selector)
	if err != nil {
		return nil, err
	}

	// list all statefulsets to include the statefulsets that don't match the etcd`s selector
	// anymore but has the stale controller ref.
	statefulSets := &appsv1.StatefulSetList{}
	err = m.cl.List(ctx, statefulSets, client.InNamespace(etcd.Namespace), client.MatchingLabelsSelector{Selector: selector})
	if err != nil {
		return nil, err
	}

	return statefulSets, err
}

// AdoptResource sends a patch to take control of the Etcd. It returns the error if
// the patching fails.
func (m *EtcdDruidRefManager) AdoptResource(ctx context.Context, obj client.Object) error {
	if err := m.CanAdopt(); err != nil {
		return fmt.Errorf("can't adopt resource %v/%v (%v): %v", obj.GetNamespace(), obj.GetName(), obj.GetUID(), err)
	}

	var clone client.Object
	switch objType := obj.(type) {
	case *appsv1.StatefulSet:
		clone = obj.(*appsv1.StatefulSet).DeepCopy()
		m.disown(clone)
		annotations := clone.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		objectKey, err := cache.MetaNamespaceKeyFunc(m.Controller)
		if err != nil {
			return err
		}

		annotations[common.GardenerOwnedBy] = objectKey
		annotations[common.GardenerOwnerType] = strings.ToLower(etcdGVK.Kind)
		clone.SetAnnotations(annotations)
	case *batchv1.Job:
		clone = obj.(*batchv1.Job).DeepCopy()
		// Note that ValidateOwnerReferences() will reject this patch if another
		// OwnerReference exists with controller=true.
		if err := controllerutil.SetControllerReference(m.Controller, clone, m.scheme); err != nil {
			return err
		}
	case *corev1.ServiceAccount:
		clone = obj.(*corev1.ServiceAccount).DeepCopy()
		// Note that ValidateOwnerReferences() will reject this patch if another
		// OwnerReference exists with controller=true.
		if err := controllerutil.SetControllerReference(m.Controller, clone, m.scheme); err != nil {
			return err
		}
	case *rbacv1.Role:
		clone = obj.(*rbacv1.Role).DeepCopy()
		// Note that ValidateOwnerReferences() will reject this patch if another
		// OwnerReference exists with controller=true.
		if err := controllerutil.SetControllerReference(m.Controller, clone, m.scheme); err != nil {
			return err
		}
	case *rbacv1.RoleBinding:
		clone = obj.(*rbacv1.RoleBinding).DeepCopy()
		// Note that ValidateOwnerReferences() will reject this patch if another
		// OwnerReference exists with controller=true.
		if err := controllerutil.SetControllerReference(m.Controller, clone, m.scheme); err != nil {
			return err
		}
	case *policyv1beta1.PodDisruptionBudget:
		clone = obj.(*policyv1beta1.PodDisruptionBudget).DeepCopy()
		// Note that ValidateOwnerReferences() will reject this patch if another
		// OwnerReference exists with controller=true.
		if err := controllerutil.SetControllerReference(m.Controller, clone, m.scheme); err != nil {
			return err
		}
	default:
		return fmt.Errorf("cannot adopt resource: %s", objType)
	}

	return m.cl.Patch(ctx, clone, client.MergeFrom(obj))
}

// ReleaseResource sends a patch to free the resource from the control of the controller.
// It returns the error if the patching fails. 404 and 422 errors are ignored.
func (m *EtcdDruidRefManager) ReleaseResource(ctx context.Context, obj client.Object) error {
	var clone client.Object
	switch objType := obj.(type) {
	case *appsv1.StatefulSet:
		clone = obj.(*appsv1.StatefulSet).DeepCopy()
		// The statefulset is annotated with reference to the owner.
		// This is to ensure compatibility with VPA which does not
		// recommend if statefulset has an owner reference set.
		delete(clone.GetAnnotations(), common.GardenerOwnedBy)
		delete(clone.GetAnnotations(), common.GardenerOwnerType)
	case *policyv1beta1.PodDisruptionBudget:
		clone = obj.(*policyv1beta1.PodDisruptionBudget).DeepCopy()
	default:
		return fmt.Errorf("cannot release resource: %s", objType)
	}

	m.disown(clone)

	err := client.IgnoreNotFound(m.cl.Patch(ctx, clone, client.MergeFrom(obj)))
	if errors.IsInvalid(err) {
		// Invalid error will be returned in two cases: 1. the etcd
		// has no owner reference, 2. the uid of the etcd doesn't
		// match, which means the etcd is deleted and then recreated.
		// In both cases, the error can be ignored.

		// TODO: If the etcd has owner references, but none of them
		// has the owner.UID, server will silently ignore the patch.
		// Investigate why.
		return nil
	}

	return err
}

func (m *EtcdDruidRefManager) disown(obj metav1.Object) {
	owners := obj.GetOwnerReferences()
	ownersCopy := []metav1.OwnerReference{}
	for i := range owners {
		owner := &owners[i]
		if owner.UID == m.Controller.GetUID() {
			continue
		}
		ownersCopy = append(ownersCopy, *owner)
	}
	if len(ownersCopy) == 0 {
		obj.SetOwnerReferences(nil)
	} else {
		obj.SetOwnerReferences(ownersCopy)
	}
}

// RecheckDeletionTimestamp returns a CanAdopt() function to recheck deletion.
//
// The CanAdopt() function calls getObject() to fetch the latest value,
// and denies adoption attempts if that object has a non-nil DeletionTimestamp.
func RecheckDeletionTimestamp(getObject func() (metav1.Object, error)) func() error {
	return func() error {
		obj, err := getObject()
		if err != nil {
			return fmt.Errorf("can't recheck DeletionTimestamp: %v", err)
		}
		if obj.GetDeletionTimestamp() != nil {
			return fmt.Errorf("%v/%v has just been deleted at %v", obj.GetNamespace(), obj.GetName(), obj.GetDeletionTimestamp())
		}
		return nil
	}
}
