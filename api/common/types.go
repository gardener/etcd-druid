// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package common

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LastOperationState is a string alias representing the state of the last operation.
type LastOperationState string

// LastOperationType is a string alias representing type of the last operation.
type LastOperationType string

// LastOperation holds the information on the last operation done on the resource.
type LastOperation struct {
	// Type is the type of last operation.
	Type LastOperationType `json:"type"`
	// State is the state of the last operation.
	State LastOperationState `json:"state"`
	// Description describes the last operation.
	Description string `json:"description"`
	// RunID correlates an operation with a reconciliation run.
	// Every time the resource is reconciled (barring status reconciliation which is periodic), a unique ID is
	// generated which can be used to correlate all actions done as part of a single reconcile run. Capturing this
	// as part of LastOperation aids in establishing this correlation. This further helps in also easily filtering
	// reconcile logs as all structured logs in a reconciliation run should have the `runID` referenced.
	RunID string `json:"runID"`
	// LastUpdateTime is the time at which the operation was last updated.
	LastUpdateTime metav1.Time `json:"lastUpdateTime"`
}

// ErrorCode is a string alias representing an error code that identifies an error.
type ErrorCode string

// LastError stores details of the most recent error encountered for a resource.
type LastError struct {
	// Code is an error code that uniquely identifies an error.
	Code ErrorCode `json:"code"`
	// Description is a human-readable message indicating details of the error.
	Description string `json:"description"`
	// ObservedAt is the time the error was observed.
	ObservedAt metav1.Time `json:"observedAt"`
}
