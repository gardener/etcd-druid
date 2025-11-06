package listresources

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

// resourceMeta captures details needed to operate against a resource type.
type resourceMeta struct {
	GVR        schema.GroupVersionResource
	Kind       string
	Namespaced bool
}

// apiResourceResolver resolves user tokens to resource metadata using discovery.
type apiResourceResolver struct {
	byName map[string]resourceMeta // resource plural or short names -> meta
	byKind map[string]resourceMeta // kind (singular) -> meta
}

// newAPIResourceResolver builds a resolver using the server's preferred resources.
func newAPIResourceResolver(disco discovery.DiscoveryInterface) (*apiResourceResolver, error) {
	lists, err := disco.ServerPreferredResources()
	if err != nil && !discovery.IsGroupDiscoveryFailedError(err) {
		return nil, fmt.Errorf("failed to discover server resources: %w", err)
	}

	r := &apiResourceResolver{
		byName: map[string]resourceMeta{},
		byKind: map[string]resourceMeta{},
	}

	for _, l := range lists {
		if l == nil {
			continue
		}
		gv, err := schema.ParseGroupVersion(l.GroupVersion)
		if err != nil {
			continue
		}
		for _, ar := range l.APIResources {
			// skip subresources like pods/status
			if strings.Contains(ar.Name, "/") {
				continue
			}
			meta := resourceMeta{
				GVR: schema.GroupVersionResource{
					Group:    gv.Group,
					Version:  gv.Version,
					Resource: ar.Name,
				},
				Kind:       ar.Kind,
				Namespaced: ar.Namespaced,
			}
			// Index by resource plural name (e.g: pods) and short names
			nameKey := strings.ToLower(ar.Name)
			if _, exists := r.byName[nameKey]; !exists {
				r.byName[nameKey] = meta
			}
			for _, sn := range ar.ShortNames {
				snKey := strings.ToLower(sn)
				if _, exists := r.byName[snKey]; !exists {
					r.byName[snKey] = meta
				}
			}
			// Index by Kind (singular), prefer first seen (preferred)
			kindKey := strings.ToLower(ar.Kind)
			if _, exists := r.byKind[kindKey]; !exists {
				r.byKind[kindKey] = meta
			}
		}
	}
	return r, nil
}

// resolve converts tokens (short names, resource names, kinds) into resource metadata list.
func (r *apiResourceResolver) resolve(tokens []string) ([]resourceMeta, error) {
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
		var meta resourceMeta
		var ok bool
		if meta, ok = r.byName[tok]; !ok {
			if meta, ok = r.byKind[tok]; !ok {
				// try fully qualified like leases.coordination.k8s.io -> resource.group
				if gr := schema.ParseGroupResource(tok); gr.Resource != "" {
					// Find by resource name match then filter by group if possible
					if m, ok2 := r.byName[gr.Resource]; ok2 {
						if m.GVR.Group == gr.Group {
							meta = m
							ok = true
						}
					}
				}
			}
		}
		if !ok {
			unknown = append(unknown, t)
			continue
		}
		if _, dup := seen[meta.GVR]; dup {
			continue
		}
		seen[meta.GVR] = struct{}{}
		out = append(out, meta)
	}
	if len(unknown) > 0 {
		return nil, &unknownResourcesError{Tokens: unknown, Known: r.sampleKnown()}
	}
	return out, nil
}

// unknownResourcesError conveys which tokens failed and a subset of known tokens.
type unknownResourcesError struct {
	Tokens []string
	Known  []string
}

func (e *unknownResourcesError) Error() string {
	return fmt.Sprintf("unknown resource tokens: %v; known examples: %v", e.Tokens, e.Known)
}

// sampleKnown returns a subset of known tokens for hinting.
func (r *apiResourceResolver) sampleKnown() []string {
	out := make([]string, 0, 16)
	common := []string{"po", "pods", "pod", "sts", "statefulsets", "svc", "services", "cm", "configmaps", "pvc", "secrets", "lease", "leases", "pdb", "role", "rolebinding", "sa", "serviceaccount"}
	for _, c := range common {
		if _, ok := r.byName[c]; ok {
			out = append(out, c)
		}
	}
	// Fill up with any other names if needed
	if len(out) < 10 {
		for name := range r.byName {
			out = append(out, name)
			if len(out) >= 20 {
				break
			}
		}
	}
	return out
}
