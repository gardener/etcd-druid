// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Package v1alpha1 contains API Schema definitions for the druid v1alpha1 API group
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GroupName is the name of the druid API group.
const GroupName = "druid.gardener.cloud"

// nolint:gochecknoglobals
var (
	// SchemeGroupVersion is group version used to register objects from the etcd-druid API.
	SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: "v1alpha1"}
	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	SchemeBuilder      = runtime.NewSchemeBuilder(addKnownTypes)
	localSchemeBuilder = &SchemeBuilder
	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = localSchemeBuilder.AddToScheme
)

// Kind takes an unqualified kind and returns a Group qualified GroupKind.
func Kind(kind string) schema.GroupKind {
	return SchemeGroupVersion.WithKind(kind).GroupKind()
}

// Resource takes an unqualified resource and returns a Group qualified GroupResource.
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

// Adds the list of known types to the given scheme.
func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&Etcd{},
		&EtcdList{},
		&EtcdCopyBackupsTask{},
		&EtcdCopyBackupsTaskList{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}
