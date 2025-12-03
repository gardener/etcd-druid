// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

var (
	defaultTTLSecondsAfterFinished = int32(3600)
)

type EtcdOpsTaskBuilder struct {
	task *druidv1alpha1.EtcdOpsTask
}

func EtcdOpsTaskBuilderWithDefaults(name, namespace string) *EtcdOpsTaskBuilder {
	builder := EtcdOpsTaskBuilder{}
	builder.task = getDefaultEtcdOpsTask(name, namespace)
	return &builder
}

func EtcdOpsTaskBuilderWithoutDefaults(name, namespace string) *EtcdOpsTaskBuilder {
	builder := EtcdOpsTaskBuilder{}
	builder.task = getEtcdOpsTaskWithoutDefaults(name, namespace)
	return &builder
}

func (eb *EtcdOpsTaskBuilder) WithEtcdName(etcdName string) *EtcdOpsTaskBuilder {
	if eb == nil || eb.task == nil {
		return nil
	}
	eb.task.Spec.EtcdName = ptr.To(etcdName)
	return eb
}

func (eb *EtcdOpsTaskBuilder) WithTTLSecondsAfterFinished(ttl int32) *EtcdOpsTaskBuilder {
	if eb == nil || eb.task == nil {
		return nil
	}
	eb.task.Spec.TTLSecondsAfterFinished = ptr.To(ttl)
	return eb
}

func (eb *EtcdOpsTaskBuilder) WithOnDemandSnapshotConfig(config *druidv1alpha1.OnDemandSnapshotConfig) *EtcdOpsTaskBuilder {
	if eb == nil || eb.task == nil {
		return nil
	}
	eb.task.Spec.Config.OnDemandSnapshot = config
	return eb
}

func (eb *EtcdOpsTaskBuilder) WithState(state druidv1alpha1.TaskState) *EtcdOpsTaskBuilder {
	if eb == nil || eb.task == nil {
		return nil
	}
	eb.task.Status.State = &state
	return eb
}

func (eb *EtcdOpsTaskBuilder) WithLastTransitionTime(time metav1.Time) *EtcdOpsTaskBuilder {
	if eb == nil || eb.task == nil {
		return nil
	}
	eb.task.Status.LastTransitionTime = &time
	return eb
}

func (eb *EtcdOpsTaskBuilder) WithStartedAt(time metav1.Time) *EtcdOpsTaskBuilder {
	if eb == nil || eb.task == nil {
		return nil
	}
	eb.task.Status.StartedAt = &time
	return eb
}

func (eb *EtcdOpsTaskBuilder) WithLastOperation(lastOp *druidapicommon.LastOperation) *EtcdOpsTaskBuilder {
	if eb == nil || eb.task == nil {
		return nil
	}
	eb.task.Status.LastOperation = lastOp
	return eb
}

func (eb *EtcdOpsTaskBuilder) Build() *druidv1alpha1.EtcdOpsTask {
	return eb.task
}

func getEtcdOpsTaskWithoutDefaults(name, namespace string) *druidv1alpha1.EtcdOpsTask {
	return &druidv1alpha1.EtcdOpsTask{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: druidv1alpha1.EtcdOpsTaskSpec{
			Config: druidv1alpha1.EtcdOpsTaskConfig{},
		},
	}
}

func getDefaultEtcdOpsTask(name, namespace string) *druidv1alpha1.EtcdOpsTask {
	return &druidv1alpha1.EtcdOpsTask{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: druidv1alpha1.EtcdOpsTaskSpec{
			TTLSecondsAfterFinished: &defaultTTLSecondsAfterFinished,
			Config:                  druidv1alpha1.EtcdOpsTaskConfig{},
		},
	}
}
