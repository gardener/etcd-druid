// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package condition

import (
	"context"
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/utils"

	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type dataVolumesReady struct {
	cl client.Client
}

func (d *dataVolumesReady) Check(ctx context.Context, etcd druidv1alpha1.Etcd) Result {
	res := &result{
		conType: druidv1alpha1.ConditionTypeDataVolumesReady,
		status:  druidv1alpha1.ConditionUnknown,
	}

	sts, err := utils.GetStatefulSet(ctx, d.cl, &etcd)
	if errors.IsNotFound(err) || sts == nil {
		res.reason = "StatefulSetNotFound"
		res.message = fmt.Sprintf("StatefulSet %s not found for etcd", etcd.Name)
		return res
	} else if err != nil {
		res.reason = "UnableToFetchStatefulSet"
		res.message = fmt.Sprintf("Unable to fetch StatefulSet for etcd: %s", err.Error())
		return res
	}

	pvcEvents, err := utils.FetchPVCWarningMessagesForStatefulSet(ctx, d.cl, sts)
	if err != nil {
		res.reason = "UnableToFetchWarningEventsForDataVolumes"
		res.message = fmt.Sprintf("Unable to fetch warning events for PVCs used by StatefulSet %v: %s", kutil.Key(sts.Name, sts.Namespace), err.Error())
		return res
	}

	if pvcEvents != "" {
		res.reason = "FoundWarningsForDataVolumes"
		res.message = pvcEvents
		res.status = druidv1alpha1.ConditionFalse
		return res
	}

	res.reason = "NoWarningsFoundForDataVolumes"
	res.message = fmt.Sprintf("No warning events found for PVCs used by StatefulSet %v", kutil.Key(sts.Name, sts.Namespace))
	res.status = druidv1alpha1.ConditionTrue
	return res
}

// DataVolumesReadyCheck returns a check for the "DataVolumesReady" condition.
func DataVolumesReadyCheck(cl client.Client) Checker {
	return &dataVolumesReady{
		cl: cl,
	}
}
