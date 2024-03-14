// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// defaultImageVector is a constant for the path to the default image vector file.
	defaultImageVector = "images.yaml"
)

// getImageYAMLPath returns the path to the image vector YAML file.
// The path to the default image vector YAML path is returned, unless `useEtcdWrapperImageVector`
// is set to true, in which case the path to the etcd wrapper image vector YAML is returned.
func getImageYAMLPath() string {
	return filepath.Join(common.ChartPath, defaultImageVector)
}

// CreateImageVector creates an image vector from the default images.yaml file or the images-wrapper.yaml file.
func CreateImageVector() (imagevector.ImageVector, error) {
	imageVector, err := imagevector.ReadGlobalImageVectorWithEnvOverride(getImageYAMLPath())
	if err != nil {
		return nil, err
	}
	return imageVector, nil
}

// ContainsFinalizer checks if an object has a finalizer present on it.
// TODO: With the controller-runtime version 0.16.x onwards this is provided by controllerutil.ContainsFinalizer.
// TODO: Remove this function once we move to this version.
func ContainsFinalizer(o client.Object, finalizer string) bool {
	finalizers := o.GetFinalizers()
	for _, f := range finalizers {
		if f == finalizer {
			return true
		}
	}
	return false
}

func GetLatestEtcd(ctx context.Context, client client.Client, objectKey client.ObjectKey, etcd *druidv1alpha1.Etcd) ReconcileStepResult {
	if err := client.Get(ctx, objectKey, etcd); err != nil {
		if apierrors.IsNotFound(err) {
			return DoNotRequeue()
		}
		return ReconcileWithError(err)
	}
	return ContinueReconcile()
}

type ReconcileStepResult struct {
	result            ctrl.Result
	errs              []error
	description       string
	continueReconcile bool
}

func (r ReconcileStepResult) ReconcileResult() (ctrl.Result, error) {
	return r.result, errors.Join(r.errs...)
}

func (r ReconcileStepResult) GetErrors() []error {
	return r.errs
}

func (r ReconcileStepResult) GetResult() ctrl.Result {
	return r.result
}

func (r ReconcileStepResult) HasErrors() bool {
	return len(r.errs) > 0
}

func (r ReconcileStepResult) GetDescription() string {
	if len(r.errs) > 0 {
		return fmt.Sprintf("%s %s", r.description, errors.Join(r.errs...).Error())
	}
	return r.description
}

func DoNotRequeue() ReconcileStepResult {
	return ReconcileStepResult{
		continueReconcile: false,
		result:            ctrl.Result{Requeue: false},
	}
}

func ContinueReconcile() ReconcileStepResult {
	return ReconcileStepResult{
		continueReconcile: true,
	}
}

func ReconcileWithError(errs ...error) ReconcileStepResult {
	return ReconcileStepResult{
		continueReconcile: false,
		result:            ctrl.Result{Requeue: true},
		errs:              errs,
	}
}

func ReconcileAfter(period time.Duration, description string) ReconcileStepResult {
	return ReconcileStepResult{
		continueReconcile: false,
		result:            ctrl.Result{RequeueAfter: period},
		description:       description,
	}
}

func ReconcileWithErrorAfter(period time.Duration, errs ...error) ReconcileStepResult {
	return ReconcileStepResult{
		result:            ctrl.Result{RequeueAfter: period},
		errs:              errs,
		continueReconcile: false,
	}
}

func ShortCircuitReconcileFlow(result ReconcileStepResult) bool {
	return !result.continueReconcile
}
