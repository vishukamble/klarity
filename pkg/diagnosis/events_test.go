package diagnosis

import (
	"testing"
	"time"

	"github.com/vishukamble/klarity/pkg/kube"
)

func TestEventClassifier(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		results ScanResults
		wantLen int
	}{
		{
			name:    "empty input",
			results: ScanResults{},
			wantLen: 0,
		},
		{
			name: "single warning event",
			results: ScanResults{
				EnvName:    "prod",
				ClusterCtx: "prod-1",
				Events: []kube.EventIssue{
					{
						Namespace:    "default",
						ObjectName:   "web-abc",
						ObjectKind:   "Pod",
						Reason:       "FailedScheduling",
						Message:      "0/3 nodes are available: insufficient memory",
						Count:        5,
						LastTimestamp: now,
					},
				},
			},
			wantLen: 1,
		},
		{
			name: "long message is truncated",
			results: ScanResults{
				Events: []kube.EventIssue{
					{
						Namespace:    "default",
						ObjectName:   "pod-x",
						ObjectKind:   "Pod",
						Reason:       "BackOff",
						Message:      "Back-off restarting failed container — this is a very long message that goes on and on to exceed the maximum display length for the one-liner summary field in the findings output",
						Count:        1,
						LastTimestamp: now,
					},
				},
			},
			wantLen: 1,
		},
		{
			name: "multiple events",
			results: ScanResults{
				Events: []kube.EventIssue{
					{ObjectName: "pod-a", Reason: "BackOff", Message: "restart", Count: 1, LastTimestamp: now},
					{ObjectName: "pod-b", Reason: "Unhealthy", Message: "readiness probe failed", Count: 3, LastTimestamp: now},
				},
			},
			wantLen: 2,
		},
	}

	c := EventClassifier{}
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
			if f.Category != CategoryWarningEvent {
				t.Errorf("category = %q, want %q", f.Category, CategoryWarningEvent)
			}
			if f.Severity != SeverityInfo {
				t.Errorf("severity = %q, want %q", f.Severity, SeverityInfo)
			}
			if f.DetailFields["reason"] == "" {
				t.Error("DetailFields missing reason")
			}
			if f.DetailFields["object_name"] == "" {
				t.Error("DetailFields missing object_name")
			}
			if f.DetailFields["count"] == "" {
				t.Error("DetailFields missing count")
			}
			// One-liner should not exceed ~120 chars.
			if len(f.OneLiner) > 130 {
				t.Errorf("OneLiner too long (%d chars): %q", len(f.OneLiner), f.OneLiner)
			}
		})
	}
}
