// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/store"
	"github.com/gardener/etcd-druid/test/e2e/testenv"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

var knownBackupProviders = []druidv1alpha1.StorageProvider{
	"none",
	store.Local, druidv1alpha1.StorageProvider(strings.ToLower(store.Local)),
	store.S3, druidv1alpha1.StorageProvider(strings.ToLower(store.S3)),
	store.ABS, druidv1alpha1.StorageProvider(strings.ToLower(store.ABS)),
	store.GCS, druidv1alpha1.StorageProvider(strings.ToLower(store.GCS)),
}

// getKubeconfig returns a Kubeconfig built from the config file in the specified path.
func getKubeconfig(kubeconfigPath string) (*rest.Config, error) {
	return clientcmd.BuildConfigFromFlags("", kubeconfigPath)
}

// GetKubernetesClient creates a Kubernetes client using the provided kubeconfig path.
func GetKubernetesClient(kubeconfigPath string) (client.Client, error) {
	config, err := getKubeconfig(kubeconfigPath)
	if err != nil {
		return nil, err
	}

	return client.New(config, client.Options{})
}

// ParseBackupProviders parses a comma-separated string of backup providers
func ParseBackupProviders(providers string) ([]druidv1alpha1.StorageProvider, error) {
	var (
		providerList []druidv1alpha1.StorageProvider
		errs         error
	)

	if providers != "" {
		for p := range strings.SplitSeq(providers, ",") {
			provider := druidv1alpha1.StorageProvider("none")
			if p != "none" {
				prov, err := store.StorageProviderFromInfraProvider(ptr.To(druidv1alpha1.StorageProvider(strings.TrimSpace(p))))
				if err != nil {
					errs = errors.Join(errs, fmt.Errorf("failed to parse provider %s: %w", p, err))
				}
				provider = druidv1alpha1.StorageProvider(prov)
			}
			if slices.Contains(knownBackupProviders, provider) && !slices.Contains(providerList, provider) {
				providerList = append(providerList, provider)
			}
		}
	}

	if len(providerList) == 0 {
		providerList = append(providerList, "none")
	}

	return providerList, nil
}

// ScaleStatefulSetToZero scales the Etcd's StatefulSet down to zero replicas and waits until no replicas are ready.
func ScaleStatefulSetToZero(g *WithT, testEnv *testenv.TestEnvironment, etcd *druidv1alpha1.Etcd) {
	stsName := druidv1alpha1.GetStatefulSetName(etcd.ObjectMeta)
	sts := &appsv1.StatefulSet{}
	g.Expect(testEnv.Client().Get(testEnv.Context(), types.NamespacedName{
		Name:      stsName,
		Namespace: etcd.Namespace,
	}, sts)).To(Succeed())

	patch := client.MergeFrom(sts.DeepCopy())
	sts.Spec.Replicas = ptr.To[int32](0)
	g.Expect(testEnv.Client().Patch(testEnv.Context(), sts, patch)).To(Succeed())

	g.Eventually(func() int32 {
		updated := &appsv1.StatefulSet{}
		if err := testEnv.Client().Get(testEnv.Context(), types.NamespacedName{
			Name:      stsName,
			Namespace: etcd.Namespace,
		}, updated); err != nil {
			return -1
		}
		return updated.Status.ReadyReplicas
	}, 60*time.Second, 2*time.Second).Should(Equal(int32(0)))
}
