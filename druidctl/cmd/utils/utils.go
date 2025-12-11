// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gardener/etcd-druid/druidctl/internal/client"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

var (
	configFlags     *genericclioptions.ConfigFlags
	configFlagsOnce sync.Once
)

// GetConfigFlags returns a singleton *ConfigFlags for kubeconfig and context handling.
func GetConfigFlags() *genericclioptions.ConfigFlags {
	configFlagsOnce.Do(func() {
		configFlags = genericclioptions.NewConfigFlags(true)
	})
	return configFlags
}

// GetEtcdList returns a list of Etcd objects based on the provided references or all namespaces flag.
func GetEtcdList(ctx context.Context, cl client.EtcdClientInterface, etcdRefList []types.NamespacedName, allNamespaces bool) (*druidv1alpha1.EtcdList, error) {
	etcdList := &druidv1alpha1.EtcdList{}
	var err error
	if allNamespaces {
		etcdList, err = cl.ListEtcds(ctx, "")
		if err != nil {
			return nil, fmt.Errorf("unable to list etcd objects: %w", err)
		}
	} else {
		for _, ref := range etcdRefList {
			if ref.Name == "*" {
				nsEtcdList, err := cl.ListEtcds(ctx, ref.Namespace)
				if err != nil {
					return nil, fmt.Errorf("unable to list etcd objects in namespace %s: %w", ref.Namespace, err)
				}
				etcdList.Items = append(etcdList.Items, nsEtcdList.Items...)
				continue
			}
			etcd, err := cl.GetEtcd(ctx, ref.Namespace, ref.Name)
			if err != nil {
				return nil, fmt.Errorf("unable to get etcd object: %w", err)
			}
			etcdList.Items = append(etcdList.Items, *etcd)
		}
	}
	return etcdList, nil
}

// ShortDuration renders a duration in a compact time unit form (s, m, h, d).
func ShortDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	days := int(d.Hours()) / 24
	return fmt.Sprintf("%dd", days)
}

// SplitQualifiedName splits a qualified resource name into namespace and name.
func SplitQualifiedName(str string) (string, string) {
	parts := strings.Split(str, "/")
	if len(parts) < 2 {
		return "default", str
	}
	return parts[0], parts[1]
}

// GetEtcdRefList parses a comma-separated list of namespaced etcd resource names and returns a slice of NamespacedName.
func GetEtcdRefList(resourcesRef string) []types.NamespacedName {
	resources := strings.Split(resourcesRef, ",")
	etcdList := make([]types.NamespacedName, 0, len(resources))
	for _, resource := range resources {
		namespace, name := SplitQualifiedName(resource)
		etcdList = append(etcdList, types.NamespacedName{Namespace: namespace, Name: name})
	}
	return etcdList
}

// ValidateResourceNames checks if the provided resource names are valid.
func ValidateResourceNames(resourcesRef string) error {
	resources := strings.Split(resourcesRef, ",")
	for _, resource := range resources {
		if len(strings.Split(resource, "/")) > 2 {
			return fmt.Errorf("invalid resource name: %s", resource)
		}
	}
	return nil
}
