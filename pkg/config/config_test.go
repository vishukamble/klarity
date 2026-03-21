package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// validConfig returns a minimal valid Config for use in tests.
func validConfig() *Config {
	return &Config{
		Version: 1,
		Environments: []Environment{
			{
				Name: "prod",
				Tier: TierCritical,
				Clusters: []Cluster{
					{
						Context: "prod-us-east-1",
						Namespaces: NamespaceFilter{
							Mode:    NamespaceModeAll,
							Exclude: []string{"kube-system"},
						},
					},
				},
			},
		},
		Settings: Settings{
			LogTailLines:         50,
			ParallelClusters:     4,
			ScanIntervalSeconds:  300,
			ExcludeCompletedJobs: true,
			DefaultNsExclude:     []string{"kube-system", "kube-public"},
		},
	}
}

// validYAML is the canonical YAML representation of validConfig().
const validYAML = `version: 1
environments:
  - name: prod
    tier: critical
    clusters:
      - context: prod-us-east-1
        namespaces:
          mode: all
          exclude:
            - kube-system
settings:
  log_tail_lines: 50
  parallel_clusters: 4
  scan_interval_seconds: 300
  exclude_completed_jobs: true
  default_ns_exclude:
    - kube-system
    - kube-public
`

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "klarityconfig.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeTemp: %v", err)
	}
	return path
}

// ──────────────────────────────────────────────
// Load tests
// ──────────────────────────────────────────────

func TestLoad_ValidConfig(t *testing.T) {
	path := writeTemp(t, validYAML)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.Version != 1 {
		t.Errorf("version: want 1, got %d", cfg.Version)
	}
	if len(cfg.Environments) != 1 {
		t.Fatalf("environments: want 1, got %d", len(cfg.Environments))
	}
	env := cfg.Environments[0]
	if env.Name != "prod" {
		t.Errorf("env name: want prod, got %s", env.Name)
	}
	if env.Tier != TierCritical {
		t.Errorf("tier: want %s, got %s", TierCritical, env.Tier)
	}
	if len(env.Clusters) != 1 {
		t.Fatalf("clusters: want 1, got %d", len(env.Clusters))
	}
	cl := env.Clusters[0]
	if cl.Context != "prod-us-east-1" {
		t.Errorf("context: want prod-us-east-1, got %s", cl.Context)
	}
	if cl.Namespaces.Mode != NamespaceModeAll {
		t.Errorf("ns mode: want all, got %s", cl.Namespaces.Mode)
	}
	if cfg.Settings.ParallelClusters != 4 {
		t.Errorf("parallel_clusters: want 4, got %d", cfg.Settings.ParallelClusters)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found', got: %v", err)
	}
}

func TestLoad_MalformedYAML(t *testing.T) {
	path := writeTemp(t, "version: [this is not valid yaml for an int\n")
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for malformed YAML, got nil")
	}
	if !strings.Contains(err.Error(), "malformed YAML") {
		t.Errorf("error should mention 'malformed YAML', got: %v", err)
	}
}

func TestLoad_VersionMismatch(t *testing.T) {
	yaml := strings.ReplaceAll(validYAML, "version: 1", "version: 99")
	path := writeTemp(t, yaml)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for version mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported config version") {
		t.Errorf("error should mention version, got: %v", err)
	}
}

// ──────────────────────────────────────────────
// Validate tests
// ──────────────────────────────────────────────

func TestValidate_Valid(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidate_NoEnvironments(t *testing.T) {
	cfg := validConfig()
	cfg.Environments = nil
	assertValidateError(t, cfg, "at least one environment")
}

func TestValidate_EmptyEnvName(t *testing.T) {
	cfg := validConfig()
	cfg.Environments[0].Name = ""
	assertValidateError(t, cfg, "name is required")
}

func TestValidate_InvalidTier(t *testing.T) {
	cfg := validConfig()
	cfg.Environments[0].Tier = "urgent"
	assertValidateError(t, cfg, "tier must be")
}

func TestValidate_NoClusterInEnv(t *testing.T) {
	cfg := validConfig()
	cfg.Environments[0].Clusters = nil
	assertValidateError(t, cfg, "at least one cluster")
}

func TestValidate_EmptyContextName(t *testing.T) {
	cfg := validConfig()
	cfg.Environments[0].Clusters[0].Context = ""
	assertValidateError(t, cfg, "context is required")
}

func TestValidate_MissingNamespaceMode(t *testing.T) {
	cfg := validConfig()
	cfg.Environments[0].Clusters[0].Namespaces.Mode = ""
	assertValidateError(t, cfg, "mode is required")
}

func TestValidate_InvalidNamespaceMode(t *testing.T) {
	cfg := validConfig()
	cfg.Environments[0].Clusters[0].Namespaces.Mode = "wildcard"
	assertValidateError(t, cfg, "mode must be")
}

func TestValidate_IncludeModeNoNamespaces(t *testing.T) {
	cfg := validConfig()
	cfg.Environments[0].Clusters[0].Namespaces = NamespaceFilter{
		Mode:    NamespaceModeInclude,
		Include: nil,
	}
	assertValidateError(t, cfg, "include must not be empty")
}

func TestValidate_ParallelClustersTooLow(t *testing.T) {
	cfg := validConfig()
	cfg.Settings.ParallelClusters = 0
	assertValidateError(t, cfg, "parallel_clusters must be at least 1")
}

func TestValidate_LogTailLinesTooLow(t *testing.T) {
	cfg := validConfig()
	cfg.Settings.LogTailLines = 0
	assertValidateError(t, cfg, "log_tail_lines must be at least 1")
}

func TestValidate_ScanIntervalTooLow(t *testing.T) {
	cfg := validConfig()
	cfg.Settings.ScanIntervalSeconds = 0
	assertValidateError(t, cfg, "scan_interval_seconds must be at least 1")
}

// ──────────────────────────────────────────────
// Save + Load round-trip tests
// ──────────────────────────────────────────────

func TestSave_RoundTrip(t *testing.T) {
	cfg := validConfig()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	if err := Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}

	if loaded.Version != cfg.Version {
		t.Errorf("version mismatch: want %d got %d", cfg.Version, loaded.Version)
	}
	if len(loaded.Environments) != len(cfg.Environments) {
		t.Errorf("env count mismatch: want %d got %d", len(cfg.Environments), len(loaded.Environments))
	}
	if loaded.Settings.ParallelClusters != cfg.Settings.ParallelClusters {
		t.Errorf("parallel_clusters mismatch: want %d got %d",
			cfg.Settings.ParallelClusters, loaded.Settings.ParallelClusters)
	}
}

func TestSave_InvalidConfigNotWritten(t *testing.T) {
	cfg := validConfig()
	cfg.Environments = nil
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	if err := Save(cfg, path); err == nil {
		t.Fatal("expected error saving invalid config, got nil")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should not have been created for invalid config")
	}
}

// ──────────────────────────────────────────────
// Include/Exclude mode tests
// ──────────────────────────────────────────────

func TestValidate_IncludeModeWithNamespaces(t *testing.T) {
	cfg := validConfig()
	cfg.Environments[0].Clusters[0].Namespaces = NamespaceFilter{
		Mode:    NamespaceModeInclude,
		Include: []string{"app-services", "data-pipeline"},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected no error for valid include mode, got: %v", err)
	}
}

func TestValidate_ExcludeModeNoList(t *testing.T) {
	cfg := validConfig()
	cfg.Environments[0].Clusters[0].Namespaces = NamespaceFilter{
		Mode: NamespaceModeExclude,
		// Exclude list is optional (empty exclude is valid — scans all)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected no error for exclude mode with empty list, got: %v", err)
	}
}

// ──────────────────────────────────────────────
// helpers
// ──────────────────────────────────────────────

func assertValidateError(t *testing.T, cfg *Config, wantSubstr string) {
	t.Helper()
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), wantSubstr) {
		t.Errorf("error %q should contain %q", err.Error(), wantSubstr)
	}
}
