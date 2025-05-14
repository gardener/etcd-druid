// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"
	configv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"
	"strings"
)

// ServiceAccountMatchesUsername checks whether the provided username matches the namespace and name
// Use this when checking a service account namespace and name against a known string.
func ServiceAccountMatchesUsername(namespace, serviceAccountName, username string) bool {
	saFQDN := getServiceAccountFQDN(namespace, serviceAccountName)
	return strings.TrimSpace(username) == saFQDN
}

// GetReconcilerServiceAccountFQDN constructs the fully qualified domain name of a service account from PodInfo.
// It reads the mounted files for namespace and service account name. If there is any error reading the files then it will return an error.
func GetReconcilerServiceAccountFQDN(config configv1alpha1.EtcdComponentProtectionWebhookConfiguration) (string, error) {
	if config.ServiceAccountInfo != nil {
		return getServiceAccountFQDN(config.ServiceAccountInfo.Namespace, config.ServiceAccountInfo.Name), nil
	}
	if config.ReconcilerServiceAccountFQDN != nil {
		return *config.ReconcilerServiceAccountFQDN, nil
	}
	return "", fmt.Errorf("no reconciler service account FQDN or service account info provided")
}

// getServiceAccountFQDN returns the fully qualified domain name of a service account.
func getServiceAccountFQDN(namespace, reconcilerServiceAccountName string) string {
	return fmt.Sprintf("system:serviceaccount:%s:%s", namespace, reconcilerServiceAccountName)
}
