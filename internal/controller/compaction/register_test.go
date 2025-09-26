// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package compaction

import (
	"crypto/rand"
	"math/big"
	"strconv"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"

	. "github.com/onsi/gomega"
)

func TestSnapshotRevisionChangedForCreateEvents(t *testing.T) {
	tests := []struct {
		name                   string
		isObjectLease          bool
		objectName             string
		isHolderIdentitySet    bool
		shouldAllowCreateEvent bool
	}{
		{
			name:                   "object is not a lease object",
			isObjectLease:          false,
			objectName:             "not-a-lease",
			shouldAllowCreateEvent: false,
		},
		{
			name:                   "object is a lease object, but not a snapshot lease",
			isObjectLease:          true,
			objectName:             "different-lease",
			shouldAllowCreateEvent: false,
		},
		{
			name:                   "object is a new delta-snapshot lease, but holder identity is not set",
			isObjectLease:          true,
			objectName:             "etcd-test-delta-snap",
			isHolderIdentitySet:    false,
			shouldAllowCreateEvent: true,
		},
		{
			name:                   "object is a new delta-snapshot lease, and holder identity is set",
			isObjectLease:          true,
			objectName:             "etcd-test-delta-snap",
			isHolderIdentitySet:    true,
			shouldAllowCreateEvent: true,
		},
		{
			name:                   "object is a new full-snapshot lease, but holder identity is not set",
			isObjectLease:          true,
			objectName:             "etcd-test-full-snap",
			isHolderIdentitySet:    false,
			shouldAllowCreateEvent: true,
		},
		{
			name:                   "object is a new full-snapshot lease, and holder identity is set",
			isObjectLease:          true,
			objectName:             "etcd-test-full-snap",
			isHolderIdentitySet:    true,
			shouldAllowCreateEvent: true,
		},
	}

	g := NewWithT(t)
	t.Parallel()
	predicate := snapshotRevisionChanged()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			obj, _ := createObjectsForSnapshotLeasePredicate(g, test.objectName, test.isObjectLease, true, test.isHolderIdentitySet, false)
			g.Expect(predicate.Create(event.CreateEvent{Object: obj})).To(Equal(test.shouldAllowCreateEvent))
		})
	}
}

func TestSnapshotRevisionChangedForUpdateEvents(t *testing.T) {
	tests := []struct {
		name                    string
		isObjectLease           bool
		objectName              string
		isHolderIdentityChanged bool
		shouldAllowUpdateEvent  bool
	}{
		{
			name:                   "object is not a lease object",
			isObjectLease:          false,
			objectName:             "not-a-lease",
			shouldAllowUpdateEvent: false,
		},
		{
			name:                   "object is a lease object, but not a snapshot lease",
			isObjectLease:          true,
			objectName:             "different-lease",
			shouldAllowUpdateEvent: false,
		},
		{
			name:                    "object is a delta-snapshot lease, but holder identity is not changed",
			isObjectLease:           true,
			objectName:              "etcd-test-delta-snap",
			isHolderIdentityChanged: false,
			shouldAllowUpdateEvent:  false,
		},
		{
			name:                    "object is a delta-snapshot lease, and holder identity is changed",
			isObjectLease:           true,
			objectName:              "etcd-test-delta-snap",
			isHolderIdentityChanged: true,
			shouldAllowUpdateEvent:  true,
		},
		{
			name:                    "object is a full-snapshot lease, but holder identity is not changed",
			isObjectLease:           true,
			objectName:              "etcd-test-full-snap",
			isHolderIdentityChanged: false,
			shouldAllowUpdateEvent:  false,
		},
		{
			name:                    "object is a full-snapshot lease, and holder identity is changed",
			isObjectLease:           true,
			objectName:              "etcd-test-full-snap",
			isHolderIdentityChanged: true,
			shouldAllowUpdateEvent:  true,
		},
	}

	g := NewWithT(t)
	t.Parallel()
	predicate := snapshotRevisionChanged()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			obj, oldObj := createObjectsForSnapshotLeasePredicate(g, test.objectName, test.isObjectLease, false, true, test.isHolderIdentityChanged)
			g.Expect(predicate.Update(event.UpdateEvent{ObjectOld: oldObj, ObjectNew: obj})).To(Equal(test.shouldAllowUpdateEvent))
		})
	}
}

func TestSnapshotRevisionChangedForDeleteEvents(t *testing.T) {
	g := NewWithT(t)
	t.Parallel()
	predicate := snapshotRevisionChanged()
	obj, _ := createObjectsForSnapshotLeasePredicate(g, "etcd-test-delta-snap", true, true, true, true)
	g.Expect(predicate.Delete(event.DeleteEvent{Object: obj})).To(BeFalse())
}

func TestSnapshotRevisionChangedForGenericEvents(t *testing.T) {
	g := NewWithT(t)
	t.Parallel()
	predicate := snapshotRevisionChanged()
	obj, _ := createObjectsForSnapshotLeasePredicate(g, "etcd-test-delta-snap", true, true, true, true)
	g.Expect(predicate.Generic(event.GenericEvent{Object: obj})).To(BeFalse())
}

func TestJobStatusChangedForUpdateEvents(t *testing.T) {
	tests := []struct {
		name                   string
		isObjectJob            bool
		isStatusChanged        bool
		shouldAllowUpdateEvent bool
	}{
		{
			name:                   "object is not a job",
			isObjectJob:            false,
			shouldAllowUpdateEvent: false,
		},
		{
			name:                   "object is a job, but status is not changed",
			isObjectJob:            true,
			isStatusChanged:        false,
			shouldAllowUpdateEvent: false,
		},
		{
			name:                   "object is a job, and status is changed",
			isObjectJob:            true,
			isStatusChanged:        true,
			shouldAllowUpdateEvent: true,
		},
	}

	g := NewWithT(t)
	t.Parallel()
	predicate := jobStatusChanged()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			obj, oldObj := createObjectsForJobStatusChangedPredicate(g, "etcd-test-compaction-job", test.isObjectJob, test.isStatusChanged)
			g.Expect(predicate.Update(event.UpdateEvent{ObjectOld: oldObj, ObjectNew: obj})).To(Equal(test.shouldAllowUpdateEvent))
		})
	}
}

func createObjectsForJobStatusChangedPredicate(g *WithT, name string, isJobObj, isStatusChanged bool) (obj client.Object, oldObj client.Object) {
	// if the object is not a job object, create a config map (random type chosen, could have been anything else as well).
	if !isJobObj {
		obj = createConfigMap(g, name)
		oldObj = createConfigMap(g, name)
		return
	}
	now := time.Now()
	// create job objects
	oldObj = &batchv1.Job{
		Status: batchv1.JobStatus{
			Active: 1,
			StartTime: &metav1.Time{
				Time: now,
			},
		},
	}
	if isStatusChanged {
		obj = &batchv1.Job{
			Status: batchv1.JobStatus{
				Succeeded: 1,
				StartTime: &metav1.Time{
					Time: now,
				},
				CompletionTime: &metav1.Time{
					Time: time.Now(),
				},
			},
		}
	} else {
		obj = oldObj
	}
	return
}

func createObjectsForSnapshotLeasePredicate(g *WithT, name string, isLeaseObj, isNewObject, isHolderIdentitySet, isHolderIdentityChanged bool) (obj client.Object, oldObj client.Object) {
	// if the object is not a lease object, create a config map (random type chosen, could have been anything else as well).
	if !isLeaseObj {
		obj = createConfigMap(g, name)
		oldObj = createConfigMap(g, name)
		return
	}

	// create lease objects
	var holderIdentity, newHolderIdentity *string
	// if it's a new object indicating a create event, create a new lease object and return.
	if isNewObject {
		if isHolderIdentitySet {
			holderIdentity = ptr.To(strconv.Itoa(generateRandomInt(g)))
		}
		obj = createLease(name, holderIdentity)
		return
	}

	// create old and new lease objects.
	holderIdentity = ptr.To(strconv.Itoa(generateRandomInt(g)))
	oldObj = createLease(name, holderIdentity)
	if isHolderIdentityChanged {
		newHolderIdentity = ptr.To(strconv.Itoa(generateRandomInt(g)))
	} else {
		newHolderIdentity = holderIdentity
	}
	obj = createLease(name, newHolderIdentity)

	return
}

func createLease(name string, holderIdentity *string) *coordinationv1.Lease {
	return &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity: holderIdentity,
		},
	}
}

func createConfigMap(g *WithT, name string) *corev1.ConfigMap {
	randInt := generateRandomInt(g)
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Data: map[string]string{
			"k": strconv.Itoa(randInt),
		},
	}
}

func generateRandomInt(g *WithT) int {
	randInt, err := rand.Int(rand.Reader, big.NewInt(1000))
	g.Expect(err).NotTo(HaveOccurred())
	return int(randInt.Int64())
}
