// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"fmt"
	"strings"
	"time"

	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/duration"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// fetchEventMessages gets events for the given object of the given `eventType` and returns them as a formatted output.
// The function expects that the given `involvedObj` is specified with a proper `metav1.TypeMeta`.
func fetchEventMessages(ctx context.Context, scheme *runtime.Scheme, cl client.Client, involvedObj client.Object, eventType string, eventsLimit int) (string, error) {
	events, err := listEvents(ctx, cl, involvedObj.GetNamespace())
	if err != nil {
		return "", err
	}

	events, err = filterEvents(events, scheme, involvedObj, eventType)
	if err != nil {
		return "", err
	}

	if len(events) > 0 {
		return buildEventsErrorMessage(events, eventsLimit), nil
	}
	return "", nil
}

// listEvents fetches all events in the given namespace.
func listEvents(ctx context.Context, cl client.Client, namespace string) ([]corev1.Event, error) {
	eventList := &corev1.EventList{}
	if err := cl.List(ctx, eventList, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list events: %w", err)
	}

	return eventList.Items, nil
}

// filterEvents filters the given events by the given `eventType` and `involvedObject`.
func filterEvents(events []corev1.Event, scheme *runtime.Scheme, involvedObject client.Object, eventType string) ([]corev1.Event, error) {
	gvk, err := apiutil.GVKForObject(involvedObject, scheme)
	if err != nil {
		return nil, fmt.Errorf("failed to identify GVK for object: %w", err)
	}

	apiVersion, kind := gvk.ToAPIVersionAndKind()
	if apiVersion == "" {
		return nil, fmt.Errorf("apiVersion not specified for object %s/%s", involvedObject.GetNamespace(), involvedObject.GetName())
	}
	if kind == "" {
		return nil, fmt.Errorf("kind not specified for object %s/%s", involvedObject.GetNamespace(), involvedObject.GetName())
	}

	var filteredEvents []corev1.Event
	for _, event := range events {
		if event.Type == eventType &&
			event.InvolvedObject.APIVersion == apiVersion &&
			event.InvolvedObject.Kind == kind &&
			event.InvolvedObject.Name == involvedObject.GetName() &&
			event.InvolvedObject.Namespace == involvedObject.GetNamespace() {
			filteredEvents = append(filteredEvents, event)
		}
	}

	return filteredEvents, nil
}

func buildEventsErrorMessage(events []corev1.Event, eventsLimit int) string {
	sortByLastTimestamp := func(o1, o2 client.Object) bool {
		obj1, ok1 := o1.(*corev1.Event)
		obj2, ok2 := o2.(*corev1.Event)

		if !ok1 || !ok2 {
			return false
		}

		return obj1.LastTimestamp.Time.Before(obj2.LastTimestamp.Time)
	}

	list := &corev1.EventList{Items: events}
	kutil.SortBy(sortByLastTimestamp).Sort(list)
	events = list.Items

	if len(events) > eventsLimit {
		events = events[len(events)-eventsLimit:]
	}

	var builder strings.Builder
	fmt.Fprintf(&builder, "-> Events:")
	for _, event := range events {
		var interval string
		if event.Count > 1 {
			interval = fmt.Sprintf("%s ago (x%d over %s)", translateTimestampSince(event.LastTimestamp), event.Count, translateTimestampSince(event.FirstTimestamp))
		} else {
			interval = fmt.Sprintf("%s ago", translateTimestampSince(event.FirstTimestamp))
			if event.FirstTimestamp.IsZero() {
				interval = fmt.Sprintf("%s ago", translateMicroTimestampSince(event.EventTime))
			}
		}
		source := event.Source.Component
		if source == "" {
			source = event.ReportingController
		}

		fmt.Fprintf(&builder, "\n* %s reported %s: %s", source, interval, event.Message)
	}

	return builder.String()
}

// translateTimestampSince returns the elapsed time since timestamp in
// human-readable approximation.
func translateTimestampSince(timestamp metav1.Time) string {
	if timestamp.IsZero() {
		return "<unknown>"
	}

	return duration.HumanDuration(time.Since(timestamp.Time))
}

// translateMicroTimestampSince returns the elapsed time since timestamp in
// human-readable approximation.
func translateMicroTimestampSince(timestamp metav1.MicroTime) string {
	if timestamp.IsZero() {
		return "<unknown>"
	}

	return duration.HumanDuration(time.Since(timestamp.Time))
}
