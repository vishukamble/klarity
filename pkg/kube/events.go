package kube

import (
	"context"
	"fmt"
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

// ListWarningEvents returns all Warning events in namespace whose last
// occurrence falls within the given lookback window (e.g., 15 * time.Minute).
func ListWarningEvents(ctx context.Context, cs kubernetes.Interface, namespace string, lookback time.Duration) ([]EventIssue, error) {
	events, err := cs.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: "type=" + corev1.EventTypeWarning,
	})
	if err != nil {
		return nil, fmt.Errorf("listing events in %q: %w", namespace, err)
	}

	cutoff := time.Now().Add(-lookback)
	var issues []EventIssue
	for _, ev := range events.Items {
		last := eventLastTime(ev)
		if last.IsZero() || last.Before(cutoff) {
			continue
		}
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
