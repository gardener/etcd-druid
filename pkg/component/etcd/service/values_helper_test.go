// Copyright (c) 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package service_test

import (
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	. "github.com/gardener/etcd-druid/pkg/component/etcd/service"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

var _ = Describe("#GenerateValues", func() {
	var (
		etcd *druidv1alpha1.Etcd

		labels                             map[string]string
		annotations                        map[string]string
		backupPort, clientPort, serverPort *int32
	)

	JustBeforeEach(func() {
		labels = map[string]string{
			"foo": "bar",
		}

		etcd = &druidv1alpha1.Etcd{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "etcd",
				Namespace: "default",
				UID:       "123",
			},
			Spec: druidv1alpha1.EtcdSpec{
				Selector: metav1.SetAsLabelSelector(labels),
				Backup: druidv1alpha1.BackupSpec{
					Port: backupPort,
				},
				Etcd: druidv1alpha1.EtcdConfig{
					ClientPort: clientPort,
					ServerPort: serverPort,
					ClientService: &druidv1alpha1.ClientService{
						Annotations: annotations,
					},
				},
			},
		}
	})

	Context("when ports are specified", func() {
		BeforeEach(func() {
			backupPort = pointer.Int32Ptr(1111)
			clientPort = pointer.Int32Ptr(2222)
			serverPort = pointer.Int32Ptr(3333)
		})

		It("should generate values correctly", func() {
			values := GenerateValues(etcd)

			Expect(values).To(MatchFields(IgnoreExtras, Fields{
				"BackupPort":        Equal(*etcd.Spec.Backup.Port),
				"ClientPort":        Equal(*etcd.Spec.Etcd.ClientPort),
				"ClientServiceName": Equal(fmt.Sprintf("%s-client", etcd.Name)),
				"EtcdName":          Equal(etcd.Name),
				"EtcdUID":           Equal(etcd.UID),
				"Labels":            Equal(etcd.Labels),
				"PeerServiceName":   Equal(fmt.Sprintf("%s-peer", etcd.Name)),
				"ServerPort":        Equal(*etcd.Spec.Etcd.ServerPort),
			}))
		})
	})

	Context("when ports aren't specified", func() {
		BeforeEach(func() {
			backupPort = nil
			clientPort = nil
			serverPort = nil
		})

		It("should generate values correctly", func() {
			values := GenerateValues(etcd)

			Expect(values).To(MatchFields(IgnoreExtras, Fields{
				"BackupPort":        Equal(int32(8080)),
				"ClientPort":        Equal(int32(2379)),
				"ClientServiceName": Equal(fmt.Sprintf("%s-client", etcd.Name)),
				"EtcdName":          Equal(etcd.Name),
				"EtcdUID":           Equal(etcd.UID),
				"Labels":            Equal(etcd.Labels),
				"PeerServiceName":   Equal(fmt.Sprintf("%s-peer", etcd.Name)),
				"ServerPort":        Equal(int32(2380)),
			}))
		})
	})
	Context("when client service annotations are specified", func() {
		BeforeEach(func() {
			backupPort = nil
			clientPort = nil
			serverPort = nil
			annotations = map[string]string{
				"foo1": "bar1",
				"foo2": "bar2",
			}
		})

		It("should generate values correctly", func() {
			values := GenerateValues(etcd)

			Expect(values).To(MatchFields(IgnoreExtras, Fields{
				"BackupPort":               Equal(int32(8080)),
				"ClientPort":               Equal(int32(2379)),
				"ClientServiceName":        Equal(fmt.Sprintf("%s-client", etcd.Name)),
				"EtcdName":                 Equal(etcd.Name),
				"EtcdUID":                  Equal(etcd.UID),
				"Labels":                   Equal(etcd.Labels),
				"PeerServiceName":          Equal(fmt.Sprintf("%s-peer", etcd.Name)),
				"ServerPort":               Equal(int32(2380)),
				"ClientServiceAnnotations": Equal(annotations),
			}))
		})
	})
	Context("when client service annotations are not specified", func() {
		BeforeEach(func() {
			backupPort = nil
			clientPort = nil
			serverPort = nil
			annotations = nil
		})

		It("should generate values correctly", func() {
			values := GenerateValues(etcd)

			Expect(values).To(MatchFields(IgnoreExtras, Fields{
				"BackupPort":               Equal(int32(8080)),
				"ClientPort":               Equal(int32(2379)),
				"ClientServiceName":        Equal(fmt.Sprintf("%s-client", etcd.Name)),
				"EtcdName":                 Equal(etcd.Name),
				"EtcdUID":                  Equal(etcd.UID),
				"Labels":                   Equal(etcd.Labels),
				"PeerServiceName":          Equal(fmt.Sprintf("%s-peer", etcd.Name)),
				"ServerPort":               Equal(int32(2380)),
				"ClientServiceAnnotations": Equal(annotations),
			}))
		})
	})
})
