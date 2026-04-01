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
	got, err := ResolveNamespaces(context.Background(), cs, filter, nil)
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
	got, err := ResolveNamespaces(context.Background(), cs, filter, nil)
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
	got, err := ResolveNamespaces(context.Background(), cs, filter, nil)
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
	_, err := ResolveNamespaces(context.Background(), cs, filter, nil)
	if err == nil {
		t.Fatal("expected error for invalid mode")
	}
}

func TestResolveNamespaces_AllMode_NoExclude(t *testing.T) {
	cs := fake.NewSimpleClientset(makeNamespace("ns1"), makeNamespace("ns2"))
	filter := config.NamespaceFilter{Mode: config.NamespaceModeAll}
	got, err := ResolveNamespaces(context.Background(), cs, filter, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 namespaces, got %d", len(got))
	}
}

func TestResolveNamespaces_AllMode_UsesDefaultExclude(t *testing.T) {
	// mode=all with empty cluster exclude → defaultExclude is applied.
	cs := fake.NewSimpleClientset(
		makeNamespace("app"),
		makeNamespace("kube-system"),
		makeNamespace("kube-public"),
		makeNamespace("monitoring"),
	)
	filter := config.NamespaceFilter{Mode: config.NamespaceModeAll}
	defaultExclude := []string{"kube-system", "kube-public"}

	got, err := ResolveNamespaces(context.Background(), cs, filter, defaultExclude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 namespaces, got %d: %v", len(got), got)
	}
	for _, ns := range got {
		if ns == "kube-system" || ns == "kube-public" {
			t.Errorf("namespace %q should be excluded by defaultExclude", ns)
		}
	}
}

func TestResolveNamespaces_AllMode_ExplicitExcludeWins(t *testing.T) {
	// mode=all with explicit cluster exclude → defaultExclude is ignored.
	cs := fake.NewSimpleClientset(
		makeNamespace("app"),
		makeNamespace("kube-system"),
		makeNamespace("monitoring"),
	)
	filter := config.NamespaceFilter{
		Mode:    config.NamespaceModeAll,
		Exclude: []string{"monitoring"}, // cluster-specific exclude
	}
	defaultExclude := []string{"kube-system", "kube-public"} // should be ignored

	got, err := ResolveNamespaces(context.Background(), cs, filter, defaultExclude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// monitoring excluded (cluster exclude), kube-system NOT excluded (default ignored)
	if len(got) != 2 {
		t.Fatalf("want 2 namespaces, got %d: %v", len(got), got)
	}
	found := map[string]bool{}
	for _, ns := range got {
		found[ns] = true
	}
	if found["monitoring"] {
		t.Error("monitoring should be excluded by cluster filter")
	}
	if !found["kube-system"] {
		t.Error("kube-system should NOT be excluded when explicit cluster filter overrides defaultExclude")
	}
}

func TestResolveNamespaces_IncludeMode_DefaultExcludeIgnored(t *testing.T) {
	// mode=include is unaffected by defaultExclude.
	cs := fake.NewSimpleClientset()
	filter := config.NamespaceFilter{
		Mode:    config.NamespaceModeInclude,
		Include: []string{"payments", "kube-system"},
	}
	defaultExclude := []string{"kube-system"}

	got, err := ResolveNamespaces(context.Background(), cs, filter, defaultExclude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// include mode returns exactly what's listed, defaultExclude doesn't apply
	if len(got) != 2 {
		t.Fatalf("want 2 namespaces, got %d: %v", len(got), got)
	}
}

func TestMatchNamespaces(t *testing.T) {
	tests := []struct {
		name       string
		patterns   []string
		candidates []string
		want       []string
		wantErr    bool
	}{
		{
			name:       "ExactMatch",
			patterns:   []string{"payments"},
			candidates: []string{"payments", "analytics"},
			want:       []string{"payments"},
		},
		{
			name:       "WildcardSuffix",
			patterns:   []string{"ku*"},
			candidates: []string{"kube-system", "kube-public", "payments"},
			want:       []string{"kube-system", "kube-public"},
		},
		{
			name:       "WildcardPrefix",
			patterns:   []string{"*-system"},
			candidates: []string{"kube-system", "monitoring", "log-system"},
			want:       []string{"kube-system", "log-system"},
		},
		{
			name:       "Mixed",
			patterns:   []string{"payments", "ku*"},
			candidates: []string{"kube-system", "payments", "analytics"},
			want:       []string{"kube-system", "payments"},
		},
		{
			name:       "NoMatch",
			patterns:   []string{"xyz*"},
			candidates: []string{"payments", "analytics"},
			want:       nil,
		},
		{
			name:       "ExactNoMatch",
			patterns:   []string{"missing"},
			candidates: []string{"payments"},
			want:       nil,
		},
		{
			name:       "Dedup",
			patterns:   []string{"*-system", "kube-*"},
			candidates: []string{"kube-system", "payments"},
			want:       []string{"kube-system"},
		},
		{
			name:       "OrderPreserved",
			patterns:   []string{"b*", "a*"},
			candidates: []string{"alpha", "beta", "gamma"},
			want:       []string{"alpha", "beta"},
		},
		{
			name:       "InvalidPattern",
			patterns:   []string{"[invalid"},
			candidates: []string{"payments"},
			wantErr:    true,
		},
		{
			name:       "EmptyPatterns",
			patterns:   []string{},
			candidates: []string{"payments"},
			want:       []string{"payments"},
		},
		{
			name:       "EmptyCandidates",
			patterns:   []string{"ku*"},
			candidates: []string{},
			want:       nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MatchNamespaces(tt.patterns, tt.candidates)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("want %v, got %v", tt.want, got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("index %d: want %q, got %q", i, tt.want[i], got[i])
				}
			}
		})
	}
}

func TestResolveNamespaces_ExcludeMode_DefaultExcludeIgnored(t *testing.T) {
	// mode=exclude uses cluster's explicit exclude list, not defaultExclude.
	cs := fake.NewSimpleClientset(
		makeNamespace("alpha"),
		makeNamespace("beta"),
		makeNamespace("kube-system"),
	)
	filter := config.NamespaceFilter{
		Mode:    config.NamespaceModeExclude,
		Exclude: []string{"beta"},
	}
	defaultExclude := []string{"kube-system"}

	got, err := ResolveNamespaces(context.Background(), cs, filter, defaultExclude)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// only "beta" excluded; kube-system remains (defaultExclude not applied to exclude mode)
	if len(got) != 2 {
		t.Fatalf("want 2 namespaces, got %d: %v", len(got), got)
	}
	found := map[string]bool{}
	for _, ns := range got {
		found[ns] = true
	}
	if !found["kube-system"] {
		t.Error("kube-system should be included in exclude mode (defaultExclude only applies to all mode)")
	}
}
