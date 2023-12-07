package errors

import (
	"errors"
	"fmt"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DruidError is a custom error that should be used throughout druid which encapsulates
// the underline error (cause) along with error code and contextual information captured
// as operation during which an error occurred and any custom message.
type DruidError struct {
	Code      druidv1alpha1.ErrorCode
	Cause     error
	Operation string
	Message   string
}

func (r *DruidError) Error() string {
	return fmt.Sprintf("[Operation: %s, Code: %s] %s", r.Operation, r.Code, r.Cause.Error())
}

// WrapError wraps an error and contextual information like code, operation and message to create a DruidError
// Consumers can use errors.As or errors.Is to check if the error is of type DruidError and get its constituent fields.
func WrapError(err error, code druidv1alpha1.ErrorCode, operation string, message string) error {
	if err == nil {
		return nil
	}
	return &DruidError{
		Code:      code,
		Cause:     err,
		Operation: operation,
		Message:   message,
	}
}

// MapToLastErrors maps a slice of DruidError's and maps these to druidv1alpha1.LastError slice which
// will be set in the status of an etcd resource.
func MapToLastErrors(errs []error) []druidv1alpha1.LastError {
	lastErrs := make([]druidv1alpha1.LastError, 0, len(errs))
	for _, err := range errs {
		druidErr := &DruidError{}
		if errors.As(err, &druidErr) {
			lastErr := druidv1alpha1.LastError{
				Code:           druidErr.Code,
				Description:    druidErr.Message,
				LastUpdateTime: metav1.NewTime(time.Now().UTC()),
			}
			lastErrs = append(lastErrs, lastErr)
		}
	}
	return lastErrs
}
