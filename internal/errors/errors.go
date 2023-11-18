package errors

import (
	"errors"
	"fmt"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DruidError struct {
	Code      druidv1alpha1.ErrorCode
	Cause     error
	Operation string
	Message   string
}

func (r *DruidError) Error() string {
	return fmt.Sprintf("[Operation: %s, Code: %s] %s", r.Operation, r.Code, r.Cause.Error())
}

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
