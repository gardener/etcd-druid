// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package configmap

import (
	"context"
	"fmt"
	"strings"
	"testing"

	druidconfigv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"
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
		memberNamePrefix *string
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
			name:             "should create configmap with member-name-prefix when set",
			clientTLSEnabled: true,
			peerTLSEnabled:   false,
			etcdReplicas:     1,
			memberNamePrefix: ptr.To("test-prefix"),
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
			etcd.Spec.MemberNamePrefix = tc.memberNamePrefix
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
		name                        string
		peerTLSEnabled              bool
		etcdReplicas                int32
		etcdSpecServerPort          *int32
		additionalAdvertisePeerURLs []druidv1alpha1.MemberPeerURLs
		memberNamePrefix            *string
		expectedInitialCluster      string
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
		{
			name:           "should append additional peer URLs for matching member",
			etcdReplicas:   2,
			peerTLSEnabled: false,
			additionalAdvertisePeerURLs: []druidv1alpha1.MemberPeerURLs{
				{
					MemberName: "etcd-test-0",
					URLs:       []string{"http://10.0.0.1:2380"},
				},
			},
			expectedInitialCluster: "etcd-test-0=http://etcd-test-0.etcd-test-peer.test-ns.svc:2380,etcd-test-0=http://10.0.0.1:2380,etcd-test-1=http://etcd-test-1.etcd-test-peer.test-ns.svc:2380",
		},
		{
			name:           "should append multiple additional peer URLs for single member",
			etcdReplicas:   2,
			peerTLSEnabled: true,
			additionalAdvertisePeerURLs: []druidv1alpha1.MemberPeerURLs{
				{
					MemberName: "etcd-test-1",
					URLs:       []string{"https://lb-1.example.com:2380", "https://lb-1-backup.example.com:2380"},
				},
			},
			expectedInitialCluster: "etcd-test-0=https://etcd-test-0.etcd-test-peer.test-ns.svc:2380,etcd-test-1=https://etcd-test-1.etcd-test-peer.test-ns.svc:2380,etcd-test-1=https://lb-1.example.com:2380,etcd-test-1=https://lb-1-backup.example.com:2380",
		},
		{
			name:           "should append additional URLs for multiple members",
			etcdReplicas:   3,
			peerTLSEnabled: false,
			additionalAdvertisePeerURLs: []druidv1alpha1.MemberPeerURLs{
				{
					MemberName: "etcd-test-0",
					URLs:       []string{"http://10.0.0.1:2380"},
				},
				{
					MemberName: "etcd-test-2",
					URLs:       []string{"http://10.0.0.3:2380"},
				},
			},
			expectedInitialCluster: "etcd-test-0=http://etcd-test-0.etcd-test-peer.test-ns.svc:2380,etcd-test-0=http://10.0.0.1:2380,etcd-test-1=http://etcd-test-1.etcd-test-peer.test-ns.svc:2380,etcd-test-2=http://etcd-test-2.etcd-test-peer.test-ns.svc:2380,etcd-test-2=http://10.0.0.3:2380",
		},
		{
			name:           "should ignore non-matching member names",
			etcdReplicas:   2,
			peerTLSEnabled: false,
			additionalAdvertisePeerURLs: []druidv1alpha1.MemberPeerURLs{
				{
					MemberName: "non-existing-member",
					URLs:       []string{"http://10.0.0.99:2380"},
				},
			},
			expectedInitialCluster: "etcd-test-0=http://etcd-test-0.etcd-test-peer.test-ns.svc:2380,etcd-test-1=http://etcd-test-1.etcd-test-peer.test-ns.svc:2380",
		},
		{
			name:                   "should create initial cluster with member name prefix for multi node etcd cluster",
			etcdReplicas:           3,
			peerTLSEnabled:         true,
			etcdSpecServerPort:     ptr.To[int32](2333),
			memberNamePrefix:       ptr.To("test-prefix"),
			expectedInitialCluster: "test-prefix-etcd-test-0=https://etcd-test-0.etcd-test-peer.test-ns.svc:2333,test-prefix-etcd-test-1=https://etcd-test-1.etcd-test-peer.test-ns.svc:2333,test-prefix-etcd-test-2=https://etcd-test-2.etcd-test-peer.test-ns.svc:2333",
		},
		{
			name:             "should use member name prefix in both primary and additional peer URL entries",
			etcdReplicas:     2,
			peerTLSEnabled:   false,
			memberNamePrefix: ptr.To("myprefix"),
			additionalAdvertisePeerURLs: []druidv1alpha1.MemberPeerURLs{
				{
					MemberName: "etcd-test-0",
					URLs:       []string{"http://10.0.0.1:2380"},
				},
			},
			expectedInitialCluster: "myprefix-etcd-test-0=http://etcd-test-0.etcd-test-peer.test-ns.svc:2380,myprefix-etcd-test-0=http://10.0.0.1:2380,myprefix-etcd-test-1=http://etcd-test-1.etcd-test-peer.test-ns.svc:2380",
		},
	}
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)
			etcd := buildEtcd(tc.etcdReplicas, true, tc.peerTLSEnabled)
			etcd.Spec.Etcd.ServerPort = tc.etcdSpecServerPort
			etcd.Spec.MemberNamePrefix = tc.memberNamePrefix
			if tc.additionalAdvertisePeerURLs != nil {
				etcd.Spec.Etcd.AdditionalAdvertisePeerURLs = tc.additionalAdvertisePeerURLs
			}
			peerScheme := utils.IfConditionOr(etcd.Spec.Etcd.PeerUrlTLS != nil, "https", "http")
			actualInitialCluster := prepareInitialCluster(etcd, peerScheme)
			g.Expect(actualInitialCluster).To(Equal(tc.expectedInitialCluster))
		})
	}
}

func TestGetAdvertiseURLs(t *testing.T) {
	testCases := []struct {
		name                        string
		etcdReplicas                int32
		peerTLSEnabled              bool
		advertiseURLType            string
		scheme                      string
		serverPort                  *int32
		clientPort                  *int32
		memberNamePrefix            *string
		additionalAdvertisePeerURLs []druidv1alpha1.MemberPeerURLs
		expectedURLs                map[string][]string
	}{
		{
			name:             "should return peer advertise URLs without prefix",
			etcdReplicas:     3,
			advertiseURLType: advertiseURLTypePeer,
			scheme:           "https",
			serverPort:       ptr.To[int32](2380),
			expectedURLs: map[string][]string{
				"etcd-test-0": {"https://etcd-test-0.etcd-test-peer.test-ns.svc:2380"},
				"etcd-test-1": {"https://etcd-test-1.etcd-test-peer.test-ns.svc:2380"},
				"etcd-test-2": {"https://etcd-test-2.etcd-test-peer.test-ns.svc:2380"},
			},
		},
		{
			name:             "should return peer advertise URLs with member name prefix",
			etcdReplicas:     3,
			advertiseURLType: advertiseURLTypePeer,
			scheme:           "https",
			serverPort:       ptr.To[int32](2380),
			memberNamePrefix: ptr.To("test-prefix"),
			expectedURLs: map[string][]string{
				"test-prefix-etcd-test-0": {"https://etcd-test-0.etcd-test-peer.test-ns.svc:2380"},
				"test-prefix-etcd-test-1": {"https://etcd-test-1.etcd-test-peer.test-ns.svc:2380"},
				"test-prefix-etcd-test-2": {"https://etcd-test-2.etcd-test-peer.test-ns.svc:2380"},
			},
		},
		{
			name:             "should return client advertise URLs with member name prefix",
			etcdReplicas:     1,
			advertiseURLType: advertiseURLTypeClient,
			scheme:           "https",
			clientPort:       ptr.To[int32](2379),
			memberNamePrefix: ptr.To("test-prefix"),
			expectedURLs: map[string][]string{
				"test-prefix-etcd-test-0": {"https://etcd-test-0.etcd-test-peer.test-ns.svc:2379"},
			},
		},
		{
			name:             "should return nil for unknown advertise URL type",
			etcdReplicas:     1,
			advertiseURLType: "unknown",
			scheme:           "https",
			expectedURLs:     nil,
		},
		{
			name:             "should return peer URLs without additional URLs",
			etcdReplicas:     2,
			peerTLSEnabled:   true,
			advertiseURLType: advertiseURLTypePeer,
			expectedURLs: map[string][]string{
				"etcd-test-0": {"https://etcd-test-0.etcd-test-peer.test-ns.svc:2380"},
				"etcd-test-1": {"https://etcd-test-1.etcd-test-peer.test-ns.svc:2380"},
			},
		},
		{
			name:             "should return client URLs without additional URLs",
			etcdReplicas:     2,
			peerTLSEnabled:   false,
			advertiseURLType: advertiseURLTypeClient,
			expectedURLs: map[string][]string{
				"etcd-test-0": {"http://etcd-test-0.etcd-test-peer.test-ns.svc:2379"},
				"etcd-test-1": {"http://etcd-test-1.etcd-test-peer.test-ns.svc:2379"},
			},
		},
		{
			name:             "should append additional peer URLs for peer type",
			etcdReplicas:     2,
			peerTLSEnabled:   false,
			advertiseURLType: advertiseURLTypePeer,
			additionalAdvertisePeerURLs: []druidv1alpha1.MemberPeerURLs{
				{
					MemberName: "etcd-test-0",
					URLs:       []string{"http://10.0.0.1:2380"},
				},
			},
			expectedURLs: map[string][]string{
				"etcd-test-0": {"http://etcd-test-0.etcd-test-peer.test-ns.svc:2380", "http://10.0.0.1:2380"},
				"etcd-test-1": {"http://etcd-test-1.etcd-test-peer.test-ns.svc:2380"},
			},
		},
		{
			name:             "should not append additional peer URLs for client type",
			etcdReplicas:     2,
			peerTLSEnabled:   false,
			advertiseURLType: advertiseURLTypeClient,
			additionalAdvertisePeerURLs: []druidv1alpha1.MemberPeerURLs{
				{
					MemberName: "etcd-test-0",
					URLs:       []string{"http://10.0.0.1:2380"},
				},
			},
			expectedURLs: map[string][]string{
				"etcd-test-0": {"http://etcd-test-0.etcd-test-peer.test-ns.svc:2379"},
				"etcd-test-1": {"http://etcd-test-1.etcd-test-peer.test-ns.svc:2379"},
			},
		},
		{
			name:             "should append multiple additional peer URLs",
			etcdReplicas:     2,
			peerTLSEnabled:   true,
			advertiseURLType: advertiseURLTypePeer,
			additionalAdvertisePeerURLs: []druidv1alpha1.MemberPeerURLs{
				{
					MemberName: "etcd-test-1",
					URLs:       []string{"https://lb-1.example.com:2380", "https://lb-1-backup.example.com:2380"},
				},
			},
			expectedURLs: map[string][]string{
				"etcd-test-0": {"https://etcd-test-0.etcd-test-peer.test-ns.svc:2380"},
				"etcd-test-1": {"https://etcd-test-1.etcd-test-peer.test-ns.svc:2380", "https://lb-1.example.com:2380", "https://lb-1-backup.example.com:2380"},
			},
		},
		{
			name:             "should ignore non-matching member names",
			etcdReplicas:     2,
			peerTLSEnabled:   false,
			advertiseURLType: advertiseURLTypePeer,
			additionalAdvertisePeerURLs: []druidv1alpha1.MemberPeerURLs{
				{
					MemberName: "non-existing-member",
					URLs:       []string{"http://10.0.0.99:2380"},
				},
			},
			expectedURLs: map[string][]string{
				"etcd-test-0": {"http://etcd-test-0.etcd-test-peer.test-ns.svc:2380"},
				"etcd-test-1": {"http://etcd-test-1.etcd-test-peer.test-ns.svc:2380"},
			},
		},
	}
	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			etcd := buildEtcd(tc.etcdReplicas, true, tc.peerTLSEnabled)
			etcd.Spec.Etcd.ServerPort = tc.serverPort
			etcd.Spec.Etcd.ClientPort = tc.clientPort
			etcd.Spec.MemberNamePrefix = tc.memberNamePrefix
			if tc.additionalAdvertisePeerURLs != nil {
				etcd.Spec.Etcd.AdditionalAdvertisePeerURLs = tc.additionalAdvertisePeerURLs
			}
			scheme := tc.scheme
			if scheme == "" {
				scheme = utils.IfConditionOr(etcd.Spec.Etcd.PeerUrlTLS != nil, "https", "http")
			}
			peerSvcName := druidv1alpha1.GetPeerServiceName(etcd.ObjectMeta)
			actualURLs := getAdvertiseURLs(etcd, tc.advertiseURLType, scheme, peerSvcName)
			g.Expect(actualURLs).To(Equal(tc.expectedURLs))
		})
	}
}

func TestPrepareInitialClusterWithBootstrapMembers(t *testing.T) {
	g := NewWithT(t)
	t.Parallel()

	t.Run("should append source cluster members to initial-cluster", func(t *testing.T) {
		t.Parallel()
		etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).WithReplicas(3).WithPeerTLS().Build()
		etcd.Spec.Etcd.BootstrapWithExistingCluster = &druidv1alpha1.BootstrapWithExistingCluster{
			Members: []druidv1alpha1.BootstrapExistingMember{
				{Name: "source-etcd-0", PeerURLs: []string{"https://source-etcd-0.source-etcd-peer.source-ns.svc:2380"}},
				{Name: "source-etcd-1", PeerURLs: []string{"https://source-etcd-1.source-etcd-peer.source-ns.svc:2380"}},
				{Name: "source-etcd-2", PeerURLs: []string{"https://source-etcd-2.source-etcd-peer.source-ns.svc:2380"}},
			},
		}
		actualInitialCluster := prepareInitialCluster(etcd, "https")
		g.Expect(actualInitialCluster).To(ContainSubstring("source-etcd-0=https://source-etcd-0.source-etcd-peer.source-ns.svc:2380"))
		g.Expect(actualInitialCluster).To(ContainSubstring("source-etcd-1=https://source-etcd-1.source-etcd-peer.source-ns.svc:2380"))
		g.Expect(actualInitialCluster).To(ContainSubstring("source-etcd-2=https://source-etcd-2.source-etcd-peer.source-ns.svc:2380"))
		g.Expect(actualInitialCluster).To(ContainSubstring("etcd-test-0=https://etcd-test-0.etcd-test-peer.test-ns.svc:2380"))
	})

	t.Run("should emit name=url per URL when a member has multiple peer URLs", func(t *testing.T) {
		// etcd's --initial-cluster syntax pairs each URL with its member
		// name independently (name=url1,name=url2,...). A previous
		// implementation joined the URLs into a single name=url1,url2,url3
		// entry, which etcd rejects.
		t.Parallel()
		etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).WithReplicas(1).WithPeerTLS().Build()
		etcd.Spec.Etcd.BootstrapWithExistingCluster = &druidv1alpha1.BootstrapWithExistingCluster{
			Members: []druidv1alpha1.BootstrapExistingMember{
				{
					Name: "src-0",
					PeerURLs: []string{
						"https://src-0.peer.src-ns.svc:2380",
						"https://10.0.0.1:2380",
					},
				},
				{
					Name: "src-1",
					PeerURLs: []string{
						"https://src-1.peer.src-ns.svc:2380",
						"https://10.0.0.2:2380",
					},
				},
			},
		}
		actualInitialCluster := prepareInitialCluster(etcd, "https")

		// Each URL appears paired with its member name independently — never as a comma-joined list.
		g.Expect(actualInitialCluster).To(ContainSubstring("src-0=https://src-0.peer.src-ns.svc:2380"))
		g.Expect(actualInitialCluster).To(ContainSubstring("src-0=https://10.0.0.1:2380"))
		g.Expect(actualInitialCluster).To(ContainSubstring("src-1=https://src-1.peer.src-ns.svc:2380"))
		g.Expect(actualInitialCluster).To(ContainSubstring("src-1=https://10.0.0.2:2380"))

		// Negative: the malformed shape "name=url1,url2" must NOT appear.
		g.Expect(actualInitialCluster).NotTo(ContainSubstring("src-0=https://src-0.peer.src-ns.svc:2380,https://10.0.0.1:2380"))
		g.Expect(actualInitialCluster).NotTo(ContainSubstring("src-1=https://src-1.peer.src-ns.svc:2380,https://10.0.0.2:2380"))

		// Splitting on "," yields one segment per (member,url) pair (1 target
		// + 2*2 source pairs = 5 total). Every segment is well-formed name=url.
		segments := strings.Split(actualInitialCluster, ",")
		g.Expect(segments).To(HaveLen(5))
		for _, segment := range segments {
			g.Expect(strings.Count(segment, "=")).To(Equal(1), "segment %q should be exactly one name=url pair", segment)
		}
	})

	t.Run("should not change initial-cluster-state", func(t *testing.T) {
		// etcd-backup-restore recomputes --initial-cluster-state from
		// on-disk cluster state before launching embedded etcd, so the
		// configmap value is the bootstrap default rather than the
		// source-of-truth. Druid leaves it at "new".
		t.Parallel()
		etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).WithReplicas(1).Build()
		etcd.Spec.Etcd.BootstrapWithExistingCluster = &druidv1alpha1.BootstrapWithExistingCluster{
			Members: []druidv1alpha1.BootstrapExistingMember{
				{Name: "source-0", PeerURLs: []string{"http://source-0:2380"}},
			},
		}
		cfg := createEtcdConfig(etcd)
		g.Expect(cfg.InitialClusterState).To(Equal("new"))
	})

	t.Run("should not append when bootstrapWithExistingCluster is nil", func(t *testing.T) {
		t.Parallel()
		etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).WithReplicas(3).WithPeerTLS().Build()
		actualInitialCluster := prepareInitialCluster(etcd, "https")
		g.Expect(actualInitialCluster).NotTo(ContainSubstring("source"))
	})

	t.Run("should not append source entries when spec is nil but status records joined members", func(t *testing.T) {
		// Member-removal trigger contract: when
		//   spec.etcd.bootstrapWithExistingCluster == nil
		// AND
		//   status.bootstrapWithExistingClusterMembers is non-empty
		// the controller treats it as the signal to remove source members.
		// During that window the configmap must regenerate WITHOUT source
		// entries so a restarted etcd no longer advertises the source
		// cluster as part of its initial cluster.
		t.Parallel()
		etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).WithReplicas(3).WithPeerTLS().Build()
		etcd.Spec.Etcd.BootstrapWithExistingCluster = nil
		etcd.Status.BootstrapWithExistingClusterMembers = []druidv1alpha1.BootstrapJoinedMember{
			{Name: "src-0", PeerURLs: []string{"https://src-0.peer.src-ns.svc:2380"}},
			{Name: "src-1", PeerURLs: []string{"https://src-1.peer.src-ns.svc:2380"}},
		}
		actualInitialCluster := prepareInitialCluster(etcd, "https")
		g.Expect(actualInitialCluster).NotTo(ContainSubstring("src-"))
		g.Expect(actualInitialCluster).NotTo(ContainSubstring("source"))
		// Target members must still be present.
		g.Expect(actualInitialCluster).To(ContainSubstring("etcd-test-0=https://etcd-test-0.etcd-test-peer.test-ns.svc:2380"))
		g.Expect(actualInitialCluster).To(ContainSubstring("etcd-test-1=https://etcd-test-1.etcd-test-peer.test-ns.svc:2380"))
		g.Expect(actualInitialCluster).To(ContainSubstring("etcd-test-2=https://etcd-test-2.etcd-test-peer.test-ns.svc:2380"))
	})
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
		"name":                            Equal("etcd-config"),
		"data-dir":                        Equal(fmt.Sprintf("%s/new.etcd", common.VolumeMountPathEtcdData)),
		"metrics":                         Equal(string(druidv1alpha1.Basic)),
		"snapshot-count":                  Equal(ptr.Deref(etcd.Spec.Etcd.SnapshotCount, defaultSnapshotCount)),
		"quota-backend-bytes":             Equal(etcd.Spec.Etcd.Quota.Value()),
		"initial-cluster-token":           Equal("etcd-cluster"),
		"initial-cluster-state":           Equal("new"),
		"auto-compaction-mode":            Equal(string(ptr.Deref(etcd.Spec.Common.AutoCompactionMode, druidv1alpha1.Periodic))),
		"auto-compaction-retention":       Equal(ptr.Deref(etcd.Spec.Common.AutoCompactionRetention, defaultAutoCompactionRetention)),
		"next-cluster-version-compatible": Equal(true),
	}))
	matchClientTLSRelatedConfiguration(g, etcd, actualETCDConfig)
	matchPeerTLSRelatedConfiguration(g, etcd, actualETCDConfig)
	matchMemberNamePrefixConfiguration(g, etcd, actualETCDConfig)
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
		memberName := druidv1alpha1.GetMemberName(etcd.Spec.MemberNamePrefix, podName)
		advUrlsMap[memberName] = []string{fmt.Sprintf("%s://%s.%s.%s.svc:%d", scheme, podName, druidv1alpha1.GetPeerServiceName(etcd.ObjectMeta), etcd.Namespace, port)}
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

func matchMemberNamePrefixConfiguration(g *WithT, etcd *druidv1alpha1.Etcd, actualETCDConfig map[string]any) {
	if etcd.Spec.MemberNamePrefix != nil {
		g.Expect(actualETCDConfig).To(MatchKeys(IgnoreExtras|IgnoreMissing, Keys{
			"member-name-prefix": Equal(*etcd.Spec.MemberNamePrefix),
		}))
	} else {
		g.Expect(actualETCDConfig).ToNot(HaveKey("member-name-prefix"))
	}
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

// TestPeerSkipClientSANVerification exercises the rendering of
// spec.etcd.peerUrlTls.skipClientSANVerification into the etcd config ConfigMap.
// The field is structurally peer-only (it lives on PeerTLSConfig, not the
// shared TLSConfig used by clientUrlTls), so the kube-apiserver's schema —
// not a CEL rule — guarantees it cannot be set without peerUrlTls.
//
// Cases:
//   - PeerUrlTLS=nil: peer-transport-security must be absent.
//   - PeerUrlTLS set, SkipClientSANVerification=nil: peer-transport-security present,
//     skip-client-san-verification key absent (zero + omitempty).
//   - PeerUrlTLS set, SkipClientSANVerification=ptr(false): same as above (omitempty).
//   - PeerUrlTLS set, SkipClientSANVerification=ptr(true): nested
//     peer-transport-security.skip-client-san-verification: true.
func TestPeerSkipClientSANVerification(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                       string
		peerTLSEnabled             bool
		skipClientSANVerification  *bool
		expectPeerTransportSection bool
		expectSkipKey              bool
		expectSkipValue            bool
	}{
		{
			name:                       "no peer TLS — peer-transport-security absent",
			peerTLSEnabled:             false,
			skipClientSANVerification:  nil,
			expectPeerTransportSection: false,
		},
		{
			name:                       "peer TLS, skipClientSANVerification unset — key omitted",
			peerTLSEnabled:             true,
			skipClientSANVerification:  nil,
			expectPeerTransportSection: true,
			expectSkipKey:              false,
		},
		{
			name:                       "peer TLS, skipClientSANVerification=false — key omitted (omitempty)",
			peerTLSEnabled:             true,
			skipClientSANVerification:  ptr.To(false),
			expectPeerTransportSection: true,
			expectSkipKey:              false,
		},
		{
			name:                       "peer TLS, skipClientSANVerification=true — key present and true",
			peerTLSEnabled:             true,
			skipClientSANVerification:  ptr.To(true),
			expectPeerTransportSection: true,
			expectSkipKey:              true,
			expectSkipValue:            true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)

			builder := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace)
			if tc.peerTLSEnabled {
				builder = builder.WithPeerTLS()
			}
			etcd := builder.Build()
			if tc.skipClientSANVerification != nil {
				if etcd.Spec.Etcd.PeerUrlTLS == nil {
					etcd.Spec.Etcd.PeerUrlTLS = &druidv1alpha1.PeerTLSConfig{}
				}
				etcd.Spec.Etcd.PeerUrlTLS.SkipClientSANVerification = tc.skipClientSANVerification
			}

			// Assert at the cfg (struct) level — nil-safe vs. PeerSecurity.
			cfg := createEtcdConfig(etcd)
			if tc.expectPeerTransportSection {
				g.Expect(cfg.PeerSecurity).ToNot(BeNil())
				g.Expect(cfg.PeerSecurity.SkipClientSANVerification).To(Equal(tc.expectSkipValue))
			} else {
				g.Expect(cfg.PeerSecurity).To(BeNil())
			}

			// Assert at the rendered-YAML level — guards against the
			// JSON tag drifting from etcd v3.6's wire format.
			cm := emptyConfigMap(getObjectKey(etcd.ObjectMeta))
			g.Expect(buildResource(etcd, cm)).To(Succeed())
			parsed := map[string]any{}
			g.Expect(yaml.Unmarshal([]byte(cm.Data[common.EtcdConfigFileName]), &parsed)).To(Succeed())

			if !tc.expectPeerTransportSection {
				g.Expect(parsed).ToNot(HaveKey("peer-transport-security"))
				return
			}
			g.Expect(parsed).To(HaveKey("peer-transport-security"))
			peerSec, ok := parsed["peer-transport-security"].(map[string]any)
			g.Expect(ok).To(BeTrue(), "peer-transport-security must be a map")
			if tc.expectSkipKey {
				g.Expect(peerSec).To(HaveKeyWithValue("skip-client-san-verification", tc.expectSkipValue))
			} else {
				g.Expect(peerSec).ToNot(HaveKey("skip-client-san-verification"))
			}
		})
	}
}

// TestBboltFreelistType verifies that backend-bbolt-freelist-type is written into
// the etcd config only when the UpgradeEtcdVersion feature gate is enabled.
//
// Cases:
//   - feature gate off, field unset  → key absent
//   - feature gate off, field set    → key absent (flag unknown to etcd <3.5)
//   - feature gate on,  field unset  → key present with default value (array)
//   - feature gate on,  field=array  → key present with value "array"
//   - feature gate on,  field=map    → key present with value "map"
func TestBboltFreelistType(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name               string
		featureGateEnabled bool
		freeListType       *druidv1alpha1.BboltFreelistType
		expectKeyPresent   bool
		expectedValue      string
	}{
		{
			name:               "feature gate off, field unset — key absent",
			featureGateEnabled: false,
			freeListType:       nil,
			expectKeyPresent:   false,
		},
		{
			name:               "feature gate off, field set — key still absent",
			featureGateEnabled: false,
			freeListType:       ptr.To(druidv1alpha1.BboltFreelistMap),
			expectKeyPresent:   false,
		},
		{
			name:               "feature gate on, field unset — default array",
			featureGateEnabled: true,
			freeListType:       nil,
			expectKeyPresent:   true,
			expectedValue:      string(defaultBboltFreeListType),
		},
		{
			name:               "feature gate on, field=array — value array",
			featureGateEnabled: true,
			freeListType:       ptr.To(druidv1alpha1.BboltFreelistArray),
			expectKeyPresent:   true,
			expectedValue:      "array",
		},
		{
			name:               "feature gate on, field=map — value map",
			featureGateEnabled: true,
			freeListType:       ptr.To(druidv1alpha1.BboltFreelistMap),
			expectKeyPresent:   true,
			expectedValue:      "map",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			err := druidconfigv1alpha1.DefaultFeatureGates.SetEnabledFeaturesFromMap(
				map[string]bool{druidconfigv1alpha1.UpgradeEtcdVersion: tc.featureGateEnabled},
			)
			g.Expect(err).ToNot(HaveOccurred())
			t.Cleanup(func() {
				_ = druidconfigv1alpha1.DefaultFeatureGates.SetEnabledFeaturesFromMap(
					map[string]bool{druidconfigv1alpha1.UpgradeEtcdVersion: false},
				)
			})

			etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).Build()
			etcd.Spec.Etcd.BackendBboltFreelistType = tc.freeListType

			// Assert at the cfg (struct) level.
			cfg := createEtcdConfig(etcd)
			if tc.expectKeyPresent {
				g.Expect(string(cfg.BackendBboltFreelistType)).To(Equal(tc.expectedValue))
			} else {
				g.Expect(cfg.BackendBboltFreelistType).To(BeEmpty())
			}

			// Assert at the rendered-YAML level.
			cm := emptyConfigMap(getObjectKey(etcd.ObjectMeta))
			g.Expect(buildResource(etcd, cm)).To(Succeed())
			parsed := map[string]any{}
			g.Expect(yaml.Unmarshal([]byte(cm.Data[common.EtcdConfigFileName]), &parsed)).To(Succeed())

			if tc.expectKeyPresent {
				g.Expect(parsed).To(HaveKeyWithValue("backend-bbolt-freelist-type", tc.expectedValue))
			} else {
				g.Expect(parsed).ToNot(HaveKey("backend-bbolt-freelist-type"))
			}
		})
	}
}
