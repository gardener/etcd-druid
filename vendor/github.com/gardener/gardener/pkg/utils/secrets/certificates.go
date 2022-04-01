// Copyright (c) 2018 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package secrets

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gardener/gardener/pkg/utils"
	"github.com/gardener/gardener/pkg/utils/infodata"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/clock"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CertType is a string alias for certificate types.
type CertType string

const (
	// CACert indicates that the certificate should be a certificate authority.
	CACert CertType = "ca"
	// ServerCert indicates that the certificate should have the ExtKeyUsageServerAuth usage.
	ServerCert CertType = "server"
	// ClientCert indicates that the certificate should have the ExtKeyUsageClientAuth usage.
	ClientCert CertType = "client"
	// ServerClientCert indicates that the certificate should have both the ExtKeyUsageServerAuth and ExtKeyUsageClientAuth usage.
	ServerClientCert CertType = "both"

	// DataKeyCertificate is the key in a secret data holding the certificate.
	DataKeyCertificate = "tls.crt"
	// DataKeyPrivateKey is the key in a secret data holding the private key.
	DataKeyPrivateKey = "tls.key"
	// DataKeyCertificateCA is the key in a secret data holding the CA certificate.
	DataKeyCertificateCA = "ca.crt"
	// DataKeyPrivateKeyCA is the key in a secret data holding the CA private key.
	DataKeyPrivateKeyCA = "ca.key"
)

const (
	// PKCS1 certificate format
	PKCS1 = iota
	// PKCS8 certificate format
	PKCS8
)

// CertificateSecretConfig contains the specification a to-be-generated CA, server, or client certificate.
// It always contains a 2048-bit RSA private key.
type CertificateSecretConfig struct {
	Name string

	CommonName   string
	Organization []string
	DNSNames     []string
	IPAddresses  []net.IP

	CertType  CertType
	SigningCA *Certificate
	PKCS      int

	Validity                    *time.Duration
	SkipPublishingCACertificate bool

	Clock clock.Clock
}

// Certificate contains the private key, and the certificate. It does also contain the CA certificate
// in case it is no CA. Otherwise, the <CA> field is nil.
type Certificate struct {
	Name string

	CA                          *Certificate
	SkipPublishingCACertificate bool

	PrivateKey    *rsa.PrivateKey
	PrivateKeyPEM []byte

	Certificate    *x509.Certificate
	CertificatePEM []byte
}

// GetName returns the name of the secret.
func (s *CertificateSecretConfig) GetName() string {
	return s.Name
}

// Generate implements ConfigInterface.
func (s *CertificateSecretConfig) Generate() (DataInterface, error) {
	return s.GenerateCertificate()
}

// GenerateInfoData implements ConfigInterface
func (s *CertificateSecretConfig) GenerateInfoData() (infodata.InfoData, error) {
	data, err := s.GenerateCertificate()
	if err != nil {
		return nil, err
	}

	if len(data.PrivateKeyPEM) == 0 && len(data.CertificatePEM) == 0 {
		return nil, nil
	}

	infoData := NewCertificateInfoData(data.PrivateKeyPEM, data.CertificatePEM)
	return infoData, nil
}

// GenerateFromInfoData implements ConfigInterface
func (s *CertificateSecretConfig) GenerateFromInfoData(infoData infodata.InfoData) (DataInterface, error) {
	data, ok := infoData.(*CertificateInfoData)
	if !ok {
		return nil, fmt.Errorf("could not convert InfoData entry %s to CertificateInfoData", s.Name)
	}
	certificateObj := &Certificate{
		Name:                        s.Name,
		CA:                          s.SigningCA,
		SkipPublishingCACertificate: s.SkipPublishingCACertificate,

		PrivateKeyPEM:  data.PrivateKey,
		CertificatePEM: data.Certificate,
	}

	var err error
	if s.PKCS == PKCS1 {
		certificateObj.PrivateKey, err = utils.DecodePrivateKey(data.PrivateKey)
	} else if s.PKCS == PKCS8 {
		certificateObj.PrivateKey, err = utils.DecodeRSAPrivateKeyFromPKCS8(data.PrivateKey)
	}
	if err != nil {
		return nil, err
	}

	certificateObj.Certificate, err = utils.DecodeCertificate(data.Certificate)
	if err != nil {
		return nil, err
	}
	return certificateObj, nil
}

// LoadFromSecretData implements infodata.Loader
func (s *CertificateSecretConfig) LoadFromSecretData(secretData map[string][]byte) (infodata.InfoData, error) {
	var (
		privateKeyPEM  []byte
		certificatePEM []byte
	)
	if s.CertType == CACert {
		privateKeyPEM = secretData[DataKeyPrivateKeyCA]
		certificatePEM = secretData[DataKeyCertificateCA]
	} else {
		privateKeyPEM = secretData[DataKeyPrivateKey]
		certificatePEM = secretData[DataKeyCertificate]
	}

	return NewCertificateInfoData(privateKeyPEM, certificatePEM), nil
}

// GenerateCertificate computes a CA, server, or client certificate based on the configuration.
func (s *CertificateSecretConfig) GenerateCertificate() (*Certificate, error) {
	certificateObj := &Certificate{
		Name:                        s.Name,
		CA:                          s.SigningCA,
		SkipPublishingCACertificate: s.SkipPublishingCACertificate,
	}

	// If no cert type is given then we only return a certificate object that contains the CA.
	if s.CertType != "" {
		privateKey, err := GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, err
		}

		var (
			certificate       = s.generateCertificateTemplate()
			certificateSigner = certificate
			privateKeySigner  = privateKey
		)

		if s.SigningCA != nil {
			certificateSigner = s.SigningCA.Certificate
			privateKeySigner = s.SigningCA.PrivateKey
		}

		certificatePEM, err := signCertificate(certificate, privateKey, certificateSigner, privateKeySigner)
		if err != nil {
			return nil, err
		}

		var pk []byte
		if s.PKCS == PKCS1 {
			pk = utils.EncodePrivateKey(privateKey)
		} else if s.PKCS == PKCS8 {
			pk, err = utils.EncodePrivateKeyInPKCS8(privateKey)

			if err != nil {
				return nil, err
			}
		}

		certificateObj.PrivateKey = privateKey
		certificateObj.PrivateKeyPEM = pk
		certificateObj.Certificate = certificate
		certificateObj.CertificatePEM = certificatePEM
	}

	return certificateObj, nil
}

// SecretData computes the data map which can be used in a Kubernetes secret.
func (c *Certificate) SecretData() map[string][]byte {
	data := map[string][]byte{}

	switch {
	case c.CA == nil:
		// The certificate is a CA certificate itself, so we use different keys in the secret data (for backwards-
		// compatibility).
		data[DataKeyCertificateCA] = c.CertificatePEM
		data[DataKeyPrivateKeyCA] = c.PrivateKeyPEM
	case c.CA != nil:
		// The certificate is not a CA certificate, so we add the signing CA certificate to it and use different
		// keys in the secret data.
		data[DataKeyPrivateKey] = c.PrivateKeyPEM
		data[DataKeyCertificate] = c.CertificatePEM
		if !c.SkipPublishingCACertificate {
			data[DataKeyCertificateCA] = c.CA.CertificatePEM
		}
	}

	return data
}

// LoadCertificate takes a byte slice representation of a certificate and the corresponding private key, and returns its de-serialized private
// key, certificate template and PEM certificate which can be used to sign other x509 certificates.
func LoadCertificate(name string, privateKeyPEM, certificatePEM []byte) (*Certificate, error) {
	privateKey, err := utils.DecodePrivateKey(privateKeyPEM)
	if err != nil {
		return nil, err
	}
	certificate, err := utils.DecodeCertificate(certificatePEM)
	if err != nil {
		return nil, err
	}

	return &Certificate{
		Name: name,

		PrivateKey:    privateKey,
		PrivateKeyPEM: privateKeyPEM,

		Certificate:    certificate,
		CertificatePEM: certificatePEM,
	}, nil
}

// LoadCAFromSecret loads a CA certificate from an existing Kubernetes secret object. It returns the secret, the Certificate and an error.
func LoadCAFromSecret(ctx context.Context, k8sClient client.Client, namespace, name string) (*corev1.Secret, *Certificate, error) {
	secret := &corev1.Secret{}
	if err := k8sClient.Get(ctx, kutil.Key(namespace, name), secret); err != nil {
		return nil, nil, err
	}

	certificate, err := LoadCertificate(name, secret.Data[DataKeyPrivateKeyCA], secret.Data[DataKeyCertificateCA])
	if err != nil {
		return nil, nil, err
	}

	return secret, certificate, nil
}

// generateCertificateTemplate creates a X509 Certificate object based on the provided information regarding
// common name, organization, SANs (DNS names and IP addresses). It can create a server or a client certificate
// or both, depending on the <certType> value. If <isCACert> is true, then a CA certificate is being created.
// The certificates a valid for 10 years.
func (s *CertificateSecretConfig) generateCertificateTemplate() *x509.Certificate {
	var clock clock.Clock = clock.RealClock{}

	if s.Clock != nil {
		clock = s.Clock
	}

	now := clock.Now()
	expiration := now.AddDate(10, 0, 0) // + 10 years
	if s.Validity != nil {
		expiration = now.Add(*s.Validity)
	}

	var (
		serialNumber, _ = rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
		isCA            = s.CertType == CACert

		template = &x509.Certificate{
			BasicConstraintsValid: true,
			IsCA:                  isCA,
			SerialNumber:          serialNumber,
			NotBefore:             now,
			NotAfter:              expiration,
			KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
			Subject: pkix.Name{
				CommonName:   s.CommonName,
				Organization: s.Organization,
			},
			DNSNames:    s.DNSNames,
			IPAddresses: s.IPAddresses,
		}
	)

	switch s.CertType {
	case CACert:
		template.KeyUsage |= x509.KeyUsageCertSign | x509.KeyUsageCRLSign
	case ServerCert:
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	case ClientCert:
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	case ServerClientCert:
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
	}

	return template
}

// SignCertificate takes a <certificateTemplate> and a <certificateTemplateSigner> which is used to sign
// the first. It also requires the corresponding private keys of both certificates. The created certificate
// is returned as byte slice.
func signCertificate(certificateTemplate *x509.Certificate, privateKey *rsa.PrivateKey, certificateTemplateSigner *x509.Certificate, privateKeySigner *rsa.PrivateKey) ([]byte, error) {
	certificate, err := x509.CreateCertificate(rand.Reader, certificateTemplate, certificateTemplateSigner, &privateKey.PublicKey, privateKeySigner)
	if err != nil {
		return nil, err
	}
	return utils.EncodeCertificate(certificate), nil
}

func generateCA(ctx context.Context, c client.Client, config *CertificateSecretConfig, namespace string) (*corev1.Secret, *Certificate, error) {
	certificate, err := config.GenerateCertificate()
	if err != nil {
		return nil, nil, err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.GetName(),
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: certificate.SecretData(),
	}

	if err := c.Create(ctx, secret); err != nil {
		return nil, nil, err
	}
	return secret, certificate, nil
}

func loadCA(name string, existingSecret *corev1.Secret) (*corev1.Secret, *Certificate, error) {
	certificate, err := LoadCertificate(name, existingSecret.Data[DataKeyPrivateKeyCA], existingSecret.Data[DataKeyCertificateCA])
	if err != nil {
		return nil, nil, err
	}
	return existingSecret, certificate, nil
}

// GenerateCertificateAuthorities get a map of wanted certificates and check If they exist in the existingSecretsMap based on the keys in the map. If they exist it get only the certificate from the corresponding
// existing secret and makes a certificate DataInterface from the existing secret. If there is no existing secret contaning the wanted certificate, we make one certificate and with it we deploy in K8s cluster
// a secret with that  certificate and then return the newly existing secret. The function returns a map of secrets contaning the wanted CA, a map with the wanted CA certificate and an error.
func GenerateCertificateAuthorities(ctx context.Context, c client.Client, existingSecretsMap map[string]*corev1.Secret, wantedCertificateAuthorities map[string]*CertificateSecretConfig, namespace string) (map[string]*corev1.Secret, map[string]*Certificate, error) {
	type caOutput struct {
		secret      *corev1.Secret
		certificate *Certificate
		err         error
	}

	var (
		certificateAuthorities = map[string]*Certificate{}
		generatedSecrets       = map[string]*corev1.Secret{}
		results                = make(chan *caOutput)
		wg                     sync.WaitGroup
		errorList              = []error{}
	)

	for name, config := range wantedCertificateAuthorities {
		wg.Add(1)

		if existingSecret, ok := existingSecretsMap[name]; !ok {
			go func(config *CertificateSecretConfig) {
				defer wg.Done()
				secret, certificate, err := generateCA(ctx, c, config, namespace)
				results <- &caOutput{secret, certificate, err}
			}(config)
		} else {
			go func(name string, existingSecret *corev1.Secret) {
				defer wg.Done()
				secret, certificate, err := loadCA(name, existingSecret)
				results <- &caOutput{secret, certificate, err}
			}(name, existingSecret)
		}
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	for out := range results {
		if out.err != nil {
			errorList = append(errorList, out.err)
			continue
		}
		generatedSecrets[out.secret.Name] = out.secret
		certificateAuthorities[out.secret.Name] = out.certificate
	}

	// Wait and check whether an error occurred during the parallel processing of the Secret creation.
	if len(errorList) > 0 {
		return nil, nil, fmt.Errorf("errors occurred during certificate authority generation: %+v", errorList)
	}

	return generatedSecrets, certificateAuthorities, nil
}

// TemporaryDirectoryForSelfGeneratedTLSCertificatesPattern is a constant for the pattern used when creating a temporary
// directory for self-generated certificates.
const TemporaryDirectoryForSelfGeneratedTLSCertificatesPattern = "self-generated-server-certificates-"

// SelfGenerateTLSServerCertificate generates a new CA certificate and signs a server certificate with it. It'll store
// the generated CA + server certificate bytes into a temporary directory with the default filenames, e.g. `DataKeyCertificateCA`.
// The function will return the *Certificate object as well as the path of the temporary directory where the
// certificates are stored.
func SelfGenerateTLSServerCertificate(name string, dnsNames []string, ips []net.IP) (cert *Certificate, ca *Certificate, dir string, rErr error) {
	tempDir, err := os.MkdirTemp("", TemporaryDirectoryForSelfGeneratedTLSCertificatesPattern)
	if err != nil {
		return nil, nil, "", err
	}

	caCertificateConfig := &CertificateSecretConfig{
		Name:       name,
		CommonName: name,
		CertType:   CACert,
	}
	caCertificate, err := caCertificateConfig.GenerateCertificate()
	if err != nil {
		return nil, nil, "", err
	}
	if err := os.WriteFile(filepath.Join(tempDir, DataKeyCertificateCA), caCertificate.CertificatePEM, 0644); err != nil {
		return nil, nil, "", err
	}
	if err := os.WriteFile(filepath.Join(tempDir, DataKeyPrivateKeyCA), caCertificate.PrivateKeyPEM, 0644); err != nil {
		return nil, nil, "", err
	}

	certificateConfig := &CertificateSecretConfig{
		Name:        name,
		CommonName:  name,
		DNSNames:    dnsNames,
		IPAddresses: ips,
		CertType:    ServerCert,
		SigningCA:   caCertificate,
	}
	certificate, err := certificateConfig.GenerateCertificate()
	if err != nil {
		return nil, nil, "", err
	}
	if err := os.WriteFile(filepath.Join(tempDir, DataKeyCertificate), certificate.CertificatePEM, 0644); err != nil {
		return nil, nil, "", err
	}
	if err := os.WriteFile(filepath.Join(tempDir, DataKeyPrivateKey), certificate.PrivateKeyPEM, 0644); err != nil {
		return nil, nil, "", err
	}

	return certificate, caCertificate, tempDir, nil
}

// CertificateIsExpired returns `true` if the given certificate is expired.
// The given `renewalWindow` lets the certificate expire earlier.
func CertificateIsExpired(clock clock.Clock, cert []byte, renewalWindow time.Duration) (bool, error) {
	x509, err := utils.DecodeCertificate(cert)
	if err != nil {
		return false, err
	}

	return clock.Now().After(x509.NotAfter.Add(-renewalWindow)), nil
}
