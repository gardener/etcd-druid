package listresources

import "time"

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
