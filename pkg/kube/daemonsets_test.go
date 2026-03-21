package kube

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func makeDaemonSet(name, ns string, desired, ready, misscheduled int32) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: desired,
			NumberReady:            ready,
			NumberMisscheduled:     misscheduled,
		},
	}
}

func TestListUnhealthyDaemonSets_Healthy(t *testing.T) {
	cs := fake.NewSimpleClientset(makeDaemonSet("fluentd", "default", 5, 5, 0))
	issues, err := ListUnhealthyDaemonSets(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected no issues, got %d", len(issues))
	}
}

func TestListUnhealthyDaemonSets_NotReady(t *testing.T) {
	cs := fake.NewSimpleClientset(makeDaemonSet("node-agent", "default", 10, 7, 0))
	issues, err := ListUnhealthyDaemonSets(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("want 1 issue, got %d", len(issues))
	}
	if issues[0].Desired != 10 || issues[0].Ready != 7 {
		t.Errorf("desired=%d ready=%d", issues[0].Desired, issues[0].Ready)
	}
}

func TestListUnhealthyDaemonSets_Misscheduled(t *testing.T) {
	cs := fake.NewSimpleClientset(makeDaemonSet("ds", "default", 5, 5, 2))
	issues, err := ListUnhealthyDaemonSets(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 || issues[0].Misscheduled != 2 {
		t.Errorf("expected misscheduled=2, got %v", issues)
	}
}

func TestListUnhealthyDaemonSets_Mixed(t *testing.T) {
	cs := fake.NewSimpleClientset(
		makeDaemonSet("ok-ds", "default", 3, 3, 0),
		makeDaemonSet("bad-ds", "default", 3, 1, 0),
	)
	issues, err := ListUnhealthyDaemonSets(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 || issues[0].DaemonSetName != "bad-ds" {
		t.Errorf("expected only bad-ds, got %v", issues)
	}
}
