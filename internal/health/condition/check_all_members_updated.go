// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package condition

import (
	"context"
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/utils/kubernetes"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type allMembersUpdated struct {
	cl client.Client
}

// Check evaluates AllMembersUpdated. RollingUpdate uses the fast field-comparison
// path; OnDelete uses a pod-level check because K8s < v1.37 does not promote
// status.currentRevision under OnDelete (kubernetes/kubernetes#73492, #106055;
// fix in #136833). The pod-level path is not gated on K8s version.
func (a *allMembersUpdated) Check(ctx context.Context, etcd druidv1alpha1.Etcd) Result {
	res := &result{
		conType: druidv1alpha1.ConditionTypeAllMembersUpdated,
	}

	sts := &appsv1.StatefulSet{}
	if err := a.cl.Get(ctx, client.ObjectKey{Namespace: etcd.Namespace, Name: druidv1alpha1.GetStatefulSetName(etcd.ObjectMeta)}, sts); err != nil {
		res.status = druidv1alpha1.ConditionUnknown
		if apierrors.IsNotFound(err) {
			res.reason = "StatefulSetNotFound"
			res.message = fmt.Sprintf("StatefulSet not found for etcd %s", etcd.Name)
		} else {
			res.reason = "UnableToFetchStatefulSet"
			res.message = fmt.Sprintf("Unable to fetch StatefulSet for etcd: %s", err.Error())
		}
		return res
	}
	if !metav1.IsControlledBy(sts, &etcd) {
		res.status = druidv1alpha1.ConditionUnknown
		res.reason = "StatefulSetNotControlledByEtcd"
		res.message = fmt.Sprintf("StatefulSet %s exists but is not controlled by etcd %s", sts.Name, etcd.Name)
		return res
	}

	if sts.Status.ObservedGeneration != sts.Generation {
		res.status = druidv1alpha1.ConditionFalse
		res.reason = "NotAllMembersUpdated"
		res.message = "StatefulSet controller has not yet observed the latest generation"
		return res
	}

	allUpdated, err := kubernetes.AreStatefulSetReplicasUpdated(ctx, a.cl, sts)
	if err != nil {
		res.status = druidv1alpha1.ConditionUnknown
		res.reason = "UnableToDetermineMemberRevisions"
		res.message = fmt.Sprintf("Unable to determine whether all replicas of StatefulSet %s are updated: %s", sts.Name, err.Error())
		return res
	}
	if allUpdated {
		res.status = druidv1alpha1.ConditionTrue
		res.reason = "AllMembersUpdated"
		res.message = "All members reflect latest desired spec"
		return res
	}
	res.status = druidv1alpha1.ConditionFalse
	res.reason = "NotAllMembersUpdated"
	res.message = "At least one member is not yet updated"
	return res
}

// AllMembersUpdatedCheck returns a check for the "AllMembersUpdated" condition.
func AllMembersUpdatedCheck(cl client.Client) Checker {
	return &allMembersUpdated{
		cl: cl,
	}
}
