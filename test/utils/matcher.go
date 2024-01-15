package utils

import (
	"fmt"
	"strings"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	. "github.com/onsi/gomega/gstruct"
	gomegatypes "github.com/onsi/gomega/types"
	"k8s.io/apimachinery/pkg/types"
)

// mapMatcher matches map[string]string and produces a convenient and easy to consume error message which includes
// the difference between the actual and expected.
type mapMatcher struct {
	fieldName string
	expected  map[string]string
	diff      []string
}

func (m mapMatcher) Match(actual interface{}) (bool, error) {
	if actual == nil {
		return false, nil
	}
	actualMap, okType := actual.(map[string]string)
	if !okType {
		return false, fmt.Errorf("expected a map[string]string. got: %s", format.Object(actual, 1))
	}
	for k, v := range m.expected {
		actualVal, ok := actualMap[k]
		if !ok {
			m.diff = append(m.diff, fmt.Sprintf("expected key: %s to be present", k))
		}
		if v != actualVal {
			m.diff = append(m.diff, fmt.Sprintf("expected val: %s for key; %s, found val:%s instead", v, k, actualVal))
		}
	}
	return len(m.diff) == 0, nil
}

func (m mapMatcher) FailureMessage(actual interface{}) string {
	return m.createMessage(actual, "to be")
}

func (m mapMatcher) NegatedFailureMessage(actual interface{}) string {
	return m.createMessage(actual, "to not be")
}

func (m mapMatcher) createMessage(actual interface{}, message string) string {
	msgBuilder := strings.Builder{}
	msgBuilder.WriteString(format.Message(actual, message, m.expected))
	if len(m.diff) > 0 {
		msgBuilder.WriteString(fmt.Sprintf("\nFound difference:\n"))
		msgBuilder.WriteString(strings.Join(m.diff, "\n"))
	}
	return msgBuilder.String()
}

// MatchResourceAnnotations returns a custom gomega matcher which matches annotations set on a resource against the expected annotations.
func MatchResourceAnnotations(expected map[string]string) gomegatypes.GomegaMatcher {
	return &mapMatcher{
		fieldName: "ObjectMeta.Annotations",
		expected:  expected,
	}
}

// MatchResourceLabels returns a custom gomega matcher which matches labels set on the resource against the expected labels.
func MatchResourceLabels(expected map[string]string) gomegatypes.GomegaMatcher {
	return &mapMatcher{
		fieldName: "ObjectMeta.Labels",
		expected:  expected,
	}
}

func MatchSpecLabelSelector(expected map[string]string) gomegatypes.GomegaMatcher {
	return PointTo(MatchFields(IgnoreExtras, Fields{
		"MatchLabels": MatchResourceLabels(expected),
	}))
}

// MatchEtcdOwnerReference is a custom gomega matcher which creates a matcher for ObjectMeta.OwnerReferences
func MatchEtcdOwnerReference(etcdName string, etcdUID types.UID) gomegatypes.GomegaMatcher {
	return ConsistOf(MatchFields(IgnoreExtras, Fields{
		"APIVersion":         Equal(druidv1alpha1.GroupVersion.String()),
		"Kind":               Equal("Etcd"),
		"Name":               Equal(etcdName),
		"UID":                Equal(etcdUID),
		"Controller":         PointTo(BeTrue()),
		"BlockOwnerDeletion": PointTo(BeTrue()),
	}))
}
