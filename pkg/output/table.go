package output

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"

	"github.com/vishukamble/klarity/pkg/config"
	"github.com/vishukamble/klarity/pkg/diagnosis"
)

// ── Category metadata ─────────────────────────────────────────────────────────

type catSpec struct {
	icon    string
	label   string
	headers []string
	rowFn   func(f diagnosis.Finding) []string
}

func detailOr(f diagnosis.Finding, key, fallback string) string {
	if v := f.DetailFields[key]; v != "" {
		return v
	}
	return fallback
}

var categorySpecs = map[diagnosis.Category]catSpec{
	diagnosis.CategoryNodeIssue: {
		icon:    "🖥️ ",
		label:   "Node Issues",
		headers: []string{"Node", "Condition", "Why"},
		rowFn: func(f diagnosis.Finding) []string {
			return []string{
				detailOr(f, "node_name", "-"),
				detailOr(f, "condition", "-"),
				wrapText(f.OneLiner, wrapWidth),
			}
		},
	},
	diagnosis.CategoryOOMKilled: {
		icon:    "💀",
		label:   "OOMKilled",
		headers: []string{"Namespace", "Pod", "Container", "Image", "Restarts"},
		rowFn: func(f diagnosis.Finding) []string {
			return []string{
				f.Namespace,
				f.PodName,
				f.ContainerName,
				detailOr(f, "image", "-"),
				detailOr(f, "restart_count", "-"),
			}
		},
	},
	diagnosis.CategoryImagePull: {
		icon:    "🖼️ ",
		label:   "Image Pull Errors",
		headers: []string{"Namespace", "Pod", "Container", "Image", "Type"},
		rowFn: func(f diagnosis.Finding) []string {
			subtype := detailOr(f, "subtype", "unknown")
			return []string{
				f.Namespace,
				f.PodName,
				f.ContainerName,
				detailOr(f, "image", "-"),
				prettySubtype(subtype),
			}
		},
	},
	diagnosis.CategoryCrashLoop: {
		icon:    "🔥",
		label:   "CrashLoopBackOff",
		headers: []string{"Namespace", "Pod", "Restarts", "Root Cause"},
		rowFn: func(f diagnosis.Finding) []string {
			return []string{
				f.Namespace,
				f.PodName,
				detailOr(f, "restart_count", "-"),
				wrapText(f.OneLiner, wrapWidth),
			}
		},
	},
	diagnosis.CategoryPending: {
		icon:    "⏳",
		label:   "Pending Pods",
		headers: []string{"Namespace", "Pod", "Pending For", "Reason"},
		rowFn: func(f diagnosis.Finding) []string {
			reason := detailOr(f, "subtype", "unknown")
			if msg := f.DetailFields["message"]; msg != "" {
				reason = msg
			}
			return []string{
				f.Namespace,
				f.PodName,
				detailOr(f, "pending_duration", "-"),
				wrapLines(reason, wrapWidth),
			}
		},
	},
	diagnosis.CategoryHPACeiling: {
		icon:    "📈",
		label:   "HPA Scaling Issues",
		headers: []string{"Namespace", "HPA", "Current/Max", "CPU % of Target", "CPU Target"},
		rowFn: func(f diagnosis.Finding) []string {
			hpaName := detailOr(f, "hpa_name", "-")
			cur := detailOr(f, "current_replicas", "?")
			max := detailOr(f, "max_replicas", "?")
			cpuNowRaw := detailOr(f, "cpu_current_percent", "")
			cpuTgtRaw := detailOr(f, "cpu_target_percent", "")

			cpuNow := "-"
			if cpuNowRaw != "" {
				val, errN := strconv.Atoi(cpuNowRaw)
				if errN == nil {
					tgtVal, errT := strconv.Atoi(cpuTgtRaw)
					if errT == nil && val > 200 {
						cpuNow = fmt.Sprintf("%d%% (%.1f×)", val, float64(val)/float64(tgtVal))
					} else {
						cpuNow = fmt.Sprintf("%d%%", val)
					}
					if val > 150 {
						cpuNow = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(cpuNow)
					}
				}
			}

			cpuTgt := "-"
			if cpuTgtRaw != "" {
				cpuTgt = cpuTgtRaw + "%"
			}

			return []string{
				f.Namespace,
				hpaName,
				cur + "/" + max,
				cpuNow,
				cpuTgt,
			}
		},
	},
	// Remaining categories use a generic renderer (populated by future classifiers).
	diagnosis.CategoryNoEndpoints: {
		icon:    "🔌",
		label:   "Services with No Endpoints",
		headers: []string{"Namespace", "Service", "Summary"},
		rowFn:   genericRow,
	},
	diagnosis.CategoryQuotaExhausted: {
		icon:    "📊",
		label:   "Quota Exhausted",
		headers: []string{"Namespace", "Resource", "Summary"},
		rowFn:   genericRow,
	},
	diagnosis.CategoryPVCPending: {
		icon:    "💾",
		label:   "PVC Pending",
		headers: []string{"Namespace", "PVC", "Summary"},
		rowFn:   genericRow,
	},
	diagnosis.CategoryJobFailed: {
		icon:    "❌",
		label:   "Failed Jobs",
		headers: []string{"Namespace", "Job", "Summary"},
		rowFn:   genericRow,
	},
	diagnosis.CategoryCronJobSuspended: {
		icon:    "⏸️ ",
		label:   "Suspended CronJobs",
		headers: []string{"Namespace", "CronJob", "Summary"},
		rowFn:   genericRow,
	},
	diagnosis.CategoryDaemonSetDegraded: {
		icon:    "🔧",
		label:   "DaemonSet Degraded",
		headers: []string{"Namespace", "DaemonSet", "Summary"},
		rowFn:   genericRow,
	},
	diagnosis.CategoryStatefulSetDegraded: {
		icon:    "🔧",
		label:   "StatefulSet Degraded",
		headers: []string{"Namespace", "StatefulSet", "Summary"},
		rowFn:   genericRow,
	},
	diagnosis.CategoryWarningEvent: {
		icon:    "⚠️ ",
		label:   "Warning Events",
		headers: []string{"Namespace", "Object", "Category", "Why"},
		rowFn: func(f diagnosis.Finding) []string {
			objName := detailOr(f, "object_name", "-")
			reason := detailOr(f, "reason", "-")
			return []string{f.Namespace, objName, reason, wrapText(f.OneLiner, wrapWidth)}
		},
	},
}

func genericRow(f diagnosis.Finding) []string {
	name := f.PodName
	if name == "" {
		name = "-"
	}
	return []string{f.Namespace, name, wrapText(f.OneLiner, wrapWidth)}
}

func prettySubtype(s string) string {
	switch s {
	case "auth_error":
		return "Auth / 401-403"
	case "tag_not_found":
		return "Tag not found"
	case "registry_unreachable":
		return "Registry unreachable"
	default:
		return s
	}
}

// ── Env ordering ──────────────────────────────────────────────────────────────

// sortedEnvs returns environments in display order: critical first, then by
// original config position (stable).
func sortedEnvs(cfg *config.Config) []config.Environment {
	envs := make([]config.Environment, len(cfg.Environments))
	copy(envs, cfg.Environments)
	sort.SliceStable(envs, func(i, j int) bool {
		ti := envTierRank(envs[i])
		tj := envTierRank(envs[j])
		return ti < tj
	})
	return envs
}

func envTierRank(e config.Environment) int {
	if e.Tier == config.TierCritical {
		return 0
	}
	return 1
}

// ── Category ordering ─────────────────────────────────────────────────────────

// categoryOrder defines the display order within a cluster section.
var categoryOrder = []diagnosis.Category{
	diagnosis.CategoryNodeIssue,
	diagnosis.CategoryOOMKilled,
	diagnosis.CategoryImagePull,
	diagnosis.CategoryCrashLoop,
	diagnosis.CategoryPending,
	diagnosis.CategoryHPACeiling,
	diagnosis.CategoryNoEndpoints,
	diagnosis.CategoryQuotaExhausted,
	diagnosis.CategoryPVCPending,
	diagnosis.CategoryJobFailed,
	diagnosis.CategoryCronJobSuspended,
	diagnosis.CategoryDaemonSetDegraded,
	diagnosis.CategoryStatefulSetDegraded,
	diagnosis.CategoryWarningEvent,
}

// ── Main renderer ─────────────────────────────────────────────────────────────

const sepWidth = 72

// RenderReport writes the full terminal report to w.
func RenderReport(
	w io.Writer,
	findings []diagnosis.Finding,
	cfg *config.Config,
	startTime time.Time,
	scanErrors []string,
) {
	// Index findings by (envName, clusterCtx, category).
	type key struct{ env, cluster string; cat diagnosis.Category }
	byKey := make(map[key][]diagnosis.Finding)
	for _, f := range findings {
		k := key{f.EnvName, f.ClusterCtx, f.Category}
		byKey[k] = append(byKey[k], f)
	}

	// Count totals.
	envTotals := make(map[string]int)
	for _, f := range findings {
		envTotals[f.EnvName]++
	}
	total := len(findings)

	// ── Title banner ────────────────────────────────────────────────────────
	banner := fmt.Sprintf(
		"  klarity scan — %s\n  Environments: %d | Clusters: %d scanned | Issues: %d found",
		startTime.Format("2006-01-02 15:04:05 MST"),
		len(cfg.Environments),
		countClusters(cfg),
		total,
	)
	topLine := "╔" + strings.Repeat("═", sepWidth) + "╗"
	botLine := "╚" + strings.Repeat("═", sepWidth) + "╝"
	fmt.Fprintln(w, BoldStyle.Render(topLine))
	for _, line := range strings.Split(banner, "\n") {
		// Pad line to sepWidth.
		content := "║" + padRight(line, sepWidth) + "║"
		fmt.Fprintln(w, BoldStyle.Render(content))
	}
	fmt.Fprintln(w, BoldStyle.Render(botLine))
	fmt.Fprintln(w)

	// Print scan errors (if any) before the report body.
	if len(scanErrors) > 0 {
		for _, e := range scanErrors {
			fmt.Fprintf(w, "%s\n", SeverityStyle(diagnosis.SeverityWarning).Render("⚠ "+e))
		}
		fmt.Fprintln(w)
	}

	// ── Env sections ─────────────────────────────────────────────────────────
	for _, env := range sortedEnvs(cfg) {
		headerStyle := EnvHeaderStyle(env)
		emoji := EnvEmoji(env)

		envLine := envSep(emoji+" "+strings.ToUpper(env.Name), sepWidth)
		fmt.Fprintln(w, headerStyle.Render(envLine))
		fmt.Fprintln(w)

		for _, cluster := range env.Clusters {
			clusterLine := clusterSep(cluster.Context, sepWidth)
			fmt.Fprintln(w, DimStyle.Render(clusterLine))
			fmt.Fprintln(w)

			// Gather findings for this cluster.
			clusterHasFindings := false
			for _, cat := range categoryOrder {
				k := key{env.Name, cluster.Context, cat}
				fs := byKey[k]
				if len(fs) == 0 {
					continue
				}
				clusterHasFindings = true
				renderCategorySection(w, cat, fs)
			}

			if !clusterHasFindings {
				fmt.Fprintln(w, DimStyle.Render("  ✅ No issues found."))
				if kubeSystemExcluded(cluster, cfg.Settings.DefaultNsExclude) {
					fmt.Fprintln(w, DimStyle.Render("   ℹ kube-system excluded — use --namespace kube-system to scan it"))
				}
			}
			fmt.Fprintln(w)
		}
	}

	// ── Footer ───────────────────────────────────────────────────────────────
	renderFooter(w, cfg, envTotals, startTime)
}

// renderCategorySection prints the category header and its lipgloss table.
func renderCategorySection(w io.Writer, cat diagnosis.Category, findings []diagnosis.Finding) {
	spec, ok := categorySpecs[cat]
	if !ok {
		// Unknown category — fall back to generic.
		spec = catSpec{icon: "•", label: string(cat), headers: []string{"Namespace", "Resource", "Summary"}, rowFn: genericRow}
	}

	// Determine severity from first finding (all in same category share it).
	sev := findings[0].Severity
	catStyle := SeverityStyle(sev)

	header := fmt.Sprintf("%s %s (%d)", spec.icon, spec.label, len(findings))
	fmt.Fprintln(w, catStyle.Render(header))

	// Build the lipgloss table.
	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(TableBorderStyle).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return TableHeaderStyle
			}
			return TableCellStyle
		}).
		Headers(spec.headers...).
		Wrap(false)

	for _, f := range findings {
		t.Row(spec.rowFn(f)...)
	}

	fmt.Fprintln(w, t.Render())
	fmt.Fprintln(w)
}

// ── Separator helpers ─────────────────────────────────────────────────────────

// envSep renders: ━━━ LABEL ━━━━━━━━━━━━━━...
func envSep(label string, width int) string {
	prefix := "━━━ " + label + " "
	remain := width - len([]rune(prefix))
	if remain < 0 {
		remain = 0
	}
	return prefix + strings.Repeat("━", remain)
}

// clusterSep renders: ── label ──────────────────...
func clusterSep(label string, width int) string {
	prefix := "── " + label + " "
	remain := width - len([]rune(prefix))
	if remain < 0 {
		remain = 0
	}
	return prefix + strings.Repeat("─", remain)
}

// padRight pads s to exactly width runes with spaces.
func padRight(s string, width int) string {
	runes := []rune(s)
	if len(runes) >= width {
		return string(runes[:width])
	}
	return s + strings.Repeat(" ", width-len(runes))
}

func countClusters(cfg *config.Config) int {
	n := 0
	for _, e := range cfg.Environments {
		n += len(e.Clusters)
	}
	return n
}

// wrapText inserts a single newline at the nearest word boundary before
// maxWidth. If the string fits within maxWidth or is empty, it is returned
// unchanged. When no space exists before maxWidth the string is hard-broken.
// Width is measured in Unicode code points (runes) to handle multi-byte characters.
func wrapText(s string, maxWidth int) string {
	runes := []rune(s)
	if len(runes) <= maxWidth {
		return s
	}

	// Find the last space at or before maxWidth (rune-indexed).
	breakAt := -1
	for i := maxWidth - 1; i >= 0; i-- {
		if runes[i] == ' ' {
			breakAt = i
			break
		}
	}
	if breakAt <= 0 {
		// No word boundary — hard-break at maxWidth.
		breakAt = maxWidth
	}

	// Skip the space at the break point so the next line has no leading space.
	if breakAt < len(runes) && runes[breakAt] == ' ' {
		return string(runes[:breakAt]) + "\n" + string(runes[breakAt+1:])
	}
	return string(runes[:breakAt]) + "\n" + string(runes[breakAt:])
}

// wrapLines applies wrapText to each existing line independently. This is safe
// for pre-formatted multi-line strings (e.g. bulleted scheduling reasons from
// formatSchedulingReasons) where each individual line is already < maxWidth
// but the total string length exceeds it.
func wrapLines(s string, maxWidth int) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = wrapText(line, maxWidth)
	}
	return strings.Join(lines, "\n")
}

const wrapWidth = 80

// kubeSystemExcluded reports whether kube-system would be excluded from scanning
// for the given cluster configuration and default exclusion list.
func kubeSystemExcluded(cluster config.Cluster, defaultExclude []string) bool {
	ns := cluster.Namespaces
	switch ns.Mode {
	case config.NamespaceModeInclude:
		// kube-system is excluded unless explicitly listed in the include set.
		for _, n := range ns.Include {
			if n == "kube-system" {
				return false
			}
		}
		return true
	case config.NamespaceModeExclude:
		for _, n := range ns.Exclude {
			if n == "kube-system" {
				return true
			}
		}
		return false
	default: // all
		// Cluster-specific exclude list takes precedence over default.
		if len(ns.Exclude) > 0 {
			for _, n := range ns.Exclude {
				if n == "kube-system" {
					return true
				}
			}
			return false
		}
		for _, n := range defaultExclude {
			if n == "kube-system" {
				return true
			}
		}
		return false
	}
}
