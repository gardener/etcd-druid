package compaction

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	druidmetrics "github.com/gardener/etcd-druid/internal/metrics"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	etcdbrFullSnapshotReqTimeout = 30 * time.Second
)

type httpClientInterface interface {
	Do(req *http.Request) (*http.Response, error)
}

func (r *Reconciler) triggerFullSnapshot(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd, accumulatedEtcdRevisions, triggerFullSnapshotThreshold int64) error {
	var reason string
	if lastJobCompletionReason != nil && *lastJobCompletionReason == batchv1.JobReasonDeadlineExceeded {
		reason = "previous compaction job deadline exceeded"
	} else {
		reason = "delta revisions have crossed the upper threshold"
	}
	logger.Info("Taking full snapshot",
		"namespace", etcd.Namespace,
		"accumulatedRevisions", accumulatedEtcdRevisions,
		"triggerFullSnapshotThreshold", triggerFullSnapshotThreshold,
		"reason", reason)
	// Take full snapshot
	if err := r.takeFullSnapshot(ctx, etcd); err != nil {
		logger.Error(err, "Error while triggering full snapshot",
			"namespace", etcd.Namespace,
			"accumulatedRevisions", accumulatedEtcdRevisions,
			"triggerFullSnapshotThreshold", triggerFullSnapshotThreshold)
		recordFullSnapshotsTriggered(druidmetrics.ValueSucceededFalse, etcd.Namespace)
		return fmt.Errorf("error while triggering full snapshot: %v", err)
	}
	recordFullSnapshotsTriggered(druidmetrics.ValueSucceededTrue, etcd.Namespace)
	logger.Info("Full snapshot taken successfully", "name", etcd.Name, "namespace", etcd.Namespace)
	// Reset the last job completion reason after triggering full snapshot
	lastJobCompletionReason = nil
	return nil
}

// takeFullSnapshot takes a full snapshot for the given Etcd resource's associated etcd cluster.
func (r *Reconciler) takeFullSnapshot(ctx context.Context, etcd *druidv1alpha1.Etcd) error {
	var (
		httpClient httpClientInterface
		httpScheme string
	)

	if r.EtcdbrHTTPClient != nil {
		httpClient = r.EtcdbrHTTPClient
		httpScheme = "http"
	} else {
		var err error
		httpClient, httpScheme, err = newHTTPClient(ctx, r.Client, etcd)
		if err != nil {
			return err
		}
	}
	return fullSnapshot(ctx, etcd, httpClient, httpScheme)
}

// newHTTPClient creates an HTTP client for etcd-backup-restore server with optional TLS configuration based on the Etcd resource.
func newHTTPClient(ctx context.Context, cl client.Client, etcd *druidv1alpha1.Etcd) (*http.Client, string, error) {
	httpScheme := "http"
	httpTransport := &http.Transport{}

	if tlsConfig := etcd.Spec.Backup.TLS; tlsConfig != nil {
		httpScheme = "https"
		etcdbrCASecret := &v1.Secret{}
		dataKey := ptr.Deref(tlsConfig.TLSCASecretRef.DataKey, "bundle.crt")
		if err := cl.Get(ctx, types.NamespacedName{Namespace: etcd.Namespace, Name: tlsConfig.TLSCASecretRef.Name}, etcdbrCASecret); err != nil {
			return nil, "", fmt.Errorf("failed to get etcdbr CA secret %s/%s: %w", etcd.Namespace, tlsConfig.TLSCASecretRef.Name, err)
		}
		certData, ok := etcdbrCASecret.Data[dataKey]
		if !ok {
			return nil, "", fmt.Errorf("CA cert data key %q not found in secret %s/%s", dataKey, etcdbrCASecret.Namespace, etcdbrCASecret.Name)
		}
		caCerts := x509.NewCertPool()
		if !caCerts.AppendCertsFromPEM(certData) {
			return nil, "", fmt.Errorf("failed to append CA certs from secret %s/%s", etcdbrCASecret.Namespace, etcdbrCASecret.Name)
		}
		httpTransport.TLSClientConfig = &tls.Config{
			RootCAs:    caCerts,
			MinVersion: tls.VersionTLS12,
		}
	}

	httpClient := &http.Client{
		Timeout:   etcdbrFullSnapshotReqTimeout,
		Transport: httpTransport,
	}
	return httpClient, httpScheme, nil
}

// fullSnapshot makes an HTTP GET request to the etcd client service for a full snapshot.
func fullSnapshot(ctx context.Context, etcd *druidv1alpha1.Etcd, httpClient httpClientInterface, httpScheme string) error {
	fullSnapshotURL := fmt.Sprintf(
		"%s://%s.%s.svc.cluster.local:%d/snapshot/full",
		httpScheme,
		druidv1alpha1.GetClientServiceName(etcd.ObjectMeta),
		etcd.Namespace,
		*etcd.Spec.Backup.Port,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullSnapshotURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request for full snapshot: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to trigger full snapshot: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to take full snapshot, received status code: %d", resp.StatusCode)
	}
	return nil
}
