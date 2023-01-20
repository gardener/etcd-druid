// Copyright (c) 2023 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package etcdcopybackupstask

import (
	"github.com/gardener/etcd-druid/controllers/utils"
	"github.com/gardener/gardener/pkg/chartrenderer"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	"github.com/go-logr/logr"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// Reconciler reconciles EtcdCopyBackupsTask object.
type Reconciler struct {
	client.Client
	imageVector  imagevector.ImageVector
	chartApplier kubernetes.ChartApplier
	logger       logr.Logger
}

// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcdcopybackupstasks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcdcopybackupstasks/status;etcdcopybackupstasks/finalizers,verbs=get;update;patch;create

// NewReconciler creates a new EtcdCopyBackupsTaskReconciler.
func NewReconciler(mgr manager.Manager, withImageVector bool) (*Reconciler, error) {
	var (
		chartApplier kubernetes.ChartApplier
		imageVector  imagevector.ImageVector
		err          error
	)

	if chartApplier, err = createChartApplier(mgr.GetConfig()); err != nil {
		return nil, err
	}
	if withImageVector {
		if imageVector, err = utils.CreateDefaultImageVector(); err != nil {
			return nil, err
		}
	}
	return &Reconciler{
		Client:       mgr.GetClient(),
		chartApplier: chartApplier,
		imageVector:  imageVector,
		logger:       log.Log.WithName("etcd-copy-backups-task-controller"),
	}, nil
}

func createChartApplier(config *rest.Config) (kubernetes.ChartApplier, error) {
	renderer, err := chartrenderer.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	applier, err := kubernetes.NewApplierForConfig(config)
	if err != nil {
		return nil, err
	}
	return kubernetes.NewChartApplier(renderer, applier), nil
}
