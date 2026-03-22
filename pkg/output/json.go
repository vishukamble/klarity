package output

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/vishukamble/klarity/pkg/config"
	"github.com/vishukamble/klarity/pkg/diagnosis"
)

// ── JSON output types ────────────────────────────────────────────────────────

type jsonOutput struct {
	ScanTime     string            `json:"scan_time"`
	Environments []jsonEnvironment `json:"environments"`
	Summary      jsonSummary       `json:"summary"`
}

type jsonEnvironment struct {
	Name     string        `json:"name"`
	Tier     string        `json:"tier"`
	Clusters []jsonCluster `json:"clusters"`
}

type jsonCluster struct {
	Context     string               `json:"context"`
	Findings    map[string][]jsonItem `json:"findings"`
	TotalIssues int                  `json:"total_issues"`
}

type jsonItem struct {
	Severity  string            `json:"severity"`
	Namespace string            `json:"namespace,omitempty"`
	Pod       string            `json:"pod,omitempty"`
	Container string            `json:"container,omitempty"`
	Summary   string            `json:"summary"`
	Detail    map[string]string `json:"detail,omitempty"`
}

type jsonSummary struct {
	TotalIssues   int            `json:"total_issues"`
	ByEnvironment map[string]int `json:"by_environment"`
}

// categoryJSONKey maps Finding categories to JSON field names.
var categoryJSONKey = map[diagnosis.Category]string{
	diagnosis.CategoryNodeIssue:          "node_issues",
	diagnosis.CategoryOOMKilled:          "oom",
	diagnosis.CategoryImagePull:          "image_pull",
	diagnosis.CategoryCrashLoop:          "crashloop",
	diagnosis.CategoryPending:            "pending_pods",
	diagnosis.CategoryHPACeiling:         "hpa",
	diagnosis.CategoryNoEndpoints:        "services_no_endpoints",
	diagnosis.CategoryQuotaExhausted:     "quota",
	diagnosis.CategoryPVCPending:         "pvc_pending",
	diagnosis.CategoryJobFailed:          "jobs",
	diagnosis.CategoryCronJobSuspended:   "cronjobs",
	diagnosis.CategoryDaemonSetDegraded:  "daemonsets",
	diagnosis.CategoryStatefulSetDegraded: "statefulsets",
	diagnosis.CategoryWarningEvent:       "warning_events",
}

// RenderJSON writes findings in structured JSON format to w.
// It never calls lipgloss — safe to use when stdout is not a TTY.
func RenderJSON(findings []diagnosis.Finding, w io.Writer, cfg *config.Config, scanTime time.Time) error {
	// Index findings by (env, cluster).
	type clusterKey struct{ env, cluster string }
	byCluster := make(map[clusterKey][]diagnosis.Finding)
	envTotals := make(map[string]int)
	for _, f := range findings {
		k := clusterKey{f.EnvName, f.ClusterCtx}
		byCluster[k] = append(byCluster[k], f)
		envTotals[f.EnvName]++
	}

	out := jsonOutput{
		ScanTime: scanTime.UTC().Format(time.RFC3339),
		Summary: jsonSummary{
			TotalIssues:   len(findings),
			ByEnvironment: envTotals,
		},
	}

	for _, env := range cfg.Environments {
		jenv := jsonEnvironment{
			Name: env.Name,
			Tier: env.Tier,
		}
		for _, cl := range env.Clusters {
			k := clusterKey{env.Name, cl.Context}
			clusterFindings := byCluster[k]

			// Group by category.
			grouped := make(map[string][]jsonItem)
			for _, f := range clusterFindings {
				key := categoryJSONKey[f.Category]
				if key == "" {
					key = string(f.Category)
				}
				grouped[key] = append(grouped[key], jsonItem{
					Severity:  string(f.Severity),
					Namespace: f.Namespace,
					Pod:       f.PodName,
					Container: f.ContainerName,
					Summary:   f.OneLiner,
					Detail:    f.DetailFields,
				})
			}

			jenv.Clusters = append(jenv.Clusters, jsonCluster{
				Context:     cl.Context,
				Findings:    grouped,
				TotalIssues: len(clusterFindings),
			})
		}
		out.Environments = append(out.Environments, jenv)
	}

	// Ensure by_environment includes all envs (even those with 0 issues).
	for _, env := range cfg.Environments {
		if _, ok := out.Summary.ByEnvironment[env.Name]; !ok {
			out.Summary.ByEnvironment[env.Name] = 0
		}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("encoding findings as JSON: %w", err)
	}
	return nil
}
