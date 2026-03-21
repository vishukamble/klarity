package config

import (
	"reflect"
	"testing"
)

// ──────────────────────────────────────────────
// DetectEnvironments — label matching
// ──────────────────────────────────────────────

func TestDetectEnvironments_HappyPath(t *testing.T) {
	contexts := []string{
		"prod-us-east-1",
		"prod-us-west-2",
		"staging-eu-west-1",
		"dev-team-alpha",
	}
	result, ok := DetectEnvironments(contexts)
	if !ok {
		t.Fatal("expected all contexts to match, got ok=false")
	}
	assertContextsInEnv(t, result, "prod", []string{"prod-us-east-1", "prod-us-west-2"})
	assertContextsInEnv(t, result, "staging", []string{"staging-eu-west-1"})
	assertContextsInEnv(t, result, "dev", []string{"dev-team-alpha"})
	if len(result.Unmatched) != 0 {
		t.Errorf("expected no unmatched, got: %v", result.Unmatched)
	}
}

func TestDetectEnvironments_FallbackPath(t *testing.T) {
	// None of these names contain any of our keywords.
	contexts := []string{"us-east-aks-01", "eu-central-aks-01", "tools-central"}
	result, ok := DetectEnvironments(contexts)
	if ok {
		t.Fatal("expected ok=false when no contexts match keywords")
	}
	if len(result.Unmatched) != 3 {
		t.Errorf("expected 3 unmatched, got %d: %v", len(result.Unmatched), result.Unmatched)
	}
	if len(result.Envs) != 0 {
		t.Errorf("expected no envs, got: %v", result.Envs)
	}
}

func TestDetectEnvironments_PartialFallback(t *testing.T) {
	// Mix of matching and non-matching — should return ok=false.
	contexts := []string{"prod-us-east-1", "tools-central"}
	result, ok := DetectEnvironments(contexts)
	if ok {
		t.Fatal("expected ok=false when at least one context is unmatched")
	}
	if len(result.Unmatched) != 1 || result.Unmatched[0] != "tools-central" {
		t.Errorf("unmatched: want [tools-central], got %v", result.Unmatched)
	}
}

func TestDetectEnvironments_EmptyContextList(t *testing.T) {
	result, ok := DetectEnvironments(nil)
	if ok {
		t.Fatal("expected ok=false for empty context list")
	}
	if len(result.Envs) != 0 {
		t.Errorf("expected empty envs map, got %v", result.Envs)
	}
}

// ──────────────────────────────────────────────
// Keyword matching edge cases
// ──────────────────────────────────────────────

var matchLabelTests = []struct {
	context string
	want    string
}{
	// prod variants
	{"prod-us-east-1", "prod"},
	{"production-eu", "prod"},
	{"my-prod-cluster", "prod"},
	// staging variants
	{"staging-us-east-1", "staging"},
	{"stg-eu-west", "staging"},
	{"my-staging-env", "staging"},
	// dev variants
	{"dev-us-east-1", "dev"},
	{"development-team-a", "dev"},
	{"team-dev-cluster", "dev"},
	// qa
	{"qa-cluster-1", "qa"},
	{"cluster-qa", "qa"},
	// uat
	{"uat-us", "uat"},
	// sandbox
	{"sandbox-team-a", "sandbox"},
	// test
	{"test-cluster", "test"},
	{"testing-env", "test"},
	// no match
	{"us-east-aks-01", ""},
	{"tools-central", ""},
	// should NOT match "prod" inside another word
	{"reproduced-cluster", ""},
	// should NOT match "dev" inside "devops" (it's a word boundary: devops → dev+ops)
	// devops has no separator after dev, so it should NOT match
	{"devops-tools", ""},
	// context with dots (sanitization concern — label should still match)
	{"prod.us.east.1", "prod"},
	// uppercase context names
	{"PROD-CLUSTER", "prod"},
	{"Staging-EU", "staging"},
}

func TestMatchLabel(t *testing.T) {
	for _, tc := range matchLabelTests {
		t.Run(tc.context, func(t *testing.T) {
			got := matchLabel(tc.context)
			if got != tc.want {
				t.Errorf("matchLabel(%q) = %q, want %q", tc.context, got, tc.want)
			}
		})
	}
}

// ──────────────────────────────────────────────
// DetectedEnvs.Order
// ──────────────────────────────────────────────

func TestDetectEnvironments_OrderPreserved(t *testing.T) {
	contexts := []string{
		"dev-cluster",
		"prod-cluster",
		"staging-cluster",
	}
	result, _ := DetectEnvironments(contexts)
	want := []string{"dev", "prod", "staging"}
	if !reflect.DeepEqual(result.Order, want) {
		t.Errorf("order: want %v, got %v", want, result.Order)
	}
}

// ──────────────────────────────────────────────
// BuildDetectedConfig
// ──────────────────────────────────────────────

func TestBuildDetectedConfig_Basic(t *testing.T) {
	defaults := DefaultConfig()
	selected := map[string][]string{
		"prod":    {"prod-us-east-1", "prod-eu-west-1"},
		"staging": {"staging-us-east-1"},
	}
	order := []string{"prod", "staging"}

	cfg := BuildDetectedConfig(selected, order, defaults)

	if err := cfg.Validate(); err != nil {
		t.Fatalf("generated config invalid: %v", err)
	}
	if len(cfg.Environments) != 2 {
		t.Fatalf("want 2 environments, got %d", len(cfg.Environments))
	}

	prod := cfg.Environments[0]
	if prod.Name != "prod" {
		t.Errorf("env[0] name: want prod, got %s", prod.Name)
	}
	if prod.Tier != TierCritical {
		t.Errorf("prod tier: want critical, got %s", prod.Tier)
	}
	if len(prod.Clusters) != 2 {
		t.Errorf("prod clusters: want 2, got %d", len(prod.Clusters))
	}

	staging := cfg.Environments[1]
	if staging.Tier != TierStandard {
		t.Errorf("staging tier: want standard, got %s", staging.Tier)
	}
}

func TestBuildDetectedConfig_NamespaceModeAll(t *testing.T) {
	defaults := DefaultConfig()
	selected := map[string][]string{"dev": {"dev-cluster-1"}}
	cfg := BuildDetectedConfig(selected, []string{"dev"}, defaults)

	cl := cfg.Environments[0].Clusters[0]
	if cl.Namespaces.Mode != NamespaceModeAll {
		t.Errorf("want namespace mode all, got %s", cl.Namespaces.Mode)
	}
	if len(cl.Namespaces.Exclude) == 0 {
		t.Error("expected default ns excludes to be set")
	}
}

func TestBuildDetectedConfig_EmptySelectionOmitted(t *testing.T) {
	defaults := DefaultConfig()
	// staging was deselected entirely
	selected := map[string][]string{
		"prod":    {"prod-cluster"},
		"staging": {},
	}
	cfg := BuildDetectedConfig(selected, []string{"prod", "staging"}, defaults)

	if len(cfg.Environments) != 1 {
		t.Errorf("want 1 environment (staging skipped), got %d", len(cfg.Environments))
	}
}

// ──────────────────────────────────────────────
// BuildManualConfig
// ──────────────────────────────────────────────

func TestBuildManualConfig_Basic(t *testing.T) {
	defaults := DefaultConfig()
	names := []string{"production", "tooling"}
	clusters := map[string][]string{
		"production": {"us-east-aks-01", "us-west-aks-02"},
		"tooling":    {"tools-central"},
	}

	cfg := BuildManualConfig(names, clusters, defaults)

	if err := cfg.Validate(); err != nil {
		t.Fatalf("generated config invalid: %v", err)
	}
	if len(cfg.Environments) != 2 {
		t.Fatalf("want 2 environments, got %d", len(cfg.Environments))
	}

	prod := cfg.Environments[0]
	if prod.Name != "production" {
		t.Errorf("env[0] name: want production, got %s", prod.Name)
	}
	// "production" should get critical tier
	if prod.Tier != TierCritical {
		t.Errorf("production tier: want critical, got %s", prod.Tier)
	}
	if len(prod.Clusters) != 2 {
		t.Errorf("production clusters: want 2, got %d", len(prod.Clusters))
	}

	tooling := cfg.Environments[1]
	if tooling.Tier != TierStandard {
		t.Errorf("tooling tier: want standard, got %s", tooling.Tier)
	}
}

func TestBuildManualConfig_EmptyClustersSkipped(t *testing.T) {
	defaults := DefaultConfig()
	names := []string{"prod", "empty-env"}
	clusters := map[string][]string{
		"prod":      {"prod-cluster"},
		"empty-env": {},
	}
	cfg := BuildManualConfig(names, clusters, defaults)
	if len(cfg.Environments) != 1 {
		t.Errorf("want 1 env (empty-env skipped), got %d", len(cfg.Environments))
	}
}

// ──────────────────────────────────────────────
// helpers
// ──────────────────────────────────────────────

func assertContextsInEnv(t *testing.T, result DetectedEnvs, label string, want []string) {
	t.Helper()
	got, ok := result.Envs[label]
	if !ok {
		t.Errorf("expected env %q to exist in result", label)
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("env %q contexts: want %v, got %v", label, want, got)
	}
}
