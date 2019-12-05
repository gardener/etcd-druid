/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

This file was copied and modified from the kubernetes/kubernetes project
https://github.com/kubernetes/kubernetes/release-1.8/pkg/controller/controller_ref_manager.go

Modifications Copyright (c) 2017 SAP SE or an SAP affiliate company. All rights reserved.
*/

// Package controllers is used to provide the core functionalities of hvpa-controller
package controllers

import (
	"context"
	"fmt"
	"sync"

	//"github.com/gardener/hvpa-controller/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
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

// ClaimObject tries to take ownership of an object for this controller.
//
// It will reconcile the following:
//   * Adopt orphans if the match function returns true.
//   * Release owned objects if the match function returns false.
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
func (m *BaseControllerRefManager) ClaimObject(obj metav1.Object, match func(metav1.Object) bool, adopt, release func(metav1.Object) error) (bool, error) {

	controllerRef := metav1.GetControllerOf(obj)
	if controllerRef != nil {
		if controllerRef.UID != m.Controller.GetUID() {
			// Owned by someone else. Ignore.
			return false, nil
		}
		if match(obj) {
			// We already own it and the selector matches.
			// Return true (successfully claimed) before checking deletion timestamp.
			// We're still allowed to claim things we already own while being deleted
			// because doing so requires taking no actions.
			return true, nil
		}
		// Owned by us but selector doesn't match.
		// Try to release, unless we're being deleted.
		if m.Controller.GetDeletionTimestamp() != nil {
			return false, nil
		}
		if err := release(obj); err != nil {
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

	// Selector matches. Try to adopt.
	if err := adopt(obj); err != nil {
		// If the object no longer exists, ignore the error.
		if errors.IsNotFound(err) {
			return false, nil
		}

		// Either someone else claimed it first, or there was a transient error.
		// The controller should requeue and try again if it's still orphaned.
		return false, err
	}
	// Successfully adopted.
	return true, nil
}

// EtcdDruidRefManager is the struct used to manage its child objects
type EtcdDruidRefManager struct {
	BaseControllerRefManager
	controllerKind schema.GroupVersionKind
	reconciler     *EtcdReconciler
}

// NewEtcdDruidRefManager returns a EtcdDruidRefManager that exposes
// methods to manage the controllerRef of its child objects.
//
// The CanAdopt() function can be used to perform a potentially expensive check
// (such as a live GET from the API server) prior to the first adoption.
// It will only be called (at most once) if an adoption is actually attempted.
// If CanAdopt() returns a non-nil error, all adoptions will fail.
func NewEtcdDruidRefManager(
	reconciler *EtcdReconciler,
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
		reconciler:     reconciler,
	}
}

// ClaimStatefulsets tries to take ownership of a list of Statefulsets.
//
// It will reconcile the following:
//   * Adopt orphans if the selector matches.
//   * Release owned objects if the selector no longer matches.
//
// Optional: If one or more filters are specified, a Statefulset will only be claimed if
// all filters return true.
//
// A non-nil error is returned if some form of reconciliation was attempted and
// failed. Usually, controllers should try again later in case reconciliation
// is still needed.
//
// If the error is nil, either the reconciliation succeeded, or no
// reconciliation was necessary. The list of statefulsets that you now own is returned.
func (m *EtcdDruidRefManager) ClaimStatefulsets(etcds *appsv1.StatefulSetList, filters ...func(*appsv1.StatefulSet) bool) ([]*appsv1.StatefulSet, error) {
	var claimed []*appsv1.StatefulSet
	var errlist []error

	match := func(obj metav1.Object) bool {
		ss := obj.(*appsv1.StatefulSet)
		// Check selector first so filters only run on potentially matching statefulsets.
		if !m.Selector.Matches(labels.Set(ss.Labels)) {
			return false
		}
		for _, filter := range filters {
			if !filter(ss) {
				return false
			}
		}
		return true
	}

	adopt := func(obj metav1.Object) error {
		return m.AdoptStatefulSet(obj.(*appsv1.StatefulSet))
	}
	release := func(obj metav1.Object) error {
		return m.ReleaseStatefulSet(obj.(*appsv1.StatefulSet))
	}

	for k := range etcds.Items {
		etcd := &etcds.Items[k]
		ok, err := m.ClaimObject(etcd, match, adopt, release)

		if err != nil {
			errlist = append(errlist, err)
			continue
		}
		if ok {
			claimed = append(claimed, etcd)
		}
	}
	return claimed, utilerrors.NewAggregate(errlist)
}

// AdoptStatefulSet sends a patch to take control of the Etcd. It returns the error if
// the patching fails.
func (m *EtcdDruidRefManager) AdoptStatefulSet(etcd *appsv1.StatefulSet) error {
	if err := m.CanAdopt(); err != nil {
		return fmt.Errorf("can't adopt statefulset %v/%v (%v): %v", etcd.Namespace, etcd.Name, etcd.UID, err)
	}

	etcdClone := etcd.DeepCopy()
	// Note that ValidateOwnerReferences() will reject this patch if another
	// OwnerReference exists with controller=true.
	if err := controllerutil.SetControllerReference(m.Controller, etcdClone, m.reconciler.Scheme); err != nil {
		return err
	}

	return m.reconciler.Patch(context.TODO(), etcdClone, client.MergeFrom(etcd))
}

// ReleaseStatefulSet sends a patch to free the statefulset from the control of the controller.
// It returns the error if the patching fails. 404 and 422 errors are ignored.
func (m *EtcdDruidRefManager) ReleaseStatefulSet(etcd *appsv1.StatefulSet) error {

	etcdClone := etcd.DeepCopy()
	owners := etcdClone.GetOwnerReferences()
	ownersCopy := []metav1.OwnerReference{}
	for i := range owners {
		owner := &owners[i]
		if owner.UID == m.Controller.GetUID() {
			continue
		}
		ownersCopy = append(ownersCopy, *owner)
	}
	if len(ownersCopy) == 0 {
		etcdClone.OwnerReferences = nil
	} else {
		etcdClone.OwnerReferences = ownersCopy
	}

	err := client.IgnoreNotFound(m.reconciler.Patch(context.TODO(), etcdClone, client.MergeFrom(etcd)))
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

// ClaimServices tries to take ownership of a list of Services.
//
// It will reconcile the following:
//   * Adopt orphans if the selector matches.
//   * Release owned objects if the selector no longer matches.
//
// Optional: If one or more filters are specified, a Service will only be claimed if
// all filters return true.
//
// A non-nil error is returned if some form of reconciliation was attempted and
// failed. Usually, controllers should try again later in case reconciliation
// is still needed.
//
// If the error is nil, either the reconciliation succeeded, or no
// reconciliation was necessary. The list of Services that you now own is returned.
func (m *EtcdDruidRefManager) ClaimServices(etcds *corev1.ServiceList, filters ...func(*corev1.Service) bool) ([]*corev1.Service, error) {
	var claimed []*corev1.Service
	var errlist []error

	match := func(obj metav1.Object) bool {
		ss := obj.(*corev1.Service)
		// Check selector first so filters only run on potentially matching services.
		if !m.Selector.Matches(labels.Set(ss.Labels)) {
			return false
		}
		for _, filter := range filters {
			if !filter(ss) {
				return false
			}
		}
		return true
	}

	adopt := func(obj metav1.Object) error {
		return m.AdoptService(obj.(*corev1.Service))
	}
	release := func(obj metav1.Object) error {
		return m.ReleaseService(obj.(*corev1.Service))
	}

	for k := range etcds.Items {
		etcd := &etcds.Items[k]
		ok, err := m.ClaimObject(etcd, match, adopt, release)

		if err != nil {
			errlist = append(errlist, err)
			continue
		}
		if ok {
			claimed = append(claimed, etcd)
		}
	}
	return claimed, utilerrors.NewAggregate(errlist)
}

// AdoptService sends a patch to take control of the Etcd. It returns the error if
// the patching fails.
func (m *EtcdDruidRefManager) AdoptService(etcd *corev1.Service) error {
	if err := m.CanAdopt(); err != nil {
		return fmt.Errorf("can't adopt services %v/%v (%v): %v", etcd.Namespace, etcd.Name, etcd.UID, err)
	}

	etcdClone := etcd.DeepCopy()
	// Note that ValidateOwnerReferences() will reject this patch if another
	// OwnerReference exists with controller=true.
	if err := controllerutil.SetControllerReference(m.Controller, etcdClone, m.reconciler.Scheme); err != nil {
		return err
	}

	return m.reconciler.Patch(context.TODO(), etcdClone, client.MergeFrom(etcd))
}

// ReleaseService sends a patch to free the service from the control of the controller.
// It returns the error if the patching fails. 404 and 422 errors are ignored.
func (m *EtcdDruidRefManager) ReleaseService(etcd *corev1.Service) error {

	etcdClone := etcd.DeepCopy()
	owners := etcdClone.GetOwnerReferences()
	ownersCopy := []metav1.OwnerReference{}
	for i := range owners {
		owner := &owners[i]
		if owner.UID == m.Controller.GetUID() {
			continue
		}
		ownersCopy = append(ownersCopy, *owner)
	}
	if len(ownersCopy) == 0 {
		etcdClone.OwnerReferences = nil
	} else {
		etcdClone.OwnerReferences = ownersCopy
	}

	err := client.IgnoreNotFound(m.reconciler.Patch(context.TODO(), etcdClone, client.MergeFrom(etcd)))
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

// ClaimConfigMaps tries to take ownership of a list of ConfigMaps.
//
// It will reconcile the following:
//   * Adopt orphans if the selector matches.
//   * Release owned objects if the selector no longer matches.
//
// Optional: If one or more filters are specified, a Service will only be claimed if
// all filters return true.
//
// A non-nil error is returned if some form of reconciliation was attempted and
// failed. Usually, controllers should try again later in case reconciliation
// is still needed.
//
// If the error is nil, either the reconciliation succeeded, or no
// reconciliation was necessary. The list of Services that you now own is returned.
func (m *EtcdDruidRefManager) ClaimConfigMaps(etcds *corev1.ConfigMapList, filters ...func(*corev1.ConfigMap) bool) ([]*corev1.ConfigMap, error) {
	var claimed []*corev1.ConfigMap
	var errlist []error

	match := func(obj metav1.Object) bool {
		ss := obj.(*corev1.ConfigMap)
		// Check selector first so filters only run on potentially matching configmaps.
		if !m.Selector.Matches(labels.Set(ss.Labels)) {
			return false
		}
		for _, filter := range filters {
			if !filter(ss) {
				return false
			}
		}
		return true
	}

	adopt := func(obj metav1.Object) error {
		return m.AdoptConfigMap(obj.(*corev1.ConfigMap))
	}
	release := func(obj metav1.Object) error {
		return m.ReleaseConfigMap(obj.(*corev1.ConfigMap))
	}

	for k := range etcds.Items {
		etcd := &etcds.Items[k]
		ok, err := m.ClaimObject(etcd, match, adopt, release)

		if err != nil {
			errlist = append(errlist, err)
			continue
		}
		if ok {
			claimed = append(claimed, etcd)
		}
	}
	return claimed, utilerrors.NewAggregate(errlist)
}

// AdoptConfigMap sends a patch to take control of the Etcd. It returns the error if
// the patching fails.
func (m *EtcdDruidRefManager) AdoptConfigMap(etcd *corev1.ConfigMap) error {
	if err := m.CanAdopt(); err != nil {
		return fmt.Errorf("can't adopt configmap %v/%v (%v): %v", etcd.Namespace, etcd.Name, etcd.UID, err)
	}

	etcdClone := etcd.DeepCopy()
	// Note that ValidateOwnerReferences() will reject this patch if another
	// OwnerReference exists with controller=true.
	if err := controllerutil.SetControllerReference(m.Controller, etcdClone, m.reconciler.Scheme); err != nil {
		return err
	}

	return m.reconciler.Patch(context.TODO(), etcdClone, client.MergeFrom(etcd))
}

// ReleaseConfigMap sends a patch to free the configmap from the control of the controller.
// It returns the error if the patching fails. 404 and 422 errors are ignored.
func (m *EtcdDruidRefManager) ReleaseConfigMap(etcd *corev1.ConfigMap) error {

	etcdClone := etcd.DeepCopy()
	owners := etcdClone.GetOwnerReferences()
	ownersCopy := []metav1.OwnerReference{}
	for i := range owners {
		owner := &owners[i]
		if owner.UID == m.Controller.GetUID() {
			continue
		}
		ownersCopy = append(ownersCopy, *owner)
	}
	if len(ownersCopy) == 0 {
		etcdClone.OwnerReferences = nil
	} else {
		etcdClone.OwnerReferences = ownersCopy
	}

	err := client.IgnoreNotFound(m.reconciler.Patch(context.TODO(), etcdClone, client.MergeFrom(etcd)))
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
