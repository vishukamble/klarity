package kube

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/vishukamble/klarity/pkg/config"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// fakeBuilder returns a ClientsetBuilder that always succeeds with a new
// fake.Clientset. Use when the test doesn't care about client content.
func fakeBuilder() ClientsetBuilder {
	return func(_, _ string) (kubernetes.Interface, error) {
		return fake.NewSimpleClientset(), nil
	}
}

// errorBuilder returns a ClientsetBuilder that always fails with msg.
func errorBuilder(msg string) ClientsetBuilder {
	return func(_, contextName string) (kubernetes.Interface, error) {
		return nil, fmt.Errorf("%s: %s", contextName, msg)
	}
}

// minimalCfg builds a Config with all given contexts in one "test" environment.
func minimalCfg(parallelClusters int, contexts ...string) *config.Config {
	clusters := make([]config.Cluster, len(contexts))
	for i, ctx := range contexts {
		clusters[i] = config.Cluster{
			Context:    ctx,
			Namespaces: config.NamespaceFilter{Mode: config.NamespaceModeAll},
		}
	}
	return &config.Config{
		Version: 1,
		Environments: []config.Environment{
			{Name: "test", Tier: config.TierStandard, Clusters: clusters},
		},
		Settings: config.Settings{
			LogTailLines:        50,
			ParallelClusters:    parallelClusters,
			ScanIntervalSeconds: 300,
		},
	}
}

// ── ScanAll: call routing ─────────────────────────────────────────────────────

func TestScanAll_CallsEachCluster(t *testing.T) {
	cfg := minimalCfg(4, "ctx-a", "ctx-b", "ctx-c")

	var mu sync.Mutex
	called := map[string]bool{}

	err := ScanAll(context.Background(), cfg, "", fakeBuilder(),
		func(_ context.Context, _ config.Environment, cl config.Cluster, _ kubernetes.Interface) error {
			mu.Lock()
			called[cl.Context] = true
			mu.Unlock()
			return nil
		})

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	for _, ctx := range []string{"ctx-a", "ctx-b", "ctx-c"} {
		if !called[ctx] {
			t.Errorf("ScanFunc not called for context %q", ctx)
		}
	}
}

func TestScanAll_CallCountMatchesClusters(t *testing.T) {
	cfg := minimalCfg(4, "ctx-1", "ctx-2", "ctx-3", "ctx-4", "ctx-5")

	var count atomic.Int32
	err := ScanAll(context.Background(), cfg, "", fakeBuilder(),
		func(_ context.Context, _ config.Environment, _ config.Cluster, _ kubernetes.Interface) error {
			count.Add(1)
			return nil
		})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count.Load() != 5 {
		t.Errorf("expected ScanFunc called 5 times, got %d", count.Load())
	}
}

func TestScanAll_PassesCorrectEnvAndCluster(t *testing.T) {
	cfg := &config.Config{
		Version: 1,
		Environments: []config.Environment{
			{Name: "prod", Tier: config.TierCritical, Clusters: []config.Cluster{
				{Context: "prod-east", Namespaces: config.NamespaceFilter{Mode: config.NamespaceModeAll}},
				{Context: "prod-west", Namespaces: config.NamespaceFilter{Mode: config.NamespaceModeAll}},
			}},
			{Name: "dev", Tier: config.TierStandard, Clusters: []config.Cluster{
				{Context: "dev-east", Namespaces: config.NamespaceFilter{Mode: config.NamespaceModeAll}},
			}},
		},
		Settings: config.Settings{LogTailLines: 50, ParallelClusters: 4, ScanIntervalSeconds: 300},
	}

	type call struct{ env, ctx string }
	var mu sync.Mutex
	var calls []call

	if err := ScanAll(context.Background(), cfg, "", fakeBuilder(),
		func(_ context.Context, env config.Environment, cl config.Cluster, _ kubernetes.Interface) error {
			mu.Lock()
			calls = append(calls, call{env.Name, cl.Context})
			mu.Unlock()
			return nil
		}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(calls) != 3 {
		t.Fatalf("expected 3 calls, got %d: %v", len(calls), calls)
	}

	lookup := map[string]string{}
	for _, c := range calls {
		lookup[c.ctx] = c.env
	}
	if lookup["prod-east"] != "prod" {
		t.Errorf("prod-east: want env=prod, got %q", lookup["prod-east"])
	}
	if lookup["prod-west"] != "prod" {
		t.Errorf("prod-west: want env=prod, got %q", lookup["prod-west"])
	}
	if lookup["dev-east"] != "dev" {
		t.Errorf("dev-east: want env=dev, got %q", lookup["dev-east"])
	}
}

// ── ScanAll: error handling ───────────────────────────────────────────────────

func TestScanAll_ScanFuncErrorPropagates(t *testing.T) {
	cfg := minimalCfg(4, "ctx-a", "ctx-b", "ctx-c")

	err := ScanAll(context.Background(), cfg, "", fakeBuilder(),
		func(_ context.Context, _ config.Environment, cl config.Cluster, _ kubernetes.Interface) error {
			if cl.Context == "ctx-b" {
				return errors.New("scan failed")
			}
			return nil
		})

	if err == nil {
		t.Fatal("expected error from ScanAll, got nil")
	}
	if !strings.Contains(err.Error(), "scan failed") {
		t.Errorf("error should contain 'scan failed', got: %v", err)
	}
}

func TestScanAll_BuilderErrorPropagates(t *testing.T) {
	cfg := minimalCfg(4, "ctx-a")

	err := ScanAll(context.Background(), cfg, "", errorBuilder("cannot connect"), nil)
	if err == nil {
		t.Fatal("expected error when builder fails, got nil")
	}
	if !strings.Contains(err.Error(), "cannot connect") {
		t.Errorf("error should contain 'cannot connect', got: %v", err)
	}
}

func TestScanAll_ErrorWrapsContextName(t *testing.T) {
	cfg := minimalCfg(4, "my-special-context")

	err := ScanAll(context.Background(), cfg, "", fakeBuilder(),
		func(_ context.Context, _ config.Environment, _ config.Cluster, _ kubernetes.Interface) error {
			return errors.New("something broke")
		})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "my-special-context") {
		t.Errorf("error should mention context name, got: %v", err)
	}
}

func TestScanAll_EmptyConfig(t *testing.T) {
	cfg := &config.Config{
		Version:      1,
		Environments: nil,
		Settings:     config.Settings{LogTailLines: 50, ParallelClusters: 4, ScanIntervalSeconds: 300},
	}

	called := false
	err := ScanAll(context.Background(), cfg, "", fakeBuilder(),
		func(_ context.Context, _ config.Environment, _ config.Cluster, _ kubernetes.Interface) error {
			called = true
			return nil
		})
	if err != nil {
		t.Fatalf("expected no error for empty config, got: %v", err)
	}
	if called {
		t.Error("ScanFunc should not be called when there are no clusters")
	}
}

// ── ScanAll: nil builder uses BuildClientset ──────────────────────────────────

func TestScanAll_NilBuilderUsesDefault(t *testing.T) {
	// We pass a nonexistent kubeconfig — the default builder will be used and
	// should fail (which proves it was invoked, not ignored).
	cfg := minimalCfg(1, "any-context")
	err := ScanAll(context.Background(), cfg, "/nonexistent/kubeconfig", nil, nil)
	if err == nil {
		t.Fatal("expected error from default builder with bad kubeconfig, got nil")
	}
}

// ── ScanAll: concurrency ──────────────────────────────────────────────────────

// TestScanAll_ConcurrencyLimit verifies that at most `limit` goroutines execute
// ScanFunc simultaneously. It uses a barrier: the first `limit` goroutines open
// a gate that lets all of them (and only them) proceed, so the measured peak is
// deterministic for the "at most limit" assertion.
func TestScanAll_ConcurrencyLimit(t *testing.T) {
	const numClusters = 8
	const limit = 3

	contexts := make([]string, numClusters)
	for i := range contexts {
		contexts[i] = fmt.Sprintf("ctx-%d", i)
	}
	cfg := minimalCfg(limit, contexts...)

	var (
		mu          sync.Mutex
		current     int
		maxObserved int
	)

	var started atomic.Int32
	gate := make(chan struct{})

	err := ScanAll(context.Background(), cfg, "", fakeBuilder(),
		func(_ context.Context, _ config.Environment, _ config.Cluster, _ kubernetes.Interface) error {
			mu.Lock()
			current++
			if current > maxObserved {
				maxObserved = current
			}
			mu.Unlock()

			// Once `limit` goroutines have arrived, open the gate.
			if started.Add(1) == int32(limit) {
				close(gate)
			}
			<-gate // all goroutines wait here until the first batch opens it

			mu.Lock()
			current--
			mu.Unlock()
			return nil
		})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if maxObserved > limit {
		t.Errorf("concurrency limit exceeded: max observed %d > limit %d", maxObserved, limit)
	}
	if maxObserved < 1 {
		t.Errorf("expected at least 1 concurrent execution, got %d", maxObserved)
	}
}

func TestScanAll_ZeroParallelDefaultsToOne(t *testing.T) {
	// parallel_clusters: 0 should be coerced to 1 rather than deadlocking.
	cfg := minimalCfg(0, "ctx-a", "ctx-b")

	var count atomic.Int32
	err := ScanAll(context.Background(), cfg, "", fakeBuilder(),
		func(_ context.Context, _ config.Environment, _ config.Cluster, _ kubernetes.Interface) error {
			count.Add(1)
			return nil
		})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count.Load() != 2 {
		t.Errorf("expected 2 calls, got %d", count.Load())
	}
}

// ── BuildClientset error paths ────────────────────────────────────────────────

func TestBuildClientset_MissingKubeconfig(t *testing.T) {
	_, err := BuildClientset("/nonexistent/path/config", "some-context")
	if err == nil {
		t.Fatal("expected error for nonexistent kubeconfig, got nil")
	}
}

func TestBuildClientset_NoMatchingContext(t *testing.T) {
	// A syntactically valid kubeconfig with no contexts.
	dir := t.TempDir()
	path := dir + "/kubeconfig"
	if err := os.WriteFile(path, []byte(`apiVersion: v1
kind: Config
clusters: []
contexts: []
users: []
current-context: ""
`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := BuildClientset(path, "nonexistent-context")
	if err == nil {
		t.Fatal("expected error for missing context, got nil")
	}
}

// ── DefaultKubeconfigPath ─────────────────────────────────────────────────────

func TestDefaultKubeconfigPath_EnvOverride(t *testing.T) {
	t.Setenv("KUBECONFIG", "/custom/kubeconfig")
	got := DefaultKubeconfigPath()
	if got != "/custom/kubeconfig" {
		t.Errorf("want /custom/kubeconfig, got %s", got)
	}
}

func TestDefaultKubeconfigPath_FallbackToHome(t *testing.T) {
	t.Setenv("KUBECONFIG", "")
	got := DefaultKubeconfigPath()
	if !strings.HasSuffix(got, ".kube/config") {
		t.Errorf("default path should end in .kube/config, got %s", got)
	}
}
