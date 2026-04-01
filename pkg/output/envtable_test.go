package output

import (
	"strings"
	"testing"

	"github.com/vishukamble/klarity/pkg/config"
)

func TestRenderEnvTable_Empty(t *testing.T) {
	got := RenderEnvTable(nil, true)
	if got != "No environments configured." {
		t.Errorf("want %q, got %q", "No environments configured.", got)
	}
	got = RenderEnvTable([]config.Environment{}, true)
	if got != "No environments configured." {
		t.Errorf("empty slice: want %q, got %q", "No environments configured.", got)
	}
}

func TestRenderEnvTable_SingleStandardEnv(t *testing.T) {
	envs := []config.Environment{
		{
			Name: "dev",
			Tier: config.TierStandard,
			Clusters: []config.Cluster{
				{Context: "dev-cluster"},
			},
		},
	}
	got := RenderEnvTable(envs, true)
	if !strings.Contains(got, "dev") {
		t.Errorf("expected env name in output; got:\n%s", got)
	}
	if !strings.Contains(got, "standard") {
		t.Errorf("expected tier 'standard' in output; got:\n%s", got)
	}
	if !strings.Contains(got, "dev-cluster") {
		t.Errorf("expected context name in output; got:\n%s", got)
	}
	if !strings.Contains(got, "1") {
		t.Errorf("expected cluster count '1' in output; got:\n%s", got)
	}
}

func TestRenderEnvTable_CriticalEnv_NoColor(t *testing.T) {
	envs := []config.Environment{
		{
			Name: "prod",
			Tier: config.TierCritical,
			Clusters: []config.Cluster{
				{Context: "prod-aks-eastus"},
			},
		},
	}
	// noColor=true: must not panic and must still contain the env/tier data.
	got := RenderEnvTable(envs, true)
	if !strings.Contains(got, "prod") {
		t.Errorf("expected env name in output; got:\n%s", got)
	}
	if !strings.Contains(got, "critical") {
		t.Errorf("expected tier 'critical' in output; got:\n%s", got)
	}
}

func TestRenderEnvTable_CriticalEnv_WithColor(t *testing.T) {
	envs := []config.Environment{
		{
			Name: "prod",
			Tier: config.TierCritical,
			Clusters: []config.Cluster{
				{Context: "prod-aks-eastus"},
			},
		},
	}
	// noColor=false: must not panic; output contains ANSI but still contains the strings.
	got := RenderEnvTable(envs, false)
	if !strings.Contains(got, "prod") {
		t.Errorf("expected env name in output; got:\n%s", got)
	}
	if !strings.Contains(got, "critical") {
		t.Errorf("expected tier 'critical' in output; got:\n%s", got)
	}
}

func TestRenderEnvTable_MultipleEnvs(t *testing.T) {
	envs := []config.Environment{
		{
			Name: "dev",
			Tier: config.TierStandard,
			Clusters: []config.Cluster{
				{Context: "dev-ctx"},
			},
		},
		{
			Name: "prod",
			Tier: config.TierCritical,
			Clusters: []config.Cluster{
				{Context: "prod-ctx-1"},
				{Context: "prod-ctx-2"},
			},
		},
	}
	got := RenderEnvTable(envs, true)
	for _, want := range []string{"dev", "prod", "standard", "critical", "dev-ctx", "prod-ctx-1", "prod-ctx-2"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in output; got:\n%s", want, got)
		}
	}
	// prod has 2 clusters
	if !strings.Contains(got, "2") {
		t.Errorf("expected cluster count '2' in output; got:\n%s", got)
	}
}

func TestRenderEnvTable_MultiLineContextNames(t *testing.T) {
	envs := []config.Environment{
		{
			Name: "staging",
			Tier: config.TierStandard,
			Clusters: []config.Cluster{
				{Context: "staging-east"},
				{Context: "staging-west"},
				{Context: "staging-central"},
			},
		},
	}
	got := RenderEnvTable(envs, true)
	// All three contexts must appear in the output.
	for _, ctx := range []string{"staging-east", "staging-west", "staging-central"} {
		if !strings.Contains(got, ctx) {
			t.Errorf("expected context %q in output; got:\n%s", ctx, got)
		}
	}
	// Cluster count should be 3.
	if !strings.Contains(got, "3") {
		t.Errorf("expected cluster count '3' in output; got:\n%s", got)
	}
}

func TestRenderEnvTable_ContainsHeaders(t *testing.T) {
	envs := []config.Environment{
		{Name: "dev", Tier: config.TierStandard, Clusters: []config.Cluster{{Context: "ctx"}}},
	}
	got := RenderEnvTable(envs, true)
	for _, header := range []string{"Environment", "Tier", "Clusters", "Context Names"} {
		if !strings.Contains(got, header) {
			t.Errorf("expected header %q in output; got:\n%s", header, got)
		}
	}
}
