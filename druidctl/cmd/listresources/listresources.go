// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ResourceKey identifies a Kubernetes resource kind by group/version/resource/Kind.
type ResourceKey struct {
	Group    string `json:"group" yaml:"group"`
	Version  string `json:"version" yaml:"version"`
	Resource string `json:"resource" yaml:"resource"`
	Kind     string `json:"kind" yaml:"kind"`
}

// ResourceRef references a single namespaced resource and includes its age.
type ResourceRef struct {
	Namespace string        `json:"namespace" yaml:"namespace"`
	Name      string        `json:"name" yaml:"name"`
	Age       time.Duration `json:"age" yaml:"age"`
}

// EtcdRef identifies an Etcd custom resource by name and namespace.
type EtcdRef struct {
	Name      string `json:"name" yaml:"name"`
	Namespace string `json:"namespace" yaml:"namespace"`
}

// ResourceListPerKey groups resources under a single ResourceKey.
type ResourceListPerKey struct {
	Key       ResourceKey   `json:"key" yaml:"key"`
	Resources []ResourceRef `json:"resources" yaml:"resources"`
}

// EtcdResourceResult contains all managed resources for a single Etcd.
type EtcdResourceResult struct {
	Etcd  EtcdRef              `json:"etcd" yaml:"etcd"`
	Items []ResourceListPerKey `json:"items" yaml:"items"`
}

// Result is the top-level aggregation for managed resources across Etcds.
type Result struct {
	Etcds []EtcdResourceResult `json:"etcds" yaml:"etcds"`
	Kind  string               `json:"kind" yaml:"kind"`
}

func (l *listResourcesCmdCtx) complete(options *cmdutils.GlobalOptions) error {
	etcdClient, err := options.Clients.EtcdClient()
	if err != nil {
		options.Logger.Error(l.IOStreams.ErrOut, "Unable to create etcd client: ", err)
		return err
	}
	l.EtcdClient = etcdClient

	genericClient, err := options.Clients.GenericClient()
	if err != nil {
		options.Logger.Error(l.IOStreams.ErrOut, "Unable to create generic kube clients: ", err)
		return err
	}
	l.GenericClient = genericClient

	l.Printer, err = printer.NewFormatter(printer.OutputFormat(l.OutputFormat))
	if err != nil {
		options.Logger.Error(l.IOStreams.ErrOut, "Failed to create formatter: ", err)
		return err
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
	if l.AllNamespaces {
		l.Logger.Info(l.IOStreams.Out, "Listing all Managed resources for Etcds across all namespaces")
	} else {
		l.Logger.Info(l.IOStreams.Out, "Listing Managed resources for selected Etcds")
	}
	out := l.Logger

	etcdClient := l.EtcdClient
	genClient := l.GenericClient

	tokens := parseFilter(l.Filter)
	if len(tokens) == 0 || (len(tokens) == 1 && tokens[0] == "all") {
		tokens = defaultResourceTokens()
	}
	resolver := newRESTMapperResolver(genClient.RESTMapper())
	metas, err := resolver.resolve(tokens)
	if err != nil {
		return err
	}

	// Identify etcds to operate on
	etcdList, err := cmdutils.GetEtcdList(ctx, etcdClient, l.etcdRefList, l.AllNamespaces, l.GetNamespace(), l.LabelSelector)
	if err != nil {
		return err
	}
	if len(etcdList.Items) == 0 {
		if !l.AllNamespaces {
			return fmt.Errorf("no Etcd resources found in namespace %q", l.GetNamespace())
		}
		out.Info(l.IOStreams.Out, "No Etcd resources found across all namespaces")
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
		for _, resMeta := range metas {
			// Skip cluster-scoped if not intended; most curated resources are namespaced.
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
	seen := map[string]struct{}{}
	for _, p := range parts {
		t := strings.ToLower(strings.TrimSpace(p))
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

// defaultResourceTokens returns the curated default set for "all"
func defaultResourceTokens() []string {
	return []string{"po", "sts", "svc", "cm", "secret", "pvc", "lease", "pdb", "role", "rolebinding", "sa"}
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
