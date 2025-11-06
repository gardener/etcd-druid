package utils

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/types"
)

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
