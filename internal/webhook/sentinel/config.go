// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package sentinel

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	flag "github.com/spf13/pflag"
)

const (
	enableSentinelWebhookFlagName    = "enable-sentinel-webhook"
	reconcilerServiceAccountFlagName = "reconciler-service-account"
	exemptServiceAccountsFlagName    = "sentinel-exempt-service-accounts"

	defaultEnableSentinelWebhook    = false
	defaultReconcilerServiceAccount = "system:serviceaccount:default:etcd-druid"

	reconcilerServiceAccountTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"
)

var (
	defaultExemptServiceAccounts []string
)

// Config defines the configuration for the Sentinel Webhook.
type Config struct {
	// Enabled indicates whether the Sentinel Webhook is enabled.
	Enabled bool
	// ReconcilerServiceAccount is the name of the service account used by etcd-druid for reconciling etcd resources.
	ReconcilerServiceAccount string
	// ExemptServiceAccounts is a list of service accounts that are exempt from Sentinel Webhook checks.
	ExemptServiceAccounts []string
}

// InitFromFlags initializes the config from the provided CLI flag set.
func InitFromFlags(fs *flag.FlagSet, cfg *Config) {
	fs.BoolVar(&cfg.Enabled, enableSentinelWebhookFlagName, defaultEnableSentinelWebhook,
		"Enable Sentinel Webhook to prevent unintended changes to resources managed by etcd-druid.")

	reconcilerServiceAccount, err := getReconcilerServiceAccountName()
	if err != nil {
		reconcilerServiceAccount = defaultReconcilerServiceAccount
	}
	fs.StringVar(&cfg.ReconcilerServiceAccount, reconcilerServiceAccountFlagName, reconcilerServiceAccount,
		fmt.Sprintf("The fully qualified name of the service account used by etcd-druid for reconciling etcd resources. Default: %s", defaultReconcilerServiceAccount))

	fs.StringSliceVar(&cfg.ExemptServiceAccounts, exemptServiceAccountsFlagName, defaultExemptServiceAccounts,
		"The comma-separated list of fully qualified names of service accounts that are exempt from Sentinel Webhook checks.")
}

func getReconcilerServiceAccountName() (string, error) {
	saToken, err := os.ReadFile(reconcilerServiceAccountTokenPath)
	if err != nil {
		return "", err
	}
	tokens := strings.Split(string(saToken), ".")
	if len(tokens) != 3 {
		return "", fmt.Errorf("invalid token format")
	}

	decodedClaims, err := base64.StdEncoding.DecodeString(getPaddedBase64EncodedString(tokens[1]))
	if err != nil {
		return "", err
	}

	claims := &jwt.RegisteredClaims{}
	if err = json.Unmarshal(decodedClaims, claims); err != nil {
		return "", err
	}

	return claims.Subject, nil
}

func getPaddedBase64EncodedString(encoded string) string {
	padding := 4 - len(encoded)%4
	if padding != 4 {
		for i := 0; i < padding; i++ {
			encoded += "="
		}
	}
	return encoded
}
