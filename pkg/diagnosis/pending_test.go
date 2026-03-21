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
