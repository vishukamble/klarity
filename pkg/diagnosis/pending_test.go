package diagnosis

import (
	"strings"
	"testing"
	"time"

	"github.com/vishukamble/klarity/pkg/kube"
)

func TestPendingClassifier(t *testing.T) {
	fixedNow := time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC)
	pendingSince := fixedNow.Add(-5 * time.Minute)

	tests := []struct {
		name        string
		results     ScanResults
		wantLen     int
		wantSev     Severity
		wantSubtype string
		checkDetail func(map[string]string) bool
	}{
		{
			name:    "empty input",
			results: ScanResults{},
			wantLen: 0,
		},
		{
			name: "non-pending pod ignored",
			results: ScanResults{
				Pods: []kube.PodIssue{
					{PodName: "x", Reason: "CrashLoopBackOff"},
					{PodName: "y", Reason: "OOMKilled"},
				},
			},
			wantLen: 0,
		},
		{
			name: "pending insufficient cpu",
			results: ScanResults{
				Pods: []kube.PodIssue{
					{
						PodName:      "big-job",
						Reason:       "Pending",
						Message:      "0/3 nodes are available: 3 Insufficient cpu.",
						PendingSince: pendingSince,
					},
				},
			},
			wantLen:     1,
			wantSev:     SeverityWarning,
			wantSubtype: string(pendingInsufficientCPU),
		},
		{
			name: "pending insufficient memory",
			results: ScanResults{
				Pods: []kube.PodIssue{
					{
						PodName:      "mem-hungry",
						Reason:       "Pending",
						Message:      "0/3 nodes are available: 3 Insufficient memory.",
						PendingSince: pendingSince,
					},
				},
			},
			wantLen:     1,
			wantSubtype: string(pendingInsufficientMemory),
		},
		{
			name: "pending taint/unschedulable",
			results: ScanResults{
				Pods: []kube.PodIssue{
					{
						PodName: "spot-worker",
						Reason:  "Pending",
						Message: "0/3 nodes are available: 3 node(s) had taint {spot: true}, that the pod didn't tolerate.",
					},
				},
			},
			wantLen:     1,
			wantSubtype: string(pendingUnschedulable),
		},
		{
			name: "pending pvc not bound",
			results: ScanResults{
				Pods: []kube.PodIssue{
					{
						PodName: "stateful-0",
						Reason:  "Pending",
						Message: "persistentvolumeclaim \"data-pvc\" not found",
					},
				},
			},
			wantLen:     1,
			wantSubtype: string(pendingPVCNotBound),
		},
		{
			name: "pending duration in detail",
			results: ScanResults{
				Pods: []kube.PodIssue{
					{
						PodName:      "slow-pod",
						Reason:       "Pending",
						PendingSince: pendingSince,
					},
				},
			},
			wantLen: 1,
			checkDetail: func(d map[string]string) bool {
				return strings.Contains(d["pending_duration"], "5m")
			},
		},
		{
			name: "multiple pending pods",
			results: ScanResults{
				Pods: []kube.PodIssue{
					{PodName: "a", Reason: "Pending"},
					{PodName: "b", Reason: "Running"},
					{PodName: "c", Reason: "Pending"},
				},
			},
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := PendingClassifier{Now: func() time.Time { return fixedNow }}
			got := c.Classify(tt.results)
			if len(got) != tt.wantLen {
				t.Fatalf("got %d findings, want %d", len(got), tt.wantLen)
			}
			if tt.wantLen == 0 {
				return
			}
			f := got[0]
			if f.Category != CategoryPending {
				t.Errorf("category = %q, want %q", f.Category, CategoryPending)
			}
			if tt.wantSev != "" && f.Severity != tt.wantSev {
				t.Errorf("severity = %q, want %q", f.Severity, tt.wantSev)
			}
			if tt.wantSubtype != "" && f.DetailFields["subtype"] != tt.wantSubtype {
				t.Errorf("subtype = %q, want %q", f.DetailFields["subtype"], tt.wantSubtype)
			}
			if tt.checkDetail != nil && !tt.checkDetail(f.DetailFields) {
				t.Errorf("detail check failed: %v", f.DetailFields)
			}
		})
	}
}

func TestPendingClassifier_MissingPVC_WithSuggestion(t *testing.T) {
	c := PendingClassifier{}
	results := ScanResults{
		EnvName:    "prod",
		ClusterCtx: "prod-east",
		Pods: []kube.PodIssue{
			{
				Namespace:        "payments",
				PodName:          "app-0",
				Reason:           "Pending",
				VolumeClaimNames: []string{"data-pvcc"}, // typo: extra 'c'
			},
		},
		AllPVCNames: map[string][]string{
			"payments": {"data-pvc", "logs-pvc"},
		},
	}

	got := c.Classify(results)
	// PVC hint is folded into the single pending finding (no extra row).
	if len(got) != 1 {
		t.Fatalf("want 1 finding, got %d: %+v", len(got), got)
	}

	f := got[0]
	msg := f.DetailFields["message"]
	if !strings.Contains(msg, "data-pvcc") {
		t.Errorf("message should mention missing PVC name, got %q", msg)
	}
	if !strings.Contains(msg, "did you mean 'data-pvc'") {
		t.Errorf("message should suggest 'data-pvc', got %q", msg)
	}
}

func TestPendingClassifier_MissingPVC_NoSuggestion(t *testing.T) {
	c := PendingClassifier{}
	results := ScanResults{
		Pods: []kube.PodIssue{
			{
				Namespace:        "payments",
				PodName:          "app-0",
				Reason:           "Pending",
				VolumeClaimNames: []string{"completely-different-name"},
			},
		},
		AllPVCNames: map[string][]string{
			"payments": {"data-pvc", "logs-pvc"},
		},
	}

	got := c.Classify(results)
	if len(got) != 1 {
		t.Fatalf("want 1 finding, got %d", len(got))
	}

	msg := got[0].DetailFields["message"]
	if !strings.Contains(msg, "not found") {
		t.Errorf("message should say 'not found', got %q", msg)
	}
	if strings.Contains(msg, "did you mean") {
		t.Errorf("message should NOT suggest when distance > 2, got %q", msg)
	}
}

func TestPendingClassifier_PVCExists_NoExtraFinding(t *testing.T) {
	c := PendingClassifier{}
	results := ScanResults{
		Pods: []kube.PodIssue{
			{
				Namespace:        "payments",
				PodName:          "app-0",
				Reason:           "Pending",
				Message:          "persistentvolumeclaim \"data-pvc\" not found",
				VolumeClaimNames: []string{"data-pvc"},
			},
		},
		AllPVCNames: map[string][]string{
			"payments": {"data-pvc", "logs-pvc"},
		},
	}

	got := c.Classify(results)
	// PVC exists — only the normal pending finding, no extra PVC-not-found finding.
	if len(got) != 1 {
		t.Fatalf("want 1 finding (PVC exists), got %d: %+v", len(got), got)
	}
}

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"data-pvc", "data-pvcc", 1},  // insertion
		{"data-pvcc", "data-pvc", 1},  // deletion
		{"data-pvc", "data-pvd", 1},   // substitution
		{"data-pvc", "data-pvce", 1},  // append
		{"abc", "xyz", 3},            // completely different
		{"kitten", "sitting", 3},
	}
	for _, tt := range tests {
		got := levenshtein(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestClosestPVCName(t *testing.T) {
	existing := []string{"data-pvc", "logs-pvc", "cache-vol"}

	tests := []struct {
		target string
		want   string
	}{
		{"data-pvcc", "data-pvc"},      // distance 1 → suggest
		{"data-pv", "data-pvc"},        // distance 1 → suggest
		{"dta-pvc", "data-pvc"},        // distance 1 → suggest
		{"data-pvce", "data-pvc"},      // distance 1 → suggest
		{"logs-pvd", "logs-pvc"},       // distance 1 → suggest
		{"completely-unrelated", ""},   // distance > 2 → no suggestion
	}
	for _, tt := range tests {
		got := closestPVCName(tt.target, existing)
		if got != tt.want {
			t.Errorf("closestPVCName(%q) = %q, want %q", tt.target, got, tt.want)
		}
	}
}

func TestClassifyPendingMessage(t *testing.T) {
	tests := []struct {
		msg  string
		want pendingSubtype
	}{
		{"0/3 nodes are available: 3 Insufficient cpu.", pendingInsufficientCPU},
		{"Insufficient memory.", pendingInsufficientMemory},
		{"node(s) had taint", pendingUnschedulable},
		{"didn't match node affinity", pendingUnschedulable},
		{"node selector", pendingUnschedulable},
		{"persistentvolumeclaim not found", pendingPVCNotBound},
		{"volume not bound", pendingPVCNotBound},
		{"", pendingUnknown},
		{"some other message", pendingUnknown},
	}
	for _, tt := range tests {
		got := classifyPendingMessage(tt.msg)
		if got != tt.want {
			t.Errorf("classifyPendingMessage(%q) = %q, want %q", tt.msg, got, tt.want)
		}
	}
}

// ── parseSchedulingMessage tests ─────────────────────────────────────────────

func TestParseSchedulingMessage_SingleTaint(t *testing.T) {
	msg := `0/3 nodes are available: 3 node(s) had untolerated taint {spot: true}.`
	reasons := parseSchedulingMessage(msg)
	if len(reasons) != 1 {
		t.Fatalf("want 1 reason, got %d: %+v", len(reasons), reasons)
	}
	r := reasons[0]
	if r.Count != 3 {
		t.Errorf("Count = %d, want 3", r.Count)
	}
	if r.Kind != "taint" {
		t.Errorf("Kind = %q, want taint", r.Kind)
	}
	if !strings.Contains(r.Summary, "spot=true") {
		t.Errorf("Summary should mention taint key=value, got %q", r.Summary)
	}
	if !strings.Contains(r.Summary, "toleration") {
		t.Errorf("Summary should suggest toleration, got %q", r.Summary)
	}
}

func TestParseSchedulingMessage_CompoundFiveReasons(t *testing.T) {
	msg := `0/49 nodes are available: 1 Insufficient cpu, 1 Insufficient nvidia.com/mig-2g.20gb, 1 node(s) had untolerated taint {role: training-cpu}, 3 node(s) had untolerated taint {CriticalAddonsOnly: true}, 4 node(s) had untolerated taint {role: training-gpu}, 40 node(s) didn't match Pod's node affinity/selector. preemption: 0/49 nodes are available: 49 Preemption is not helpful for scheduling.`
	reasons := parseSchedulingMessage(msg)
	if len(reasons) != 6 {
		t.Fatalf("want 6 reasons, got %d: %+v", len(reasons), reasons)
	}
	// Should be sorted by Count descending.
	if reasons[0].Count != 40 {
		t.Errorf("first reason Count = %d, want 40 (highest)", reasons[0].Count)
	}
	if reasons[0].Kind != "affinity" {
		t.Errorf("first reason Kind = %q, want affinity", reasons[0].Kind)
	}
	if reasons[1].Count != 4 {
		t.Errorf("second reason Count = %d, want 4", reasons[1].Count)
	}

	// Verify specific classifications.
	kindCounts := map[string]int{}
	for _, r := range reasons {
		kindCounts[r.Kind]++
	}
	if kindCounts["taint"] != 3 {
		t.Errorf("want 3 taint reasons, got %d", kindCounts["taint"])
	}
	if kindCounts["affinity"] != 1 {
		t.Errorf("want 1 affinity reason, got %d", kindCounts["affinity"])
	}
	if kindCounts["resource"] != 2 {
		t.Errorf("want 2 resource reasons, got %d", kindCounts["resource"])
	}
}

func TestParseSchedulingMessage_GPUTaint(t *testing.T) {
	msg := `0/5 nodes are available: 4 node(s) had untolerated taint {role: training-gpu}.`
	reasons := parseSchedulingMessage(msg)
	if len(reasons) != 1 {
		t.Fatalf("want 1 reason, got %d", len(reasons))
	}
	r := reasons[0]
	if !strings.Contains(r.Summary, "GPU pool") {
		t.Errorf("Summary should mention GPU pool, got %q", r.Summary)
	}
	if !strings.Contains(r.Summary, "nvidia.com/gpu") {
		t.Errorf("Summary should mention nvidia.com/gpu, got %q", r.Summary)
	}
}

func TestParseSchedulingMessage_CriticalAddonsOnly(t *testing.T) {
	msg := `0/3 nodes are available: 3 node(s) had untolerated taint {CriticalAddonsOnly: true}.`
	reasons := parseSchedulingMessage(msg)
	if len(reasons) != 1 {
		t.Fatalf("want 1 reason, got %d", len(reasons))
	}
	r := reasons[0]
	if !strings.Contains(r.Summary, "system addons") {
		t.Errorf("Summary should mention system addons, got %q", r.Summary)
	}
	if !strings.Contains(r.Summary, "CriticalAddonsOnly") {
		t.Errorf("Summary should mention CriticalAddonsOnly, got %q", r.Summary)
	}
}

func TestParseSchedulingMessage_AffinityOnly(t *testing.T) {
	msg := `0/10 nodes are available: 10 node(s) didn't match Pod's node affinity/selector.`
	reasons := parseSchedulingMessage(msg)
	if len(reasons) != 1 {
		t.Fatalf("want 1 reason, got %d", len(reasons))
	}
	r := reasons[0]
	if r.Kind != "affinity" {
		t.Errorf("Kind = %q, want affinity", r.Kind)
	}
	if r.Count != 10 {
		t.Errorf("Count = %d, want 10", r.Count)
	}
	if !strings.Contains(r.Summary, "nodeAffinity") {
		t.Errorf("Summary should mention nodeAffinity, got %q", r.Summary)
	}
}

func TestParseSchedulingMessage_MixedResourcesAndTaints(t *testing.T) {
	msg := `0/5 nodes are available: 2 Insufficient memory, 3 node(s) had untolerated taint {dedicated: batch}.`
	reasons := parseSchedulingMessage(msg)
	if len(reasons) != 2 {
		t.Fatalf("want 2 reasons, got %d", len(reasons))
	}
	// Sorted descending: 3 taint first, then 2 resource.
	if reasons[0].Kind != "taint" || reasons[0].Count != 3 {
		t.Errorf("first reason should be taint/3, got %s/%d", reasons[0].Kind, reasons[0].Count)
	}
	if reasons[1].Kind != "resource" || reasons[1].Count != 2 {
		t.Errorf("second reason should be resource/2, got %s/%d", reasons[1].Kind, reasons[1].Count)
	}
	if !strings.Contains(reasons[1].Summary, "memory") {
		t.Errorf("resource summary should mention memory, got %q", reasons[1].Summary)
	}
}

func TestParseSchedulingMessage_KeyVaultCSI(t *testing.T) {
	msg := `KeyVault Secret "my-db-password" does not exist`
	reasons := parseSchedulingMessage(msg)
	if len(reasons) != 1 {
		t.Fatalf("want 1 reason, got %d", len(reasons))
	}
	r := reasons[0]
	if r.Kind != "other" {
		t.Errorf("Kind = %q, want other", r.Kind)
	}
	if !strings.Contains(r.Summary, "my-db-password") {
		t.Errorf("Summary should mention secret name, got %q", r.Summary)
	}
	if !strings.Contains(r.Summary, "CSI") {
		t.Errorf("Summary should mention CSI driver, got %q", r.Summary)
	}
}

func TestParseSchedulingMessage_DockerDesktop(t *testing.T) {
	msg := `0/1 nodes are available: 1 Insufficient cpu.`
	reasons := parseSchedulingMessage(msg)
	if len(reasons) != 1 {
		t.Fatalf("want 1 reason, got %d", len(reasons))
	}
	if reasons[0].Kind != "resource" {
		t.Errorf("Kind = %q, want resource", reasons[0].Kind)
	}
	if !strings.Contains(reasons[0].Summary, "CPU") {
		t.Errorf("Summary should mention CPU, got %q", reasons[0].Summary)
	}
}

func TestParseSchedulingMessage_EmptyAndGarbage(t *testing.T) {
	// Empty.
	if reasons := parseSchedulingMessage(""); reasons != nil {
		t.Errorf("empty message should return nil, got %+v", reasons)
	}
	// Garbage — no patterns match, should return nil.
	if reasons := parseSchedulingMessage("everything is fine"); reasons != nil {
		t.Errorf("garbage message should return nil, got %+v", reasons)
	}
}

func TestParseSchedulingMessage_StripPreemption(t *testing.T) {
	msg := `0/3 nodes are available: 3 Insufficient cpu. preemption: 0/3 nodes are available: 3 No preemption victims found.`
	reasons := parseSchedulingMessage(msg)
	if len(reasons) != 1 {
		t.Fatalf("want 1 reason, got %d", len(reasons))
	}
	// Preemption section should be stripped — only the CPU reason remains.
	if reasons[0].Kind != "resource" {
		t.Errorf("Kind = %q, want resource", reasons[0].Kind)
	}
}

func TestParseSchedulingMessage_GPUResource(t *testing.T) {
	msg := `0/5 nodes are available: 5 Insufficient nvidia.com/gpu.`
	reasons := parseSchedulingMessage(msg)
	if len(reasons) != 1 {
		t.Fatalf("want 1 reason, got %d", len(reasons))
	}
	r := reasons[0]
	if !strings.Contains(r.Summary, "no GPU") {
		t.Errorf("Summary should mention no GPU, got %q", r.Summary)
	}
}

func TestParseSchedulingMessage_MIGSlice(t *testing.T) {
	msg := `0/5 nodes are available: 1 Insufficient nvidia.com/mig-2g.20gb.`
	reasons := parseSchedulingMessage(msg)
	if len(reasons) != 1 {
		t.Fatalf("want 1 reason, got %d", len(reasons))
	}
	r := reasons[0]
	if !strings.Contains(r.Summary, "MIG slice") {
		t.Errorf("Summary should mention MIG slice, got %q", r.Summary)
	}
	if !strings.Contains(r.Summary, "2g.20gb") {
		t.Errorf("Summary should mention slice name, got %q", r.Summary)
	}
}

func TestParseSchedulingMessage_Autoscaler(t *testing.T) {
	msg := `0/5 nodes are available: 5 max node group size reached.`
	reasons := parseSchedulingMessage(msg)
	if len(reasons) != 1 {
		t.Fatalf("want 1 reason, got %d", len(reasons))
	}
	r := reasons[0]
	if r.Kind != "autoscaler" {
		t.Errorf("Kind = %q, want autoscaler", r.Kind)
	}
	if !strings.Contains(r.Summary, "max capacity") {
		t.Errorf("Summary should mention max capacity, got %q", r.Summary)
	}
}

func TestParseSchedulingMessage_NoScaleUp(t *testing.T) {
	msg := `pod didn't trigger scale-up: 1 max node group size reached`
	reasons := parseSchedulingMessage(msg)
	// This has "pod didn't trigger scale-up" and "1 max node group size reached"
	// but "pod didn't trigger scale-up" is in the prefix, not after ": ".
	// The whole message goes through as a single string.
	if len(reasons) == 0 {
		t.Fatal("want at least 1 reason, got 0")
	}
	hasAutoscaler := false
	for _, r := range reasons {
		if r.Kind == "autoscaler" {
			hasAutoscaler = true
		}
	}
	if !hasAutoscaler {
		t.Errorf("should have autoscaler reason, got %+v", reasons)
	}
}

func TestParseSchedulingMessage_NotReadyTaint(t *testing.T) {
	msg := `0/3 nodes are available: 2 node(s) had untolerated taint {node.kubernetes.io/not-ready: }.`
	reasons := parseSchedulingMessage(msg)
	if len(reasons) != 1 {
		t.Fatalf("want 1 reason, got %d", len(reasons))
	}
	if !strings.Contains(reasons[0].Summary, "not ready") {
		t.Errorf("Summary should mention not ready, got %q", reasons[0].Summary)
	}
}

func TestFormatSchedulingReasons_Single(t *testing.T) {
	reasons := []SchedulingReason{
		{Count: 3, Kind: "resource", Summary: "3 nodes have insufficient CPU"},
	}
	got := formatSchedulingReasons(reasons)
	// Single reason: no bullet.
	if strings.Contains(got, "•") {
		t.Errorf("single reason should not have bullet, got %q", got)
	}
	if got != "3 nodes have insufficient CPU" {
		t.Errorf("got %q", got)
	}
}

func TestFormatSchedulingReasons_Multiple(t *testing.T) {
	reasons := []SchedulingReason{
		{Count: 40, Kind: "affinity", Summary: "40 nodes rejected by nodeAffinity"},
		{Count: 3, Kind: "taint", Summary: "3 nodes reserved for system addons"},
	}
	got := formatSchedulingReasons(reasons)
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d: %q", len(lines), got)
	}
	if !strings.HasPrefix(lines[0], "• ") {
		t.Errorf("first line should start with bullet, got %q", lines[0])
	}
}

func TestFormatSchedulingReasons_Empty(t *testing.T) {
	if got := formatSchedulingReasons(nil); got != "" {
		t.Errorf("empty reasons should return empty string, got %q", got)
	}
}

func TestNodeWord(t *testing.T) {
	if got := nodeWord(1); got != "1 node" {
		t.Errorf("nodeWord(1) = %q, want %q", got, "1 node")
	}
	if got := nodeWord(5); got != "5 nodes" {
		t.Errorf("nodeWord(5) = %q, want %q", got, "5 nodes")
	}
}

func TestParseSchedulingMessage_TopologySpread(t *testing.T) {
	msg := `0/10 nodes are available: 4 node(s) didn't match pod topology spread constraints, 6 Insufficient cpu.`
	reasons := parseSchedulingMessage(msg)
	if len(reasons) != 2 {
		t.Fatalf("want 2 reasons, got %d: %+v", len(reasons), reasons)
	}
	// Sorted: 6 cpu first, then 4 topology.
	if reasons[0].Count != 6 {
		t.Errorf("first reason Count = %d, want 6", reasons[0].Count)
	}
	if reasons[1].Kind != "affinity" {
		t.Errorf("topology spread Kind = %q, want affinity", reasons[1].Kind)
	}
	if !strings.Contains(reasons[1].Summary, "topology spread") {
		t.Errorf("Summary should mention topology spread, got %q", reasons[1].Summary)
	}
}

// Integration test: PendingClassifier uses parseSchedulingMessage for
// compound messages.
func TestPendingClassifier_CompoundMessage(t *testing.T) {
	c := PendingClassifier{}
	results := ScanResults{
		Pods: []kube.PodIssue{
			{
				PodName: "gpu-trainer",
				Reason:  "Pending",
				Message: `0/10 nodes are available: 4 node(s) had untolerated taint {role: training-gpu}, 6 node(s) didn't match Pod's node affinity/selector. preemption: not helpful.`,
			},
		},
	}
	got := c.Classify(results)
	if len(got) != 1 {
		t.Fatalf("want 1 finding, got %d", len(got))
	}
	msg := got[0].DetailFields["message"]
	if !strings.Contains(msg, "•") {
		t.Errorf("compound message should use bullet format, got %q", msg)
	}
	if !strings.Contains(msg, "GPU pool") {
		t.Errorf("message should mention GPU pool, got %q", msg)
	}
	if !strings.Contains(msg, "nodeAffinity") {
		t.Errorf("message should mention nodeAffinity, got %q", msg)
	}
	// Preemption should be stripped.
	if strings.Contains(msg, "preemption") {
		t.Errorf("message should not contain preemption section, got %q", msg)
	}
}
