package compaction

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	druidmetrics "github.com/gardener/etcd-druid/internal/metrics"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
)

// triggerFullSnapshot triggers a full snapshot for the given Etcd resource's associated etcd cluster.
func (r *Reconciler) triggerFullSnapshot(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd) error {
	httpScheme := "http"
	httpTransport := &http.Transport{}
	if tlsConfig := etcd.Spec.Backup.TLS; tlsConfig != nil {
		etcdbrCASecret := &corev1.Secret{}
		httpScheme = "https"
		dataKey := ptr.Deref(tlsConfig.TLSCASecretRef.DataKey, "bundle.crt")
		if err := r.Client.Get(ctx, types.NamespacedName{Namespace: etcd.Namespace, Name: tlsConfig.TLSCASecretRef.Name}, etcdbrCASecret); err != nil {
			logger.Error(err, "Failed to get etcdbr CA secret", "secretName", tlsConfig.TLSCASecretRef.Name)
			return err
		}
		certData, ok := etcdbrCASecret.Data[dataKey]
		if !ok {
			return fmt.Errorf("CA cert data key %q not found in secret %s/%s", dataKey, etcdbrCASecret.Namespace, etcdbrCASecret.Name)
		}
		caCerts := x509.NewCertPool()
		if !caCerts.AppendCertsFromPEM(certData) {
			return fmt.Errorf("failed to append CA certs from secret %s/%s", etcdbrCASecret.Namespace, etcdbrCASecret.Name)
		}
		httpTransport.TLSClientConfig = &tls.Config{
			RootCAs:    caCerts,
			MinVersion: tls.VersionTLS12,
		}
	}

	httpClient := &http.Client{
		Transport: httpTransport,
	}

	fullSnapshotURL := fmt.Sprintf(
		"%s://%s.%s.svc.cluster.local:%d/snapshot/full",
		httpScheme,
		druidv1alpha1.GetClientServiceName(etcd.ObjectMeta),
		etcd.Namespace,
		*etcd.Spec.Backup.Port,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullSnapshotURL, nil)
	if err != nil {
		logger.Error(err, "Failed to create HTTP request for full snapshot", "url", fullSnapshotURL)
		return err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		logger.Error(err, "Failed to trigger full snapshot", "url", fullSnapshotURL)
		return fmt.Errorf("failed to trigger full snapshot: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to trigger full snapshot, received status code: %d", resp.StatusCode)
	}
	logger.Info("Successfully triggered full snapshot", "etcd", etcd.Name, "namespace", etcd.Namespace)
	return nil
}

// recordFullSnapshotsTriggered increments the full snapshot triggered metric with the given labels.
func recordFullSnapshotsTriggered(succeeded, namespace string) {
	metricFullSnapshotsTriggered.With(prometheus.Labels{
		druidmetrics.LabelSucceeded:     succeeded,
		druidmetrics.LabelEtcdNamespace: namespace,
	}).Inc()
}
