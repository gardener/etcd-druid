// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConditionType is the type of condition.
type ConditionType string

// ConditionStatus is the status of a condition.
type ConditionStatus string

const (
	// ConditionTrue means a resource is in the condition.
	ConditionTrue ConditionStatus = "True"
	// ConditionFalse means a resource is not in the condition.
	ConditionFalse ConditionStatus = "False"
	// ConditionUnknown means Gardener can't decide if a resource is in the condition or not.
	ConditionUnknown ConditionStatus = "Unknown"
	// ConditionProgressing means the condition was seen true, failed but stayed within a predefined failure threshold.
	// In the future, we could add other intermediate conditions, e.g. ConditionDegraded.
	// Deprecated: Will be removed in the future since druid conditions will be replaced by metav1.Condition
	// which has only three status options: True, False, Unknown.
	ConditionProgressing ConditionStatus = "Progressing"
	// ConditionCheckError is a constant for a reason in condition.
	// Deprecated: Will be removed in the future since druid conditions will be replaced by metav1.Condition
	// which has only three status options: True, False, Unknown.
	ConditionCheckError ConditionStatus = "ConditionCheckError"
)

const (
	// ConditionReasonFullSnapshotTakenSuccessfully is the reason for a successful full snapshot.
	ConditionReasonFullSnapshotTakenSuccessfully string = "FullSnapshotTakenSuccessfully"
	// ConditionReasonFullSnapshotError is the reason for a failed full snapshot.
	ConditionReasonFullSnapshotError string = "ErrorTriggeringFullSnapshot"
)

// Condition holds the information about the state of a resource.
type Condition struct {
	// Type of the Etcd condition.
	Type ConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status ConditionStatus `json:"status"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime"`
	// Last time the condition was updated.
	LastUpdateTime metav1.Time `json:"lastUpdateTime"`
	// The reason for the condition's last transition.
	Reason string `json:"reason"`
	// A human-readable message indicating details about the transition.
	Message string `json:"message"`
}
