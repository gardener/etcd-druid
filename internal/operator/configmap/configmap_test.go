package configmap

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/operator/resource"
	"github.com/gardener/etcd-druid/internal/utils"
	testutils "github.com/gardener/etcd-druid/test/utils"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
			name:                   "should return the existing congigmap name",
			cmExists:               true,
			expectedConfigMapNames: []string{etcd.GetConfigMapName()},
		},
		{
			name:     "should return error when get client get fails",
			cmExists: true,
			getErr:   testutils.TestAPIInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrGetConfigMap,
				Cause:     testutils.TestAPIInternalErr,
				Operation: "GetExistingResourceNames",
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeClientBuilder := testutils.NewFakeClientBuilder().WithGetError(tc.getErr)
			if tc.cmExists {
				fakeClientBuilder.WithObjects(newConfigMap(g, etcd))
			}
			operator := New(fakeClientBuilder.Build())
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			cmNames, err := operator.GetExistingResourceNames(opCtx, etcd)
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
		name        string
		createErr   *apierrors.StatusError
		expectedErr *druiderr.DruidError
	}{
		{
			name: "should create when no configmap exists",
		},
		{
			name:      "return error when create client request fails",
			createErr: testutils.TestAPIInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrSyncConfigMap,
				Cause:     testutils.TestAPIInternalErr,
				Operation: "Sync",
			},
		},
	}
	g := NewWithT(t)
	t.Parallel()
	etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).WithClientTLS().WithPeerTLS().Build()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cl := testutils.NewFakeClientBuilder().WithCreateError(tc.createErr).Build()
			operator := New(cl)
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
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
				Operation: "Sync",
			},
		},
	}
	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			originalEtcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).WithClientTLS().Build()
			cl := testutils.NewFakeClientBuilder().
				WithPatchError(tc.patchErr).
				WithObjects(newConfigMap(g, originalEtcd)).
				Build()
			updatedEtcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).WithClientTLS().WithPeerTLS().Build()
			updatedEtcd.UID = originalEtcd.UID
			operator := New(cl)
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
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
				Operation: "TriggerDelete",
			},
		},
	}
	g := NewWithT(t)
	t.Parallel()

	etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).Build()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// ********************* Setup *********************
			cl := testutils.NewFakeClientBuilder().WithDeleteError(tc.deleteErr).Build()
			operator := New(cl)
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			if tc.cmExists {
				syncErr := operator.Sync(opCtx, etcd)
				g.Expect(syncErr).ToNot(HaveOccurred())
				ensureConfigMapExists(g, cl, etcd)
			}
			// ********************* Test trigger delete *********************
			triggerDeleteErr := operator.TriggerDelete(opCtx, etcd)
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
	cm := emptyConfigMap(getObjectKey(etcd))
	err := buildResource(etcd, cm)
	g.Expect(err).ToNot(HaveOccurred())
	return cm
}

func ensureConfigMapExists(g *WithT, cl client.WithWatch, etcd *druidv1alpha1.Etcd) {
	cm, err := getLatestConfigMap(cl, etcd)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(cm).ToNot(BeNil())
}

func getLatestConfigMap(cl client.Client, etcd *druidv1alpha1.Etcd) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{}
	err := cl.Get(context.Background(), client.ObjectKey{Name: etcd.GetConfigMapName(), Namespace: etcd.Namespace}, cm)
	return cm, err
}

func matchConfigMap(g *WithT, etcd *druidv1alpha1.Etcd, actualConfigMap corev1.ConfigMap) {
	expectedLabels := utils.MergeMaps[string, string](etcd.GetDefaultLabels(), map[string]string{
		druidv1alpha1.LabelComponentKey: common.ConfigMapComponentName,
		druidv1alpha1.LabelAppNameKey:   etcd.GetConfigMapName(),
	})
	g.Expect(actualConfigMap).To(MatchFields(IgnoreExtras|IgnoreMissing, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras|IgnoreMissing, Fields{
			"Name":            Equal(etcd.GetConfigMapName()),
			"Namespace":       Equal(etcd.Namespace),
			"Labels":          testutils.MatchResourceLabels(expectedLabels),
			"OwnerReferences": testutils.MatchEtcdOwnerReference(etcd.Name, etcd.UID),
		}),
		"Spec": MatchFields(IgnoreExtras|IgnoreMissing, Fields{
			"Data": Not(BeNil()),
		}),
	}))
	// Validate the etcd config data
	actualETCDConfigYAML := actualConfigMap.Data[etcdConfigKey]
	actualETCDConfig := make(map[string]interface{})
	err := yaml.Unmarshal([]byte(actualETCDConfigYAML), &actualETCDConfig)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(actualETCDConfig).To(MatchKeys(IgnoreExtras|IgnoreMissing, Keys{
		"name":                      Equal(fmt.Sprintf("etcd-%s", etcd.UID[:6])),
		"data-dir":                  Equal("/var/etcd/data/new.etcd"),
		"metrics":                   Equal(string(druidv1alpha1.Basic)),
		"snapshot-count":            Equal(int64(75000)),
		"enable-v2":                 Equal(false),
		"quota-backend-bytes":       Equal(etcd.Spec.Etcd.Quota.Value()),
		"initial-cluster-token":     Equal("etcd-cluster"),
		"initial-cluster-state":     Equal("new"),
		"auto-compaction-mode":      Equal(string(utils.TypeDeref[druidv1alpha1.CompactionMode](etcd.Spec.Common.AutoCompactionMode, druidv1alpha1.Periodic))),
		"auto-compaction-retention": Equal(utils.TypeDeref[string](etcd.Spec.Common.AutoCompactionRetention, defaultAutoCompactionRetention)),
	}))
	matchClientTLSRelatedConfiguration(g, etcd, actualETCDConfig)
	matchPeerTLSRelatedConfiguration(g, etcd, actualETCDConfig)
}

func matchClientTLSRelatedConfiguration(g *WithT, etcd *druidv1alpha1.Etcd, actualETCDConfig map[string]interface{}) {
	if etcd.Spec.Etcd.ClientUrlTLS != nil {
		g.Expect(actualETCDConfig).To(MatchKeys(IgnoreExtras|IgnoreMissing, Keys{
			"listen-client-urls": Equal(fmt.Sprintf("https://0.0.0.0:%d", utils.TypeDeref[int32](etcd.Spec.Etcd.ClientPort, defaultClientPort))),
			"client-transport-security": MatchKeys(IgnoreExtras, Keys{
				"cert-file":        Equal("/var/etcd/ssl/client/server/tls.crt"),
				"key-file":         Equal("/var/etcd/ssl/client/server/tls.key"),
				"client-cert-auth": Equal(true),
				"trusted-ca-file":  Equal("/var/etcd/ssl/client/ca/ca.crt"),
				"auto-tls":         Equal(false),
			}),
		}))
	} else {
		g.Expect(actualETCDConfig).To(MatchKeys(IgnoreExtras|IgnoreMissing, Keys{
			"listen-client-urls": Equal(fmt.Sprintf("http://0.0.0.0:%d", utils.TypeDeref[int32](etcd.Spec.Etcd.ClientPort, defaultClientPort))),
		}))
		g.Expect(actualETCDConfig).ToNot(HaveKey("client-transport-security"))
	}
}

func matchPeerTLSRelatedConfiguration(g *WithT, etcd *druidv1alpha1.Etcd, actualETCDConfig map[string]interface{}) {
	if etcd.Spec.Etcd.PeerUrlTLS != nil {
		g.Expect(actualETCDConfig).To(MatchKeys(IgnoreExtras|IgnoreMissing, Keys{
			"peer-transport-security": MatchKeys(IgnoreExtras, Keys{
				"cert-file":        Equal("/var/etcd/ssl/peer/server/tls.crt"),
				"key-file":         Equal("/var/etcd/ssl/peer/server/tls.key"),
				"client-cert-auth": Equal(true),
				"trusted-ca-file":  Equal("/var/etcd/ssl/peer/ca/ca.crt"),
				"auto-tls":         Equal(false),
			}),
			"advertise-client-urls":       Equal(fmt.Sprintf("https@%s@%s@%d", etcd.GetPeerServiceName(), etcd.Namespace, utils.TypeDeref[int32](etcd.Spec.Etcd.ClientPort, defaultClientPort))),
			"listen-peer-urls":            Equal(fmt.Sprintf("https://0.0.0.0:%d", utils.TypeDeref[int32](etcd.Spec.Etcd.ServerPort, defaultServerPort))),
			"initial-advertise-peer-urls": Equal(fmt.Sprintf("https@%s@%s@%s", etcd.GetPeerServiceName(), etcd.Namespace, strconv.Itoa(int(pointer.Int32Deref(etcd.Spec.Etcd.ServerPort, defaultServerPort))))),
		}))
	} else {
		g.Expect(actualETCDConfig).To(MatchKeys(IgnoreExtras|IgnoreMissing, Keys{
			"advertise-client-urls":       Equal(fmt.Sprintf("http@%s@%s@%d", etcd.GetPeerServiceName(), etcd.Namespace, utils.TypeDeref[int32](etcd.Spec.Etcd.ClientPort, defaultClientPort))),
			"listen-peer-urls":            Equal(fmt.Sprintf("http://0.0.0.0:%d", utils.TypeDeref[int32](etcd.Spec.Etcd.ServerPort, defaultServerPort))),
			"initial-advertise-peer-urls": Equal(fmt.Sprintf("http@%s@%s@%s", etcd.GetPeerServiceName(), etcd.Namespace, strconv.Itoa(int(pointer.Int32Deref(etcd.Spec.Etcd.ServerPort, defaultServerPort))))),
		}))
		g.Expect(actualETCDConfig).ToNot(HaveKey("peer-transport-security"))
	}
}
