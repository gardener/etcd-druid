package compaction

import (
	"context"
	"fmt"
	"net/http"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	druidmetrics "github.com/gardener/etcd-druid/internal/metrics"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
)

type httpClientInterface interface {
	Do(req *http.Request) (*http.Response, error)
}

func fullSnapshot(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd, httpClient httpClientInterface, httpScheme string) error {
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
