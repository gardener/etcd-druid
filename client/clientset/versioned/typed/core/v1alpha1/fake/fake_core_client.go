// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0
// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	v1alpha1 "github.com/gardener/etcd-druid/client/clientset/versioned/typed/core/v1alpha1"
	rest "k8s.io/client-go/rest"
	testing "k8s.io/client-go/testing"
)

type FakeDruidV1alpha1 struct {
	*testing.Fake
}

func (c *FakeDruidV1alpha1) Etcds(namespace string) v1alpha1.EtcdInterface {
	return newFakeEtcds(c, namespace)
}

func (c *FakeDruidV1alpha1) EtcdCopyBackupsTasks(namespace string) v1alpha1.EtcdCopyBackupsTaskInterface {
	return newFakeEtcdCopyBackupsTasks(c, namespace)
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *FakeDruidV1alpha1) RESTClient() rest.Interface {
	var ret *rest.RESTClient
	return ret
}
