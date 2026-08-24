// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"fmt"
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/utils/ptr"

	. "github.com/onsi/gomega"
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
	g.Expect(peerServiceName).To(Equal(etcdObjMeta.Name + "-peer"))
}

func TestGetClientServiceName(t *testing.T) {
	g := NewWithT(t)
	etcdObjMeta := createEtcdObjectMetadata(uuid.NewUUID(), nil, nil, false)
	clientServiceName := GetClientServiceName(etcdObjMeta)
	g.Expect(clientServiceName).To(Equal(etcdObjMeta.Name + "-client"))
}

func TestGetClientHostnameWithDruidManagedMembers(t *testing.T) {
	g := NewWithT(t)
	etcdObjMeta := createEtcdObjectMetadata(uuid.NewUUID(), nil, nil, false)
	etcd := &Etcd{
		ObjectMeta: etcdObjMeta,
		Spec: EtcdSpec{
			Replicas: 3,
		},
	}
	clientHostname := GetClientHostname(etcd)
	g.Expect(clientHostname).To(Equal(fmt.Sprintf("%s.%s.svc", GetClientServiceName(etcd.ObjectMeta), etcd.Namespace)))
}

func TestGetClientHostnameWithExternallyManagedMembers(t *testing.T) {
	g := NewWithT(t)
	etcdObjMeta := createEtcdObjectMetadata(uuid.NewUUID(), nil, nil, false)
	etcd := &Etcd{
		ObjectMeta: etcdObjMeta,
		Spec: EtcdSpec{
			Replicas: 3,
			ExternallyManagedMemberAddresses: []string{
				"1.1.1.1",
				"1.1.1.2",
				"1.1.1.3",
			},
		},
	}
	clientHostname := GetClientHostname(etcd)
	g.Expect(clientHostname).Should(BeElementOf([]string{"1.1.1.1", "1.1.1.2", "1.1.1.3"}))
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
	g.Expect(configMapName).To(Equal(etcdObjMeta.Name + "-config"))
}

func TestGetCompactionJobName(t *testing.T) {
	g := NewWithT(t)
	etcdObjMeta := createEtcdObjectMetadata(uuid.NewUUID(), nil, nil, false)
	compactionJobName := GetCompactionJobName(etcdObjMeta)
	g.Expect(compactionJobName).To(Equal(etcdObjMeta.Name + "-compactor"))
}

func TestGetOrdinalPodName(t *testing.T) {
	g := NewWithT(t)
	etcdObjMeta := createEtcdObjectMetadata(uuid.NewUUID(), nil, nil, false)
	ordinalPodName := GetOrdinalPodName(etcdObjMeta, 1)
	g.Expect(ordinalPodName).To(Equal(etcdObjMeta.Name + "-1"))
}

func TestGetDeltaSnapshotLeaseName(t *testing.T) {
	g := NewWithT(t)
	etcdObjMeta := createEtcdObjectMetadata(uuid.NewUUID(), nil, nil, false)
	deltaSnapshotLeaseName := GetDeltaSnapshotLeaseName(etcdObjMeta)
	g.Expect(deltaSnapshotLeaseName).To(Equal(etcdObjMeta.Name + "-delta-snap"))
}

func TestGetFullSnapshotLeaseName(t *testing.T) {
	g := NewWithT(t)
	etcdObjMeta := createEtcdObjectMetadata(uuid.NewUUID(), nil, nil, false)
	fullSnapshotLeaseName := GetFullSnapshotLeaseName(etcdObjMeta)
	g.Expect(fullSnapshotLeaseName).To(Equal(etcdObjMeta.Name + "-full-snap"))
}

func TestGetMemberLeaseNames(t *testing.T) {
	tests := []struct {
		name                     string
		replicas                 int
		memberNamePrefix         *string
		expectedMemberLeases     func(etcdName string) []string
		externallyManagedMembers []string
	}{
		{
			name:             "no member name prefix",
			replicas:         3,
			memberNamePrefix: nil,
			expectedMemberLeases: func(etcdName string) []string {
				return []string{etcdName + "-0", etcdName + "-1", etcdName + "-2"}
			},
		},
		{
			name:             "with member name prefix",
			replicas:         3,
			memberNamePrefix: ptr.To("myprefix"),
			expectedMemberLeases: func(etcdName string) []string {
				return []string{"myprefix-" + etcdName + "-0", "myprefix-" + etcdName + "-1", "myprefix-" + etcdName + "-2"}
			},
		},
		{
			name:                     "externally managed with no member name prefix",
			replicas:                 3,
			memberNamePrefix:         nil,
			externallyManagedMembers: []string{"1.1.1.1", "1.1.1.2", "1.1.1.3"},
			expectedMemberLeases: func(etcdName string) []string {
				return []string{etcdName + "-1.1.1.1", etcdName + "-1.1.1.2", etcdName + "-1.1.1.3"}
			},
		},
		{
			name:                     "externally managed with member name prefix",
			replicas:                 3,
			memberNamePrefix:         ptr.To("myprefix"),
			externallyManagedMembers: []string{"1.1.1.1", "1.1.1.2", "1.1.1.3"},
			expectedMemberLeases: func(etcdName string) []string {
				return []string{"myprefix-" + etcdName + "-1.1.1.1", "myprefix-" + etcdName + "-1.1.1.2", "myprefix-" + etcdName + "-1.1.1.3"}
			},
		},
	}
	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)
			etcdObjMeta := createEtcdObjectMetadata(uuid.NewUUID(), nil, nil, false)
			etcd := &Etcd{
				ObjectMeta: etcdObjMeta,
				Spec: EtcdSpec{
					Replicas:                         3,
					MemberNamePrefix:                 test.memberNamePrefix,
					ExternallyManagedMemberAddresses: test.externallyManagedMembers,
				},
			}
			leaseNames := GetMemberLeaseNames(etcd)
			g.Expect(leaseNames).To(Equal(test.expectedMemberLeases(etcdObjMeta.Name)))
		})
	}
}

func TestGetMemberNameFromAddress(t *testing.T) {
	g := NewWithT(t)
	etcdObjMeta := createEtcdObjectMetadata(uuid.NewUUID(), nil, nil, false)
	etcd := &Etcd{
		ObjectMeta: etcdObjMeta,
		Spec: EtcdSpec{
			Replicas: 3,
		},
	}
	memberName := GetMemberNameFromAddress(etcd, "1.1.1.1")
	g.Expect(memberName).To(Equal(etcdObjMeta.Name + "-1.1.1.1"))
}

func TestGetPodDisruptionBudgetName(t *testing.T) {
	g := NewWithT(t)
	etcdObjMeta := createEtcdObjectMetadata(uuid.NewUUID(), nil, nil, false)
	podDisruptionBudgetName := GetPodDisruptionBudgetName(etcdObjMeta)
	g.Expect(podDisruptionBudgetName).To(Equal(etcdObjMeta.Name))
}

func TestGetRoleName(t *testing.T) {
	g := NewWithT(t)
	etcdObjMeta := createEtcdObjectMetadata(uuid.NewUUID(), nil, nil, false)
	roleName := GetRoleName(etcdObjMeta)
	g.Expect(roleName).To(Equal("druid.gardener.cloud:etcd:" + etcdObjMeta.Name))
}

func TestGetRoleBindingName(t *testing.T) {
	g := NewWithT(t)
	etcdObjMeta := createEtcdObjectMetadata(uuid.NewUUID(), nil, nil, false)
	roleBindingName := GetRoleBindingName(etcdObjMeta)
	g.Expect(roleBindingName).To(Equal("druid.gardener.cloud:etcd:" + etcdObjMeta.Name))
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
			expectedAnnotationKey: ptr.To(SuspendEtcdSpecReconcileAnnotation),
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
		APIVersion:         SchemeGroupVersion.String(),
		Kind:               "Etcd",
		Name:               etcdName,
		UID:                uid,
		Controller:         ptr.To(true),
		BlockOwnerDeletion: ptr.To(true),
	}))
}

func TestIsResourceMarkedForDeletion(t *testing.T) {
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
			objMeta := createEtcdObjectMetadata(uuid.NewUUID(), nil, nil, test.markedForDeletion)
			isMarkedForDeletion := IsResourceMarkedForDeletion(objMeta)
			g.Expect(isMarkedForDeletion).To(Equal(test.expectedIsMarkedForDeletion))
		})
	}
}

func TestHasReconcileOperationAnnotation(t *testing.T) {
	tests := []struct {
		name                        string
		annotations                 map[string]string
		expectedHasReconcileOpAnnot bool
	}{
		{
			name:                        "No annotation is set",
			annotations:                 nil,
			expectedHasReconcileOpAnnot: false,
		},
		{
			name: "Does not have the operation annotation",
			annotations: map[string]string{
				"dummy-annotation": "dummy-value",
			},
			expectedHasReconcileOpAnnot: false,
		},
		{
			name: "Contains gardener operation annotation but its value is not reconcile",
			annotations: map[string]string{
				GardenerOperationAnnotation: "migrate",
			},
			expectedHasReconcileOpAnnot: false,
		},
		{
			name: "Contains druid operation annotation but its value is not reconcile",
			annotations: map[string]string{
				DruidOperationAnnotation: "dummy",
			},
			expectedHasReconcileOpAnnot: false,
		},
		{
			name: "Contains gardener operation annotation and its value is reconcile",
			annotations: map[string]string{
				GardenerOperationAnnotation: "reconcile",
			},
			expectedHasReconcileOpAnnot: true,
		},
		{
			name: "Contains druid operation annotation and its value is reconcile",
			annotations: map[string]string{
				DruidOperationAnnotation: "reconcile",
			},
			expectedHasReconcileOpAnnot: true,
		},
		{
			name: "Contains both druid operation annotation and gardener operation annotation and its value is reconcile",
			annotations: map[string]string{
				GardenerOperationAnnotation: "reconcile",
				DruidOperationAnnotation:    "reconcile",
			},
			expectedHasReconcileOpAnnot: true,
		},
		{
			name: "Contains druid operation annotation with value not reconcile and gardener operation annotation with value reconcile",
			annotations: map[string]string{
				GardenerOperationAnnotation: "reconcile",
				DruidOperationAnnotation:    "dummy",
			},
			expectedHasReconcileOpAnnot: true,
		},
		{
			name: "Contains druid operation annotation with value reconcile and gardener operation annotation with value not reconcile",
			annotations: map[string]string{
				GardenerOperationAnnotation: "dummy",
				DruidOperationAnnotation:    "reconcile",
			},
			expectedHasReconcileOpAnnot: true,
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			etcdObjMeta := createEtcdObjectMetadata(uuid.NewUUID(), test.annotations, nil, false)
			g.Expect(HasReconcileOperationAnnotation(etcdObjMeta)).To(Equal(test.expectedHasReconcileOpAnnot))
		})
	}
}

func TestRemoveOperationAnnotation(t *testing.T) {
	tests := []struct {
		name                string
		annotations         map[string]string
		expectedAnnotations map[string]string
	}{
		{
			name:                "No annotations are set",
			annotations:         nil,
			expectedAnnotations: nil,
		},
		{
			name:                "No reconcile annotation is set",
			annotations:         map[string]string{"dummy-annot": "dummy-val"},
			expectedAnnotations: map[string]string{"dummy-annot": "dummy-val"},
		},
		{
			name: "Gardener reconcile annotation is set",
			annotations: map[string]string{
				"dummy-annot":               "dummy-val",
				GardenerOperationAnnotation: "reconcile",
			},
			expectedAnnotations: map[string]string{"dummy-annot": "dummy-val"},
		},
		{
			name: "Druid reconcile annotation is set",
			annotations: map[string]string{
				"dummy-annot":            "dummy-val",
				DruidOperationAnnotation: "reconcile",
			},
			expectedAnnotations: map[string]string{"dummy-annot": "dummy-val"},
		},
		{
			name: "Gardener and Druid reconcile annotations are set",
			annotations: map[string]string{
				"dummy-annot":               "dummy-val",
				DruidOperationAnnotation:    "reconcile",
				GardenerOperationAnnotation: "reconcile",
			},
			expectedAnnotations: map[string]string{"dummy-annot": "dummy-val"},
		},
	}
	g := NewWithT(t)
	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			etcdObjMeta := createEtcdObjectMetadata(uuid.NewUUID(), test.annotations, nil, false)
			RemoveOperationAnnotation(etcdObjMeta)
			if test.annotations == nil {
				g.Expect(etcdObjMeta.Annotations).To(BeNil())
			} else {
				g.Expect(len(etcdObjMeta.Annotations)).To(Equal(len(test.expectedAnnotations)))
				g.Expect(reflect.DeepEqual(etcdObjMeta.Annotations, test.expectedAnnotations)).To(BeTrue())
			}
		})
	}
}

func TestGetReconcileOperationAnnotationKey(t *testing.T) {
	tests := []struct {
		name                  string
		annotations           map[string]string
		expectedAnnotationKey *string
	}{
		{
			name:                  "No annotations are set",
			annotations:           nil,
			expectedAnnotationKey: nil,
		},
		{
			name:                  "No reconcile annotation is set",
			annotations:           map[string]string{"dummy-annot": "dummy-val"},
			expectedAnnotationKey: nil,
		},
		{
			name: "Gardener reconcile annotation is set",
			annotations: map[string]string{
				"dummy-annot":               "dummy-val",
				GardenerOperationAnnotation: "reconcile",
			},
			expectedAnnotationKey: ptr.To(GardenerOperationAnnotation),
		},
		{
			name: "Druid reconcile annotation is set",
			annotations: map[string]string{
				"dummy-annot":            "dummy-val",
				DruidOperationAnnotation: "reconcile",
			},
			expectedAnnotationKey: ptr.To(DruidOperationAnnotation),
		},
		{
			name: "Gardener and Druid reconcile annotations are set",
			annotations: map[string]string{
				"dummy-annot":               "dummy-val",
				DruidOperationAnnotation:    "reconcile",
				GardenerOperationAnnotation: "reconcile",
			},
			expectedAnnotationKey: ptr.To(DruidOperationAnnotation),
		},
	}
	g := NewWithT(t)
	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			etcdObjMeta := createEtcdObjectMetadata(uuid.NewUUID(), test.annotations, nil, false)
			actualAnnotKey := GetReconcileOperationAnnotationKey(etcdObjMeta)
			g.Expect(actualAnnotKey).To(Equal(test.expectedAnnotationKey))
		})
	}
}

func TestIsPodManagementEnabled(t *testing.T) {
	tests := []struct {
		name                        string
		hasExternallyManagedMembers bool
		expected                    bool
	}{
		{
			name:                        "Pod management is enabled",
			hasExternallyManagedMembers: false,
			expected:                    true,
		},
		{
			name:                        "Pod management is disabled",
			hasExternallyManagedMembers: true,
			expected:                    false,
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			etcdObjMeta := createEtcdObjectMetadata(uuid.NewUUID(), nil, nil, false)
			etcd := &Etcd{
				ObjectMeta: etcdObjMeta,
				Spec: EtcdSpec{
					Replicas: 3,
					ExternallyManagedMemberAddresses: func() []string {
						if test.hasExternallyManagedMembers {
							return []string{"1.1.1.1", "1.1.1.2", "1.1.1.3"}
						}
						return nil
					}(),
				},
			}
			actual := ArePodsManagedByEtcdDruid(etcd)
			g.Expect(actual).To(Equal(test.expected))
		})
	}
}

func TestIsAdditionalPeerURLConfigured(t *testing.T) {
	tests := []struct {
		name     string
		spec     *AdditionalPeerURLsSpec
		expected bool
	}{
		{
			name:     "nil spec — not configured",
			spec:     nil,
			expected: false,
		},
		{
			name:     "empty Members slice — not configured",
			spec:     &AdditionalPeerURLsSpec{Members: []MemberPeerURLs{}},
			expected: false,
		},
		{
			name: "one member entry — configured",
			spec: &AdditionalPeerURLsSpec{
				Members: []MemberPeerURLs{{Name: "etcd-test-0", URLs: []string{"http://10.0.0.1:2380"}}},
			},
			expected: true,
		},
	}
	g := NewWithT(t)
	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			etcd := &Etcd{Spec: EtcdSpec{Etcd: EtcdConfig{AdditionalAdvertisePeerURLs: test.spec}}}
			g.Expect(IsAdditionalPeerURLConfigured(etcd)).To(Equal(test.expected))
		})
	}
}

func TestIsOverrideDefaultURLEnabled(t *testing.T) {
	tests := []struct {
		name     string
		spec     *AdditionalPeerURLsSpec
		expected bool
	}{
		{
			name:     "nil spec — disabled",
			spec:     nil,
			expected: false,
		},
		{
			name:     "OverrideDefaultURL nil — disabled",
			spec:     &AdditionalPeerURLsSpec{Members: []MemberPeerURLs{{Name: "etcd-test-0", URLs: []string{"http://10.0.0.1:2380"}}}},
			expected: false,
		},
		{
			name: "OverrideDefaultURL=false — disabled",
			spec: &AdditionalPeerURLsSpec{
				OverrideDefaultURL: ptr.To(false),
				Members:            []MemberPeerURLs{{Name: "etcd-test-0", URLs: []string{"http://10.0.0.1:2380"}}},
			},
			expected: false,
		},
		{
			name: "OverrideDefaultURL=true — enabled",
			spec: &AdditionalPeerURLsSpec{
				OverrideDefaultURL: ptr.To(true),
				Members:            []MemberPeerURLs{{Name: "etcd-test-0", URLs: []string{"http://10.0.0.1:2380"}}},
			},
			expected: true,
		},
	}
	g := NewWithT(t)
	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			etcd := &Etcd{Spec: EtcdSpec{Etcd: EtcdConfig{AdditionalAdvertisePeerURLs: test.spec}}}
			g.Expect(IsOverrideDefaultURLEnabled(etcd)).To(Equal(test.expected))
		})
	}
}

func TestGetAdditionalAdvertisePeerURLs(t *testing.T) {
	tests := []struct {
		name             string
		memberNamePrefix *string
		spec             *AdditionalPeerURLsSpec
		podName          string
		expectedURLs     []string
		expectedOverride bool
	}{
		{
			name:             "nil spec — no URLs, no override",
			spec:             nil,
			podName:          "etcd-test-0",
			expectedURLs:     nil,
			expectedOverride: false,
		},
		{
			name: "matching member, override=false",
			spec: &AdditionalPeerURLsSpec{
				OverrideDefaultURL: ptr.To(false),
				Members:            []MemberPeerURLs{{Name: "etcd-test-0", URLs: []string{"http://10.0.0.1:2380"}}},
			},
			podName:          "etcd-test-0",
			expectedURLs:     []string{"http://10.0.0.1:2380"},
			expectedOverride: false,
		},
		{
			name: "matching member, override=true",
			spec: &AdditionalPeerURLsSpec{
				OverrideDefaultURL: ptr.To(true),
				Members:            []MemberPeerURLs{{Name: "etcd-test-0", URLs: []string{"http://10.0.0.1:2380"}}},
			},
			podName:          "etcd-test-0",
			expectedURLs:     []string{"http://10.0.0.1:2380"},
			expectedOverride: true,
		},
		{
			name: "non-matching pod — no URLs, no override",
			spec: &AdditionalPeerURLsSpec{
				OverrideDefaultURL: ptr.To(true),
				Members:            []MemberPeerURLs{{Name: "etcd-test-0", URLs: []string{"http://10.0.0.1:2380"}}},
			},
			podName:          "etcd-test-1",
			expectedURLs:     nil,
			expectedOverride: false,
		},
		{
			name:             "memberNamePrefix applied — matching member found",
			memberNamePrefix: ptr.To("myprefix"),
			spec: &AdditionalPeerURLsSpec{
				OverrideDefaultURL: ptr.To(true),
				Members:            []MemberPeerURLs{{Name: "myprefix-etcd-test-0", URLs: []string{"http://10.0.0.1:2380"}}},
			},
			podName:          "etcd-test-0",
			expectedURLs:     []string{"http://10.0.0.1:2380"},
			expectedOverride: true,
		},
		{
			name:             "memberNamePrefix applied — pod without prefix does not match prefixed entry",
			memberNamePrefix: ptr.To("myprefix"),
			spec: &AdditionalPeerURLsSpec{
				Members: []MemberPeerURLs{{Name: "etcd-test-0", URLs: []string{"http://10.0.0.1:2380"}}},
			},
			podName:          "etcd-test-0",
			expectedURLs:     nil,
			expectedOverride: false,
		},
	}
	g := NewWithT(t)
	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			etcd := &Etcd{
				ObjectMeta: metav1.ObjectMeta{Name: etcdName, Namespace: etcdNamespace},
				Spec: EtcdSpec{
					MemberNamePrefix: test.memberNamePrefix,
					Etcd:             EtcdConfig{AdditionalAdvertisePeerURLs: test.spec},
				},
			}
			urls, override := GetAdditionalAdvertisePeerURLs(etcd, test.podName)
			g.Expect(urls).To(Equal(test.expectedURLs))
			g.Expect(override).To(Equal(test.expectedOverride))
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
