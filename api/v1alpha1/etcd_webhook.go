// Copyright (c) 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1alpha1

import (
	"fmt"
	"time"

	"github.com/robfig/cron"

	"github.com/gardener/etcd-druid/pkg/common"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	"github.com/hashicorp/go-multierror"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var etcdlog = logf.Log.WithName("etcd-resource")

// SetupWebhookWithManager registers the webhook for etcd resource with manager.
func (r *Etcd) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-druid-gardener-cloud-v1alpha1-etcd,mutating=true,failurePolicy=fail,groups=druid.gardener.cloud,resources=etcds,verbs=create;update,versions=v1alpha1,name=metcd.kb.io

var _ webhook.Defaulter = &Etcd{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Etcd) Default() {
	etcdlog.Info("default", "name", r.Name)

	if r.Spec.Replicas == nil {
		r.Spec.Replicas = new(uint32)
		*r.Spec.Replicas = common.DefaultEtcdClusterSize
	}

	if r.Spec.Etcd.ClientPort == nil {
		r.Spec.Etcd.ClientPort = new(int32)
		*r.Spec.Etcd.ClientPort = common.DefaultEtcdClientPort
	}
	if r.Spec.Etcd.ServerPort == nil {
		r.Spec.Etcd.ServerPort = new(int32)
		*r.Spec.Etcd.ServerPort = common.DefaultEtcdServerPort
	}

	// TODO: allow only supported images
	if r.Spec.Etcd.Image == nil || r.Spec.Backup.Image == nil {
		var result error
		var images map[string]*imagevector.Image

		imageNames := []string{
			common.Etcd,
			common.BackupRestore,
		}

		iv, err := imagevector.ReadGlobalImageVectorWithEnvOverride(common.DefaultImageVector)
		if err != nil {
			// Since image.yaml is part of binary. This code path won't execute.
			result = multierror.Append(result, err)
		} else {
			images, err = imagevector.FindImages(iv, imageNames)
			if err != nil {
				// Since image.yaml is part of binary. This code path won't execute.
				result = multierror.Append(result, err)
			}
		}

		if r.Spec.Etcd.Image == nil {
			val, ok := images[common.Etcd]
			if !ok {
				err = fmt.Errorf("either etcd resource or image vector should have %s image", common.Etcd)
				result = multierror.Append(result, err)
			} else {
				etcdImage := val.String()
				r.Spec.Etcd.Image = &etcdImage
			}
		}

		if r.Spec.Backup.Image == nil {
			val, ok := images[common.BackupRestore]
			if !ok {
				err := fmt.Errorf("either etcd resource or image vector should have %s image", common.Etcd)
				result = multierror.Append(result, err)
			} else {
				backupImage := val.String()
				r.Spec.Backup.Image = &backupImage
			}
		}
		if result != nil {
			// Since image.yaml is part of binary. This code path won't execute.
			panic(result)
		}
	}

	if r.Spec.VolumeClaimTemplate == nil {
		r.Spec.VolumeClaimTemplate = new(string)
		*r.Spec.VolumeClaimTemplate = r.Name
	}
	if r.Spec.StorageCapacity == nil {
		r.Spec.StorageCapacity = resource.NewScaledQuantity(20, resource.Giga)
	}

	// TODO: should we(is it feasible to) import etcd-backup-restore for such defaults?
	if r.Spec.Etcd.DefragmentationSchedule == nil {
		defaultDefragmentationSchedule := "0 0 */3 * *" //Per 3 day
		r.Spec.Etcd.DefragmentationSchedule = &defaultDefragmentationSchedule
	}
	r.defaultBackupSpec()

}

func (r *Etcd) defaultBackupSpec() {
	if r.Spec.Backup.DeltaSnapshotMemoryLimit == nil {
		r.Spec.Backup.DeltaSnapshotMemoryLimit = resource.NewScaledQuantity(100, resource.Mega)
	}
	if r.Spec.Backup.DeltaSnapshotPeriod == nil {
		r.Spec.Backup.DeltaSnapshotPeriod = &metav1.Duration{Duration: time.Minute * 5} //5m
	}

	if r.Spec.Backup.FullSnapshotSchedule == nil {
		defaultFullSnapshotSchedule := "0 0 * * *" //Per day
		r.Spec.Backup.FullSnapshotSchedule = &defaultFullSnapshotSchedule
	}

	if r.Spec.Backup.GarbageCollectionPeriod == nil {
		r.Spec.Backup.GarbageCollectionPeriod = &metav1.Duration{Duration: time.Hour * 12} //12hr
	}
	if r.Spec.Backup.GarbageCollectionPolicy == nil {
		r.Spec.Backup.GarbageCollectionPolicy = new(GarbageCollectionPolicy)
		*r.Spec.Backup.GarbageCollectionPolicy = GarbageCollectionPolicyExponential
	}

	if r.Spec.Backup.Port == nil {
		r.Spec.Backup.Port = new(int32)
		*r.Spec.Backup.Port = common.DefaultBackupServerPort
	}
}

// +kubebuilder:webhook:path=/validate-druid-gardener-cloud-v1alpha1-etcd,mutating=false,failurePolicy=fail,groups=druid.gardener.cloud,resources=etcds,verbs=create;update,versions=v1alpha1,name=vetcd.kb.io

var _ webhook.Validator = &Etcd{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Etcd) ValidateCreate() error {
	etcdlog.Info("validate create", "name", r.Name)

	return r.validateEtcd()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Etcd) ValidateUpdate(old runtime.Object) error {
	etcdlog.Info("validate update", "name", r.Name)

	var result error

	if err := r.ValidateCreate(); err != nil {
		result = multierror.Append(result, err)
	}

	oldEtcd, ok := old.(*Etcd)
	if !ok {
		result = multierror.Append(result, fmt.Errorf("invalid old object type"))
	} else {

		if err := r.validateEtcdUpdate(oldEtcd); err != nil {
			result = multierror.Append(result, err)
		}
	}

	return result
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Etcd) ValidateDelete() error {
	etcdlog.Info("validate delete", "name", r.Name)

	return nil
}

func (r *Etcd) validateEtcd() error {
	var allErrs field.ErrorList

	allErrs = append(allErrs, validation.ValidateObjectMeta(&r.ObjectMeta, true, validation.NameIsDNSSubdomain, field.NewPath("metadata"))...)
	allErrs = append(allErrs, r.validateEtcdSpec(field.NewPath("spec"))...)

	if len(allErrs) == 0 {
		return nil
	}

	return errors.NewInvalid(schema.GroupKind{Group: GroupVersion.Group, Kind: "Etcd"}, r.Name, allErrs)
}

func (r *Etcd) validateEtcdUpdate(old *Etcd) error {
	var allErrs field.ErrorList

	allErrs = append(allErrs, validation.ValidateImmutableField(r.Spec.Backup.Store, r.Spec.Backup.Store, field.NewPath("spec", "backup", "store"))...)
	if len(allErrs) == 0 {
		return nil
	}

	return errors.NewInvalid(schema.GroupKind{Group: GroupVersion.Group, Kind: "Etcd"}, r.Name, allErrs)
}

func (r *Etcd) validateEtcdSpec(fldPath *field.Path) field.ErrorList {
	// The field helpers from the kubernetes API machinery help us return nicely
	// structured validation errors.
	var allErrs field.ErrorList

	allErrs = append(allErrs, validateScheduleFormat(r.Spec.Etcd.DefragmentationSchedule, fldPath.Child("defragmentationSchedule")))
	allErrs = append(allErrs, validatePort(r.Spec.Etcd.ServerPort, fldPath.Child("etcd", "serverPort")))
	allErrs = append(allErrs, validatePort(r.Spec.Etcd.ClientPort, fldPath.Child("etcd", "clientPort")))
	allErrs = append(allErrs, validatePort(r.Spec.Backup.Port, fldPath.Child("backup", "port")))

	if r.Spec.Replicas != nil && *r.Spec.Replicas > 1 {
		// TODO : with multi-node support this validation will be removed
		allErrs = append(allErrs, field.NotSupported(fldPath.Child("replica"), *r.Spec.Replicas, []string{"0", "1"}))
	}

	if r.Spec.Backup.Store != nil {
		if r.Spec.Backup.Store.Provider != nil && len(*r.Spec.Backup.Store.Provider) > 0 {

			supportedProviderList := []string{"aws", "azure", "gcp", "alicloud", "openstack", "Local", "S3", "ABS", "GCS", "OSS", "Swift"}
			found := false
			for _, val := range supportedProviderList {
				if val == string(*r.Spec.Backup.Store.Provider) {
					found = true
					break
				}
			}
			if found {
				allErrs = append(allErrs, field.NotSupported(fldPath.Child("backup", "store", "provider"), *r.Spec.Backup.Store.Provider, supportedProviderList))
			}

			if *r.Spec.Backup.Store.Provider != "Local" && r.Spec.Backup.Store.SecretRef == nil {
				allErrs = append(allErrs, field.Forbidden(fldPath.Child("backup", "store", "secretRef"), fmt.Sprintf("must be provided for storage provider %s", *r.Spec.Backup.Store.Provider)))
			}

			allErrs = append(allErrs, validateScheduleFormat(r.Spec.Backup.FullSnapshotSchedule, fldPath.Child("backup", "fullSnapshotSchedule")))
		}
	}

	return allErrs
}

func validateScheduleFormat(schedule *string, fldPath *field.Path) *field.Error {
	if schedule != nil && len(*schedule) != 0 {
		if _, err := cron.ParseStandard(*schedule); err != nil {
			return field.Invalid(fldPath, schedule, err.Error())
		}
	}
	return nil
}

func validatePort(port *int32, fldPath *field.Path) *field.Error {
	if port != nil && (*port <= 0 || *port >= 65536) {
		return &field.Error{
			Type:     field.ErrorTypeNotSupported,
			Field:    fldPath.String(),
			BadValue: *port,
			Detail:   "supported values: (0,65536)",
		}
	}

	return nil
}
