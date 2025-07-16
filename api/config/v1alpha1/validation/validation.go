// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"strings"

	druidconfigv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
)

// ValidateOperatorConfiguration validates the operator configuration.
func ValidateOperatorConfiguration(config *druidconfigv1alpha1.OperatorConfiguration) field.ErrorList {
	allErrs := field.ErrorList{}

	allErrs = append(allErrs, validateClientConnectionConfiguration(config.ClientConnection, field.NewPath("clientConnection"))...)
	allErrs = append(allErrs, validateLeaderElectionConfiguration(config.LeaderElection, field.NewPath("leaderElection"))...)
	allErrs = append(allErrs, validateControllerConfiguration(config.Controllers, field.NewPath("controllers"))...)
	allErrs = append(allErrs, validateLogConfiguration(config.Logging, field.NewPath("log"))...)
	allErrs = append(allErrs, validateWebhookConfiguration(config.Webhooks, field.NewPath("webhooks"))...)

	return allErrs
}

func validateClientConnectionConfiguration(clientConnConfig druidconfigv1alpha1.ClientConnectionConfiguration, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if clientConnConfig.Burst < 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("burst"), clientConnConfig.Burst, "must be non-negative"))
	}
	return allErrs
}

func validateLeaderElectionConfiguration(leaderElectionConfig druidconfigv1alpha1.LeaderElectionConfiguration, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if !leaderElectionConfig.Enabled {
		return allErrs
	}
	allErrs = append(allErrs, mustBeGreaterThanZeroDuration(leaderElectionConfig.LeaseDuration, fldPath.Child("leaseDuration"))...)
	allErrs = append(allErrs, mustBeGreaterThanZeroDuration(leaderElectionConfig.RenewDeadline, fldPath.Child("renewDeadline"))...)
	allErrs = append(allErrs, mustBeGreaterThanZeroDuration(leaderElectionConfig.RetryPeriod, fldPath.Child("retryPeriod"))...)

	if leaderElectionConfig.LeaseDuration.Duration <= leaderElectionConfig.RenewDeadline.Duration {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("leaseDuration"), leaderElectionConfig.RenewDeadline, "LeaseDuration must be greater than RenewDeadline"))
	}
	if len(leaderElectionConfig.ResourceLock) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("resourceLock"), "resourceLock is required"))
	}
	if len(leaderElectionConfig.ResourceName) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("resourceName"), "resourceName is required"))
	}
	return allErrs
}

func validateLogConfiguration(logConfig druidconfigv1alpha1.LogConfiguration, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if logConfig.LogLevel != "" && !sets.New(druidconfigv1alpha1.AllLogLevels...).Has(logConfig.LogLevel) {
		allErrs = append(allErrs, field.NotSupported(fldPath.Child("logLevel"), logConfig.LogLevel, druidconfigv1alpha1.AllLogLevels))
	}
	if logConfig.LogFormat != "" && !sets.New(druidconfigv1alpha1.AllLogFormats...).Has(logConfig.LogFormat) {
		allErrs = append(allErrs, field.NotSupported(fldPath.Child("logFormat"), logConfig.LogFormat, druidconfigv1alpha1.AllLogFormats))
	}
	return allErrs
}

func validateControllerConfiguration(controllerConfig druidconfigv1alpha1.ControllerConfiguration, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, validateEtcdControllerConfiguration(controllerConfig.Etcd, fldPath.Child("etcd"))...)
	allErrs = append(allErrs, validateSecretControllerConfiguration(controllerConfig.Secret, fldPath.Child("secret"))...)
	allErrs = append(allErrs, validateCompactionControllerConfiguration(controllerConfig.Compaction, fldPath.Child("compaction"))...)
	allErrs = append(allErrs, validateEtcdCopyBackupsTaskControllerConfiguration(controllerConfig.EtcdCopyBackupsTask, fldPath.Child("etcdCopyBackupsTask"))...)
	allErrs = append(allErrs, validateEtcdOpsTaskControllerConfiguration(controllerConfig.EtcdOpsTask, fldPath.Child("etcdOpsTask"))...)
	return allErrs
}

func validateEtcdControllerConfiguration(etcdControllerConfig druidconfigv1alpha1.EtcdControllerConfiguration, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, validateConcurrentSyncs(etcdControllerConfig.ConcurrentSyncs, fldPath.Child("concurrentSyncs"))...)
	allErrs = append(allErrs, mustBeGreaterThanZeroDuration(etcdControllerConfig.EtcdStatusSyncPeriod, fldPath.Child("etcdStatusSyncPeriod"))...)
	allErrs = append(allErrs, mustBeGreaterThanZeroDuration(etcdControllerConfig.EtcdMember.NotReadyThreshold, fldPath.Child("etcdMember", "notReadyThreshold"))...)
	allErrs = append(allErrs, mustBeGreaterThanZeroDuration(etcdControllerConfig.EtcdMember.UnknownThreshold, fldPath.Child("etcdMember", "unknownThreshold"))...)
	return allErrs
}

func validateCompactionControllerConfiguration(compactionControllerConfig druidconfigv1alpha1.CompactionControllerConfiguration, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if !compactionControllerConfig.Enabled {
		return allErrs
	}
	allErrs = append(allErrs, validateConcurrentSyncs(compactionControllerConfig.ConcurrentSyncs, fldPath.Child("concurrentSyncs"))...)
	allErrs = append(allErrs, mustBeGreaterThanZeroDuration(compactionControllerConfig.ActiveDeadlineDuration, fldPath.Child("activeDeadlineDuration"))...)
	allErrs = append(allErrs, mustBeEqualToOrGreaterThanZeroDuration(compactionControllerConfig.MetricsScrapeWaitDuration, fldPath.Child("metricsScrapeWaitDuration"))...)
	if compactionControllerConfig.EventsThreshold <= 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("eventsThreshold"), compactionControllerConfig.EventsThreshold, "must be greater than 0"))
	}
	if compactionControllerConfig.TriggerFullSnapshotThreshold <= 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("triggerFullSnapshotThreshold"), compactionControllerConfig.TriggerFullSnapshotThreshold, "must be greater than 0"))
	}
	return allErrs
}

func validateEtcdCopyBackupsTaskControllerConfiguration(etcdCopyBackupsTaskControllerConfig druidconfigv1alpha1.EtcdCopyBackupsTaskControllerConfiguration, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if !etcdCopyBackupsTaskControllerConfig.Enabled {
		return allErrs
	}
	allErrs = append(allErrs, validateConcurrentSyncs(etcdCopyBackupsTaskControllerConfig.ConcurrentSyncs, fldPath.Child("concurrentSyncs"))...)
	return allErrs
}

func validateEtcdOpsTaskControllerConfiguration(etcdOpsTaskControllerConfig druidconfigv1alpha1.EtcdOpsTaskControllerConfiguration, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if !etcdOpsTaskControllerConfig.Enabled {
		return allErrs
	}
	allErrs = append(allErrs, validateConcurrentSyncs(etcdOpsTaskControllerConfig.ConcurrentSyncs, fldPath.Child("concurrentSyncs"))...)
	allErrs = append(allErrs, mustBeGreaterThanZeroDuration(etcdOpsTaskControllerConfig.RequeueInterval, fldPath.Child("requeueInterval"))...)
	return allErrs
}

func validateSecretControllerConfiguration(secretControllerConfig druidconfigv1alpha1.SecretControllerConfiguration, fldPath *field.Path) field.ErrorList {
	return validateConcurrentSyncs(secretControllerConfig.ConcurrentSyncs, fldPath.Child("concurrentSyncs"))
}

func validateWebhookConfiguration(webhookConfig druidconfigv1alpha1.WebhookConfiguration, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, validateEtcdComponentProtectionWebhookConfiguration(webhookConfig.EtcdComponentProtection, fldPath.Child("etcdComponentProtection"))...)
	return allErrs
}

func validateEtcdComponentProtectionWebhookConfiguration(webhookConfig druidconfigv1alpha1.EtcdComponentProtectionWebhookConfiguration, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if !webhookConfig.Enabled {
		return allErrs
	}
	// Ensure that at least one of ReconcilerServiceAccountFQDN or ServiceAccountInfo is set.
	if webhookConfig.ReconcilerServiceAccountFQDN == nil && webhookConfig.ServiceAccountInfo == nil {
		allErrs = append(allErrs, field.Required(fldPath, "either reconcilerServiceAccountFQDN or serviceAccountInfo must be set"))
	}
	// Ensure that only one of ReconcilerServiceAccountFQDN or ServiceAccountInfo is set.
	if webhookConfig.ReconcilerServiceAccountFQDN != nil && webhookConfig.ServiceAccountInfo != nil {
		allErrs = append(allErrs, field.Invalid(fldPath, webhookConfig, "only one of reconcilerServiceAccountFQDN or serviceAccountInfo can be set but not both."))
	}
	if webhookConfig.ServiceAccountInfo != nil {
		allErrs = append(allErrs, validateServiceAccountInfo(webhookConfig.ServiceAccountInfo, fldPath.Child("serviceAccountInfo"))...)
	} else {
		if webhookConfig.ReconcilerServiceAccountFQDN != nil && len(strings.TrimSpace(*webhookConfig.ReconcilerServiceAccountFQDN)) == 0 {
			allErrs = append(allErrs, field.Required(fldPath.Child("reconcilerServiceAccountFQDN"), "must not be empty"))
		}
	}
	return allErrs
}

func validateServiceAccountInfo(serviceAccountInfo *druidconfigv1alpha1.ServiceAccountInfo, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if len(strings.TrimSpace(serviceAccountInfo.Name)) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("name"), "name is required"))
	}
	if len(strings.TrimSpace(serviceAccountInfo.Namespace)) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("namespace"), "namespace is required"))
	}
	return allErrs
}

func validateConcurrentSyncs(val *int, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if ptr.Deref(val, 0) <= 0 {
		allErrs = append(allErrs, field.Invalid(fldPath, val, "must be greater than 0"))
	}
	return allErrs
}

func mustBeGreaterThanZeroDuration(duration metav1.Duration, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if duration.Duration <= 0 {
		allErrs = append(allErrs, field.Invalid(fldPath, duration, "must be greater than 0"))
	}
	return allErrs
}

func mustBeEqualToOrGreaterThanZeroDuration(duration metav1.Duration, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if duration.Duration < 0 {
		allErrs = append(allErrs, field.Invalid(fldPath, duration, "must be greater than or equal to 0"))
	}
	return allErrs
}
