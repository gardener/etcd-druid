// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"strings"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apivalidation "k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// ValidateEtcd validates a Etcd object.
func ValidateEtcd(etcd *druidv1alpha1.Etcd) field.ErrorList {
	var allErrs field.ErrorList

	allErrs = append(allErrs, apivalidation.ValidateObjectMeta(&etcd.ObjectMeta, true, apivalidation.NameIsDNSSubdomain, field.NewPath("metadata"))...)
	allErrs = append(allErrs, ValidateEtcdSpec(&etcd.Spec, etcd.Name, etcd.Namespace, field.NewPath("spec"))...)

	return allErrs
}

// ValidateEtcdUpdate validates a Etcd object before an update.
func ValidateEtcdUpdate(new, old *druidv1alpha1.Etcd) field.ErrorList {
	var allErrs field.ErrorList

	allErrs = append(allErrs, apivalidation.ValidateObjectMetaUpdate(&new.ObjectMeta, &old.ObjectMeta, field.NewPath("metadata"))...)
	allErrs = append(allErrs, ValidateEtcdSpecUpdate(&new.Spec, &old.Spec, new.DeletionTimestamp != nil, field.NewPath("spec"))...)
	allErrs = append(allErrs, ValidateEtcd(new)...)

	return allErrs
}

// ValidateEtcdSpec validates the specification of an Etcd object.
func ValidateEtcdSpec(spec *druidv1alpha1.EtcdSpec, name, namespace string, path *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if spec.Backup.Store != nil {
		allErrs = append(allErrs, validateStore(spec.Backup.Store, name, namespace, path.Child("backup.store"))...)
	}

	return allErrs
}

// ValidateEtcdSpecUpdate validates the specification of an Etcd object before an update.
func ValidateEtcdSpecUpdate(new, old *druidv1alpha1.EtcdSpec, deletionTimestampSet bool, path *field.Path) field.ErrorList {
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

func validateStore(store *druidv1alpha1.StoreSpec, name, namespace string, path *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if !(strings.Contains(store.Prefix, namespace) && strings.Contains(store.Prefix, name)) {
		allErrs = append(allErrs, field.Invalid(path.Child("prefix"), store.Prefix, "must contain object name and namespace"))
	}

	if store.Provider != nil && *store.Provider != "" {
		if _, err := storageProviderFromInfraProvider(store.Provider); err != nil {
			allErrs = append(allErrs, field.Invalid(path.Child("provider"), store.Provider, err.Error()))
		}
	}

	return allErrs
}

func validateStoreUpdate(new, old *druidv1alpha1.StoreSpec, path *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	allErrs = append(allErrs, apivalidation.ValidateImmutableField(new.Prefix, old.Prefix, path.Child("prefix"))...)

	return allErrs
}

// NOTE: Constants and storageProviderFromInfraProvider has been duplicated from etcd-druid/internal/store/store.go
// Once the gardener adopts to using CEL expressions then the entire validation package will be removed.
// Having a dependency to etcd-druid/internal within the API package is incorrect and should never be allowed.
// Therefore, let this code be here for a while till the entire validation package itself gets removed.
// --------------------------------------------------------------------------------------------------------------
const (
	aws       = "aws"
	azure     = "azure"
	gcp       = "gcp"
	gdch      = "gdch"
	alicloud  = "alicloud"
	openstack = "openstack"
	dell      = "dell"
	openshift = "openshift"
	stackit   = "stackit"
	// s3 is a constant for the AWS and s3 compliant storage provider.
	s3 = "S3"
	// abs is a constant for the Azure storage provider.
	abs = "ABS"
	// gcs is a constant for the Google storage provider.
	gcs = "GCS"
	// oss is a constant for the Alicloud storage provider.
	oss = "OSS"
	// swift is a constant for the OpenStack storage provider.
	swift = "Swift"
	// local is a constant for the Local storage provider.
	local = "Local"
	// ecs is a constant for the EMC storage provider.
	ecs = "ECS"
	// ocs is a constant for the OpenShift storage provider.
	ocs = "OCS"
)

// storageProviderFromInfraProvider converts infra to object store provider.
func storageProviderFromInfraProvider(infra *druidv1alpha1.StorageProvider) (string, error) {
	if infra == nil {
		return "", nil
	}

	switch *infra {
	case aws, s3:
		return s3, nil
	// s3-compatible providers
	case stackit:
		return s3, nil
	case azure, abs:
		return abs, nil
	case alicloud, oss:
		return oss, nil
	case openstack, swift:
		return swift, nil
	case gcp, gcs:
		return gcs, nil
	case gdch:
		return s3, nil
	case dell, ecs:
		return ecs, nil
	case openshift, ocs:
		return ocs, nil
	case local, druidv1alpha1.StorageProvider(strings.ToLower(local)):
		return local, nil
	default:
		return "", fmt.Errorf("unsupported storage provider: '%v'", *infra)
	}
}

// --------------------------------------------------------------------------------------------------------------
