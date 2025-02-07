// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package errors

import (
	"errors"
	"fmt"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ErrRequeueAfter is a special error code that indicates that the current step should be re-queued after a certain time,
// as some conditions during reconciliation are not met. This should not be used in case
// there is an actual error during reconciliation.
const ErrRequeueAfter = "ERR_REQUEUE_AFTER"

// DruidError is a custom error that should be used throughout druid which encapsulates
// the underline error (cause) along with error code and contextual information captured
// as operation during which an error occurred and any custom message.
type DruidError struct {
	// Code indicates the category of error adding contextual information to the underline error.
	Code druidv1alpha1.ErrorCode
	// Cause is the underline error.
	Cause error
	// Operation is the semantic operation during which this error is created/wrapped.
	Operation string
	// Message is the custom message providing additional context for the error.
	Message string
	// ObservedAt is the time at which the error was observed.
	ObservedAt time.Time
}

func (e *DruidError) Error() string {
	var msg string
	if e.Cause != nil {
		msg = e.Cause.Error()
	} else {
		msg = e.Message
	}
	return fmt.Sprintf("[Operation: %s, Code: %s] %s", e.Operation, e.Code, msg)
}

// WithCause sets the underline error and returns the DruidError.
func (e *DruidError) WithCause(err error) error {
	e.Cause = err
	return e
}

// New creates a new DruidError with the given error code, operation and message.
func New(code druidv1alpha1.ErrorCode, operation string, message string) error {
	return &DruidError{
		Code:       code,
		Operation:  operation,
		Message:    message,
		ObservedAt: time.Now().UTC(),
	}
}

// WrapError wraps an error and contextual information like code, operation and message to create a DruidError
// Consumers can use errors.As or errors.Is to check if the error is of type DruidError and get its constituent fields.
func WrapError(err error, code druidv1alpha1.ErrorCode, operation string, message string) error {
	if err == nil {
		return nil
	}
	return &DruidError{
		Code:       code,
		Cause:      err,
		Operation:  operation,
		Message:    message,
		ObservedAt: time.Now().UTC(),
	}
}

// MapToLastErrors maps a slice of DruidError's and maps these to druidv1alpha1.LastError slice which
// will be set in the status of an etcd resource.
func MapToLastErrors(errs []error) []druidv1alpha1.LastError {
	lastErrs := make([]druidv1alpha1.LastError, 0, len(errs))
	for _, err := range errs {
		druidErr := &DruidError{}
		if errors.As(err, &druidErr) {
			desc := fmt.Sprintf("[Operation: %s, Code: %s] message: %s", druidErr.Operation, druidErr.Code, druidErr.Message)
			if druidErr.Cause != nil {
				desc += fmt.Sprintf(", cause: %s", druidErr.Cause.Error())
			}
			lastErr := druidv1alpha1.LastError{
				Code:        druidErr.Code,
				Description: desc,
				ObservedAt:  metav1.NewTime(druidErr.ObservedAt),
			}
			lastErrs = append(lastErrs, lastErr)
		}
	}
	return lastErrs
}

// AsDruidError returns the given error as a DruidError if it is of type DruidError, otherwise returns nil.
func AsDruidError(err error) *DruidError {
	druidErr := &DruidError{}
	if errors.As(err, &druidErr) {
		return druidErr
	}
	return nil
}
