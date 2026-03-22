package kube

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func makeEvent(name, ns, kind, reason, msg string, lastSeen time.Time, count int32) *corev1.Event {
	return makeTypedEvent(name, ns, kind, reason, msg, corev1.EventTypeWarning, lastSeen, count)
}

func makeTypedEvent(name, ns, kind, reason, msg, evType string, lastSeen time.Time, count int32) *corev1.Event {
	return &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		InvolvedObject: corev1.ObjectReference{
			Name: name + "-obj",
			Kind: kind,
		},
		Type:          evType,
		Reason:        reason,
		Message:       msg,
		Count:         count,
		LastTimestamp: metav1.NewTime(lastSeen),
	}
}

func TestListWarningEvents_WithinWindow(t *testing.T) {
	recent := time.Now().Add(-5 * time.Minute)
	cs := fake.NewSimpleClientset(
		makeEvent("ev1", "default", "Pod", "BackOff", "Back-off restarting", recent, 3),
	)
	issues, err := ListWarningEvents(context.Background(), cs, "default", 15*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("want 1 issue, got %d", len(issues))
	}
	if issues[0].Reason != "BackOff" {
		t.Errorf("reason: want BackOff, got %s", issues[0].Reason)
	}
	if issues[0].Count != 3 {
		t.Errorf("count: want 3, got %d", issues[0].Count)
	}
}

func TestListWarningEvents_OutsideWindow(t *testing.T) {
	old := time.Now().Add(-30 * time.Minute)
	cs := fake.NewSimpleClientset(
		makeEvent("ev-old", "default", "Pod", "BackOff", "old event", old, 1),
	)
	issues, err := ListWarningEvents(context.Background(), cs, "default", 15*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected no issues for old event, got %d", len(issues))
	}
}

func TestListWarningEvents_MixedTimes(t *testing.T) {
	recent := time.Now().Add(-2 * time.Minute)
	old := time.Now().Add(-20 * time.Minute)
	cs := fake.NewSimpleClientset(
		makeEvent("new-ev", "default", "Pod", "OOMKilling", "OOM killed", recent, 1),
		makeEvent("old-ev", "default", "Pod", "BackOff", "old", old, 5),
	)
	issues, err := ListWarningEvents(context.Background(), cs, "default", 15*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 || issues[0].Reason != "OOMKilling" {
		t.Errorf("expected only OOMKilling, got %v", issues)
	}
}

func TestListWarningEvents_ObjectInfo(t *testing.T) {
	recent := time.Now().Add(-1 * time.Minute)
	ev := makeEvent("crash-ev", "kube-system", "Pod", "CrashLoopBackOff", "container keeps crashing", recent, 10)
	cs := fake.NewSimpleClientset(ev)
	issues, err := ListWarningEvents(context.Background(), cs, "kube-system", 15*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("want 1 issue, got %d", len(issues))
	}
	if issues[0].ObjectKind != "Pod" {
		t.Errorf("ObjectKind: want Pod, got %s", issues[0].ObjectKind)
	}
	if issues[0].Namespace != "kube-system" {
		t.Errorf("Namespace: want kube-system, got %s", issues[0].Namespace)
	}
}

func TestListWarningEvents_Empty(t *testing.T) {
	cs := fake.NewSimpleClientset()
	issues, err := ListWarningEvents(context.Background(), cs, "default", 15*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected no issues, got %d", len(issues))
	}
}

func TestListWarningEvents_IncludesNormalBackOff(t *testing.T) {
	recent := time.Now().Add(-5 * time.Minute)
	cs := fake.NewSimpleClientset(
		// Warning/Failed — captured by first list call.
		makeTypedEvent("ev-failed", "default", "Pod", "Failed", `Error: ImagePullBackOff`, corev1.EventTypeWarning, recent, 2),
		// Normal/BackOff — captured by second list call; contains image name.
		makeTypedEvent("ev-backoff", "default", "Pod", "BackOff", `Back-off pulling image "nginx:1.25"`, corev1.EventTypeNormal, recent, 5),
		// Normal/Pulling — should be excluded (reason != BackOff).
		makeTypedEvent("ev-pulling", "default", "Pod", "Pulling", `Pulling image "nginx:1.25"`, corev1.EventTypeNormal, recent, 1),
	)
	issues, err := ListWarningEvents(context.Background(), cs, "default", 15*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Collect reasons for easy lookup.
	reasons := make(map[string]bool)
	for _, iss := range issues {
		reasons[iss.Reason] = true
	}

	if !reasons["Failed"] {
		t.Error("want Warning/Failed event to be included")
	}
	if !reasons["BackOff"] {
		t.Error("want Normal/BackOff event to be included")
	}
	if reasons["Pulling"] {
		t.Error("Normal/Pulling event should be excluded (only BackOff reason included from Normal events)")
	}
}

func TestListWarningEvents_DeduplicatesBackOff(t *testing.T) {
	// The fake clientset ignores field selectors so both list calls return all
	// events. Verify that deduplication prevents duplicate findings.
	recent := time.Now().Add(-3 * time.Minute)
	cs := fake.NewSimpleClientset(
		makeTypedEvent("ev1", "default", "Pod", "BackOff", `Back-off pulling image "redis:7"`, corev1.EventTypeNormal, recent, 4),
	)
	issues, err := ListWarningEvents(context.Background(), cs, "default", 15*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	backOffCount := 0
	for _, iss := range issues {
		if iss.Reason == "BackOff" {
			backOffCount++
		}
	}
	if backOffCount != 1 {
		t.Errorf("want exactly 1 BackOff issue after dedup, got %d", backOffCount)
	}
}
