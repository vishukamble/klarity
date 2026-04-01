package diagnosis

import (
	"strings"
	"testing"

	"github.com/vishukamble/klarity/pkg/kube"
)

func TestHPAClassifier(t *testing.T) {
	tests := []struct {
		name     string
		results  ScanResults
		wantLen  int
		wantSev  Severity
		checkOne func(string) bool
	}{
		{
			name:    "empty input",
			results: ScanResults{},
			wantLen: 0,
		},
		{
			name: "healthy HPA ignored",
			results: ScanResults{
				HPAs: []kube.HPAIssue{
					{
						HPAName:         "web-hpa",
						MinReplicas:     2,
						MaxReplicas:     10,
						CurrentReplicas: 5,
						DesiredReplicas: 5,
						AtCeiling:       false,
						ScalingLimited:  false,
					},
				},
			},
			wantLen: 0,
		},
		{
			name: "at ceiling with cpu overshoot — critical",
			results: ScanResults{
				EnvName:    "prod",
				ClusterCtx: "prod-us",
				HPAs: []kube.HPAIssue{
					{
						Namespace:         "default",
						HPAName:           "api-hpa",
						TargetRef:         "api",
						TargetKind:        "Deployment",
						MinReplicas:       2,
						MaxReplicas:       10,
						CurrentReplicas:   10,
						DesiredReplicas:   12,
						AtCeiling:         true,
						ScalingLimited:    true,
						CurrentCPUPercent: 95,
						TargetCPUPercent:  70,
					},
				},
			},
			wantLen: 1,
			wantSev: SeverityCritical,
			checkOne: func(s string) bool {
				return strings.Contains(s, "95%") && strings.Contains(s, "70%")
			},
		},
		{
			name: "at ceiling without cpu — critical",
			results: ScanResults{
				HPAs: []kube.HPAIssue{
					{
						HPAName:         "worker-hpa",
						MaxReplicas:     5,
						CurrentReplicas: 5,
						DesiredReplicas: 5,
						AtCeiling:       true,
					},
				},
			},
			wantLen: 1,
			wantSev: SeverityCritical,
			checkOne: func(s string) bool {
				return strings.Contains(s, "max replicas")
			},
		},
		{
			name: "scaling limited not at ceiling — warning",
			results: ScanResults{
				HPAs: []kube.HPAIssue{
					{
						HPAName:         "batch-hpa",
						MaxReplicas:     20,
						CurrentReplicas: 8,
						DesiredReplicas: 15,
						AtCeiling:       false,
						ScalingLimited:  true,
					},
				},
			},
			wantLen: 1,
			wantSev: SeverityWarning,
			checkOne: func(s string) bool {
				return strings.Contains(s, "scaling limited")
			},
		},
		{
			name: "detail fields populated",
			results: ScanResults{
				HPAs: []kube.HPAIssue{
					{
						HPAName:         "svc-hpa",
						TargetRef:       "svc",
						TargetKind:      "Deployment",
						MinReplicas:     1,
						MaxReplicas:     8,
						CurrentReplicas: 8,
						DesiredReplicas: 8,
						AtCeiling:       true,
					},
				},
			},
			wantLen: 1,
		},
		{
			name: "multiple HPAs mixed",
			results: ScanResults{
				HPAs: []kube.HPAIssue{
					{HPAName: "healthy", AtCeiling: false, ScalingLimited: false},
					{HPAName: "bad-a", AtCeiling: true},
					{HPAName: "bad-b", ScalingLimited: true},
				},
			},
			wantLen: 2,
		},
	}

	// ── Multiplier OneLiner tests ────────────────────────────────────────────

	t.Run("TestHPAClassifier_HighOvershoot", func(t *testing.T) {
		results := ScanResults{
			HPAs: []kube.HPAIssue{
				{
					HPAName:           "api-hpa",
					MaxReplicas:       10,
					CurrentReplicas:   10,
					DesiredReplicas:   20,
					AtCeiling:         true,
					CurrentCPUPercent: 500,
					TargetCPUPercent:  80,
				},
			},
		}
		findings := HPAClassifier{}.Classify(results)
		if len(findings) != 1 {
			t.Fatalf("want 1 finding, got %d", len(findings))
		}
		ol := findings[0].OneLiner
		// 500/80 = 6.25 → "6.2×" (Go uses banker's rounding for %.1f)
		if !strings.Contains(ol, "6.2×") {
			t.Errorf("OneLiner %q should contain multiplier 6.2×", ol)
		}
		if !strings.Contains(ol, "500%") {
			t.Errorf("OneLiner %q should contain 500%%", ol)
		}
	})

	t.Run("TestHPAClassifier_ModerateOvershoot", func(t *testing.T) {
		results := ScanResults{
			HPAs: []kube.HPAIssue{
				{
					HPAName:           "web-hpa",
					MaxReplicas:       10,
					CurrentReplicas:   10,
					DesiredReplicas:   12,
					AtCeiling:         true,
					CurrentCPUPercent: 89,
					TargetCPUPercent:  80,
				},
			},
		}
		findings := HPAClassifier{}.Classify(results)
		if len(findings) != 1 {
			t.Fatalf("want 1 finding, got %d", len(findings))
		}
		ol := findings[0].OneLiner
		if strings.Contains(ol, "×") {
			t.Errorf("OneLiner %q should NOT contain multiplier (89/80 < 2×)", ol)
		}
		if !strings.Contains(ol, "89%") {
			t.Errorf("OneLiner %q should contain 89%%", ol)
		}
	})

	c := HPAClassifier{}
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
			if f.Category != CategoryHPACeiling {
				t.Errorf("category = %q, want %q", f.Category, CategoryHPACeiling)
			}
			if tt.wantSev != "" && f.Severity != tt.wantSev {
				t.Errorf("severity = %q, want %q", f.Severity, tt.wantSev)
			}
			if tt.checkOne != nil && !tt.checkOne(f.OneLiner) {
				t.Errorf("one-liner %q failed check", f.OneLiner)
			}
			if f.DetailFields["max_replicas"] == "" {
				t.Error("DetailFields missing max_replicas")
			}
			if f.DetailFields["current_replicas"] == "" {
				t.Error("DetailFields missing current_replicas")
			}
		})
	}
}
