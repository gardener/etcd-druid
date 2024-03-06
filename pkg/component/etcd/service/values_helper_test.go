// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package service_test

import (
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	. "github.com/gardener/etcd-druid/pkg/component/etcd/service"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

var _ = Describe("#GenerateValues", func() {
	var (
		etcd *druidv1alpha1.Etcd

		labels                           map[string]string
		clientServiceAnnotations         map[string]string
		clientServiceLabels              map[string]string
		backupPort, clientPort, peerPort *int32
		expectedLabels                   = map[string]string{
			"instance": "etcd",
			"name":     "etcd",
		}
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
				Labels:   labels,
				Selector: metav1.SetAsLabelSelector(map[string]string{"baz": "qax"}),
				Backup: druidv1alpha1.BackupSpec{
					Port: backupPort,
				},
				Etcd: druidv1alpha1.EtcdConfig{
					ClientPort: clientPort,
					ServerPort: peerPort,
					ClientService: &druidv1alpha1.ClientService{
						Annotations: clientServiceAnnotations,
						Labels:      clientServiceLabels,
					},
				},
			},
		}
	})

	Context("when ports are specified", func() {
		BeforeEach(func() {
			backupPort = pointer.Int32(1111)
			clientPort = pointer.Int32(2222)
			peerPort = pointer.Int32(3333)
		})

		It("should generate values correctly", func() {
			values := GenerateValues(etcd)

			Expect(values).To(MatchFields(IgnoreExtras, Fields{
				"BackupPort":        Equal(*etcd.Spec.Backup.Port),
				"ClientPort":        Equal(*etcd.Spec.Etcd.ClientPort),
				"ClientServiceName": Equal(fmt.Sprintf("%s-client", etcd.Name)),
				"Labels":            Equal(expectedLabels),
				"PeerServiceName":   Equal(fmt.Sprintf("%s-peer", etcd.Name)),
				"PeerPort":          Equal(*etcd.Spec.Etcd.ServerPort),
			}))
		})
	})

	Context("when ports aren't specified", func() {
		BeforeEach(func() {
			backupPort = nil
			clientPort = nil
			peerPort = nil
		})

		It("should generate values correctly", func() {
			values := GenerateValues(etcd)

			Expect(values).To(MatchFields(IgnoreExtras, Fields{
				"BackupPort":        Equal(int32(8080)),
				"ClientPort":        Equal(int32(2379)),
				"ClientServiceName": Equal(fmt.Sprintf("%s-client", etcd.Name)),
				"Labels":            Equal(expectedLabels),
				"PeerServiceName":   Equal(fmt.Sprintf("%s-peer", etcd.Name)),
				"PeerPort":          Equal(int32(2380)),
				"SelectorLabels":    Equal(expectedLabels),
				"OwnerReference":    Equal(etcd.GetAsOwnerReference()),
			}))
		})
	})
	Context("when client service annotations are specified", func() {
		BeforeEach(func() {
			backupPort = nil
			clientPort = nil
			peerPort = nil
			clientServiceAnnotations = map[string]string{
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
				"Labels":                   Equal(expectedLabels),
				"PeerServiceName":          Equal(fmt.Sprintf("%s-peer", etcd.Name)),
				"PeerPort":                 Equal(int32(2380)),
				"ClientServiceAnnotations": Equal(clientServiceAnnotations),
				"SelectorLabels":           Equal(expectedLabels),
				"OwnerReference":           Equal(etcd.GetAsOwnerReference()),
			}))
		})
	})
	Context("when client service annotations are not specified", func() {
		BeforeEach(func() {
			backupPort = nil
			clientPort = nil
			peerPort = nil
			clientServiceAnnotations = nil
		})

		It("should generate values correctly", func() {
			values := GenerateValues(etcd)

			Expect(values).To(MatchFields(IgnoreExtras, Fields{
				"BackupPort":               Equal(int32(8080)),
				"ClientPort":               Equal(int32(2379)),
				"ClientServiceName":        Equal(fmt.Sprintf("%s-client", etcd.Name)),
				"Labels":                   Equal(expectedLabels),
				"PeerServiceName":          Equal(fmt.Sprintf("%s-peer", etcd.Name)),
				"PeerPort":                 Equal(int32(2380)),
				"ClientServiceAnnotations": Equal(clientServiceAnnotations),
				"SelectorLabels":           Equal(expectedLabels),
				"OwnerReference":           Equal(etcd.GetAsOwnerReference()),
			}))
		})
	})
	Context("when client service labels are specified", func() {
		BeforeEach(func() {
			backupPort = nil
			clientPort = nil
			peerPort = nil
			clientServiceLabels = map[string]string{
				"foo1": "bar1",
				"foo2": "bar2",
			}
		})

		It("should generate values correctly", func() {
			values := GenerateValues(etcd)

			Expect(values).To(MatchFields(IgnoreExtras, Fields{
				"BackupPort":          Equal(int32(8080)),
				"ClientPort":          Equal(int32(2379)),
				"ClientServiceName":   Equal(fmt.Sprintf("%s-client", etcd.Name)),
				"Labels":              Equal(expectedLabels),
				"PeerServiceName":     Equal(fmt.Sprintf("%s-peer", etcd.Name)),
				"PeerPort":            Equal(int32(2380)),
				"ClientServiceLabels": Equal(clientServiceLabels),
				"SelectorLabels":      Equal(expectedLabels),
				"OwnerReference":      Equal(etcd.GetAsOwnerReference()),
			}))
		})
	})
	Context("when client service labels are not specified", func() {
		BeforeEach(func() {
			backupPort = nil
			clientPort = nil
			peerPort = nil
			clientServiceLabels = nil
		})

		It("should generate values correctly", func() {
			values := GenerateValues(etcd)

			Expect(values).To(MatchFields(IgnoreExtras, Fields{
				"BackupPort":          Equal(int32(8080)),
				"ClientPort":          Equal(int32(2379)),
				"ClientServiceName":   Equal(fmt.Sprintf("%s-client", etcd.Name)),
				"Labels":              Equal(expectedLabels),
				"PeerServiceName":     Equal(fmt.Sprintf("%s-peer", etcd.Name)),
				"PeerPort":            Equal(int32(2380)),
				"ClientServiceLabels": Equal(clientServiceLabels),
				"SelectorLabels":      Equal(expectedLabels),
				"OwnerReference":      Equal(etcd.GetAsOwnerReference()),
			}))
		})
	})
})
