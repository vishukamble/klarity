package kube

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// EventIssue describes a Kubernetes Warning event.
type EventIssue struct {
	Namespace     string
	ObjectName    string
	ObjectKind    string
	Reason        string
	Message       string
	Count         int32
	LastTimestamp time.Time
}

// ListWarningEvents returns Warning events plus Normal/BackOff events (which
// carry image-pull context) in namespace whose last occurrence falls within
// the given lookback window (e.g., 15 * time.Minute).
func ListWarningEvents(ctx context.Context, cs kubernetes.Interface, namespace string, lookback time.Duration) ([]EventIssue, error) {
	// First call: all Warning events.
	warnList, err := cs.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: "type=" + corev1.EventTypeWarning,
	})
	if err != nil {
		return nil, fmt.Errorf("listing warning events in %q: %w", namespace, err)
	}

	// Second call: Normal/BackOff events — these carry the image name in their
	// message and are needed by classifyImagePullGroup().
	normalList, err := cs.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: "type=" + corev1.EventTypeNormal + ",reason=BackOff",
	})
	if err != nil {
		// Some clusters don't support compound field selectors — fall back to
		// fetching all Normal events and filtering by reason in Go.
		if strings.Contains(err.Error(), "field label not supported") {
			normalList, err = cs.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
				FieldSelector: "type=" + corev1.EventTypeNormal,
			})
			if err != nil {
				return nil, fmt.Errorf("listing normal events in %q: %w", namespace, err)
			}
		} else {
			return nil, fmt.Errorf("listing backoff events in %q: %w", namespace, err)
		}
	}

	cutoff := time.Now().Add(-lookback)
	seen := make(map[string]bool)
	var issues []EventIssue

	addEvent := func(ev corev1.Event) {
		// For Normal events only include BackOff reason — everything else is noise.
		if ev.Type == corev1.EventTypeNormal && ev.Reason != "BackOff" {
			return
		}
		last := eventLastTime(ev)
		if last.IsZero() || last.Before(cutoff) {
			return
		}
		// Deduplicate by (namespace, objectName, reason, message).
		key := ev.Namespace + "\x00" + ev.InvolvedObject.Name + "\x00" + ev.Reason + "\x00" + ev.Message
		if seen[key] {
			return
		}
		seen[key] = true
		issues = append(issues, EventIssue{
			Namespace:     ev.Namespace,
			ObjectName:    ev.InvolvedObject.Name,
			ObjectKind:    ev.InvolvedObject.Kind,
			Reason:        ev.Reason,
			Message:       ev.Message,
			Count:         ev.Count,
			LastTimestamp: last,
		})
	}

	for _, ev := range warnList.Items {
		addEvent(ev)
	}
	for _, ev := range normalList.Items {
		addEvent(ev)
	}
	return issues, nil
}

// eventLastTime returns the best available timestamp for an event.
// It prefers LastTimestamp, then EventTime, then FirstTimestamp.
func eventLastTime(ev corev1.Event) time.Time {
	if !ev.LastTimestamp.IsZero() {
		return ev.LastTimestamp.Time
	}
	if !ev.EventTime.IsZero() {
		return ev.EventTime.Time
	}
	return ev.FirstTimestamp.Time
}
