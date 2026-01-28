// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package listresources

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	cmdutils "github.com/gardener/etcd-druid/druidctl/cmd/utils"
	"github.com/gardener/etcd-druid/druidctl/internal/log"
	"github.com/gardener/etcd-druid/druidctl/internal/printer"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// resourceMeta captures details needed to operate against a resource type.
type resourceMeta struct {
	GVR        schema.GroupVersionResource
	Kind       string
	Namespaced bool
}

func (l *listResourcesCmdCtx) complete(options *cmdutils.GlobalOptions) error {
	etcdClient, err := options.Clients.EtcdClient()
	if err != nil {
		return fmt.Errorf("unable to create etcd client: %w", err)
	}
	l.EtcdClient = etcdClient

	genericClient, err := options.Clients.GenericClient()
	if err != nil {
		return fmt.Errorf("unable to create generic kube clients: %w", err)
	}
	l.GenericClient = genericClient

	l.Printer, err = printer.NewFormatter(printer.OutputFormat(l.OutputFormat))
	if err != nil {
		return fmt.Errorf("failed to create formatter: %w", err)
	}

	// Build etcd reference list from resource args using kubectl-compatible parsing
	l.etcdRefList = options.BuildEtcdRefList()
	return nil
}

func (l *listResourcesCmdCtx) validate() error {
	return l.GlobalOptions.ValidateResourceSelection()
}

// execute lists the managed resources for the selected etcd resources based on the filter.
func (l *listResourcesCmdCtx) execute(ctx context.Context) error {
	if l.Verbose {
		if l.AllNamespaces {
			l.Logger.Info(l.IOStreams.Out, "Listing all Managed resources for Etcds across all namespaces")
		} else {
			l.Logger.Info(l.IOStreams.Out, "Listing Managed resources for selected Etcds")
		}
	}
	out := l.Logger

	etcdClient := l.EtcdClient
	genClient := l.GenericClient

	resourceTokens := parseFilter(l.Filter)
	if len(resourceTokens) == 0 || (len(resourceTokens) == 1 && resourceTokens[0] == "all") {
		resourceTokens = defaultResourceTokens()
	}

	mapper := genClient.RESTMapper()
	resourceMetas := make([]resourceMeta, 0, len(resourceTokens))
	seen := make(map[schema.GroupVersionResource]bool)

	for _, token := range resourceTokens {
		gvr, err := mapper.ResourceFor(schema.GroupVersionResource{Resource: token})
		if err != nil {
			return fmt.Errorf("unknown resource '%s': %w", token, err)
		}
		if seen[gvr] {
			continue
		}
		seen[gvr] = true

		gvk, err := mapper.KindFor(gvr)
		if err != nil {
			return fmt.Errorf("failed to get kind for '%s': %w", token, err)
		}
		mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		if err != nil {
			return fmt.Errorf("failed to get mapping for '%s': %w", token, err)
		}
		resourceMetas = append(resourceMetas, resourceMeta{
			GVR:        gvr,
			Kind:       gvk.Kind,
			Namespaced: mapping.Scope.Name() == meta.RESTScopeNameNamespace,
		})
	}

	// Identify etcds to operate on
	etcdList, err := cmdutils.GetEtcdList(ctx, etcdClient, l.etcdRefList, l.AllNamespaces, l.GetNamespace(), l.LabelSelector)
	if err != nil {
		return err
	}
	if len(etcdList.Items) == 0 {
		if l.AllNamespaces {
			out.Info(l.IOStreams.Out, "No Etcd resources found across all namespaces")
		} else {
			out.Info(l.IOStreams.Out, "no Etcd resources found in namespace %q", l.GetNamespace())
		}
		return nil
	}

	result := Result{
		Etcds: make([]EtcdResourceResult, 0, len(etcdList.Items)),
		Kind:  "EtcdResourceList",
	}
	for _, etcd := range etcdList.Items {
		etcdResult := EtcdResourceResult{
			Etcd:  EtcdRef{Name: etcd.Name, Namespace: etcd.Namespace},
			Items: make([]ResourceListPerKey, 0),
		}

		labelSelector := fmt.Sprintf("app.kubernetes.io/part-of=%s", etcd.Name)
		for _, resMeta := range resourceMetas {
			resourceNamespace := ""
			if resMeta.Namespaced {
				resourceNamespace = etcd.Namespace
			}
			resourceList, err := genClient.Dynamic().Resource(resMeta.GVR).Namespace(resourceNamespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
			if err != nil {
				out.Warning(l.IOStreams.Out, "Failed to list ", resMeta.GVR.Resource, " for etcd ", etcd.Name, ": ", err.Error())
				continue
			}
			if len(resourceList.Items) == 0 {
				continue
			}
			resourceKey := ResourceKey{Group: resMeta.GVR.Group, Version: resMeta.GVR.Version, Resource: resMeta.GVR.Resource, Kind: resMeta.Kind}
			for _, item := range resourceList.Items {
				found := false
				for idx, resourceListPerKey := range etcdResult.Items {
					if resourceListPerKey.Key == resourceKey {
						etcdResult.Items[idx].Resources = append(etcdResult.Items[idx].Resources, toResourceRef(&item))
						found = true
						break
					}
				}
				if !found {
					etcdResult.Items = append(etcdResult.Items, ResourceListPerKey{
						Key:       resourceKey,
						Resources: []ResourceRef{toResourceRef(&item)},
					})
				}
			}
		}
		// Sort within each resource kind by namespace/name for determinism
		for kindIdx := range etcdResult.Items {
			sort.Slice(etcdResult.Items[kindIdx].Resources, func(i, j int) bool {
				resourceA, resourceB := etcdResult.Items[kindIdx].Resources[i], etcdResult.Items[kindIdx].Resources[j]
				if resourceA.Namespace == resourceB.Namespace {
					return resourceA.Name < resourceB.Name
				}
				return resourceA.Namespace < resourceB.Namespace
			})
		}
		result.Etcds = append(result.Etcds, etcdResult)
	}

	// Sort etcds by namespace/name
	sort.Slice(result.Etcds, func(i, j int) bool {
		if result.Etcds[i].Etcd.Namespace == result.Etcds[j].Etcd.Namespace {
			return result.Etcds[i].Etcd.Name < result.Etcds[j].Etcd.Name
		}
		return result.Etcds[i].Etcd.Namespace < result.Etcds[j].Etcd.Namespace
	})
	if l.Printer != nil {
		var outputData []byte
		outputData, err = l.Printer.Print(result)
		if err != nil {
			return fmt.Errorf("failed to marshal result to desired format: %w", err)
		}
		fmt.Fprintf(l.IOStreams.Out, "%s\n", string(outputData))
		return nil
	}
	l.renderListResources(out, result.Etcds)
	return nil
}

// parseFilter splits and normalizes the filter string
func parseFilter(filter string) []string {
	if strings.TrimSpace(filter) == "" {
		return nil
	}
	parts := strings.Split(filter, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.ToLower(strings.TrimSpace(p))
		if t == "" {
			continue
		}
		out = append(out, t)
	}
	return out
}

// defaultResourceTokens returns the curated default set for "all"
func defaultResourceTokens() []string {
	return []string{"pods", "statefulsets", "services", "configmaps", "secrets", "persistentvolumeclaims", "leases", "poddisruptionbudgets", "roles", "rolebindings", "serviceaccounts"}
}

func toResourceRef(u *unstructured.Unstructured) ResourceRef {
	age := time.Since(u.GetCreationTimestamp().Time)
	return ResourceRef{
		Namespace: u.GetNamespace(),
		Name:      u.GetName(),
		Age:       age,
	}
}

// renderListResources prints results in a grouped, neat format using the Logger.
func (l *listResourcesCmdCtx) renderListResources(log log.Logger, results []EtcdResourceResult) {
	for _, etcdResourceResult := range results {
		log.RawHeader(l.IOStreams.Out, fmt.Sprintf("Etcd [%s/%s]", etcdResourceResult.Etcd.Namespace, etcdResourceResult.Etcd.Name))
		if len(etcdResourceResult.Items) == 0 {
			log.Info(l.IOStreams.Out, "No resources found for selected filters")
			continue
		}

		keyList := etcdResourceResult.Items
		sort.Slice(keyList, func(i, j int) bool {
			if keyList[i].Key.Kind == keyList[j].Key.Kind {
				return len(keyList[i].Key.Resource) < len(keyList[j].Key.Resource)
			}
			return len(keyList[i].Key.Kind) < len(keyList[j].Key.Kind)
		})
		table := SetupListResourcesTable(keyList)
		if _, err := fmt.Fprintln(l.IOStreams.Out, table); err != nil {
			log.Warning(l.IOStreams.ErrOut, "Failed writing resources table: ", err.Error())
		}
	}
}
