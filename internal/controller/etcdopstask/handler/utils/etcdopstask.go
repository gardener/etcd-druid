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

// GetEtcdOpsTask fetches the EtcdOpsTask object with the given taskReference.
func GetEtcdOpsTask(ctx context.Context, k8sClient client.Client, taskReference types.NamespacedName, phase druidapicommon.LastOperationType) (task *druidv1alpha1.EtcdOpsTask, errResult *taskhandler.Result) {
	task = &druidv1alpha1.EtcdOpsTask{}
	if err := k8sClient.Get(ctx, taskReference, task); err != nil {
		if apierrors.IsNotFound(err) {
			errResult = &taskhandler.Result{
				Description: "EtcdOpsTask object not found",
				Error:       druiderr.WrapError(err, taskhandler.ErrGetEtcdOpsTask, string(phase), "etcdopstask object not found"),
				Requeue:     false,
			}
			return
		}

		errResult = &taskhandler.Result{
			Description: "Failed to get EtcdOpsTask object",
			Error:       druiderr.WrapError(err, taskhandler.ErrGetEtcdOpsTask, string(phase), "failed to get etcdopstask object"),
			Requeue:     true,
		}
		return
	}

	return
}
