// Copyright 2023 SAP SE or an SAP affiliate company
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

package etcdcopybackupstask

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/controllers/utils"
	"github.com/gardener/etcd-druid/pkg/common"
	druidutils "github.com/gardener/etcd-druid/pkg/utils"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	defaultEtcdCopyBackupChartPath = filepath.Join("charts", "etcd-copy-backups")
	jobChartPath                   = filepath.Join("etcd-copy-backups", "templates", "etcd-copy-backups-job.yaml")
)

func (r *Reconciler) decodeJobFromChart(ctx context.Context, task *druidv1alpha1.EtcdCopyBackupsTask) (*batchv1.Job, error) {
	// Get chart values
	values, err := r.getChartValues(ctx, task)
	if err != nil {
		return nil, fmt.Errorf("could not get chart values: %w", err)
	}

	// Render chart
	// TODO(AleksandarSavchev): .Render is deprecated. Refactor or adapt code to use RenderEmbeddedFS https://github.com/gardener/gardener/pull/6165
	renderedChart, err := r.chartRenderer.Render(r.chartBasePath, task.Name, task.Namespace, values) //nolint:staticcheck
	if err != nil {
		return nil, fmt.Errorf("could not render chart: %w", err)
	}

	// Decode job object from chart
	job := &batchv1.Job{}
	if err := utils.DecodeObject(renderedChart, jobChartPath, &job); err != nil {
		return nil, fmt.Errorf("could not decode job object from chart: %w", err)
	}
	return job, nil
}

func (r *Reconciler) getChartValues(ctx context.Context, task *druidv1alpha1.EtcdCopyBackupsTask) (map[string]interface{}, error) {
	values := map[string]interface{}{
		"name":      getCopyBackupsJobName(task),
		"ownerName": task.Name,
		"ownerUID":  task.UID,
	}

	sourceStoreValues, err := getStoreValues(ctx, r.Client, r.logger, &task.Spec.SourceStore, task.Namespace)
	if err != nil {
		return nil, err
	}
	targetStoreValues, err := getStoreValues(ctx, r.Client, r.logger, &task.Spec.TargetStore, task.Namespace)
	if err != nil {
		return nil, err
	}
	values["sourceStore"] = sourceStoreValues
	values["targetStore"] = targetStoreValues

	if task.Spec.MaxBackupAge != nil {
		values["maxBackupAge"] = *task.Spec.MaxBackupAge
	}
	if task.Spec.MaxBackups != nil {
		values["maxBackups"] = *task.Spec.MaxBackups
	}

	if task.Spec.WaitForFinalSnapshot != nil {
		values["waitForFinalSnapshot"] = task.Spec.WaitForFinalSnapshot.Enabled
		if task.Spec.WaitForFinalSnapshot.Timeout != nil {
			values["waitForFinalSnapshotTimeout"] = task.Spec.WaitForFinalSnapshot.Timeout
		}
	}

	images, err := imagevector.FindImages(r.imageVector, []string{common.BackupRestore})
	if err != nil {
		return nil, err
	}
	val, ok := images[common.BackupRestore]
	if !ok {
		return nil, fmt.Errorf("%s image not found", common.BackupRestore)
	}
	values["image"] = val.String()

	return values, nil
}

// getStoreValues converts the values in the StoreSpec to a map, or returns an error if the storage provider is unsupported.
func getStoreValues(ctx context.Context, client client.Client, logger logr.Logger, store *druidv1alpha1.StoreSpec, namespace string) (map[string]interface{}, error) {
	storageProvider, err := druidutils.StorageProviderFromInfraProvider(store.Provider)
	if err != nil {
		return nil, err
	}
	storeValues := map[string]interface{}{
		"storePrefix":     store.Prefix,
		"storageProvider": storageProvider,
	}
	if strings.EqualFold(string(*store.Provider), druidutils.Local) {
		mountPath, err := druidutils.GetHostMountPathFromSecretRef(ctx, client, logger, store, namespace)
		if err != nil {
			return nil, err
		}
		storeValues["storageMountPath"] = mountPath
	}
	if store.Container != nil {
		storeValues["storageContainer"] = store.Container
	}
	if store.SecretRef != nil {
		storeValues["storeSecret"] = store.SecretRef.Name
	}
	return storeValues, nil
}
