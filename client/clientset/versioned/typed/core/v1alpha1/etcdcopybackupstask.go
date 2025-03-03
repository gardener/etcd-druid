// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0
// Code generated by client-gen. DO NOT EDIT.

package v1alpha1

import (
	context "context"

	corev1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	scheme "github.com/gardener/etcd-druid/client/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	gentype "k8s.io/client-go/gentype"
)

// EtcdCopyBackupsTasksGetter has a method to return a EtcdCopyBackupsTaskInterface.
// A group's client should implement this interface.
type EtcdCopyBackupsTasksGetter interface {
	EtcdCopyBackupsTasks(namespace string) EtcdCopyBackupsTaskInterface
}

// EtcdCopyBackupsTaskInterface has methods to work with EtcdCopyBackupsTask resources.
type EtcdCopyBackupsTaskInterface interface {
	Create(ctx context.Context, etcdCopyBackupsTask *corev1alpha1.EtcdCopyBackupsTask, opts v1.CreateOptions) (*corev1alpha1.EtcdCopyBackupsTask, error)
	Update(ctx context.Context, etcdCopyBackupsTask *corev1alpha1.EtcdCopyBackupsTask, opts v1.UpdateOptions) (*corev1alpha1.EtcdCopyBackupsTask, error)
	// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
	UpdateStatus(ctx context.Context, etcdCopyBackupsTask *corev1alpha1.EtcdCopyBackupsTask, opts v1.UpdateOptions) (*corev1alpha1.EtcdCopyBackupsTask, error)
	Delete(ctx context.Context, name string, opts v1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error
	Get(ctx context.Context, name string, opts v1.GetOptions) (*corev1alpha1.EtcdCopyBackupsTask, error)
	List(ctx context.Context, opts v1.ListOptions) (*corev1alpha1.EtcdCopyBackupsTaskList, error)
	Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *corev1alpha1.EtcdCopyBackupsTask, err error)
	EtcdCopyBackupsTaskExpansion
}

// etcdCopyBackupsTasks implements EtcdCopyBackupsTaskInterface
type etcdCopyBackupsTasks struct {
	*gentype.ClientWithList[*corev1alpha1.EtcdCopyBackupsTask, *corev1alpha1.EtcdCopyBackupsTaskList]
}

// newEtcdCopyBackupsTasks returns a EtcdCopyBackupsTasks
func newEtcdCopyBackupsTasks(c *DruidV1alpha1Client, namespace string) *etcdCopyBackupsTasks {
	return &etcdCopyBackupsTasks{
		gentype.NewClientWithList[*corev1alpha1.EtcdCopyBackupsTask, *corev1alpha1.EtcdCopyBackupsTaskList](
			"etcdcopybackupstasks",
			c.RESTClient(),
			scheme.ParameterCodec,
			namespace,
			func() *corev1alpha1.EtcdCopyBackupsTask { return &corev1alpha1.EtcdCopyBackupsTask{} },
			func() *corev1alpha1.EtcdCopyBackupsTaskList { return &corev1alpha1.EtcdCopyBackupsTaskList{} },
		),
	}
}
