package diagnosis

import (
	"fmt"
	"strings"
	"time"
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
		subtype := classifyPendingMessage(p.Message)

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
		if p.Message != "" {
			detail["message"] = p.Message
		}

		findings = append(findings, Finding{
			Category:   CategoryPending,
			Severity:   SeverityWarning,
			EnvName:    results.EnvName,
			ClusterCtx: results.ClusterCtx,
			Namespace:  p.Namespace,
			PodName:    p.PodName,
			OneLiner:   oneLiner,
			DetailFields: detail,
		})
	}
	return findings
}
