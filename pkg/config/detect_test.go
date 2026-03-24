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
// Docker-desktop fallback scenario
// ──────────────────────────────────────────────

func TestDetectEnvironments_DockerDesktop(t *testing.T) {
	// docker-desktop has no environment keyword — must trigger fallback.
	contexts := []string{"docker-desktop"}
	result, ok := DetectEnvironments(contexts)
	if ok {
		t.Fatal("expected ok=false for docker-desktop (no env keyword)")
	}
	if len(result.Envs) != 0 {
		t.Errorf("expected no detected envs, got %v", result.Envs)
	}
	if len(result.Unmatched) != 1 || result.Unmatched[0] != "docker-desktop" {
		t.Errorf("unmatched: want [docker-desktop], got %v", result.Unmatched)
	}
}

func TestBuildManualConfig_DockerDesktop(t *testing.T) {
	// Simulate fallback: user names env "local", assigns docker-desktop.
	defaults := DefaultConfig()
	names := []string{"local"}
	clusters := map[string][]string{
		"local": {"docker-desktop"},
	}
	cfg := BuildManualConfig(names, clusters, defaults)

	if err := cfg.Validate(); err != nil {
		t.Fatalf("generated config should be valid: %v", err)
	}
	if len(cfg.Environments) != 1 {
		t.Fatalf("want 1 environment, got %d", len(cfg.Environments))
	}
	env := cfg.Environments[0]
	if env.Name != "local" {
		t.Errorf("env name: want local, got %s", env.Name)
	}
	if env.Tier != TierStandard {
		t.Errorf("local tier: want standard, got %s", env.Tier)
	}
	if len(env.Clusters) != 1 || env.Clusters[0].Context != "docker-desktop" {
		t.Errorf("want cluster docker-desktop, got %v", env.Clusters)
	}
}

func TestBuildManualConfig_NoClustersSelected(t *testing.T) {
	// If user selects zero clusters for all envs, config should have no environments.
	defaults := DefaultConfig()
	names := []string{"local"}
	clusters := map[string][]string{
		"local": {}, // no clusters selected
	}
	cfg := BuildManualConfig(names, clusters, defaults)

	if len(cfg.Environments) != 0 {
		t.Errorf("want 0 environments when no clusters selected, got %d", len(cfg.Environments))
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error for config with no environments")
	}
}

// ──────────────────────────────────────────────
// AKS pattern matching
// ──────────────────────────────────────────────

var matchAKSTests = []struct {
	context string
	want    string
}{
	{"aks-intel-dev-cus-app", "dev-intel"},
	{"aks-intel-prod-eus-web", "prod-intel"},
	{"aks-intel-prod-cus-api", "prod-intel"},
	{"aks-ravn-staging-wus-svc", "staging-ravn"},
	{"aks-myorg-production-eus-01", "prod-myorg"},
	{"aks-myorg-development-cus-02", "dev-myorg"},
	// AKS prefix required
	{"not-aks-intel-dev-cus", ""},
	// Level must be a known keyword
	{"aks-intel-unknown-cus-app", ""},
	// Case insensitive
	{"AKS-INTEL-DEV-CUS-APP", "dev-intel"},
}

func TestMatchAKSPattern(t *testing.T) {
	for _, tc := range matchAKSTests {
		t.Run(tc.context, func(t *testing.T) {
			got := matchAKSPattern(tc.context)
			if got != tc.want {
				t.Errorf("matchAKSPattern(%q) = %q, want %q", tc.context, got, tc.want)
			}
		})
	}
}

// ──────────────────────────────────────────────
// EKS/AWS pattern matching
// ──────────────────────────────────────────────

var matchEKSTests = []struct {
	context string
	want    string
}{
	{"myapp-prod-web-01", "prod-myapp"},
	{"myapp-production-web-01", "prod-myapp"},
	{"project-staging-api", "staging-project"},
	{"project-stg-api", "staging-project"},
	{"project-dev-worker", "dev-project"},
	{"project-development-worker", "dev-project"},
	{"project-qa-cluster", "qa-project"},
	{"project-uat-cluster", "uat-project"},
	{"project-test-cluster", "test-project"},
	{"project-testing-cluster", "test-project"},
	{"project-sandbox-cluster", "sandbox-project"},
	// Level must be recognised keyword
	{"project-unknown-cluster", ""},
	// Needs trailing dash after level
	{"myapp-prod", ""},
	// Case insensitive
	{"MyApp-Prod-Web-01", "prod-myapp"},
}

func TestMatchEKSPattern(t *testing.T) {
	for _, tc := range matchEKSTests {
		t.Run(tc.context, func(t *testing.T) {
			got := matchEKSPattern(tc.context)
			if got != tc.want {
				t.Errorf("matchEKSPattern(%q) = %q, want %q", tc.context, got, tc.want)
			}
		})
	}
}

// ──────────────────────────────────────────────
// DetectEnvironments with AKS/EKS patterns
// ──────────────────────────────────────────────

func TestDetectEnvironments_AKSPattern(t *testing.T) {
	contexts := []string{
		"aks-intel-prod-cus-web",
		"aks-intel-prod-eus-api",
		"aks-intel-dev-cus-app",
		"aks-ravn-dev-wus-svc",
	}
	result, ok := DetectEnvironments(contexts)
	if !ok {
		t.Fatal("expected all AKS contexts to match, got ok=false")
	}
	assertContextsInEnv(t, result, "prod-intel", []string{
		"aks-intel-prod-cus-web",
		"aks-intel-prod-eus-api",
	})
	assertContextsInEnv(t, result, "dev-intel", []string{"aks-intel-dev-cus-app"})
	assertContextsInEnv(t, result, "dev-ravn", []string{"aks-ravn-dev-wus-svc"})
	if len(result.Unmatched) != 0 {
		t.Errorf("expected no unmatched, got: %v", result.Unmatched)
	}
}

func TestDetectEnvironments_EKSPattern(t *testing.T) {
	contexts := []string{
		"myapp-prod-web-01",
		"myapp-prod-api-02",
		"myapp-dev-worker",
	}
	result, ok := DetectEnvironments(contexts)
	if !ok {
		t.Fatal("expected all EKS contexts to match, got ok=false")
	}
	assertContextsInEnv(t, result, "prod-myapp", []string{
		"myapp-prod-web-01",
		"myapp-prod-api-02",
	})
	assertContextsInEnv(t, result, "dev-myapp", []string{"myapp-dev-worker"})
}

func TestDetectEnvironments_MixedStrategies(t *testing.T) {
	// AKS, EKS, and generic keyword clusters in one kubeconfig.
	contexts := []string{
		"aks-intel-prod-cus-web", // AKS → prod-intel
		"myapp-dev-worker",       // EKS → dev-myapp
		"prod-us-east-1",         // generic → prod
	}
	result, ok := DetectEnvironments(contexts)
	if !ok {
		t.Fatal("expected all contexts to match, got ok=false")
	}
	if len(result.Unmatched) != 0 {
		t.Errorf("expected no unmatched, got: %v", result.Unmatched)
	}
	if _, ok := result.Envs["prod-intel"]; !ok {
		t.Error("expected prod-intel group from AKS pattern")
	}
	if _, ok := result.Envs["dev-myapp"]; !ok {
		t.Error("expected dev-myapp group from EKS pattern")
	}
	if _, ok := result.Envs["prod"]; !ok {
		t.Error("expected prod group from generic pattern")
	}
}

func TestDetectEnvironments_UnmatchedGoesToUnmatched(t *testing.T) {
	contexts := []string{
		"aks-intel-prod-cus-web",
		"legacy-cluster", // no keyword → unmatched
		"tools-central",  // no keyword → unmatched
	}
	result, _ := DetectEnvironments(contexts)
	if len(result.Unmatched) != 2 {
		t.Errorf("expected 2 unmatched, got %d: %v", len(result.Unmatched), result.Unmatched)
	}
}

// ──────────────────────────────────────────────
// BestGuessGroup
// ──────────────────────────────────────────────

var bestGuessGroupTests = []struct {
	context string
	want    string
}{
	{"ravn-dev-cus", "dev-ravn"},
	{"legacy-cluster-01", ""},       // no env keyword → no suggestion
	{"my-dev-cluster", "dev-cluster"}, // "my" is 2 chars → skipped; "cluster" is first non-skip org token
	{"intel-prod-eus", "prod-intel"},
	{"staging-myorg-01", "staging-myorg"},
	{"tooling-central", ""},         // no keyword
	{"qa-team-alpha", "qa-team"},
	{"prod", "prod"},                // keyword only, no org token
	{"dev-01", "dev"},               // only digits after keyword
}

func TestBestGuessGroup(t *testing.T) {
	for _, tc := range bestGuessGroupTests {
		t.Run(tc.context, func(t *testing.T) {
			got := BestGuessGroup(tc.context)
			if got != tc.want {
				t.Errorf("BestGuessGroup(%q) = %q, want %q", tc.context, got, tc.want)
			}
		})
	}
}

// ──────────────────────────────────────────────
// tierForLabel — contains-word behaviour
// ──────────────────────────────────────────────

var tierForLabelTests = []struct {
	label    string
	wantTier string
}{
	{"prod", TierCritical},
	{"production", TierCritical},
	{"prod-intel", TierCritical},
	{"prod-myapp", TierCritical},
	{"production-east", TierCritical},
	{"dev", TierStandard},
	{"dev-ravn", TierStandard},
	{"dev-intel", TierStandard},
	{"staging", TierStandard},
	{"staging-intel", TierStandard},
	{"qa", TierStandard},
	{"tooling", TierStandard},
	// "reproduced" should NOT be critical despite containing "prod" substring
	{"reproduced-cluster", TierStandard},
}

func TestTierForLabel(t *testing.T) {
	for _, tc := range tierForLabelTests {
		t.Run(tc.label, func(t *testing.T) {
			got := tierForLabel(tc.label)
			if got != tc.wantTier {
				t.Errorf("tierForLabel(%q) = %q, want %q", tc.label, got, tc.wantTier)
			}
		})
	}
}

// ──────────────────────────────────────────────
// HasEnvKeyword
// ──────────────────────────────────────────────

func TestHasEnvKeyword(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"prod-intel", true},
		{"dev-ravn", true},
		{"staging-myorg", true},
		{"tooling", false},
		{"legacy-cluster", false},
		{"intel-team", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := HasEnvKeyword(tc.name)
			if got != tc.want {
				t.Errorf("HasEnvKeyword(%q) = %v, want %v", tc.name, got, tc.want)
			}
		})
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
