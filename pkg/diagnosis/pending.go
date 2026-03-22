package diagnosis

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/vishukamble/klarity/pkg/kube"
)

// pendingSubtype identifies why a pod is stuck Pending.
type pendingSubtype string

const (
	pendingInsufficientCPU    pendingSubtype = "insufficient_cpu"
	pendingInsufficientMemory pendingSubtype = "insufficient_memory"
	pendingUnschedulable      pendingSubtype = "unschedulable"   // taint/affinity/node selector
	pendingPVCNotBound        pendingSubtype = "pvc_not_bound"
	pendingUnknown            pendingSubtype = "unknown"
)

func classifyPendingMessage(msg string) pendingSubtype {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "insufficient cpu"):
		return pendingInsufficientCPU
	case strings.Contains(lower, "insufficient memory"):
		return pendingInsufficientMemory
	case strings.Contains(lower, "taint") ||
		strings.Contains(lower, "affinity") ||
		strings.Contains(lower, "node selector") ||
		strings.Contains(lower, "unschedulable"):
		return pendingUnschedulable
	case strings.Contains(lower, "persistentvolumeclaim") ||
		strings.Contains(lower, "pvc") ||
		strings.Contains(lower, "volume"):
		return pendingPVCNotBound
	default:
		return pendingUnknown
	}
}

// ── Scheduling message parser ────────────────────────────────────────────────

// SchedulingReason represents a single parsed reason from a compound K8s
// scheduling message.
type SchedulingReason struct {
	Count   int    // the N in "N node(s) had..."
	Kind    string // taint, affinity, resource, autoscaler, other
	Summary string // human-readable one-liner
}

// Regex patterns for parsing scheduling reasons.
var (
	// "N node(s) had untolerated taint {KEY: VALUE}"
	taintRe = regexp.MustCompile(`(\d+)\s+node\(s\)\s+had\s+untolerated\s+taint\s+\{([^:}]+):\s*([^}]*)\}`)

	// "N node(s) didn't match Pod's node affinity/selector"
	affinityRe = regexp.MustCompile(`(\d+)\s+node\(s\)\s+didn't\s+match\s+Pod's\s+node\s+affinity`)

	// "N Insufficient cpu" or "N Insufficient memory" or "N Insufficient nvidia.com/gpu"
	insufficientRe = regexp.MustCompile(`(\d+)\s+Insufficient\s+([\w./\-]+)`)

	// "N max node group size reached"
	maxNodeGroupRe = regexp.MustCompile(`(\d+)\s+max\s+node\s+group\s+size\s+reached`)

	// "pod didn't trigger scale-up"
	noScaleUpRe = regexp.MustCompile(`pod\s+didn't\s+trigger\s+scale-up`)

	// "N didn't match pod topology spread constraints"
	topologySpreadRe = regexp.MustCompile(`(\d+)\s+(?:node\(s\)\s+)?didn't\s+match\s+pod\s+topology\s+spread\s+constraints`)

	// KeyVault secret error: KeyVault Secret "NAME" or "does not exist"
	keyVaultSecretRe = regexp.MustCompile(`(?i)keyvault\s+secret\s+"([^"]+)"`)
)

// stripPreemption removes the "preemption: ..." suffix from scheduling messages.
func stripPreemption(msg string) string {
	lower := strings.ToLower(msg)
	if idx := strings.Index(lower, "preemption:"); idx >= 0 {
		msg = strings.TrimRight(msg[:idx], " .;,")
	}
	return msg
}

// parseSchedulingMessage splits a compound K8s scheduling message into
// individual classified reasons, sorted by Count descending.
func parseSchedulingMessage(message string) []SchedulingReason {
	if message == "" {
		return nil
	}

	// Check for KeyVault/CSI errors first (separate from scheduling).
	if m := keyVaultSecretRe.FindStringSubmatch(message); len(m) > 1 {
		return []SchedulingReason{{
			Count:   1,
			Kind:    "other",
			Summary: fmt.Sprintf("KeyVault secret missing: %s — verify secret exists in vault and CSI driver has access", m[1]),
		}}
	}

	cleaned := stripPreemption(message)

	// Strip leading "0/N nodes are available: " prefix.
	if idx := strings.Index(cleaned, ": "); idx >= 0 {
		lower := strings.ToLower(cleaned[:idx])
		if strings.Contains(lower, "nodes are available") || strings.Contains(lower, "node is available") {
			cleaned = cleaned[idx+2:]
		}
	}

	// Split by comma.
	parts := strings.Split(cleaned, ",")

	var reasons []SchedulingReason
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if r := classifyReasonPart(part); r != nil {
			reasons = append(reasons, *r)
		}
	}

	if len(reasons) == 0 {
		return nil
	}

	// Sort by Count descending.
	sort.Slice(reasons, func(i, j int) bool {
		return reasons[i].Count > reasons[j].Count
	})

	return reasons
}

// classifyReasonPart classifies a single comma-separated reason fragment.
func classifyReasonPart(part string) *SchedulingReason {
	// Taint
	if m := taintRe.FindStringSubmatch(part); len(m) == 4 {
		count, _ := strconv.Atoi(m[1])
		key := strings.TrimSpace(m[2])
		value := strings.TrimSpace(m[3])
		return &SchedulingReason{
			Count:   count,
			Kind:    "taint",
			Summary: classifyTaint(count, key, value),
		}
	}

	// Affinity
	if m := affinityRe.FindStringSubmatch(part); len(m) >= 2 {
		count, _ := strconv.Atoi(m[1])
		return &SchedulingReason{
			Count:   count,
			Kind:    "affinity",
			Summary: fmt.Sprintf("%s rejected by nodeAffinity/nodeSelector — check pod's nodeSelector and affinity rules match available node labels", nodeWord(count)),
		}
	}

	// Resource insufficiency
	if m := insufficientRe.FindStringSubmatch(part); len(m) == 3 {
		count, _ := strconv.Atoi(m[1])
		resource := strings.TrimRight(m[2], ".")
		return &SchedulingReason{
			Count:   count,
			Kind:    "resource",
			Summary: classifyResource(count, resource),
		}
	}

	// Autoscaler: max node group
	if m := maxNodeGroupRe.FindStringSubmatch(part); len(m) >= 2 {
		count, _ := strconv.Atoi(m[1])
		return &SchedulingReason{
			Count:   count,
			Kind:    "autoscaler",
			Summary: "Autoscaler at max capacity — increase max node count in node pool config",
		}
	}

	// Autoscaler: no scale-up
	if noScaleUpRe.MatchString(part) {
		return &SchedulingReason{
			Count:   1,
			Kind:    "autoscaler",
			Summary: "Autoscaler cannot expand — no node group matches pod requirements",
		}
	}

	// Topology spread constraints
	if m := topologySpreadRe.FindStringSubmatch(part); len(m) >= 2 {
		count, _ := strconv.Atoi(m[1])
		return &SchedulingReason{
			Count:   count,
			Kind:    "affinity",
			Summary: fmt.Sprintf("%s: topology spread violated — pods unevenly distributed across zones", nodeWord(count)),
		}
	}

	return nil
}

func classifyTaint(count int, key, value string) string {
	nw := nodeWord(count)
	switch {
	case key == "CriticalAddonsOnly":
		return fmt.Sprintf("%s reserved for system addons — add toleration for CriticalAddonsOnly", nw)
	case key == "role" && strings.Contains(strings.ToLower(value), "gpu"):
		return fmt.Sprintf("%s are GPU pool — pod needs nvidia.com/gpu resource request + toleration", nw)
	case key == "role" && strings.Contains(strings.ToLower(value), "cpu"):
		return fmt.Sprintf("%s are CPU training pool — add toleration for role=%s", nw, value)
	case key == "node.kubernetes.io/not-ready":
		return fmt.Sprintf("%s not ready — cluster may be scaling or unhealthy", nw)
	case key == "node.kubernetes.io/unreachable":
		return fmt.Sprintf("%s unreachable — possible node failure", nw)
	default:
		return fmt.Sprintf("%s have taint %s=%s — pod needs matching toleration", nw, key, value)
	}
}

func classifyResource(count int, resource string) string {
	nw := nodeWord(count)
	lower := strings.ToLower(resource)
	switch {
	case lower == "cpu":
		return fmt.Sprintf("%s have insufficient CPU — lower requests or scale up node pool", nw)
	case lower == "memory":
		return fmt.Sprintf("%s have insufficient memory — lower requests or scale up node pool", nw)
	case lower == "nvidia.com/gpu":
		return fmt.Sprintf("%s have no GPU — request a GPU node pool or remove gpu resource request", nw)
	case strings.HasPrefix(lower, "nvidia.com/mig-"):
		slice := resource[len("nvidia.com/mig-"):]
		return fmt.Sprintf("%s have no MIG slice %s — check GPU partitioning config", nw, slice)
	default:
		return fmt.Sprintf("%s have insufficient %s — lower requests or scale up", nw, resource)
	}
}

// nodeWord returns "N nodes" or "1 node" for use in summaries.
func nodeWord(count int) string {
	if count == 1 {
		return "1 node"
	}
	return fmt.Sprintf("%d nodes", count)
}

// formatSchedulingReasons renders parsed reasons as a bulleted summary.
func formatSchedulingReasons(reasons []SchedulingReason) string {
	if len(reasons) == 0 {
		return ""
	}
	if len(reasons) == 1 {
		return reasons[0].Summary
	}
	var lines []string
	for _, r := range reasons {
		lines = append(lines, fmt.Sprintf("• %s", r.Summary))
	}
	return strings.Join(lines, "\n")
}

// ── PendingClassifier ────────────────────────────────────────────────────────

// PendingClassifier finds pods stuck in Pending phase.
type PendingClassifier struct {
	// Now allows tests to inject a fixed time; defaults to time.Now if nil.
	Now func() time.Time
}

func (pc PendingClassifier) now() time.Time {
	if pc.Now != nil {
		return pc.Now()
	}
	return time.Now()
}

func (pc PendingClassifier) Classify(results ScanResults) []Finding {
	var findings []Finding
	for _, p := range results.Pods {
		if p.Reason != "Pending" {
			continue
		}

		// Strip preemption suffix before any processing.
		msg := stripPreemption(p.Message)

		subtype := classifyPendingMessage(msg)

		var pendingDuration string
		if !p.PendingSince.IsZero() {
			d := pc.now().Sub(p.PendingSince).Truncate(time.Second)
			pendingDuration = d.String()
		}

		var oneLiner string
		if pendingDuration != "" {
			oneLiner = fmt.Sprintf("Pod %s pending for %s (%s)", p.PodName, pendingDuration, subtype)
		} else {
			oneLiner = fmt.Sprintf("Pod %s pending (%s)", p.PodName, subtype)
		}

		detail := map[string]string{
			"subtype": string(subtype),
		}
		if pendingDuration != "" {
			detail["pending_duration"] = pendingDuration
		}

		// PVC suggestion takes priority — show only the hint, not the raw message.
		if pvcHint := pvcSuggestion(p, results); pvcHint != "" {
			detail["message"] = pvcHint
		} else if msg != "" {
			// Parse compound scheduling message into structured reasons.
			if reasons := parseSchedulingMessage(msg); len(reasons) > 0 {
				detail["message"] = formatSchedulingReasons(reasons)
			} else {
				detail["message"] = msg
			}
		}

		findings = append(findings, Finding{
			Category:     CategoryPending,
			Severity:     SeverityWarning,
			EnvName:      results.EnvName,
			ClusterCtx:   results.ClusterCtx,
			Namespace:    p.Namespace,
			PodName:      p.PodName,
			OneLiner:     oneLiner,
			DetailFields: detail,
		})
	}
	return findings
}

// pvcSuggestion checks if a pending pod references PVCs that don't exist
// and returns a hint string with typo suggestions. Returns "" if all PVCs
// exist or no volume claims are present.
func pvcSuggestion(p kube.PodIssue, results ScanResults) string {
	if len(p.VolumeClaimNames) == 0 || results.AllPVCNames == nil {
		return ""
	}

	existing := results.AllPVCNames[p.Namespace]
	existingSet := make(map[string]bool, len(existing))
	for _, name := range existing {
		existingSet[name] = true
	}

	var hints []string
	for _, claim := range p.VolumeClaimNames {
		if existingSet[claim] {
			continue
		}
		hint := fmt.Sprintf("PVC '%s' not found", claim)
		if suggestion := closestPVCName(claim, existing); suggestion != "" {
			hint += fmt.Sprintf(" (did you mean '%s'?)", suggestion)
		}
		hints = append(hints, hint)
	}
	if len(hints) == 0 {
		return ""
	}
	return strings.Join(hints, "; ")
}

// closestPVCName returns the existing PVC name with the smallest Levenshtein
// distance to target, provided that distance is <= 2. Returns "" if no close
// match exists.
func closestPVCName(target string, existing []string) string {
	best := ""
	bestDist := 3 // only suggest if distance <= 2
	for _, name := range existing {
		d := levenshtein(target, name)
		if d < bestDist {
			bestDist = d
			best = name
		}
	}
	return best
}

// levenshtein computes the Levenshtein edit distance between two strings.
func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	// Use single-row DP to save memory.
	prev := make([]int, lb+1)
	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr := make([]int, lb+1)
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			ins := curr[j-1] + 1
			del := prev[j] + 1
			sub := prev[j-1] + cost
			curr[j] = min3(ins, del, sub)
		}
		prev = curr
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
