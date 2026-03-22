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

func TestClassifyJobFailure(t *testing.T) {
	tests := []struct {
		name       string
		jobName    string
		failed     int32
		conditions []string
		wantPart   string // substring expected in result
	}{
		{
			name:       "BackoffLimitExceeded",
			jobName:    "etl-daily",
			failed:     5,
			conditions: []string{"Job has reached the specified backoff limit", "BackoffLimitExceeded"},
			wantPart:   "hit retry limit",
		},
		{
			name:       "DeadlineExceeded",
			jobName:    "report-gen",
			failed:     1,
			conditions: []string{"Job was active longer than specified deadline", "DeadlineExceeded"},
			wantPart:   "timed out",
		},
		{
			name:       "generic failure with condition",
			jobName:    "sync-job",
			failed:     2,
			conditions: []string{"some other failure reason"},
			wantPart:   "2 failed pod(s)",
		},
		{
			name:       "generic failure no condition",
			jobName:    "import-job",
			failed:     1,
			conditions: nil,
			wantPart:   "1 failed pod(s)",
		},
		{
			name:       "BackoffLimitExceeded includes failed count",
			jobName:    "ml-train",
			failed:     3,
			conditions: []string{"BackoffLimitExceeded"},
			wantPart:   "3 failures",
		},
		{
			name:       "DeadlineExceeded includes failed count",
			jobName:    "backup",
			failed:     2,
			conditions: []string{"DeadlineExceeded"},
			wantPart:   "2 failures",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyJobFailure(tt.jobName, tt.failed, tt.conditions)
			if !strings.Contains(got, tt.wantPart) {
				t.Errorf("classifyJobFailure(%q, %d, %v)\n  got:  %q\n  want substring: %q", tt.jobName, tt.failed, tt.conditions, got, tt.wantPart)
			}
		})
	}
}
