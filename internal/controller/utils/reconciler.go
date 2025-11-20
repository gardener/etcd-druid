// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"errors"
	"fmt"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetLatestEtcd returns the latest version of the Etcd object.
func GetLatestEtcd(ctx context.Context, client client.Client, objectKey client.ObjectKey, etcd *druidv1alpha1.Etcd) ReconcileStepResult {
	if err := client.Get(ctx, objectKey, etcd); err != nil {
		if apierrors.IsNotFound(err) {
			return DoNotRequeue()
		}
		return ReconcileWithError(err)
	}
	return ContinueReconcile()
}

// ReconcileStepResult holds the result of a reconcile step.
type ReconcileStepResult struct {
	result            ctrl.Result
	errs              []error
	description       string
	continueReconcile bool
}

// ReconcileResult returns the result and error from the reconcile step.
func (r ReconcileStepResult) ReconcileResult() (ctrl.Result, error) {
	return r.result, errors.Join(r.errs...)
}

// GetErrors returns the errors from the reconcile step.
func (r ReconcileStepResult) GetErrors() []error {
	return r.errs
}

// GetCombinedError returns the combined error from the reconcile step.
func (r ReconcileStepResult) GetCombinedError() error {
	return errors.Join(r.errs...)
}

// GetResult returns the result from the reconcile step.
func (r ReconcileStepResult) GetResult() ctrl.Result {
	return r.result
}

// HasErrors returns true if there are errors from the reconcile step.
func (r ReconcileStepResult) HasErrors() bool {
	return len(r.errs) > 0
}

// NeedsRequeue returns true if reconciler should requeue the request.
func (r ReconcileStepResult) NeedsRequeue() bool {
	return r.HasErrors() || r.result.Requeue || r.result.RequeueAfter > 0
}

// GetDescription returns the description of the reconcile step.
func (r ReconcileStepResult) GetDescription() string {
	if len(r.errs) > 0 {
		return fmt.Sprintf("%s %s", r.description, errors.Join(r.errs...).Error())
	}
	return r.description
}

// DoNotRequeue returns a ReconcileStepResult that does not requeue the reconciliation.
func DoNotRequeue() ReconcileStepResult {
	return ReconcileStepResult{
		continueReconcile: false,
		result:            ctrl.Result{Requeue: false},
	}
}

// ContinueReconcile returns a ReconcileStepResult that continues the reconciliation.
func ContinueReconcile() ReconcileStepResult {
	return ReconcileStepResult{
		continueReconcile: true,
	}
}

// ReconcileWithError returns a ReconcileStepResult with the given errors.
func ReconcileWithError(errs ...error) ReconcileStepResult {
	return ReconcileStepResult{
		continueReconcile: false,
		result:            ctrl.Result{Requeue: true},
		errs:              errs,
	}
}

// ReconcileAfter returns a ReconcileStepResult that requeues the reconciliation after the given period.
func ReconcileAfter(period time.Duration, description string) ReconcileStepResult {
	return ReconcileStepResult{
		continueReconcile: false,
		result:            ctrl.Result{RequeueAfter: period},
		description:       description,
	}
}

// ReconcileWithErrorAfter returns a ReconcileStepResult that requeues the reconciliation after the given period with the given errors.
func ReconcileWithErrorAfter(period time.Duration, errs ...error) ReconcileStepResult {
	return ReconcileStepResult{
		result:            ctrl.Result{RequeueAfter: period},
		errs:              errs,
		continueReconcile: false,
	}
}

// ShortCircuitReconcileFlow indicates whether to short-circuit the reconciliation.
func ShortCircuitReconcileFlow(result ReconcileStepResult) bool {
	return !result.continueReconcile
}
