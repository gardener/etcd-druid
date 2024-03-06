// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Package v1alpha1 contains API Schema definitions for the druid v1alpha1 API group
// +kubebuilder:object:generate=true
// +groupName=druid.gardener.cloud
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// nolint:gochecknoglobals
var (
	localSchemeBuilder = &SchemeBuilder
	// GroupVersion is group version used to register these objects
	GroupVersion = schema.GroupVersion{Group: "druid.gardener.cloud", Version: "v1alpha1"}
	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = localSchemeBuilder.AddToScheme
)

// Adds the list of known types to the given scheme.
func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion,
		&Etcd{},
		&EtcdList{},
		&EtcdCopyBackupsTask{},
		&EtcdCopyBackupsTaskList{},
	)
	metav1.AddToGroupVersion(scheme, GroupVersion)

	return nil
}
