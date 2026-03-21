package diagnosis

import (
	"strings"
	"testing"

	"github.com/vishukamble/klarity/pkg/kube"
)

func TestDaemonSetClassifier(t *testing.T) {
	tests := []struct {
		name           string
		results        ScanResults
		wantLen        int
		wantMisschedIn bool // expect "misscheduled" in one-liner
	}{
		{
			name:    "empty input",
			results: ScanResults{},
			wantLen: 0,
		},
		{
			name: "degraded daemonset without misscheduled",
			results: ScanResults{
				EnvName:    "prod",
				ClusterCtx: "prod-1",
				DaemonSets: []kube.DaemonSetIssue{
					{
						Namespace:     "kube-system",
						DaemonSetName: "fluentd",
						Desired:       5,
						Ready:         3,
						Misscheduled:  0,
					},
				},
			},
			wantLen:        1,
			wantMisschedIn: false,
		},
		{
			name: "daemonset with misscheduled pods",
			results: ScanResults{
				DaemonSets: []kube.DaemonSetIssue{
					{
						Namespace:     "monitoring",
						DaemonSetName: "node-exporter",
						Desired:       10,
						Ready:         8,
						Misscheduled:  2,
					},
				},
			},
			wantLen:        1,
			wantMisschedIn: true,
		},
		{
			name: "multiple degraded daemonsets",
			results: ScanResults{
				DaemonSets: []kube.DaemonSetIssue{
					{DaemonSetName: "ds-a", Desired: 3, Ready: 1},
					{DaemonSetName: "ds-b", Desired: 5, Ready: 5, Misscheduled: 1},
				},
			},
			wantLen: 2,
		},
	}

	c := DaemonSetClassifier{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.Classify(tt.results)
			if len(got) != tt.wantLen {
				t.Fatalf("got %d findings, want %d", len(got), tt.wantLen)
			}
			if tt.wantLen == 0 {
				return
			}
			f := got[0]
			if f.Category != CategoryDaemonSetDegraded {
				t.Errorf("category = %q, want %q", f.Category, CategoryDaemonSetDegraded)
			}
			if f.Severity != SeverityWarning {
				t.Errorf("severity = %q, want %q", f.Severity, SeverityWarning)
			}
			hasMissched := strings.Contains(f.OneLiner, "misscheduled")
			if tt.wantMisschedIn && !hasMissched {
				t.Errorf("OneLiner %q should contain 'misscheduled'", f.OneLiner)
			}
			if !tt.wantMisschedIn && hasMissched {
				t.Errorf("OneLiner %q should not contain 'misscheduled'", f.OneLiner)
			}
			if f.DetailFields["desired"] == "" {
				t.Error("DetailFields missing desired")
			}
			if f.DetailFields["ready"] == "" {
				t.Error("DetailFields missing ready")
			}
		})
	}
}
