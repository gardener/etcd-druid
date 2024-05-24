// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"strings"

	"github.com/gardener/etcd-druid/api/v1alpha1"
	druidstore "github.com/gardener/etcd-druid/internal/store"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apivalidation "k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// ValidateEtcd validates a Etcd object.
func ValidateEtcd(etcd *v1alpha1.Etcd) field.ErrorList {
	var allErrs field.ErrorList

	allErrs = append(allErrs, apivalidation.ValidateObjectMeta(&etcd.ObjectMeta, true, apivalidation.NameIsDNSSubdomain, field.NewPath("metadata"))...)
	allErrs = append(allErrs, ValidateEtcdSpec(&etcd.Spec, etcd.Name, etcd.Namespace, field.NewPath("spec"))...)

	return allErrs
}

// ValidateEtcdUpdate validates a Etcd object before an update.
func ValidateEtcdUpdate(new, old *v1alpha1.Etcd) field.ErrorList {
	var allErrs field.ErrorList

	allErrs = append(allErrs, apivalidation.ValidateObjectMetaUpdate(&new.ObjectMeta, &old.ObjectMeta, field.NewPath("metadata"))...)
	allErrs = append(allErrs, ValidateEtcdSpecUpdate(&new.Spec, &old.Spec, new.DeletionTimestamp != nil, field.NewPath("spec"))...)
	allErrs = append(allErrs, ValidateEtcd(new)...)

	return allErrs
}

// ValidateEtcdSpec validates the specification of a Etcd object.
func ValidateEtcdSpec(spec *v1alpha1.EtcdSpec, name, namespace string, path *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if spec.Backup.Store != nil {
		allErrs = append(allErrs, validateStore(spec.Backup.Store, name, namespace, path.Child("backup.store"))...)
	}

	return allErrs
}

// ValidateEtcdSpecUpdate validates the specification of a Etcd object before an update.
func ValidateEtcdSpecUpdate(new, old *v1alpha1.EtcdSpec, deletionTimestampSet bool, path *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if deletionTimestampSet && !apiequality.Semantic.DeepEqual(new, old) {
		allErrs = append(allErrs, apivalidation.ValidateImmutableField(new, old, path)...)
		return allErrs
	}

	if new.Backup.Store != nil && old.Backup.Store != nil {
		allErrs = append(allErrs, validateStoreUpdate(new.Backup.Store, old.Backup.Store, path.Child("backup.store"))...)
	}

	return allErrs
}

func validateStore(store *v1alpha1.StoreSpec, name, namespace string, path *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if !(strings.Contains(store.Prefix, namespace) && strings.Contains(store.Prefix, name)) {
		allErrs = append(allErrs, field.Invalid(path.Child("prefix"), store.Prefix, "must contain object name and namespace"))
	}

	if store.Provider != nil && *store.Provider != "" {
		if _, err := druidstore.StorageProviderFromInfraProvider(store.Provider); err != nil {
			allErrs = append(allErrs, field.Invalid(path.Child("provider"), store.Provider, err.Error()))
		}
	}

	return allErrs
}

func validateStoreUpdate(new, old *v1alpha1.StoreSpec, path *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	allErrs = append(allErrs, apivalidation.ValidateImmutableField(new.Prefix, old.Prefix, path.Child("prefix"))...)

	return allErrs
}
