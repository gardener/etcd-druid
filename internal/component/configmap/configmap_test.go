// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package configmap

import (
	"context"
	"fmt"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/component"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/utils"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

// ------------------------ GetExistingResourceNames ------------------------
func TestGetExistingResourceNames(t *testing.T) {
	etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).Build()
	testCases := []struct {
		name                   string
		cmExists               bool
		getErr                 *apierrors.StatusError
		expectedErr            *druiderr.DruidError
		expectedConfigMapNames []string
	}{
		{
			name:                   "should return empty slice when no configmap found",
			cmExists:               false,
			expectedConfigMapNames: []string{},
		},
		{
			name:                   "should return the existing configmap name",
			cmExists:               true,
			expectedConfigMapNames: []string{druidv1alpha1.GetConfigMapName(etcd.ObjectMeta)},
		},
		{
			name:     "should return error when get client get fails",
			cmExists: true,
			getErr:   testutils.TestAPIInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrGetConfigMap,
				Cause:     testutils.TestAPIInternalErr,
				Operation: component.OperationGetExistingResourceNames,
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var existingObjects []client.Object
			if tc.cmExists {
				existingObjects = append(existingObjects, newConfigMap(g, etcd))
			}
			cl := testutils.CreateTestFakeClientForObjects(tc.getErr, nil, nil, nil, existingObjects, getObjectKey(etcd.ObjectMeta))
			operator := New(cl)
			opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			cmNames, err := operator.GetExistingResourceNames(opCtx, etcd.ObjectMeta)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, err)
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(cmNames).To(Equal(tc.expectedConfigMapNames))
			}
		})
	}
}

// ----------------------------------- Sync -----------------------------------
func TestSyncWhenNoConfigMapExists(t *testing.T) {
	testCases := []struct {
		name             string
		etcdReplicas     int32
		createErr        *apierrors.StatusError
		clientTLSEnabled bool
		peerTLSEnabled   bool
		expectedErr      *druiderr.DruidError
	}{
		{
			name:             "should create when no configmap exists for single node etcd cluster",
			clientTLSEnabled: true,
			peerTLSEnabled:   false,
			etcdReplicas:     1,
		},
		{
			name:             "should create when no configmap exists for multi-node etcd cluster",
			clientTLSEnabled: true,
			peerTLSEnabled:   true,
			etcdReplicas:     3,
		},
		{
			name:             "return error when create client request fails",
			etcdReplicas:     3,
			clientTLSEnabled: true,
			peerTLSEnabled:   true,
			createErr:        testutils.TestAPIInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrSyncConfigMap,
				Cause:     testutils.TestAPIInternalErr,
				Operation: component.OperationSync,
			},
		},
	}
	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			etcd := buildEtcd(tc.etcdReplicas, tc.clientTLSEnabled, tc.peerTLSEnabled)
			cl := testutils.CreateTestFakeClientForObjects(nil, tc.createErr, nil, nil, nil, getObjectKey(etcd.ObjectMeta))
			operator := New(cl)
			opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			err := operator.Sync(opCtx, etcd)
			latestConfigMap, getErr := getLatestConfigMap(cl, etcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, err)
				g.Expect(apierrors.IsNotFound(getErr)).To(BeTrue())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(latestConfigMap).ToNot(BeNil())
				matchConfigMap(g, etcd, *latestConfigMap)
			}
		})
	}
}

func TestPrepareInitialCluster(t *testing.T) {
	testCases := []struct {
		name                   string
		peerTLSEnabled         bool
		etcdReplicas           int32
		etcdSpecServerPort     *int32
		expectedInitialCluster string
	}{
		{
			name:                   "should create initial cluster for single node etcd cluster when peer TLS is enabled",
			etcdReplicas:           1,
			peerTLSEnabled:         true,
			etcdSpecServerPort:     ptr.To[int32](2222),
			expectedInitialCluster: "etcd-test-0=https://etcd-test-0.etcd-test-peer.test-ns.svc:2222",
		},
		{
			name:                   "should create initial cluster for single node etcd cluster when peer TLS is disabled",
			etcdReplicas:           1,
			peerTLSEnabled:         false,
			expectedInitialCluster: "etcd-test-0=http://etcd-test-0.etcd-test-peer.test-ns.svc:2380",
		},
		{
			name:                   "should create initial cluster for multi node etcd cluster when peer TLS is enabled",
			etcdReplicas:           3,
			peerTLSEnabled:         true,
			etcdSpecServerPort:     ptr.To[int32](2333),
			expectedInitialCluster: "etcd-test-0=https://etcd-test-0.etcd-test-peer.test-ns.svc:2333,etcd-test-1=https://etcd-test-1.etcd-test-peer.test-ns.svc:2333,etcd-test-2=https://etcd-test-2.etcd-test-peer.test-ns.svc:2333",
		},
	}
	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			etcd := buildEtcd(tc.etcdReplicas, true, tc.peerTLSEnabled)
			etcd.Spec.Etcd.ServerPort = tc.etcdSpecServerPort
			peerScheme := utils.IfConditionOr(etcd.Spec.Etcd.PeerUrlTLS != nil, "https", "http")
			actualInitialCluster := prepareInitialCluster(etcd, peerScheme)
			g.Expect(actualInitialCluster).To(Equal(tc.expectedInitialCluster))
		})
	}
}

func buildEtcd(replicas int32, clientTLSEnabled, peerTLSEnabled bool) *druidv1alpha1.Etcd {
	etcdBuilder := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).WithReplicas(replicas)
	if clientTLSEnabled {
		etcdBuilder.WithClientTLS()
	}
	if peerTLSEnabled {
		etcdBuilder.WithPeerTLS()
	}
	return etcdBuilder.Build()
}

func TestSyncWhenConfigMapExists(t *testing.T) {
	testCases := []struct {
		name        string
		patchErr    *apierrors.StatusError
		expectedErr *druiderr.DruidError
	}{
		{
			name: "update configmap when peer TLS communication is enabled",
		},
		{
			name:     "returns error when patch client request fails",
			patchErr: testutils.TestAPIInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrSyncConfigMap,
				Cause:     testutils.TestAPIInternalErr,
				Operation: component.OperationSync,
			},
		},
	}
	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			originalEtcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).WithClientTLS().Build()
			cl := testutils.CreateTestFakeClientForObjects(nil, nil, tc.patchErr, nil, []client.Object{newConfigMap(g, originalEtcd)}, getObjectKey(originalEtcd.ObjectMeta))
			updatedEtcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).WithClientTLS().WithPeerTLS().Build()
			updatedEtcd.UID = originalEtcd.UID
			operator := New(cl)
			opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			err := operator.Sync(opCtx, updatedEtcd)
			latestConfigMap, getErr := getLatestConfigMap(cl, updatedEtcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, err)
				g.Expect(getErr).NotTo(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(getErr).NotTo(HaveOccurred())
				g.Expect(latestConfigMap).ToNot(BeNil())
				matchConfigMap(g, updatedEtcd, *latestConfigMap)
			}
		})
	}
}

// ----------------------------- TriggerDelete -------------------------------
func TestTriggerDelete(t *testing.T) {
	testCases := []struct {
		name        string
		cmExists    bool
		deleteErr   *apierrors.StatusError
		expectedErr *druiderr.DruidError
	}{
		{
			name:     "no-op when configmap does not exist",
			cmExists: false,
		},
		{
			name:     "successfully delete existing configmap",
			cmExists: true,
		},
		{
			name:      "return error when client delete fails",
			cmExists:  true,
			deleteErr: testutils.TestAPIInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrDeleteConfigMap,
				Cause:     testutils.TestAPIInternalErr,
				Operation: component.OperationTriggerDelete,
			},
		},
	}
	g := NewWithT(t)
	t.Parallel()

	etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).Build()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// ********************* Setup *********************
			var existingObjects []client.Object
			if tc.cmExists {
				existingObjects = append(existingObjects, newConfigMap(g, etcd))
			}
			cl := testutils.CreateTestFakeClientForObjects(nil, nil, nil, tc.deleteErr, existingObjects, getObjectKey(etcd.ObjectMeta))
			operator := New(cl)
			opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			// ********************* Test trigger delete *********************
			triggerDeleteErr := operator.TriggerDelete(opCtx, etcd.ObjectMeta)
			latestConfigMap, getErr := getLatestConfigMap(cl, etcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, triggerDeleteErr)
				g.Expect(getErr).To(BeNil())
				g.Expect(latestConfigMap).ToNot(BeNil())
			} else {
				g.Expect(triggerDeleteErr).NotTo(HaveOccurred())
				g.Expect(apierrors.IsNotFound(getErr)).To(BeTrue())
			}
		})
	}
}

// ---------------------------- Helper Functions -----------------------------
func newConfigMap(g *WithT, etcd *druidv1alpha1.Etcd) *corev1.ConfigMap {
	cm := emptyConfigMap(getObjectKey(etcd.ObjectMeta))
	err := buildResource(etcd, cm)
	g.Expect(err).ToNot(HaveOccurred())
	return cm
}

func getLatestConfigMap(cl client.Client, etcd *druidv1alpha1.Etcd) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{}
	err := cl.Get(context.Background(), client.ObjectKey{Name: druidv1alpha1.GetConfigMapName(etcd.ObjectMeta), Namespace: etcd.Namespace}, cm)
	return cm, err
}

func matchConfigMap(g *WithT, etcd *druidv1alpha1.Etcd, actualConfigMap corev1.ConfigMap) {
	etcdObjMeta := etcd.ObjectMeta
	expectedLabels := utils.MergeMaps(druidv1alpha1.GetDefaultLabels(etcdObjMeta), map[string]string{
		druidv1alpha1.LabelComponentKey: common.ComponentNameConfigMap,
		druidv1alpha1.LabelAppNameKey:   druidv1alpha1.GetConfigMapName(etcdObjMeta),
	})
	g.Expect(actualConfigMap).To(MatchFields(IgnoreExtras|IgnoreMissing, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras|IgnoreMissing, Fields{
			"Name":            Equal(druidv1alpha1.GetConfigMapName(etcdObjMeta)),
			"Namespace":       Equal(etcd.Namespace),
			"Labels":          testutils.MatchResourceLabels(expectedLabels),
			"OwnerReferences": testutils.MatchEtcdOwnerReference(etcd.Name, etcd.UID),
		}),
		"Spec": MatchFields(IgnoreExtras|IgnoreMissing, Fields{
			"Data": Not(BeNil()),
		}),
	}))
	// Validate the etcd config data
	actualETCDConfigYAML := actualConfigMap.Data[common.EtcdConfigFileName]
	actualETCDConfig := make(map[string]any)
	err := yaml.Unmarshal([]byte(actualETCDConfigYAML), &actualETCDConfig)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(actualETCDConfig).To(MatchKeys(IgnoreExtras|IgnoreMissing, Keys{
		"name":                      Equal("etcd-config"),
		"data-dir":                  Equal(fmt.Sprintf("%s/new.etcd", common.VolumeMountPathEtcdData)),
		"metrics":                   Equal(string(druidv1alpha1.Basic)),
		"snapshot-count":            Equal(ptr.Deref(etcd.Spec.Etcd.SnapshotCount, defaultSnapshotCount)),
		"enable-v2":                 Equal(false),
		"quota-backend-bytes":       Equal(etcd.Spec.Etcd.Quota.Value()),
		"initial-cluster-token":     Equal("etcd-cluster"),
		"initial-cluster-state":     Equal("new"),
		"auto-compaction-mode":      Equal(string(ptr.Deref(etcd.Spec.Common.AutoCompactionMode, druidv1alpha1.Periodic))),
		"auto-compaction-retention": Equal(ptr.Deref(etcd.Spec.Common.AutoCompactionRetention, defaultAutoCompactionRetention)),
	}))
	matchClientTLSRelatedConfiguration(g, etcd, actualETCDConfig)
	matchPeerTLSRelatedConfiguration(g, etcd, actualETCDConfig)
}

func matchClientTLSRelatedConfiguration(g *WithT, etcd *druidv1alpha1.Etcd, actualETCDConfig map[string]any) {
	if etcd.Spec.Etcd.ClientUrlTLS != nil {
		g.Expect(actualETCDConfig).To(MatchKeys(IgnoreExtras|IgnoreMissing, Keys{
			"listen-client-urls":    Equal(fmt.Sprintf("https://0.0.0.0:%d", ptr.Deref(etcd.Spec.Etcd.ClientPort, common.DefaultPortEtcdClient))),
			"advertise-client-urls": Equal(expectedAdvertiseURLsAsInterface(etcd, advertiseURLTypeClient, "https")),
			"client-transport-security": MatchKeys(IgnoreExtras, Keys{
				"cert-file":        Equal("/var/etcd/ssl/server/tls.crt"),
				"key-file":         Equal("/var/etcd/ssl/server/tls.key"),
				"client-cert-auth": Equal(true),
				"trusted-ca-file":  Equal("/var/etcd/ssl/ca/ca.crt"),
				"auto-tls":         Equal(false),
			}),
		}))
	} else {
		g.Expect(actualETCDConfig).To(MatchKeys(IgnoreExtras|IgnoreMissing, Keys{
			"listen-client-urls": Equal(fmt.Sprintf("http://0.0.0.0:%d", ptr.Deref(etcd.Spec.Etcd.ClientPort, common.DefaultPortEtcdClient))),
		}))
		g.Expect(actualETCDConfig).ToNot(HaveKey("client-transport-security"))
	}
}

func expectedAdvertiseURLs(etcd *druidv1alpha1.Etcd, advertiseURLType, scheme string) map[string][]string {
	var port int32
	switch advertiseURLType {
	case advertiseURLTypePeer:
		port = ptr.Deref(etcd.Spec.Etcd.ServerPort, common.DefaultPortEtcdPeer)
	case advertiseURLTypeClient:
		port = ptr.Deref(etcd.Spec.Etcd.ClientPort, common.DefaultPortEtcdClient)
	default:
		return nil
	}
	advUrlsMap := make(map[string][]string)
	for i := 0; i < int(etcd.Spec.Replicas); i++ {
		podName := druidv1alpha1.GetOrdinalPodName(etcd.ObjectMeta, i)
		advUrlsMap[podName] = []string{fmt.Sprintf("%s://%s.%s.%s.svc:%d", scheme, podName, druidv1alpha1.GetPeerServiceName(etcd.ObjectMeta), etcd.Namespace, port)}
	}
	return advUrlsMap
}

func expectedAdvertiseURLsAsInterface(etcd *druidv1alpha1.Etcd, advertiseURLType, scheme string) map[string]any {
	advertiseUrlsMap := expectedAdvertiseURLs(etcd, advertiseURLType, scheme)
	advertiseUrlsInterface := make(map[string]any, len(advertiseUrlsMap))
	for podName, urlList := range advertiseUrlsMap {
		urlsListInterface := make([]any, len(urlList))
		for i, url := range urlList {
			urlsListInterface[i] = url
		}
		advertiseUrlsInterface[podName] = urlsListInterface
	}
	return advertiseUrlsInterface
}

func matchPeerTLSRelatedConfiguration(g *WithT, etcd *druidv1alpha1.Etcd, actualETCDConfig map[string]any) {
	if etcd.Spec.Etcd.PeerUrlTLS != nil {
		g.Expect(actualETCDConfig).To(MatchKeys(IgnoreExtras|IgnoreMissing, Keys{
			"peer-transport-security": MatchKeys(IgnoreExtras, Keys{
				"cert-file":        Equal("/var/etcd/ssl/peer/server/tls.crt"),
				"key-file":         Equal("/var/etcd/ssl/peer/server/tls.key"),
				"client-cert-auth": Equal(true),
				"trusted-ca-file":  Equal("/var/etcd/ssl/peer/ca/ca.crt"),
				"auto-tls":         Equal(false),
			}),
			"listen-peer-urls":            Equal(fmt.Sprintf("https://0.0.0.0:%d", ptr.Deref(etcd.Spec.Etcd.ServerPort, common.DefaultPortEtcdPeer))),
			"initial-advertise-peer-urls": Equal(expectedAdvertiseURLsAsInterface(etcd, advertiseURLTypePeer, "https")),
		}))
	} else {
		g.Expect(actualETCDConfig).To(MatchKeys(IgnoreExtras|IgnoreMissing, Keys{
			"listen-peer-urls":            Equal(fmt.Sprintf("http://0.0.0.0:%d", ptr.Deref(etcd.Spec.Etcd.ServerPort, common.DefaultPortEtcdPeer))),
			"initial-advertise-peer-urls": Equal(expectedAdvertiseURLsAsInterface(etcd, advertiseURLTypePeer, "http")),
		}))
		g.Expect(actualETCDConfig).ToNot(HaveKey("peer-transport-security"))
	}
}
