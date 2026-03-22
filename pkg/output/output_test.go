package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/vishukamble/klarity/pkg/config"
	"github.com/vishukamble/klarity/pkg/diagnosis"
)

// ── Helpers ───────────────────────────────────────────────────────────────────

func makeConfig(envs ...config.Environment) *config.Config {
	return &config.Config{
		Environments: envs,
		Settings: config.Settings{
			ParallelClusters:    2,
			ScanIntervalSeconds: 300,
		},
	}
}

func prodEnv(clusters ...string) config.Environment {
	clusterList := make([]config.Cluster, len(clusters))
	for i, c := range clusters {
		clusterList[i] = config.Cluster{
			Context:    c,
			Namespaces: config.NamespaceFilter{Mode: "all"},
		}
	}
	return config.Environment{Name: "prod", Tier: config.TierCritical, Clusters: clusterList}
}

func devEnv(clusters ...string) config.Environment {
	clusterList := make([]config.Cluster, len(clusters))
	for i, c := range clusters {
		clusterList[i] = config.Cluster{
			Context:    c,
			Namespaces: config.NamespaceFilter{Mode: "all"},
		}
	}
	return config.Environment{Name: "dev", Tier: config.TierStandard, Clusters: clusterList}
}

func oomFinding(env, cluster, ns, pod string) diagnosis.Finding {
	return diagnosis.Finding{
		Category:      diagnosis.CategoryOOMKilled,
		Severity:      diagnosis.SeverityCritical,
		EnvName:       env,
		ClusterCtx:    cluster,
		Namespace:     ns,
		PodName:       pod,
		ContainerName: "app",
		OneLiner:      "Container app OOMKilled (restarts: 5)",
		DetailFields: map[string]string{
			"image":         "myimage:v1",
			"restart_count": "5",
		},
	}
}

func crashFinding(env, cluster, ns, pod string) diagnosis.Finding {
	return diagnosis.Finding{
		Category:      diagnosis.CategoryCrashLoop,
		Severity:      diagnosis.SeverityCritical,
		EnvName:       env,
		ClusterCtx:    cluster,
		Namespace:     ns,
		PodName:       pod,
		ContainerName: "worker",
		OneLiner:      "panic: nil pointer dereference",
		DetailFields: map[string]string{
			"restart_count": "12",
			"log_summary":   "panic: nil pointer dereference",
		},
	}
}

// ── RenderJSON tests ──────────────────────────────────────────────────────────

func TestRenderJSON_EmptyFindings(t *testing.T) {
	cfg := makeConfig(prodEnv("prod-us"))
	var buf bytes.Buffer
	err := RenderJSON(nil, &buf, cfg, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON: %v — got: %q", err, buf.String())
	}
	if _, ok := out["scan_time"]; !ok {
		t.Error("missing scan_time field")
	}
	summary := out["summary"].(map[string]interface{})
	if summary["total_issues"].(float64) != 0 {
		t.Errorf("total_issues = %v, want 0", summary["total_issues"])
	}
}

func TestRenderJSON_Fields(t *testing.T) {
	cfg := makeConfig(prodEnv("prod-us"))
	f := oomFinding("prod", "prod-us", "payments", "pay-api-abc")
	var buf bytes.Buffer
	if err := RenderJSON([]diagnosis.Finding{f}, &buf, cfg, time.Now()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	envs := out["environments"].([]interface{})
	if len(envs) != 1 {
		t.Fatalf("want 1 environment, got %d", len(envs))
	}
	env := envs[0].(map[string]interface{})
	if env["name"] != "prod" {
		t.Errorf("env name = %v, want prod", env["name"])
	}

	clusters := env["clusters"].([]interface{})
	cl := clusters[0].(map[string]interface{})
	if cl["total_issues"].(float64) != 1 {
		t.Errorf("total_issues = %v, want 1", cl["total_issues"])
	}

	findings := cl["findings"].(map[string]interface{})
	oomItems := findings["oom"].([]interface{})
	if len(oomItems) != 1 {
		t.Fatalf("want 1 oom item, got %d", len(oomItems))
	}
	item := oomItems[0].(map[string]interface{})
	if item["summary"] == "" {
		t.Error("summary should not be empty")
	}
}

func TestRenderJSON_NoANSI(t *testing.T) {
	cfg := makeConfig(prodEnv("prod-us"))
	f := oomFinding("prod", "prod-us", "ns", "pod")
	var buf bytes.Buffer
	_ = RenderJSON([]diagnosis.Finding{f}, &buf, cfg, time.Now())

	if strings.Contains(buf.String(), "\x1b[") {
		t.Error("JSON output contains ANSI escape codes")
	}
}

func TestRenderJSON_MultipleFindings(t *testing.T) {
	cfg := makeConfig(prodEnv("prod-us"))
	findings := []diagnosis.Finding{
		oomFinding("prod", "prod-us", "ns1", "pod-a"),
		crashFinding("prod", "prod-us", "ns2", "pod-b"),
	}
	var buf bytes.Buffer
	if err := RenderJSON(findings, &buf, cfg, time.Now()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	summary := out["summary"].(map[string]interface{})
	if summary["total_issues"].(float64) != 2 {
		t.Errorf("total_issues = %v, want 2", summary["total_issues"])
	}
}

func TestRenderJSON_ByEnvironmentIncludesZero(t *testing.T) {
	cfg := makeConfig(prodEnv("prod-us"), devEnv("dev-local"))
	// Only prod has findings.
	findings := []diagnosis.Finding{
		oomFinding("prod", "prod-us", "ns1", "pod-a"),
	}
	var buf bytes.Buffer
	if err := RenderJSON(findings, &buf, cfg, time.Now()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	summary := out["summary"].(map[string]interface{})
	byEnv := summary["by_environment"].(map[string]interface{})
	if byEnv["dev"].(float64) != 0 {
		t.Errorf("dev should be 0 issues, got %v", byEnv["dev"])
	}
}

// ── RenderReport tests ────────────────────────────────────────────────────────

func TestRenderReport_EmptyFindings(t *testing.T) {
	cfg := makeConfig(prodEnv("prod-us-east-1"))
	var buf bytes.Buffer
	// Must not panic.
	RenderReport(&buf, nil, cfg, time.Now(), nil)
	out := buf.String()
	// Title banner present.
	if !strings.Contains(out, "klarity scan") {
		t.Error("output missing title banner")
	}
	// No-issues message present for the single cluster.
	if !strings.Contains(out, "No issues found") {
		t.Error("output missing 'No issues found' message")
	}
}

func TestRenderReport_ShowsFindings(t *testing.T) {
	cfg := makeConfig(prodEnv("prod-us"))
	findings := []diagnosis.Finding{
		oomFinding("prod", "prod-us", "payments", "pay-api"),
		crashFinding("prod", "prod-us", "checkout", "cart-svc"),
	}
	var buf bytes.Buffer
	RenderReport(&buf, findings, cfg, time.Now(), nil)
	out := buf.String()

	for _, want := range []string{"OOMKilled", "CrashLoopBackOff", "payments", "checkout", "PROD"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderReport_CriticalEnvFirst(t *testing.T) {
	devE := devEnv("dev-cluster")
	prodE := prodEnv("prod-cluster")
	// Config lists dev first, prod second.
	cfg := makeConfig(devE, prodE)

	var buf bytes.Buffer
	RenderReport(&buf, nil, cfg, time.Now(), nil)
	out := buf.String()

	// PROD section must appear before DEV section in output.
	prodIdx := strings.Index(out, "PROD")
	devIdx := strings.Index(out, "DEV")
	if prodIdx < 0 || devIdx < 0 {
		t.Fatalf("output missing PROD or DEV header")
	}
	if prodIdx > devIdx {
		t.Error("expected PROD to appear before DEV in output")
	}
}

func TestRenderReport_EmptyCategoryHidden(t *testing.T) {
	cfg := makeConfig(prodEnv("prod-us"))
	findings := []diagnosis.Finding{oomFinding("prod", "prod-us", "ns", "pod")}
	var buf bytes.Buffer
	RenderReport(&buf, findings, cfg, time.Now(), nil)
	out := buf.String()

	// Categories without findings must not appear.
	for _, absent := range []string{"CrashLoopBackOff", "Pending Pods", "HPA Scaling"} {
		if strings.Contains(out, absent) {
			t.Errorf("output unexpectedly shows empty category %q", absent)
		}
	}
}

func TestRenderReport_ScanErrorsShown(t *testing.T) {
	cfg := makeConfig(prodEnv("prod-us"))
	var buf bytes.Buffer
	RenderReport(&buf, nil, cfg, time.Now(), []string{"prod-us: context not found"})
	out := buf.String()
	if !strings.Contains(out, "context not found") {
		t.Error("output missing scan error message")
	}
}

func TestRenderReport_MultipleCategories(t *testing.T) {
	cfg := makeConfig(prodEnv("prod-us"))
	findings := []diagnosis.Finding{
		oomFinding("prod", "prod-us", "ml", "model-a"),
		oomFinding("prod", "prod-us", "ml", "model-b"),
		crashFinding("prod", "prod-us", "checkout", "cart"),
		{
			Category:   diagnosis.CategoryImagePull,
			Severity:   diagnosis.SeverityCritical,
			EnvName:    "prod",
			ClusterCtx: "prod-us",
			Namespace:  "payments",
			PodName:    "pay-api",
			OneLiner:   "Image pull failed",
			DetailFields: map[string]string{
				"image":   "acr.io/pay-api:v2.14-typo",
				"subtype": "tag_not_found",
			},
		},
	}
	var buf bytes.Buffer
	RenderReport(&buf, findings, cfg, time.Now(), nil)
	out := buf.String()

	for _, want := range []string{"OOMKilled", "CrashLoopBackOff", "Image Pull Errors", "Tag not found"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

// ── Colour / theming tests ────────────────────────────────────────────────────

func TestEnvColor_Critical(t *testing.T) {
	env := config.Environment{Name: "prod", Tier: config.TierCritical}
	if got := EnvColor(env); got != red {
		t.Errorf("critical env got color %v, want red", got)
	}
}

func TestEnvColor_Dev(t *testing.T) {
	env := config.Environment{Name: "dev", Tier: config.TierStandard}
	if got := EnvColor(env); got != green {
		t.Errorf("dev env got color %v, want green", got)
	}
}

func TestEnvColor_Standard(t *testing.T) {
	env := config.Environment{Name: "staging", Tier: config.TierStandard}
	if got := EnvColor(env); got != yellow {
		t.Errorf("staging env got color %v, want yellow", got)
	}
}

// ── SummaryCounts tests ───────────────────────────────────────────────────────

func TestSummaryCounts(t *testing.T) {
	findings := []diagnosis.Finding{
		{Severity: diagnosis.SeverityCritical},
		{Severity: diagnosis.SeverityCritical},
		{Severity: diagnosis.SeverityWarning},
		{Severity: diagnosis.SeverityInfo},
	}
	c, w, i := SummaryCounts(findings)
	if c != 2 || w != 1 || i != 1 {
		t.Errorf("counts: got critical=%d warning=%d info=%d, want 2,1,1", c, w, i)
	}
}

func TestSummaryCounts_Empty(t *testing.T) {
	c, w, i := SummaryCounts(nil)
	if c != 0 || w != 0 || i != 0 {
		t.Errorf("expected all zeros, got %d %d %d", c, w, i)
	}
}

// ── sortedEnvs tests ──────────────────────────────────────────────────────────

func TestSortedEnvs_CriticalFirst(t *testing.T) {
	cfg := makeConfig(
		config.Environment{Name: "dev", Tier: config.TierStandard},
		config.Environment{Name: "staging", Tier: config.TierStandard},
		config.Environment{Name: "prod", Tier: config.TierCritical},
	)
	sorted := sortedEnvs(cfg)
	if sorted[0].Name != "prod" {
		t.Errorf("first env = %q, want prod", sorted[0].Name)
	}
}

func TestSortedEnvs_StableOrder(t *testing.T) {
	// Two standard envs should stay in original order.
	cfg := makeConfig(
		config.Environment{Name: "staging", Tier: config.TierStandard},
		config.Environment{Name: "dev", Tier: config.TierStandard},
	)
	sorted := sortedEnvs(cfg)
	if sorted[0].Name != "staging" || sorted[1].Name != "dev" {
		t.Errorf("order = [%s %s], want [staging dev]", sorted[0].Name, sorted[1].Name)
	}
}

// ── wrapText tests ───────────────────────────────────────────────────────────

func TestWrapText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxWidth int
		want     string
	}{
		{
			name:     "empty string",
			input:    "",
			maxWidth: 80,
			want:     "",
		},
		{
			name:     "short string no wrap",
			input:    "hello world",
			maxWidth: 80,
			want:     "hello world",
		},
		{
			name:     "exact boundary",
			input:    "12345",
			maxWidth: 5,
			want:     "12345",
		},
		{
			name:     "long string with word break",
			input:    "0/3 nodes are available: 3 Insufficient cpu. Pod requested 4 cores but max allocatable is 2 cores per node.",
			maxWidth: 50,
			want:     "0/3 nodes are available: 3 Insufficient cpu. Pod\nrequested 4 cores but max allocatable is 2 cores per node.",
		},
		{
			name:     "no spaces hard break",
			input:    "aaaaabbbbbcccccdddddeeeee",
			maxWidth: 10,
			want:     "aaaaabbbbb\ncccccdddddeeeee",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrapText(tt.input, tt.maxWidth)
			if got != tt.want {
				t.Errorf("wrapText(%q, %d) =\n  %q\nwant:\n  %q", tt.input, tt.maxWidth, got, tt.want)
			}
		})
	}
}
