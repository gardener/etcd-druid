// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package listresources

import (
	"fmt"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// resourceMeta captures details needed to operate against a resource type.
type resourceMeta struct {
	GVR        schema.GroupVersionResource
	Kind       string
	Namespaced bool
}

// shortNameMap maps common short names to their full resource names.
var shortNameMap = map[string]schema.GroupVersionKind{
	// Core v1
	"po":  {Group: "", Version: "v1", Kind: "Pod"},
	"pod": {Group: "", Version: "v1", Kind: "Pod"},
	"svc": {Group: "", Version: "v1", Kind: "Service"},
	"cm":  {Group: "", Version: "v1", Kind: "ConfigMap"},
	"pvc": {Group: "", Version: "v1", Kind: "PersistentVolumeClaim"},
	"sa":  {Group: "", Version: "v1", Kind: "ServiceAccount"},
	// Also map plural names
	"pods":                   {Group: "", Version: "v1", Kind: "Pod"},
	"services":               {Group: "", Version: "v1", Kind: "Service"},
	"configmaps":             {Group: "", Version: "v1", Kind: "ConfigMap"},
	"secrets":                {Group: "", Version: "v1", Kind: "Secret"},
	"persistentvolumeclaims": {Group: "", Version: "v1", Kind: "PersistentVolumeClaim"},
	"serviceaccounts":        {Group: "", Version: "v1", Kind: "ServiceAccount"},
	// Apps v1
	"sts":          {Group: "apps", Version: "v1", Kind: "StatefulSet"},
	"statefulsets": {Group: "apps", Version: "v1", Kind: "StatefulSet"},
	// Coordination
	"lease":  {Group: "coordination.k8s.io", Version: "v1", Kind: "Lease"},
	"leases": {Group: "coordination.k8s.io", Version: "v1", Kind: "Lease"},
	// Policy
	"pdb":                  {Group: "policy", Version: "v1", Kind: "PodDisruptionBudget"},
	"poddisruptionbudgets": {Group: "policy", Version: "v1", Kind: "PodDisruptionBudget"},
	// RBAC
	"role":         {Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "Role"},
	"roles":        {Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "Role"},
	"rolebinding":  {Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "RoleBinding"},
	"rolebindings": {Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "RoleBinding"},
}

// restMapperResolver resolves user tokens to resource metadata using RESTMapper.
type restMapperResolver struct {
	mapper meta.RESTMapper
}

// newRESTMapperResolver creates a resolver using the provided RESTMapper.
func newRESTMapperResolver(mapper meta.RESTMapper) *restMapperResolver {
	return &restMapperResolver{mapper: mapper}
}

// resolve converts tokens (short names, resource names, kinds) into resource metadata list.
func (r *restMapperResolver) resolve(tokens []string) ([]resourceMeta, error) {
	if len(tokens) == 0 {
		return nil, fmt.Errorf("no resource tokens provided")
	}

	out := make([]resourceMeta, 0, len(tokens))
	seen := map[schema.GroupVersionResource]struct{}{}
	var unknown []string

	for _, t := range tokens {
		tok := strings.TrimSpace(strings.ToLower(t))
		if tok == "" {
			continue
		}

		m, err := r.resolveToken(tok)
		if err != nil {
			unknown = append(unknown, t)
			continue
		}

		if _, dup := seen[m.GVR]; dup {
			continue
		}
		seen[m.GVR] = struct{}{}
		out = append(out, m)
	}

	if len(unknown) > 0 {
		return nil, &unknownResourcesError{Tokens: unknown, Known: commonResourceHints()}
	}

	return out, nil
}

// resolveToken resolves a single token using the short name map and RESTMapper.
func (r *restMapperResolver) resolveToken(token string) (resourceMeta, error) {
	// Try short name map first
	if gvk, ok := shortNameMap[token]; ok {
		return r.gvkToMeta(gvk)
	}

	// Try as a Kind name
	titleCaser := cases.Title(language.English)
	capitalizedToken := titleCaser.String(token)
	mapping, err := r.mapper.RESTMapping(schema.GroupKind{Kind: capitalizedToken})
	if err == nil {
		return resourceMeta{
			GVR:        mapping.Resource,
			Kind:       mapping.GroupVersionKind.Kind,
			Namespaced: mapping.Scope.Name() == meta.RESTScopeNameNamespace,
		}, nil
	}

	return resourceMeta{}, fmt.Errorf("unknown resource: %s", token)
}

// gvkToMeta converts a GVK to resourceMeta using the RESTMapper.
func (r *restMapperResolver) gvkToMeta(gvk schema.GroupVersionKind) (resourceMeta, error) {
	mapping, err := r.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return resourceMeta{}, fmt.Errorf("failed to get mapping for %v: %w", gvk, err)
	}

	return resourceMeta{
		GVR:        mapping.Resource,
		Kind:       mapping.GroupVersionKind.Kind,
		Namespaced: mapping.Scope.Name() == meta.RESTScopeNameNamespace,
	}, nil
}

// unknownResourcesError conveys which tokens failed and a subset of known tokens.
type unknownResourcesError struct {
	Tokens []string
	Known  []string
}

func (e *unknownResourcesError) Error() string {
	return fmt.Sprintf("unknown resource tokens: %v; known examples: %v", e.Tokens, e.Known)
}

// commonResourceHints returns common resource tokens for error hints.
func commonResourceHints() []string {
	return []string{
		"po", "pods", "svc", "services", "cm", "configmaps",
		"sts", "statefulsets", "pvc", "persistentvolumeclaims",
		"secrets", "sa", "serviceaccounts", "lease", "leases",
		"pdb", "poddisruptionbudgets", "role", "rolebinding",
	}
}
