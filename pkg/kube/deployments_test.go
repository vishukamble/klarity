package kube

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func int32p(v int32) *int32 { return &v }

func healthyDeployment(name, ns string, replicas int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       appsv1.DeploymentSpec{Replicas: int32p(replicas)},
		Status: appsv1.DeploymentStatus{
			Replicas:      replicas,
			ReadyReplicas: replicas,
		},
	}
}

func unhealthyDeployment(name, ns string, desired, ready, unavailable int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       appsv1.DeploymentSpec{Replicas: int32p(desired)},
		Status: appsv1.DeploymentStatus{
			Replicas:            desired,
			ReadyReplicas:       ready,
			UnavailableReplicas: unavailable,
		},
	}
}

func TestListUnhealthyDeployments_AllHealthy(t *testing.T) {
	cs := fake.NewSimpleClientset(healthyDeployment("api", "default", 3))
	issues, err := ListUnhealthyDeployments(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected no issues, got %d", len(issues))
	}
}

func TestListUnhealthyDeployments_UnavailableReplicas(t *testing.T) {
	cs := fake.NewSimpleClientset(unhealthyDeployment("api", "default", 3, 2, 1))
	issues, err := ListUnhealthyDeployments(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("want 1 issue, got %d", len(issues))
	}
	if issues[0].UnavailableReplicas != 1 {
		t.Errorf("unavailable: want 1, got %d", issues[0].UnavailableReplicas)
	}
}

func TestListUnhealthyDeployments_ReadyLessThanDesired(t *testing.T) {
	cs := fake.NewSimpleClientset(unhealthyDeployment("worker", "default", 5, 3, 0))
	issues, err := ListUnhealthyDeployments(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("want 1 issue, got %d", len(issues))
	}
	if issues[0].DesiredReplicas != 5 || issues[0].ReadyReplicas != 3 {
		t.Errorf("replicas mismatch: desired=%d ready=%d", issues[0].DesiredReplicas, issues[0].ReadyReplicas)
	}
}

func TestListUnhealthyDeployments_NilReplicasDefaultsToOne(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "dep", Namespace: "default"},
		Spec:       appsv1.DeploymentSpec{Replicas: nil}, // nil = default 1
		Status:     appsv1.DeploymentStatus{ReadyReplicas: 0},
	}
	cs := fake.NewSimpleClientset(dep)
	issues, err := ListUnhealthyDeployments(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 || issues[0].DesiredReplicas != 1 {
		t.Errorf("expected 1 issue with desired=1, got %v", issues)
	}
}

func TestListUnhealthyDeployments_MixedHealthy(t *testing.T) {
	cs := fake.NewSimpleClientset(
		healthyDeployment("ok-api", "default", 2),
		unhealthyDeployment("bad-worker", "default", 3, 1, 2),
	)
	issues, err := ListUnhealthyDeployments(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 || issues[0].DeploymentName != "bad-worker" {
		t.Errorf("expected only bad-worker, got %v", issues)
	}
}
