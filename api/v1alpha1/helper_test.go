// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"

	"k8s.io/utils/pointer"
)

const (
	etcdName      = "etcd-test"
	etcdNamespace = "etcd-test-namespace"
)

func TestGetNamespaceName(t *testing.T) {
	g := NewWithT(t)
	etcdObjMeta := createEtcdObjectMetadata(uuid.NewUUID(), nil, nil, false)
	namespaceName := GetNamespaceName(etcdObjMeta)
	g.Expect(namespaceName.Namespace).To(Equal(etcdNamespace))
	g.Expect(namespaceName.Name).To(Equal(etcdName))
}

func TestGetPeerServiceName(t *testing.T) {
	g := NewWithT(t)
	etcdObjMeta := createEtcdObjectMetadata(uuid.NewUUID(), nil, nil, false)
	peerServiceName := GetPeerServiceName(etcdObjMeta)
	g.Expect(peerServiceName).To(Equal("etcd-test-peer"))
}

func TestGetClientServiceName(t *testing.T) {
	g := NewWithT(t)
	etcdObjMeta := createEtcdObjectMetadata(uuid.NewUUID(), nil, nil, false)
	clientServiceName := GetClientServiceName(etcdObjMeta)
	g.Expect(clientServiceName).To(Equal("etcd-test-client"))
}

func TestGetServiceAccountName(t *testing.T) {
	g := NewWithT(t)
	etcdObjMeta := createEtcdObjectMetadata(uuid.NewUUID(), nil, nil, false)
	serviceAccountName := GetServiceAccountName(etcdObjMeta)
	g.Expect(serviceAccountName).To(Equal(etcdName))
}

func TestGetConfigMapName(t *testing.T) {
	g := NewWithT(t)
	uid := uuid.NewUUID()
	etcdObjMeta := createEtcdObjectMetadata(uid, nil, nil, false)
	configMapName := GetConfigMapName(etcdObjMeta)
	g.Expect(configMapName).To(Equal("etcd-bootstrap-" + string(uid[:6])))
}

func TestGetCompactionJobName(t *testing.T) {
	g := NewWithT(t)
	etcdObjMeta := createEtcdObjectMetadata(uuid.NewUUID(), nil, nil, false)
	compactionJobName := GetCompactionJobName(etcdObjMeta)
	g.Expect(compactionJobName).To(Equal("etcd-test-compactor"))
}

func TestGetOrdinalPodName(t *testing.T) {
	g := NewWithT(t)
	etcdObjMeta := createEtcdObjectMetadata(uuid.NewUUID(), nil, nil, false)
	ordinalPodName := GetOrdinalPodName(etcdObjMeta, 1)
	g.Expect(ordinalPodName).To(Equal("etcd-test-1"))
}

func TestGetDeltaSnapshotLeaseName(t *testing.T) {
	g := NewWithT(t)
	etcdObjMeta := createEtcdObjectMetadata(uuid.NewUUID(), nil, nil, false)
	deltaSnapshotLeaseName := GetDeltaSnapshotLeaseName(etcdObjMeta)
	g.Expect(deltaSnapshotLeaseName).To(Equal("etcd-test-delta-snap"))
}

func TestGetFullSnapshotLeaseName(t *testing.T) {
	g := NewWithT(t)
	etcdObjMeta := createEtcdObjectMetadata(uuid.NewUUID(), nil, nil, false)
	fullSnapshotLeaseName := GetFullSnapshotLeaseName(etcdObjMeta)
	g.Expect(fullSnapshotLeaseName).To(Equal("etcd-test-full-snap"))
}

func TestGetMemberLeaseNames(t *testing.T) {
	g := NewWithT(t)
	etcdObjMeta := createEtcdObjectMetadata(uuid.NewUUID(), nil, nil, false)
	leaseNames := GetMemberLeaseNames(etcdObjMeta, 3)
	g.Expect(leaseNames).To(Equal([]string{"etcd-test-0", "etcd-test-1", "etcd-test-2"}))
}

func TestGetPodDisruptionBudgetName(t *testing.T) {
	g := NewWithT(t)
	etcdObjMeta := createEtcdObjectMetadata(uuid.NewUUID(), nil, nil, false)
	podDisruptionBudgetName := GetPodDisruptionBudgetName(etcdObjMeta)
	g.Expect(podDisruptionBudgetName).To(Equal("etcd-test"))
}

func TestGetRoleName(t *testing.T) {
	g := NewWithT(t)
	etcdObjMeta := createEtcdObjectMetadata(uuid.NewUUID(), nil, nil, false)
	roleName := GetRoleName(etcdObjMeta)
	g.Expect(roleName).To(Equal("druid.gardener.cloud:etcd:etcd-test"))
}

func TestGetRoleBindingName(t *testing.T) {
	g := NewWithT(t)
	etcdObjMeta := createEtcdObjectMetadata(uuid.NewUUID(), nil, nil, false)
	roleBindingName := GetRoleBindingName(etcdObjMeta)
	g.Expect(roleBindingName).To(Equal("druid.gardener.cloud:etcd:etcd-test"))
}

func TestGetSuspendEtcdSpecReconcileAnnotationKey(t *testing.T) {
	tests := []struct {
		name                  string
		annotations           map[string]string
		expectedAnnotationKey *string
	}{
		{
			name:                  "No annotation is set",
			annotations:           nil,
			expectedAnnotationKey: nil,
		},
		{
			name:                  "SuspendEtcdSpecReconcileAnnotation is set",
			annotations:           map[string]string{SuspendEtcdSpecReconcileAnnotation: ""},
			expectedAnnotationKey: pointer.String(SuspendEtcdSpecReconcileAnnotation),
		},
		{
			name:                  "IgnoreReconciliationAnnotation is set",
			annotations:           map[string]string{IgnoreReconciliationAnnotation: ""},
			expectedAnnotationKey: pointer.String(IgnoreReconciliationAnnotation),
		},
		{
			name:                  "Both annotations (SuspendEtcdSpecReconcileAnnotation and IgnoreReconciliationAnnotation) are set",
			annotations:           map[string]string{SuspendEtcdSpecReconcileAnnotation: "", IgnoreReconciliationAnnotation: ""},
			expectedAnnotationKey: pointer.String(SuspendEtcdSpecReconcileAnnotation),
		},
	}
	g := NewWithT(t)
	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			etcdObjMeta := createEtcdObjectMetadata(uuid.NewUUID(), test.annotations, nil, false)
			annotationKey := GetSuspendEtcdSpecReconcileAnnotationKey(etcdObjMeta)
			g.Expect(annotationKey).To(Equal(test.expectedAnnotationKey))
		})
	}
}

func TestAreManagedResourcesProtected(t *testing.T) {
	tests := []struct {
		name                       string
		annotations                map[string]string
		expectedResourceProtection bool
	}{
		{
			name:                       "No DisableEtcdComponentProtectionAnnotation annotation is set",
			annotations:                nil,
			expectedResourceProtection: true,
		},
		{
			name:                       "DisableEtcdComponentProtectionAnnotation is set",
			annotations:                map[string]string{DisableEtcdComponentProtectionAnnotation: ""},
			expectedResourceProtection: false,
		},
	}
	g := NewWithT(t)
	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			etcdObjMeta := createEtcdObjectMetadata(uuid.NewUUID(), test.annotations, nil, false)
			resourceProtection := AreManagedResourcesProtected(etcdObjMeta)
			g.Expect(resourceProtection).To(Equal(test.expectedResourceProtection))
		})
	}
}

func TestGetDefaultLabels(t *testing.T) {
	g := NewWithT(t)
	etcdObjMeta := createEtcdObjectMetadata(uuid.NewUUID(), nil, nil, false)
	defaultLabels := GetDefaultLabels(etcdObjMeta)
	g.Expect(defaultLabels).To(Equal(map[string]string{
		LabelManagedByKey: LabelManagedByValue,
		LabelPartOfKey:    etcdName,
	}))
}

func TestGetAsOwnerReference(t *testing.T) {
	g := NewWithT(t)
	uid := uuid.NewUUID()
	etcdObjMeta := createEtcdObjectMetadata(uid, nil, nil, false)
	ownerRef := GetAsOwnerReference(etcdObjMeta)
	g.Expect(ownerRef).To(Equal(metav1.OwnerReference{
		APIVersion:         GroupVersion.String(),
		Kind:               "Etcd",
		Name:               etcdName,
		UID:                uid,
		Controller:         pointer.Bool(true),
		BlockOwnerDeletion: pointer.Bool(true),
	}))
}

func TestIsEtcdMarkedForDeletion(t *testing.T) {
	tests := []struct {
		name                        string
		markedForDeletion           bool
		expectedIsMarkedForDeletion bool
	}{
		{
			name:                        "Etcd not marked for deletion",
			markedForDeletion:           false,
			expectedIsMarkedForDeletion: false,
		},
		{
			name:                        "Etcd marked for deletion",
			markedForDeletion:           true,
			expectedIsMarkedForDeletion: true,
		},
	}
	g := NewWithT(t)
	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			etcdObjMeta := createEtcdObjectMetadata(uuid.NewUUID(), nil, nil, test.markedForDeletion)
			isMarkedForDeletion := IsEtcdMarkedForDeletion(etcdObjMeta)
			g.Expect(isMarkedForDeletion).To(Equal(test.expectedIsMarkedForDeletion))
		})
	}
}

func createEtcdObjectMetadata(uid types.UID, annotations, labels map[string]string, markedForDeletion bool) metav1.ObjectMeta {
	etcdObjMeta := metav1.ObjectMeta{
		Name:        etcdName,
		Namespace:   etcdNamespace,
		Labels:      labels,
		Annotations: annotations,
		UID:         uid,
	}

	if markedForDeletion {
		now := metav1.Now()
		etcdObjMeta.DeletionTimestamp = &now
	}

	return etcdObjMeta
}
