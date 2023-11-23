// Copyright (c) 2023 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package condition

import (
	"context"
	"fmt"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/utils"

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
	if err != nil {
		res.reason = "UnableToFetchStatefulSet"
		res.message = fmt.Sprintf("Unable to fetch StatefulSet for etcd: %s", err.Error())
		return res
	}

	pvcEvents, err := utils.FetchPVCWarningEventsForStatefulSet(ctx, d.cl, sts)
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
