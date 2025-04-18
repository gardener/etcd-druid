// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0
// Code generated by informer-gen. DO NOT EDIT.

package v1alpha1

import (
	internalinterfaces "github.com/gardener/etcd-druid/client/informers/externalversions/internalinterfaces"
)

// Interface provides access to all the informers in this group version.
type Interface interface {
	// Etcds returns a EtcdInformer.
	Etcds() EtcdInformer
	// EtcdCopyBackupsTasks returns a EtcdCopyBackupsTaskInformer.
	EtcdCopyBackupsTasks() EtcdCopyBackupsTaskInformer
}

type version struct {
	factory          internalinterfaces.SharedInformerFactory
	namespace        string
	tweakListOptions internalinterfaces.TweakListOptionsFunc
}

// New returns a new Interface.
func New(f internalinterfaces.SharedInformerFactory, namespace string, tweakListOptions internalinterfaces.TweakListOptionsFunc) Interface {
	return &version{factory: f, namespace: namespace, tweakListOptions: tweakListOptions}
}

// Etcds returns a EtcdInformer.
func (v *version) Etcds() EtcdInformer {
	return &etcdInformer{factory: v.factory, namespace: v.namespace, tweakListOptions: v.tweakListOptions}
}

// EtcdCopyBackupsTasks returns a EtcdCopyBackupsTaskInformer.
func (v *version) EtcdCopyBackupsTasks() EtcdCopyBackupsTaskInformer {
	return &etcdCopyBackupsTaskInformer{factory: v.factory, namespace: v.namespace, tweakListOptions: v.tweakListOptions}
}
