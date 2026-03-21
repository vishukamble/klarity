package kube

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/vishukamble/klarity/pkg/config"
)

func makeNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
}

func TestResolveNamespaces_IncludeMode(t *testing.T) {
	cs := fake.NewSimpleClientset()
	filter := config.NamespaceFilter{
		Mode:    config.NamespaceModeInclude,
		Include: []string{"app", "data"},
	}
	got, err := ResolveNamespaces(context.Background(), cs, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0] != "app" || got[1] != "data" {
		t.Errorf("want [app data], got %v", got)
	}
}

func TestResolveNamespaces_AllMode(t *testing.T) {
	cs := fake.NewSimpleClientset(
		makeNamespace("app"),
		makeNamespace("kube-system"),
		makeNamespace("monitoring"),
	)
	filter := config.NamespaceFilter{
		Mode:    config.NamespaceModeAll,
		Exclude: []string{"kube-system"},
	}
	got, err := ResolveNamespaces(context.Background(), cs, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 namespaces, got %d: %v", len(got), got)
	}
	for _, ns := range got {
		if ns == "kube-system" {
			t.Error("kube-system should be excluded")
		}
	}
}

func TestResolveNamespaces_ExcludeMode(t *testing.T) {
	cs := fake.NewSimpleClientset(
		makeNamespace("alpha"),
		makeNamespace("beta"),
		makeNamespace("gamma"),
	)
	filter := config.NamespaceFilter{
		Mode:    config.NamespaceModeExclude,
		Exclude: []string{"beta"},
	}
	got, err := ResolveNamespaces(context.Background(), cs, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 namespaces, got %d: %v", len(got), got)
	}
}

func TestResolveNamespaces_InvalidMode(t *testing.T) {
	cs := fake.NewSimpleClientset()
	filter := config.NamespaceFilter{Mode: "bogus"}
	_, err := ResolveNamespaces(context.Background(), cs, filter)
	if err == nil {
		t.Fatal("expected error for invalid mode")
	}
}

func TestResolveNamespaces_AllMode_NoExclude(t *testing.T) {
	cs := fake.NewSimpleClientset(makeNamespace("ns1"), makeNamespace("ns2"))
	filter := config.NamespaceFilter{Mode: config.NamespaceModeAll}
	got, err := ResolveNamespaces(context.Background(), cs, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 namespaces, got %d", len(got))
	}
}
