// Copyright 2018 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gardener

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	resourcesv1alpha1 "github.com/gardener/gardener/pkg/apis/resources/v1alpha1"
	"github.com/gardener/gardener/pkg/utils"
	kubernetesutils "github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/gardener/gardener/pkg/utils/secrets"
)

// Domain contains information about a domain configured in the garden cluster.
type Domain struct {
	Domain     string
	Provider   string
	Zone       string
	SecretData map[string][]byte
}

// GetDefaultDomains finds all the default domain secrets within the given map and returns a list of
// objects that contains all relevant information about the default domains.
func GetDefaultDomains(secrets map[string]*corev1.Secret) ([]*Domain, error) {
	var defaultDomains []*Domain

	for key, secret := range secrets {
		if strings.HasPrefix(key, v1beta1constants.GardenRoleDefaultDomain) {
			domain, err := constructDomainFromSecret(secret)
			if err != nil {
				return nil, fmt.Errorf("error getting information out of default domain secret: %+v", err)
			}
			defaultDomains = append(defaultDomains, domain)
		}
	}

	return defaultDomains, nil
}

// GetInternalDomain finds the internal domain secret within the given map and returns the object
// that contains all relevant information about the internal domain.
func GetInternalDomain(secrets map[string]*corev1.Secret) (*Domain, error) {
	internalDomainSecret, ok := secrets[v1beta1constants.GardenRoleInternalDomain]
	if !ok {
		return nil, nil
	}

	return constructDomainFromSecret(internalDomainSecret)
}

func constructDomainFromSecret(secret *corev1.Secret) (*Domain, error) {
	provider, domain, zone, err := GetDomainInfoFromAnnotations(secret.Annotations)
	if err != nil {
		return nil, err
	}

	return &Domain{
		Domain:     domain,
		Provider:   provider,
		Zone:       zone,
		SecretData: secret.Data,
	}, nil
}

// DomainIsDefaultDomain identifies whether the given domain is a default domain.
func DomainIsDefaultDomain(domain string, defaultDomains []*Domain) *Domain {
	for _, defaultDomain := range defaultDomains {
		if strings.HasSuffix(domain, "."+defaultDomain.Domain) {
			return defaultDomain
		}
	}
	return nil
}

var gardenRoleReq = utils.MustNewRequirement(v1beta1constants.GardenRole, selection.Exists)

// ReadGardenSecrets reads the Kubernetes Secrets from the Garden cluster which are independent of Shoot clusters.
// The Secret objects are stored on the Controller in order to pass them to created Garden objects later.
func ReadGardenSecrets(
	ctx context.Context,
	log logr.Logger,
	c client.Reader,
	namespace string,
	enforceInternalDomainSecret bool,
) (
	map[string]*corev1.Secret,
	error,
) {
	var (
		logInfo                             []string
		secretsMap                          = make(map[string]*corev1.Secret)
		numberOfInternalDomainSecrets       = 0
		numberOfOpenVPNDiffieHellmanSecrets = 0
		numberOfAlertingSecrets             = 0
		numberOfGlobalMonitoringSecrets     = 0
	)

	secretList := &corev1.SecretList{}
	if err := c.List(ctx, secretList, client.InNamespace(namespace), client.MatchingLabelsSelector{Selector: labels.NewSelector().Add(gardenRoleReq)}); err != nil {
		return nil, err
	}

	for _, secret := range secretList.Items {
		// Retrieving default domain secrets based on all secrets in the Garden namespace which have
		// a label indicating the Garden role default-domain.
		if secret.Labels[v1beta1constants.GardenRole] == v1beta1constants.GardenRoleDefaultDomain {
			_, domain, _, err := GetDomainInfoFromAnnotations(secret.Annotations)
			if err != nil {
				log.Error(err, "Error getting information out of default domain secret", "secret", client.ObjectKeyFromObject(&secret))
				continue
			}

			defaultDomainSecret := secret
			secretsMap[fmt.Sprintf("%s-%s", v1beta1constants.GardenRoleDefaultDomain, domain)] = &defaultDomainSecret
			logInfo = append(logInfo, fmt.Sprintf("default domain secret %q for domain %q", secret.Name, domain))
		}

		// Retrieving internal domain secrets based on all secrets in the Garden namespace which have
		// a label indicating the Garden role internal-domain.
		if secret.Labels[v1beta1constants.GardenRole] == v1beta1constants.GardenRoleInternalDomain {
			_, domain, _, err := GetDomainInfoFromAnnotations(secret.Annotations)
			if err != nil {
				log.Error(err, "Error getting information out of internal domain secret", "secret", client.ObjectKeyFromObject(&secret))
				continue
			}

			internalDomainSecret := secret
			secretsMap[v1beta1constants.GardenRoleInternalDomain] = &internalDomainSecret
			logInfo = append(logInfo, fmt.Sprintf("internal domain secret %q for domain %q", secret.Name, domain))
			numberOfInternalDomainSecrets++
		}

		// Retrieving Diffie-Hellman secret for OpenVPN based on all secrets in the Garden namespace which have
		// a label indicating the Garden role openvpn-diffie-hellman.
		if secret.Labels[v1beta1constants.GardenRole] == v1beta1constants.GardenRoleOpenVPNDiffieHellman {
			openvpnDiffieHellman := secret
			key := "dh2048.pem"
			if _, ok := secret.Data[key]; !ok {
				return nil, fmt.Errorf("cannot use OpenVPN Diffie Hellman secret '%s' as it does not contain key '%s' (whose value should be the actual Diffie Hellman key)", secret.Name, key)
			}
			secretsMap[v1beta1constants.GardenRoleOpenVPNDiffieHellman] = &openvpnDiffieHellman
			logInfo = append(logInfo, fmt.Sprintf("OpenVPN Diffie Hellman secret %q", secret.Name))
			numberOfOpenVPNDiffieHellmanSecrets++
		}

		// Retrieve the alerting secret to configure alerting. Either in cluster email alerting or
		// external alertmanager configuration.
		if secret.Labels[v1beta1constants.GardenRole] == v1beta1constants.GardenRoleAlerting {
			authType := string(secret.Data["auth_type"])
			if authType != "smtp" && authType != "none" && authType != "basic" && authType != "certificate" {
				return nil, fmt.Errorf("invalid or missing field 'auth_type' in secret %s", secret.Name)
			}
			alertingSecret := secret
			secretsMap[v1beta1constants.GardenRoleAlerting] = &alertingSecret
			logInfo = append(logInfo, fmt.Sprintf("alerting secret %q", secret.Name))
			numberOfAlertingSecrets++
		}

		// Retrieving basic auth secret for aggregate monitoring with a label
		// indicating the Garden role global-monitoring.
		if secret.Labels[v1beta1constants.GardenRole] == v1beta1constants.GardenRoleGlobalMonitoring {
			monitoringSecret := secret
			secretsMap[v1beta1constants.GardenRoleGlobalMonitoring] = &monitoringSecret
			logInfo = append(logInfo, fmt.Sprintf("monitoring basic auth secret %q", secret.Name))
			numberOfGlobalMonitoringSecrets++
		}

		// Retrieving basic auth secret for remote write monitoring with a label
		// indicating the Garden role global-shoot-remote-write-monitoring.
		if secret.Labels[v1beta1constants.GardenRole] == v1beta1constants.GardenRoleGlobalShootRemoteWriteMonitoring {
			monitoringSecret := secret
			secretsMap[v1beta1constants.GardenRoleGlobalShootRemoteWriteMonitoring] = &monitoringSecret
			logInfo = append(logInfo, fmt.Sprintf("monitoring basic auth secret %q", secret.Name))
		}
	}

	// For each Shoot we create a LoadBalancer(LB) pointing to the API server of the Shoot. Because the technical address
	// of the LB (ip or hostname) can change we cannot directly write it into the kubeconfig of the components
	// which talk from outside (kube-proxy, kubelet etc.) (otherwise those kubeconfigs would be broken once ip/hostname
	// of LB changed; and we don't have means to exchange kubeconfigs currently).
	// Therefore, to have a stable endpoint, we create a DNS record pointing to the ip/hostname of the LB. This DNS record
	// is used in all kubeconfigs. With that we have a robust endpoint stable against underlying ip/hostname changes.
	// And there can only be one of this internal domain secret because otherwise the gardener would not know which
	// domain it should use.
	if enforceInternalDomainSecret && numberOfInternalDomainSecrets == 0 {
		return nil, fmt.Errorf("need an internal domain secret but found none")
	}

	// The VPN bridge from a Shoot's control plane running in the Seed cluster to the worker nodes of the Shoots is based
	// on OpenVPN. It requires a Diffie Hellman key. If no such key is explicitly provided as secret in the garden namespace
	// then the Gardener will use a default one (not recommended, but useful for local development). If a secret is specified
	// its key will be used for all Shoots. However, at most only one of such a secret is allowed to be specified (otherwise,
	// the Gardener cannot determine which to choose).
	if numberOfOpenVPNDiffieHellmanSecrets > 1 {
		return nil, fmt.Errorf("can only accept at most one OpenVPN Diffie Hellman secret, but found %d", numberOfOpenVPNDiffieHellmanSecrets)
	}

	// Operators can configure gardener to send email alerts or send the alerts to an external alertmanager. If no configuration
	// is provided then no alerts will be sent.
	if numberOfAlertingSecrets > 1 {
		return nil, fmt.Errorf("can only accept at most one alerting secret, but found %d", numberOfAlertingSecrets)
	}

	if numberOfGlobalMonitoringSecrets > 1 {
		return nil, fmt.Errorf("can only accept at most one global monitoring secret, but found %d", numberOfGlobalMonitoringSecrets)
	}

	log.Info("Found secrets", "namespace", namespace, "secrets", logInfo)
	return secretsMap, nil
}

const (
	// SecretNamePrefixGardenAccess is the prefix of all secrets containing credentials for accessing the garden cluster.
	SecretNamePrefixGardenAccess = "garden-access-"
	// VolumeMountPathGenericGardenKubeconfig is a constant for the path to which the generic garden kubeconfig will be mounted.
	VolumeMountPathGenericGardenKubeconfig = "/var/run/secrets/gardener.cloud/garden/generic-kubeconfig"
	// PathGardenToken is a constant for the path at which the garden token file is accessible.
	PathGardenToken = VolumeMountPathGenericGardenKubeconfig + "/" + resourcesv1alpha1.DataKeyToken
	// PathGenericGardenKubeconfig is a constant for the path at which the kubeconfig file is accessible.
	PathGenericGardenKubeconfig = VolumeMountPathGenericGardenKubeconfig + "/" + secrets.DataKeyKubeconfig
)

// NewGardenAccessSecret returns a new AccessSecret object and initializes it with an empty corev1.Secret object
// with the given name and namespace. If not already done, the name will be prefixed with the
// SecretNamePrefixGardenAccess. The ServiceAccountName field will be defaulted with the name.
func NewGardenAccessSecret(name, namespace string) *AccessSecret {
	if !strings.HasPrefix(name, SecretNamePrefixGardenAccess) {
		name = SecretNamePrefixGardenAccess + name
	}

	return &AccessSecret{
		Secret: &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		},
		ServiceAccountName: strings.TrimPrefix(name, SecretNamePrefixGardenAccess),
		Class:              resourcesv1alpha1.ResourceManagerClassGarden,
	}
}

// InjectGenericGardenKubeconfig injects the volumes, volume mounts, and env vars for the generic garden kubeconfig into
// the provided object. The access secret name must be the name of a secret containing a JWT token which should be used
// by the kubeconfig. If the object has multiple containers then the default is to inject it into all of them. If it
// should only be done for a selection of containers then their respective names must be provided.
// If any of the containers in the object already has the GARDEN_KUBECONFIG env var, the object is not mutated.
func InjectGenericGardenKubeconfig(obj runtime.Object, genericKubeconfigName, accessSecretName string, containerNames ...string) error {
	// check for presence of env var
	hasGardenKubeconfig := false

	if err := kubernetesutils.VisitPodSpec(obj, func(podSpec *corev1.PodSpec) {
		kubernetesutils.VisitContainers(podSpec, func(container *corev1.Container) {
			if kubernetesutils.HasEnvVar(*container, v1beta1constants.EnvGenericGardenKubeconfig) {
				hasGardenKubeconfig = true
			}
		}, containerNames...)
	}); err != nil {
		return err
	}

	if hasGardenKubeconfig {
		return nil
	}

	// inject volume and volumeMounts
	if err := injectGenericKubeconfig(
		obj,
		genericKubeconfigName,
		accessSecretName,
		"garden-kubeconfig",
		VolumeMountPathGenericGardenKubeconfig,
		containerNames...,
	); err != nil {
		return err
	}

	// inject env var
	return kubernetesutils.VisitPodSpec(obj, func(podSpec *corev1.PodSpec) {
		kubernetesutils.VisitContainers(podSpec, func(container *corev1.Container) {
			kubernetesutils.AddEnvVar(container, corev1.EnvVar{
				Name:  v1beta1constants.EnvGenericGardenKubeconfig,
				Value: PathGenericGardenKubeconfig,
			}, true)
		}, containerNames...)
	})
}

// PrepareGardenClientRestConfig takes a base rest config and adds an optional host and CA certificate.
func PrepareGardenClientRestConfig(baseConfig *rest.Config, address *string, caCert []byte) *rest.Config {
	gardenClientRestConfig := rest.CopyConfig(baseConfig)
	if address != nil {
		gardenClientRestConfig.Host = *address
	}
	if caCert != nil {
		gardenClientRestConfig.TLSClientConfig = rest.TLSClientConfig{
			CAData: caCert,
		}
	}
	return gardenClientRestConfig
}
