package diagnosis

import (
	"strings"
	"testing"

	"github.com/vishukamble/klarity/pkg/kube"
)

func TestCrashLoopClassifier(t *testing.T) {
	tests := []struct {
		name          string
		results       ScanResults
		wantLen       int
		wantOneLiner  string
		checkOneLiner func(string) bool
	}{
		{
			name:    "empty input",
			results: ScanResults{},
			wantLen: 0,
		},
		{
			name: "non-crashloop pod ignored",
			results: ScanResults{
				Pods: []kube.PodIssue{
					{PodName: "x", Reason: "OOMKilled"},
					{PodName: "y", Reason: "ImagePullBackOff"},
				},
			},
			wantLen: 0,
		},
		{
			name: "crashloop without log summary uses generic message",
			results: ScanResults{
				Pods: []kube.PodIssue{
					{
						PodName:       "worker-abc",
						ContainerName: "worker",
						Reason:        "CrashLoopBackOff",
						Image:         "worker:v1",
						RestartCount:  8,
					},
				},
			},
			wantLen: 1,
			checkOneLiner: func(s string) bool {
				return strings.Contains(s, "crash-looping") && strings.Contains(s, "8")
			},
		},
		{
			name: "crashloop with log summary uses log summary as one-liner",
			results: ScanResults{
				Pods: []kube.PodIssue{
					{
						PodName:       "api-xyz",
						ContainerName: "api",
						Reason:        "CrashLoopBackOff",
						RestartCount:  3,
						LogSummary:    "panic: runtime error: index out of range [1] with length 1",
					},
				},
			},
			wantLen:      1,
			wantOneLiner: "panic: runtime error: index out of range [1] with length 1",
		},
		{
			name: "log summary included in detail fields",
			results: ScanResults{
				Pods: []kube.PodIssue{
					{
						PodName:       "svc",
						ContainerName: "svc",
						Reason:        "CrashLoopBackOff",
						LogSummary:    "Error: could not connect to database",
					},
				},
			},
			wantLen: 1,
		},
		{
			name: "multiple crashloop pods",
			results: ScanResults{
				Pods: []kube.PodIssue{
					{PodName: "a", Reason: "CrashLoopBackOff", ContainerName: "c1"},
					{PodName: "b", Reason: "ImagePullBackOff", ContainerName: "c2"},
					{PodName: "c", Reason: "CrashLoopBackOff", ContainerName: "c3"},
				},
			},
			wantLen: 2,
		},
	}

	cl := CrashLoopClassifier{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cl.Classify(tt.results)
			if len(got) != tt.wantLen {
				t.Fatalf("got %d findings, want %d", len(got), tt.wantLen)
			}
			if tt.wantLen == 0 {
				return
			}
			f := got[0]
			if f.Category != CategoryCrashLoop {
				t.Errorf("category = %q, want %q", f.Category, CategoryCrashLoop)
			}
			if f.Severity != SeverityCritical {
				t.Errorf("severity = %q, want Critical", f.Severity)
			}
			if tt.wantOneLiner != "" && f.OneLiner != tt.wantOneLiner {
				t.Errorf("one-liner = %q, want %q", f.OneLiner, tt.wantOneLiner)
			}
			if tt.checkOneLiner != nil && !tt.checkOneLiner(f.OneLiner) {
				t.Errorf("one-liner %q failed check", f.OneLiner)
			}
			// Log summary in detail when present.
			for _, p := range tt.results.Pods {
				if p.Reason == "CrashLoopBackOff" && p.LogSummary != "" {
					if f.DetailFields["log_summary"] != p.LogSummary {
						t.Errorf("detail log_summary = %q, want %q", f.DetailFields["log_summary"], p.LogSummary)
					}
					break
				}
			}
		})
	}
}
