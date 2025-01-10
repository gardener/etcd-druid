// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"strings"

	"github.com/onsi/gomega/format"
	gomegatypes "github.com/onsi/gomega/types"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

// mapMatcher matches map[string]T and produces a convenient and easy to consume error message which includes
// the difference between the actual and expected. T is a generic type and accepts any type that is comparable.
type mapMatcher[T comparable] struct {
	fieldName string
	expected  map[string]T
	diff      []string
}

func (m mapMatcher[T]) Match(actual interface{}) (bool, error) {
	if actual == nil {
		return false, nil
	}
	actualMap, okType := actual.(map[string]T)
	if !okType {
		return false, fmt.Errorf("expected a map[string]string. got: %s", format.Object(actual, 1))
	}
	for k, v := range m.expected {
		actualVal, ok := actualMap[k]
		if !ok {
			m.diff = append(m.diff, fmt.Sprintf("expected key: %s to be present", k))
		}
		if v != actualVal {
			m.diff = append(m.diff, fmt.Sprintf("expected val: %s for key; %v, found val:%v instead", v, k, actualVal))
		}
	}
	return len(m.diff) == 0, nil
}

func (m mapMatcher[T]) FailureMessage(actual interface{}) string {
	return m.createMessage(actual, "to be")
}

func (m mapMatcher[T]) NegatedFailureMessage(actual interface{}) string {
	return m.createMessage(actual, "to not be")
}

func (m mapMatcher[T]) createMessage(actual interface{}, message string) string {
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
	return &mapMatcher[string]{
		fieldName: "ObjectMeta.Annotations",
		expected:  expected,
	}
}

// MatchResourceLabels returns a custom gomega matcher which matches labels set on the resource against the expected labels.
func MatchResourceLabels(expected map[string]string) gomegatypes.GomegaMatcher {
	return &mapMatcher[string]{
		fieldName: "ObjectMeta.Labels",
		expected:  expected,
	}
}

// MatchSpecLabelSelector returns a custom gomega matcher which matches label selector on a resource against the expected labels.
func MatchSpecLabelSelector(expected map[string]string) gomegatypes.GomegaMatcher {
	return PointTo(MatchFields(IgnoreExtras, Fields{
		"MatchLabels": MatchResourceLabels(expected),
	}))
}

// MatchEtcdOwnerReference is a custom gomega matcher which creates a matcher for ObjectMeta.OwnerReferences
func MatchEtcdOwnerReference(etcdName string, etcdUID types.UID) gomegatypes.GomegaMatcher {
	return ConsistOf(MatchFields(IgnoreExtras, Fields{
		"APIVersion":         Equal(druidv1alpha1.SchemeGroupVersion.String()),
		"Kind":               Equal("Etcd"),
		"Name":               Equal(etcdName),
		"UID":                Equal(etcdUID),
		"Controller":         PointTo(BeTrue()),
		"BlockOwnerDeletion": PointTo(BeTrue()),
	}))
}
