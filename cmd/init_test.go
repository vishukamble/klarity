package cmd

import (
	"strings"
	"testing"

	"github.com/vishukamble/klarity/pkg/config"
)

func TestBuildEnvironmentsFromInput_SingleEnv(t *testing.T) {
	names := []string{"local"}
	assignments := map[string][]string{
		"local": {"docker-desktop"},
	}

	envs, err := buildEnvironmentsFromInput(names, assignments)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(envs) != 1 {
		t.Fatalf("want 1 env, got %d", len(envs))
	}
	if envs[0].Name != "local" {
		t.Errorf("name = %q, want local", envs[0].Name)
	}
	if envs[0].Tier != config.TierStandard {
		t.Errorf("tier = %q, want standard", envs[0].Tier)
	}
	if len(envs[0].Clusters) != 1 {
		t.Fatalf("want 1 cluster, got %d", len(envs[0].Clusters))
	}
	if envs[0].Clusters[0].Context != "docker-desktop" {
		t.Errorf("context = %q, want docker-desktop", envs[0].Clusters[0].Context)
	}
}

func TestBuildEnvironmentsFromInput_MultipleEnvs(t *testing.T) {
	names := []string{"prod", "dev"}
	assignments := map[string][]string{
		"prod": {"aks-prod-01", "aks-prod-02"},
		"dev":  {"docker-desktop"},
	}

	envs, err := buildEnvironmentsFromInput(names, assignments)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(envs) != 2 {
		t.Fatalf("want 2 envs, got %d", len(envs))
	}
	// prod should get critical tier
	if envs[0].Tier != config.TierCritical {
		t.Errorf("prod tier = %q, want critical", envs[0].Tier)
	}
	if len(envs[0].Clusters) != 2 {
		t.Errorf("prod clusters: want 2, got %d", len(envs[0].Clusters))
	}
	// dev should get standard tier
	if envs[1].Tier != config.TierStandard {
		t.Errorf("dev tier = %q, want standard", envs[1].Tier)
	}
}

func TestBuildEnvironmentsFromInput_EmptyName(t *testing.T) {
	names := []string{""}
	assignments := map[string][]string{
		"": {"docker-desktop"},
	}

	_, err := buildEnvironmentsFromInput(names, assignments)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error should mention empty, got: %v", err)
	}
}

func TestBuildEnvironmentsFromInput_NoClusters(t *testing.T) {
	names := []string{"local"}
	assignments := map[string][]string{
		"local": {},
	}

	_, err := buildEnvironmentsFromInput(names, assignments)
	if err == nil {
		t.Fatal("expected error for no clusters")
	}
	if !strings.Contains(err.Error(), "no clusters") {
		t.Errorf("error should mention no clusters, got: %v", err)
	}
}

func TestBuildEnvironmentsFromInput_NoNames(t *testing.T) {
	_, err := buildEnvironmentsFromInput(nil, nil)
	if err == nil {
		t.Fatal("expected error for no names")
	}
}

func TestBuildEnvironmentsFromInput_NamespaceModeAll(t *testing.T) {
	names := []string{"local"}
	assignments := map[string][]string{
		"local": {"docker-desktop"},
	}

	envs, err := buildEnvironmentsFromInput(names, assignments)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify namespace filter defaults to "all"
	if envs[0].Clusters[0].Namespaces.Mode != config.NamespaceModeAll {
		t.Errorf("namespace mode = %q, want all", envs[0].Clusters[0].Namespaces.Mode)
	}
}

func TestBuildEnvironmentsFromInput_WhitespaceName(t *testing.T) {
	names := []string{"  prod  "}
	assignments := map[string][]string{
		"  prod  ": {"cluster-1"},
	}

	envs, err := buildEnvironmentsFromInput(names, assignments)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Name should be trimmed
	if envs[0].Name != "prod" {
		t.Errorf("name = %q, want trimmed 'prod'", envs[0].Name)
	}
}

func TestBuildEnvironmentsFromInput_MissingAssignment(t *testing.T) {
	names := []string{"local"}
	assignments := map[string][]string{} // no entry for "local"

	_, err := buildEnvironmentsFromInput(names, assignments)
	if err == nil {
		t.Fatal("expected error for missing assignment")
	}
}
