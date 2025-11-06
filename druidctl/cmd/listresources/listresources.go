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

type ResourceKey struct {
	Group    string `json:"group" yaml:"group"`
	Version  string `json:"version" yaml:"version"`
	Resource string `json:"resource" yaml:"resource"`
	Kind     string `json:"kind" yaml:"kind"`
}

type ResourceRef struct {
	Namespace string        `json:"namespace" yaml:"namespace"`
	Name      string        `json:"name" yaml:"name"`
	Age       time.Duration `json:"age" yaml:"age"`
}

type EtcdRef struct {
	Name      string `json:"name" yaml:"name"`
	Namespace string `json:"namespace" yaml:"namespace"`
}

type ResourceListPerKey struct {
	Key       ResourceKey   `json:"key" yaml:"key"`
	Resources []ResourceRef `json:"resources" yaml:"resources"`
}

type EtcdResourceResult struct {
	Etcd  EtcdRef              `json:"etcd" yaml:"etcd"`
	Items []ResourceListPerKey `json:"items" yaml:"items"`
}

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

	l.Formatter, err = printer.NewFormatter(printer.OutputFormat(l.OutputFormat))
	if err != nil {
		options.Logger.Error(l.IOStreams.ErrOut, "Failed to create formatter: ", err)
		return err
	}
	l.etcdRefList = cmdutils.GetEtcdRefList(l.ResourcesRef)
	return nil
}

func (l *listResourcesCmdCtx) validate() error {
	if err := cmdutils.ValidateResourceNames(l.ResourcesRef); err != nil {
		return err
	}
	return nil
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
	resolver, err := newAPIResourceResolver(genClient.Discovery())
	if err != nil {
		return fmt.Errorf("failed to initialize resource resolver: %w", err)
	}
	metas, err := resolver.resolve(tokens)
	if err != nil {
		return err
	}

	// Identify etcds to operate on
	etcdList, err := cmdutils.GetEtcdList(ctx, etcdClient, l.etcdRefList, l.AllNamespaces)
	if err != nil {
		return err
	}
	if len(etcdList.Items) == 0 {
		if l.AllNamespaces {
			out.Info(l.IOStreams.Out, "No Etcd resources found across all namespaces")
		} else {
			return fmt.Errorf("no Etcd resources found for the given selection: %s", l.ResourcesRef)
		}
		return nil
	}

	result := Result{
		Etcds: make([]EtcdResourceResult, 0, len(etcdList.Items)),
		Kind:  "EtcdResourceList",
	}
	for _, e := range etcdList.Items {
		etcdResult := EtcdResourceResult{
			Etcd:  EtcdRef{Name: e.Name, Namespace: e.Namespace},
			Items: make([]ResourceListPerKey, 0),
		}

		selector := fmt.Sprintf("app.kubernetes.io/part-of=%s", e.Name)
		for _, m := range metas {
			// Skip cluster-scoped if not intended; most curated resources are namespaced.
			namespace := ""
			if m.Namespaced {
				namespace = e.Namespace
			}
			ulist, err := genClient.Dynamic().Resource(m.GVR).Namespace(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
			if err != nil {
				out.Warning(l.IOStreams.Out, "Failed to list ", m.GVR.Resource, " for etcd ", e.Name, ": ", err.Error())
				continue
			}
			if len(ulist.Items) == 0 {
				continue
			}
			resourceKey := ResourceKey{Group: m.GVR.Group, Version: m.GVR.Version, Resource: m.GVR.Resource, Kind: m.Kind}
			for _, item := range ulist.Items {
				found := false
				for i, resourceListPerKey := range etcdResult.Items {
					if resourceListPerKey.Key == resourceKey {
						etcdResult.Items[i].Resources = append(etcdResult.Items[i].Resources, toResourceRef(&item))
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
		for k := range etcdResult.Items {
			sort.Slice(etcdResult.Items[k].Resources, func(i, j int) bool {
				ai, aj := etcdResult.Items[k].Resources[i], etcdResult.Items[k].Resources[j]
				if ai.Namespace == aj.Namespace {
					return ai.Name < aj.Name
				}
				return ai.Namespace < aj.Namespace
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
	if l.Formatter != nil {
		var outputData []byte
		outputData, err = l.Formatter.Format(result)
		if err != nil {
			return fmt.Errorf("failed to marshal result to desired format: %w", err)
		}
		fmt.Printf("%s\n", string(outputData))
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
		log.Header(l.IOStreams.Out, fmt.Sprintf("Etcd %s/%s", etcdResourceResult.Etcd.Namespace, etcdResourceResult.Etcd.Name))
		if len(etcdResourceResult.Items) == 0 {
			log.Info(l.IOStreams.Out, "No resources found for selected filters")
			continue
		}
		// Order resource kinds consistently
		// keys := make([]ResourceKey, 0, len(etcdResourceResult.Items))
		// for _, resourceListPerKey := range etcdResourceResult.Items {
		// 	keys = append(keys, resourceListPerKey.Key)
		// }
		keyList := etcdResourceResult.Items
		sort.Slice(keyList, func(i, j int) bool {
			if keyList[i].Key.Kind == keyList[j].Key.Kind {
				return keyList[i].Key.Resource < keyList[j].Key.Resource
			}
			return keyList[i].Key.Kind < keyList[j].Key.Kind
		})

		for _, resourceKey := range keyList {
			list := resourceKey.Resources
			log.RawHeader(l.IOStreams.Out, fmt.Sprintf("%s (%s.%s/%s): %d", resourceKey.Key.Kind, resourceKey.Key.Resource, resourceKey.Key.Group, resourceKey.Key.Version, len(list)))
			for _, r := range list {
				age := cmdutils.ShortDuration(r.Age)
				ns := r.Namespace
				if ns == "" {
					ns = "-"
				}
				log.Info(l.IOStreams.Out, fmt.Sprintf("%s/%s (age %s)", ns, r.Name, age))
			}
		}
	}
}
