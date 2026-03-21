package diagnosis

import (
	"strings"
	"testing"

	"github.com/vishukamble/klarity/pkg/kube"
)

func TestJobClassifier(t *testing.T) {
	tests := []struct {
		name          string
		results       ScanResults
		wantLen       int
		wantCondInMsg bool // expect condition message appended to one-liner
	}{
		{
			name:    "empty input",
			results: ScanResults{},
			wantLen: 0,
		},
		{
			name: "single failed job without conditions",
			results: ScanResults{
				EnvName:    "prod",
				ClusterCtx: "prod-1",
				Jobs: []kube.JobIssue{
					{
						Namespace: "batch",
						JobName:   "etl-run",
						Failed:    2,
					},
				},
			},
			wantLen:       1,
			wantCondInMsg: false,
		},
		{
			name: "failed job with condition message",
			results: ScanResults{
				Jobs: []kube.JobIssue{
					{
						Namespace:  "batch",
						JobName:    "report-gen",
						Failed:     1,
						Conditions: []string{"BackoffLimitExceeded"},
					},
				},
			},
			wantLen:       1,
			wantCondInMsg: true,
		},
		{
			name: "multiple failed jobs",
			results: ScanResults{
				Jobs: []kube.JobIssue{
					{JobName: "job-a", Failed: 1},
					{JobName: "job-b", Failed: 3, Conditions: []string{"DeadlineExceeded"}},
				},
			},
			wantLen: 2,
		},
	}

	c := JobClassifier{}
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
			if f.Category != CategoryJobFailed {
				t.Errorf("category = %q, want %q", f.Category, CategoryJobFailed)
			}
			if f.Severity != SeverityWarning {
				t.Errorf("severity = %q, want %q", f.Severity, SeverityWarning)
			}
			if tt.wantCondInMsg {
				if !strings.Contains(f.OneLiner, ":") {
					t.Errorf("OneLiner %q should contain condition message", f.OneLiner)
				}
			}
			if f.DetailFields["failed"] == "" {
				t.Error("DetailFields missing failed")
			}
			if f.DetailFields["conditions"] == "" {
				t.Error("DetailFields missing conditions")
			}
		})
	}
}
