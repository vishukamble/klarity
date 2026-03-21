package kube

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func runningPod(name, ns string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
}

func waitingPod(name, ns, reason string, restarts int32) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					Image:        "myimage:latest",
					RestartCount: restarts,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{Reason: reason},
					},
				},
			},
		},
	}
}

func oomPod(name, ns string, restarts int32) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					Image:        "myimage:latest",
					RestartCount: restarts,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
					},
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled"},
					},
				},
			},
		},
	}
}

func pendingPod(name, ns string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         ns,
			CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute)),
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestListUnhealthyPods_AllHealthy(t *testing.T) {
	cs := fake.NewSimpleClientset(runningPod("healthy", "default"))
	issues, err := ListUnhealthyPods(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected no issues, got %d: %v", len(issues), issues)
	}
}

func TestListUnhealthyPods_SkipsSucceeded(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "job-pod", Namespace: "default"},
		Status:     corev1.PodStatus{Phase: corev1.PodSucceeded},
	}
	cs := fake.NewSimpleClientset(pod)
	issues, err := ListUnhealthyPods(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("succeeded pods should be skipped, got %d issues", len(issues))
	}
}

func TestListUnhealthyPods_CrashLoopBackOff(t *testing.T) {
	cs := fake.NewSimpleClientset(waitingPod("crash-pod", "default", "CrashLoopBackOff", 47))
	issues, err := ListUnhealthyPods(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("want 1 issue, got %d", len(issues))
	}
	if issues[0].Reason != "CrashLoopBackOff" {
		t.Errorf("reason: want CrashLoopBackOff, got %s", issues[0].Reason)
	}
	if issues[0].RestartCount != 47 {
		t.Errorf("restart count: want 47, got %d", issues[0].RestartCount)
	}
}

func TestListUnhealthyPods_ImagePullBackOff(t *testing.T) {
	cs := fake.NewSimpleClientset(waitingPod("img-pod", "default", "ImagePullBackOff", 0))
	issues, err := ListUnhealthyPods(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 || issues[0].Reason != "ImagePullBackOff" {
		t.Errorf("expected ImagePullBackOff issue, got %v", issues)
	}
}

func TestListUnhealthyPods_ErrImagePull(t *testing.T) {
	cs := fake.NewSimpleClientset(waitingPod("err-pod", "default", "ErrImagePull", 0))
	issues, err := ListUnhealthyPods(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 || issues[0].Reason != "ErrImagePull" {
		t.Errorf("expected ErrImagePull issue, got %v", issues)
	}
}

func TestListUnhealthyPods_OOMKilled(t *testing.T) {
	cs := fake.NewSimpleClientset(oomPod("oom-pod", "default", 14))
	issues, err := ListUnhealthyPods(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Expect CrashLoopBackOff + OOMKilled as separate issues.
	if len(issues) != 2 {
		t.Fatalf("want 2 issues (CrashLoopBackOff + OOMKilled), got %d: %v", len(issues), issues)
	}
	reasons := map[string]bool{}
	for _, iss := range issues {
		reasons[iss.Reason] = true
	}
	if !reasons["CrashLoopBackOff"] || !reasons["OOMKilled"] {
		t.Errorf("expected both CrashLoopBackOff and OOMKilled, got %v", reasons)
	}
}

func TestListUnhealthyPods_Pending(t *testing.T) {
	cs := fake.NewSimpleClientset(pendingPod("pend-pod", "default"))
	issues, err := ListUnhealthyPods(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 || issues[0].Reason != "Pending" {
		t.Errorf("expected Pending issue, got %v", issues)
	}
	if issues[0].PendingSince.IsZero() {
		t.Error("PendingSince should be set for Pending pods")
	}
}

func TestListUnhealthyPods_PendingWithSchedulingCondition(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "stuck-pod",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute)),
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{
				{
					Type:    corev1.PodScheduled,
					Status:  corev1.ConditionFalse,
					Reason:  "Unschedulable",
					Message: "0/3 nodes are available: insufficient cpu",
				},
			},
		},
	}
	cs := fake.NewSimpleClientset(pod)
	issues, err := ListUnhealthyPods(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("want 1 issue, got %d", len(issues))
	}
	if issues[0].Message != "0/3 nodes are available: insufficient cpu" {
		t.Errorf("want scheduling message, got %q", issues[0].Message)
	}
}

func TestListUnhealthyPods_MultiNamespace(t *testing.T) {
	// Issues only from the queried namespace.
	cs := fake.NewSimpleClientset(
		waitingPod("pod-a", "ns-a", "CrashLoopBackOff", 1),
		waitingPod("pod-b", "ns-b", "CrashLoopBackOff", 1),
	)
	issues, err := ListUnhealthyPods(context.Background(), cs, "ns-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 || issues[0].PodName != "pod-a" {
		t.Errorf("expected only pod-a, got %v", issues)
	}
}

func TestListUnhealthyPods_InitContainerIssue(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "init-pod", Namespace: "default"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			InitContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  "init",
					Image: "init:latest",
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
					},
					RestartCount: 3,
				},
			},
		},
	}
	cs := fake.NewSimpleClientset(pod)
	issues, err := ListUnhealthyPods(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 || issues[0].ContainerName != "init" {
		t.Errorf("expected init container issue, got %v", issues)
	}
}
