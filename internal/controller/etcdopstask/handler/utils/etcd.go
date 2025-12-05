// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"

	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	taskhandler "github.com/gardener/etcd-druid/internal/controller/etcdopstask/handler"
	druiderr "github.com/gardener/etcd-druid/internal/errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetEtcd fetches the Etcd object with the given etcdReference.
func GetEtcd(ctx context.Context, k8sClient client.Client, etcdReference types.NamespacedName, phase druidapicommon.LastOperationType) (etcd *druidv1alpha1.Etcd, errResult *taskhandler.Result) {
	etcd = &druidv1alpha1.Etcd{}
	if err := k8sClient.Get(ctx, etcdReference, etcd); err != nil {
		if apierrors.IsNotFound(err) {
			errResult = &taskhandler.Result{
				Description: "Etcd object not found",
				Error:       druiderr.WrapError(err, taskhandler.ErrGetEtcd, string(phase), "etcd object not found"),
				Requeue:     false,
			}
			return
		}

		errResult = &taskhandler.Result{
			Description: "Failed to get etcd object",
			Error:       druiderr.WrapError(err, taskhandler.ErrGetEtcd, string(phase), "failed to get etcd object"),
			Requeue:     true,
		}
		return
	}

	return
}
