package kube

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func makeStatefulSet(name, ns string, desired, ready int32) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       appsv1.StatefulSetSpec{Replicas: int32p(desired)},
		Status: appsv1.StatefulSetStatus{
			Replicas:      desired,
			ReadyReplicas: ready,
		},
	}
}

func TestListUnhealthyStatefulSets_Healthy(t *testing.T) {
	cs := fake.NewSimpleClientset(makeStatefulSet("kafka", "default", 3, 3))
	issues, err := ListUnhealthyStatefulSets(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected no issues, got %d", len(issues))
	}
}

func TestListUnhealthyStatefulSets_NotReady(t *testing.T) {
	cs := fake.NewSimpleClientset(makeStatefulSet("postgres", "default", 3, 1))
	issues, err := ListUnhealthyStatefulSets(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("want 1 issue, got %d", len(issues))
	}
	if issues[0].StatefulSetName != "postgres" {
		t.Errorf("name: want postgres, got %s", issues[0].StatefulSetName)
	}
	if issues[0].Replicas != 3 || issues[0].ReadyReplicas != 1 {
		t.Errorf("replicas: desired=%d ready=%d", issues[0].Replicas, issues[0].ReadyReplicas)
	}
}

func TestListUnhealthyStatefulSets_NilReplicasDefaultsToOne(t *testing.T) {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "sts", Namespace: "default"},
		Spec:       appsv1.StatefulSetSpec{Replicas: nil},
		Status:     appsv1.StatefulSetStatus{ReadyReplicas: 0},
	}
	cs := fake.NewSimpleClientset(sts)
	issues, err := ListUnhealthyStatefulSets(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 || issues[0].Replicas != 1 {
		t.Errorf("expected 1 issue with desired=1, got %v", issues)
	}
}

func TestListUnhealthyStatefulSets_Mixed(t *testing.T) {
	cs := fake.NewSimpleClientset(
		makeStatefulSet("ok-sts", "default", 2, 2),
		makeStatefulSet("bad-sts", "default", 3, 0),
	)
	issues, err := ListUnhealthyStatefulSets(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 || issues[0].StatefulSetName != "bad-sts" {
		t.Errorf("expected only bad-sts, got %v", issues)
	}
}
