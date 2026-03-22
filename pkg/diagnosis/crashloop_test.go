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

	// Probe-killed detection tests.
	probeTests := []struct {
		name       string
		logSummary string
		hasProbe   bool
		wantProbe  bool // expect probe-killed message
	}{
		{
			name:       "nginx exit notice with probe",
			logSummary: "2026/03/22 15:58:10 [notice] 1#1: exit",
			hasProbe:   true,
			wantProbe:  true,
		},
		{
			name:       "nginx exit notice without probe",
			logSummary: "2026/03/22 15:58:10 [notice] 1#1: exit",
			hasProbe:   false,
			wantProbe:  false,
		},
		{
			name:       "SIGTERM with probe",
			logSummary: "received signal 15, shutting down",
			hasProbe:   true,
			wantProbe:  true,
		},
		{
			name:       "SIGTERM keyword with probe",
			logSummary: "caught SIGTERM, cleaning up",
			hasProbe:   true,
			wantProbe:  true,
		},
		{
			name:       "graceful shutdown with probe",
			logSummary: "graceful shutdown complete",
			hasProbe:   true,
			wantProbe:  true,
		},
		{
			name:       "graceful stop with probe",
			logSummary: "nginx graceful stop",
			hasProbe:   true,
			wantProbe:  true,
		},
		{
			name:       "server shutting down with probe",
			logSummary: "server shutting down",
			hasProbe:   true,
			wantProbe:  true,
		},
		{
			name:       "actual error is not probe-killed",
			logSummary: "panic: nil pointer dereference",
			hasProbe:   true,
			wantProbe:  false,
		},
		{
			name:       "empty log summary",
			logSummary: "",
			hasProbe:   true,
			wantProbe:  false,
		},
	}
	for _, tt := range probeTests {
		tests = append(tests, struct {
			name          string
			results       ScanResults
			wantLen       int
			wantOneLiner  string
			checkOneLiner func(string) bool
		}{
			name: tt.name,
			results: ScanResults{
				Pods: []kube.PodIssue{
					{
						PodName:          "probe-pod",
						ContainerName:    "app",
						Reason:           "CrashLoopBackOff",
						RestartCount:     5,
						LogSummary:       tt.logSummary,
						HasLivenessProbe: tt.hasProbe,
					},
				},
			},
			wantLen: 1,
			checkOneLiner: func(s string) bool {
				hasProbeMsg := strings.Contains(s, "liveness probe")
				return hasProbeMsg == tt.wantProbe
			},
		})
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
		})
	}
}

func TestLooksLikeCleanExit(t *testing.T) {
	tests := []struct {
		summary string
		want    bool
	}{
		{"2026/03/22 15:58:10 [notice] 1#1: exit", true},
		{"received signal 15, shutting down", true},
		{"caught SIGTERM, cleaning up", true},
		{"graceful shutdown complete", true},
		{"nginx graceful stop", true},
		{"server shutting down", true},
		{"panic: nil pointer dereference", false},
		{"Error: connection refused", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.summary, func(t *testing.T) {
			got := looksLikeCleanExit(tt.summary)
			if got != tt.want {
				t.Errorf("looksLikeCleanExit(%q) = %v, want %v", tt.summary, got, tt.want)
			}
		})
	}
}
