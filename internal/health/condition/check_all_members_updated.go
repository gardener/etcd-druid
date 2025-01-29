// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package condition

import (
	"context"
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/utils"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type allMembersUpdated struct {
	cl client.Client
}

func (a *allMembersUpdated) Check(ctx context.Context, etcd druidv1alpha1.Etcd) Result {
	res := &result{
		conType: druidv1alpha1.ConditionTypeAllMembersUpdated,
	}

	sts, err := utils.GetStatefulSet(ctx, a.cl, &etcd)
	if sts == nil && err == nil {
		res.status = druidv1alpha1.ConditionUnknown
		res.reason = "StatefulSetNotFound"
		res.message = fmt.Sprintf("StatefulSet %s not found for etcd", etcd.Name)
		return res
	} else if err != nil {
		res.status = druidv1alpha1.ConditionUnknown
		res.reason = "UnableToFetchStatefulSet"
		res.message = fmt.Sprintf("Unable to fetch StatefulSet for etcd: %s", err.Error())
		return res
	}

	if sts.Status.ObservedGeneration == sts.Generation &&
		sts.Status.UpdatedReplicas == *sts.Spec.Replicas &&
		sts.Status.UpdateRevision == sts.Status.CurrentRevision {
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
