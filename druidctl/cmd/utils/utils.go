// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gardener/etcd-druid/druidctl/internal/client"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// CmdContext returns the command's context, or context.Background() if nil.
// This allows tests to call RunE directly without needing to set up cobra's context.
func CmdContext(cmd *cobra.Command) context.Context {
	ctx := cmd.Context()
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

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
// namespace is the namespace to use when etcdRefList is empty (list all in namespace).
// labelSelector filters resources by label (e.g., "app=etcd-statefulset"). Empty string means no filtering.
func GetEtcdList(ctx context.Context, cl client.EtcdClientInterface, etcdRefList []types.NamespacedName, allNamespaces bool, namespace string, labelSelector string) (*druidv1alpha1.EtcdList, error) {
	etcdList := &druidv1alpha1.EtcdList{}
	var err error

	// when -A flag is set, list across all namespaces with optional label filter
	if allNamespaces {
		etcdList, err = cl.ListEtcds(ctx, "", labelSelector)
		if err != nil {
			return nil, fmt.Errorf("unable to list etcd objects across all namespaces: %w", err)
		}
		return etcdList, nil
	}

	// when no explicit refs are provided, list all in namespace (with optional label filter)
	// This handles: `-n ns` or `-l selector` or `-n ns -l selector`
	if len(etcdRefList) == 0 {
		etcdList, err = cl.ListEtcds(ctx, namespace, labelSelector)
		if err != nil {
			return nil, fmt.Errorf("unable to list etcd objects in namespace %q: %w", namespace, err)
		}
		return etcdList, nil
	}

	// when explicit refs are provided, get each specific etcd by name
	for _, ref := range etcdRefList {
		etcd, err := cl.GetEtcd(ctx, ref.Namespace, ref.Name)
		if err != nil {
			return nil, fmt.Errorf("etcd %q not found in namespace %q", ref.Name, ref.Namespace)
		}
		etcdList.Items = append(etcdList.Items, *etcd)
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
