// Package diagnosis contains the error classification engine for klarity.
// Classifiers consume raw ScanResults from pkg/kube and produce []Finding —
// structured, display-agnostic data. The pkg/output layer is the ONLY place
// that formats findings into terminal tables or JSON.
package diagnosis

import (
	"github.com/vishukamble/klarity/pkg/kube"
)

// ── Category ──────────────────────────────────────────────────────────────────

// Category identifies the root-cause type of a Finding.
type Category string

const (
	CategoryOOMKilled          Category = "OOMKilled"
	CategoryImagePull          Category = "ImagePull"
	CategoryCrashLoop          Category = "CrashLoop"
	CategoryPending            Category = "Pending"
	CategoryHPACeiling         Category = "HPACeiling"
	CategoryNoEndpoints        Category = "NoEndpoints"
	CategoryQuotaExhausted     Category = "QuotaExhausted"
	CategoryPVCPending         Category = "PVCPending"
	CategoryJobFailed          Category = "JobFailed"
	CategoryCronJobSuspended   Category = "CronJobSuspended"
	CategoryDaemonSetDegraded  Category = "DaemonSetDegraded"
	CategoryStatefulSetDegraded Category = "StatefulSetDegraded"
	CategoryWarningEvent       Category = "WarningEvent"
)

// ── Severity ──────────────────────────────────────────────────────────────────

// Severity controls display priority and colouring in the output layer.
type Severity string

const (
	SeverityCritical Severity = "Critical" // red — service is down or imminently degraded
	SeverityWarning  Severity = "Warning"  // yellow — degraded but not fully broken
	SeverityInfo     Severity = "Info"     // green / dim — informational
)

// ── Finding ───────────────────────────────────────────────────────────────────

// Finding is the atomic unit of diagnosis output. It carries enough structured
// data for the output layer to render a table row without any further API
// calls or string manipulation.
type Finding struct {
	Category      Category
	Severity      Severity
	EnvName       string // from ScanResults.EnvName
	ClusterCtx    string // from ScanResults.ClusterCtx
	Namespace     string
	PodName       string // empty for non-pod findings
	ContainerName string // empty for pod-level findings
	OneLiner      string // concise one-line summary; never contains ANSI codes
	DetailFields  map[string]string // structured detail; key=field name, val=display string
}

// ── ScanResults ───────────────────────────────────────────────────────────────

// ScanResults aggregates the output of all pkg/kube scanners for a single
// cluster. The scan loop creates one ScanResults per cluster and passes it to
// every Classifier.
type ScanResults struct {
	// Identifying context — set by the scan loop before classification.
	EnvName    string
	ClusterCtx string

	// Scanner outputs — all flat; Namespace field on each item identifies scope.
	Pods         []kube.PodIssue
	Deployments  []kube.DeploymentIssue
	HPAs         []kube.HPAIssue
	Services     []kube.ServiceIssue
	Events       []kube.EventIssue
	Quotas       []kube.QuotaIssue
	PVCs         []kube.PVCIssue
	DaemonSets   []kube.DaemonSetIssue
	StatefulSets []kube.StatefulSetIssue
	Jobs         []kube.JobIssue
	CronJobs     []kube.CronJobIssue
}

// ── Classifier interface ──────────────────────────────────────────────────────

// Classifier examines a ScanResults and returns zero or more Findings.
// Implementations must be pure functions of their input — no API calls,
// no shared state, no formatting.
type Classifier interface {
	Classify(results ScanResults) []Finding
}

// ── RunAll ────────────────────────────────────────────────────────────────────

// RunAll applies every classifier in cs to results and returns the combined
// slice of findings, preserving classifier order.
func RunAll(results ScanResults, cs []Classifier) []Finding {
	var all []Finding
	for _, c := range cs {
		all = append(all, c.Classify(results)...)
	}
	return all
}
