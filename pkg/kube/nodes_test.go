package kube

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestListUnhealthyNodes_AllConditions(t *testing.T) {
	nodes := &corev1.NodeList{
		Items: []corev1.Node{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "node-notready"},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionFalse, Message: "kubelet stopped posting status"},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "node-memory"},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
						{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionTrue, Message: "memory low"},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "node-disk"},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
						{Type: corev1.NodeDiskPressure, Status: corev1.ConditionTrue, Message: "disk full"},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "node-pid"},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
						{Type: corev1.NodePIDPressure, Status: corev1.ConditionTrue, Message: "too many pids"},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "node-net"},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
						{Type: corev1.NodeNetworkUnavailable, Status: corev1.ConditionTrue, Message: "cni down"},
					},
				},
			},
		},
	}

	cs := fake.NewSimpleClientset(nodes)
	issues, err := ListUnhealthyNodes(context.Background(), cs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 5 {
		t.Fatalf("want 5 issues, got %d", len(issues))
	}

	conditions := map[string]bool{}
	for _, iss := range issues {
		conditions[iss.Condition] = true
	}
	for _, want := range []string{"NotReady", "MemoryPressure", "DiskPressure", "PIDPressure", "NetworkUnavailable"} {
		if !conditions[want] {
			t.Errorf("missing condition %q in results", want)
		}
	}
}

func TestListUnhealthyNodes_AllHealthy(t *testing.T) {
	nodes := &corev1.NodeList{
		Items: []corev1.Node{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "healthy-node"},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
						{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse},
						{Type: corev1.NodeDiskPressure, Status: corev1.ConditionFalse},
					},
				},
			},
		},
	}

	cs := fake.NewSimpleClientset(nodes)
	issues, err := ListUnhealthyNodes(context.Background(), cs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("want 0 issues for healthy nodes, got %d", len(issues))
	}
}

func TestListUnhealthyNodes_ReadyUnknown(t *testing.T) {
	nodes := &corev1.NodeList{
		Items: []corev1.Node{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "unknown-node"},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionUnknown, Message: "lost contact"},
					},
				},
			},
		},
	}

	cs := fake.NewSimpleClientset(nodes)
	issues, err := ListUnhealthyNodes(context.Background(), cs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("want 1 issue, got %d", len(issues))
	}
	if issues[0].Condition != "NotReady" {
		t.Errorf("condition = %q, want NotReady", issues[0].Condition)
	}
}
