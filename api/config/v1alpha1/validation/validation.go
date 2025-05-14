package validation

import (
	"strings"

	configv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
)

// ValidateOperatorConfiguration validates the operator configuration.
func ValidateOperatorConfiguration(config *configv1alpha1.OperatorConfiguration) field.ErrorList {
	allErrs := field.ErrorList{}

	allErrs = append(allErrs, validateClientConnectionConfiguration(config.ClientConnection, field.NewPath("clientConnection"))...)
	allErrs = append(allErrs, validateLeaderElectionConfiguration(config.LeaderElection, field.NewPath("leaderElection"))...)
	allErrs = append(allErrs, validateControllerConfiguration(config.Controllers, field.NewPath("controllers"))...)
	allErrs = append(allErrs, validateLogConfiguration(config.LogConfiguration, field.NewPath("log"))...)
	allErrs = append(allErrs, validateWebhookConfiguration(config.Webhooks, field.NewPath("webhooks"))...)

	return allErrs
}

func validateClientConnectionConfiguration(clientConnConfig configv1alpha1.ClientConnectionConfiguration, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if clientConnConfig.Burst < 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("burst"), clientConnConfig.Burst, "must be non-negative"))
	}
	return allErrs
}

func validateLeaderElectionConfiguration(leaderElectionConfig configv1alpha1.LeaderElectionConfiguration, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if !leaderElectionConfig.Enabled {
		return allErrs
	}
	if leaderElectionConfig.LeaseDuration.Duration <= 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("leaseDuration"), leaderElectionConfig.LeaseDuration, "must be greater than zero"))
	}
	if leaderElectionConfig.RenewDeadline.Duration <= 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("renewDeadline"), leaderElectionConfig.RenewDeadline, "must be greater than zero"))
	}
	if leaderElectionConfig.RetryPeriod.Duration <= 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("retryPeriod"), leaderElectionConfig.RetryPeriod, "must be greater than zero"))
	}
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

func validateLogConfiguration(logConfig configv1alpha1.LogConfiguration, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if logConfig.LogLevel != "" && !sets.New(configv1alpha1.AllLogLevels...).Has(logConfig.LogLevel) {
		allErrs = append(allErrs, field.NotSupported(fldPath.Child("logLevel"), logConfig.LogLevel, configv1alpha1.AllLogLevels))
	}
	if logConfig.LogFormat != "" && !sets.New(configv1alpha1.AllLogFormats...).Has(logConfig.LogFormat) {
		allErrs = append(allErrs, field.NotSupported(fldPath.Child("logFormat"), logConfig.LogFormat, configv1alpha1.AllLogFormats))
	}
	return allErrs
}

func validateControllerConfiguration(controllerConfig configv1alpha1.ControllerConfiguration, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, validateEtcdControllerConfiguration(controllerConfig.Etcd, fldPath.Child("etcd"))...)
	allErrs = append(allErrs, validateSecretControllerConfiguration(controllerConfig.Secret, fldPath.Child("secret"))...)
	allErrs = append(allErrs, validateCompactionControllerConfiguration(controllerConfig.Compaction, fldPath.Child("compaction"))...)
	allErrs = append(allErrs, validateEtcdCopyBackupsTaskControllerConfiguration(controllerConfig.EtcdCopyBackupsTask, fldPath.Child("etcdCopyBackupsTask"))...)
	return allErrs
}

func validateEtcdControllerConfiguration(etcdControllerConfig configv1alpha1.EtcdControllerConfiguration, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, validateConcurrentSyncs(etcdControllerConfig.ConcurrentSyncs, fldPath.Child("concurrentSyncs"))...)
	return allErrs
}

func validateCompactionControllerConfiguration(compactionControllerConfig configv1alpha1.CompactionControllerConfiguration, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if !compactionControllerConfig.Enabled {
		return allErrs
	}
	allErrs = append(allErrs, validateConcurrentSyncs(compactionControllerConfig.ConcurrentSyncs, fldPath.Child("concurrentSyncs"))...)
	return allErrs
}

func validateEtcdCopyBackupsTaskControllerConfiguration(etcdCopyBackupsTaskControllerConfig configv1alpha1.EtcdCopyBackupsTaskControllerConfiguration, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if !etcdCopyBackupsTaskControllerConfig.Enabled {
		return allErrs
	}
	allErrs = append(allErrs, validateConcurrentSyncs(etcdCopyBackupsTaskControllerConfig.ConcurrentSyncs, fldPath.Child("concurrentSyncs"))...)
	return allErrs
}

func validateSecretControllerConfiguration(secretControllerConfig configv1alpha1.SecretControllerConfiguration, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, validateConcurrentSyncs(secretControllerConfig.ConcurrentSyncs, fldPath.Child("concurrentSyncs"))...)
	return allErrs
}

func validateWebhookConfiguration(webhookConfig configv1alpha1.WebhookConfiguration, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, validateEtcdComponentProtectionWebhookConfiguration(webhookConfig.EtcdComponentProtection, fldPath.Child("etcdComponentProtection"))...)
	return allErrs
}

func validateEtcdComponentProtectionWebhookConfiguration(webhookConfig configv1alpha1.EtcdComponentProtectionWebhookConfiguration, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if !webhookConfig.Enabled {
		return allErrs
	}
	// Ensure that at least one of ReconcilerServiceAccountFQDN or ServiceAccountInfo is set.
	if webhookConfig.ReconcilerServiceAccountFQDN == nil && webhookConfig.ServiceAccountInfo == nil {
		allErrs = append(allErrs, field.Required(fldPath, "either reconcilerServiceAccountFQDN or serviceAccountInfo must be set"))
	}
	if webhookConfig.ServiceAccountInfo != nil {
		allErrs = append(allErrs, validateServiceAccountInfo(webhookConfig.ServiceAccountInfo, fldPath.Child("serviceAccountInfo"))...)
	}
	return allErrs
}

func validateServiceAccountInfo(serviceAccountInfo *configv1alpha1.ServiceAccountInfo, fldPath *field.Path) field.ErrorList {
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
