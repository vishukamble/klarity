package kube

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func makeService(name, ns string, selector map[string]string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       corev1.ServiceSpec{Selector: selector},
	}
}

func makeEndpoints(name, ns string, addresses []corev1.EndpointAddress) *corev1.Endpoints {
	ep := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
	}
	if len(addresses) > 0 {
		ep.Subsets = []corev1.EndpointSubset{{Addresses: addresses}}
	}
	return ep
}

func TestListServicesWithNoEndpoints_Healthy(t *testing.T) {
	svc := makeService("api", "default", map[string]string{"app": "api"})
	ep := makeEndpoints("api", "default", []corev1.EndpointAddress{{IP: "10.0.0.1"}})
	cs := fake.NewSimpleClientset(svc, ep)

	issues, err := ListServicesWithNoEndpoints(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected no issues, got %d", len(issues))
	}
}

func TestListServicesWithNoEndpoints_NoReadyAddresses(t *testing.T) {
	svc := makeService("api", "default", map[string]string{"app": "api"})
	// Endpoints exist but Subsets is empty.
	ep := makeEndpoints("api", "default", nil)
	cs := fake.NewSimpleClientset(svc, ep)

	issues, err := ListServicesWithNoEndpoints(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 || issues[0].ServiceName != "api" {
		t.Errorf("expected api issue, got %v", issues)
	}
}

func TestListServicesWithNoEndpoints_MissingEndpointsObject(t *testing.T) {
	svc := makeService("orphan-svc", "default", map[string]string{"app": "orphan"})
	// No Endpoints object created.
	cs := fake.NewSimpleClientset(svc)

	issues, err := ListServicesWithNoEndpoints(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 {
		t.Errorf("expected 1 issue, got %d", len(issues))
	}
}

func TestListServicesWithNoEndpoints_SkipsNoSelector(t *testing.T) {
	// ExternalName / headless service with no selector — should be skipped.
	svc := makeService("external", "default", nil)
	cs := fake.NewSimpleClientset(svc)

	issues, err := ListServicesWithNoEndpoints(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("services with no selector should be skipped, got %d issues", len(issues))
	}
}

func TestListServicesWithNoEndpoints_Mixed(t *testing.T) {
	cs := fake.NewSimpleClientset(
		makeService("good-svc", "default", map[string]string{"app": "good"}),
		makeEndpoints("good-svc", "default", []corev1.EndpointAddress{{IP: "10.0.0.1"}}),
		makeService("bad-svc", "default", map[string]string{"app": "bad"}),
		makeEndpoints("bad-svc", "default", nil),
	)
	issues, err := ListServicesWithNoEndpoints(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 || issues[0].ServiceName != "bad-svc" {
		t.Errorf("expected only bad-svc, got %v", issues)
	}
}
