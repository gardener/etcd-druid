// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0
// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	v1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	corev1alpha1 "github.com/gardener/etcd-druid/client/clientset/versioned/typed/core/v1alpha1"
	gentype "k8s.io/client-go/gentype"
)

// fakeEtcdCopyBackupsTasks implements EtcdCopyBackupsTaskInterface
type fakeEtcdCopyBackupsTasks struct {
	*gentype.FakeClientWithList[*v1alpha1.EtcdCopyBackupsTask, *v1alpha1.EtcdCopyBackupsTaskList]
	Fake *FakeDruidV1alpha1
}

func newFakeEtcdCopyBackupsTasks(fake *FakeDruidV1alpha1, namespace string) corev1alpha1.EtcdCopyBackupsTaskInterface {
	return &fakeEtcdCopyBackupsTasks{
		gentype.NewFakeClientWithList[*v1alpha1.EtcdCopyBackupsTask, *v1alpha1.EtcdCopyBackupsTaskList](
			fake.Fake,
			namespace,
			v1alpha1.SchemeGroupVersion.WithResource("etcdcopybackupstasks"),
			v1alpha1.SchemeGroupVersion.WithKind("EtcdCopyBackupsTask"),
			func() *v1alpha1.EtcdCopyBackupsTask { return &v1alpha1.EtcdCopyBackupsTask{} },
			func() *v1alpha1.EtcdCopyBackupsTaskList { return &v1alpha1.EtcdCopyBackupsTaskList{} },
			func(dst, src *v1alpha1.EtcdCopyBackupsTaskList) { dst.ListMeta = src.ListMeta },
			func(list *v1alpha1.EtcdCopyBackupsTaskList) []*v1alpha1.EtcdCopyBackupsTask {
				return gentype.ToPointerSlice(list.Items)
			},
			func(list *v1alpha1.EtcdCopyBackupsTaskList, items []*v1alpha1.EtcdCopyBackupsTask) {
				list.Items = gentype.FromPointerSlice(items)
			},
		),
		fake,
	}
}
