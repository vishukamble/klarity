package diagnosis

import (
	"testing"

	"github.com/vishukamble/klarity/pkg/kube"
)

func TestQuotaClassifier(t *testing.T) {
	tests := []struct {
		name    string
		results ScanResults
		wantLen int
		wantSev Severity
	}{
		{
			name:    "empty input",
			results: ScanResults{},
			wantLen: 0,
		},
		{
			name: "quota at 85% is warning",
			results: ScanResults{
				EnvName:    "staging",
				ClusterCtx: "staging-1",
				Quotas: []kube.QuotaIssue{
					{
						Namespace:   "default",
						QuotaName:   "compute",
						Resource:    "cpu",
						Used:        "850m",
						Hard:        "1",
						UsedPercent: 85.0,
					},
				},
			},
			wantLen: 1,
			wantSev: SeverityWarning,
		},
		{
			name: "quota at 97% is critical",
			results: ScanResults{
				Quotas: []kube.QuotaIssue{
					{
						Namespace:   "default",
						QuotaName:   "compute",
						Resource:    "memory",
						Used:        "970Mi",
						Hard:        "1Gi",
						UsedPercent: 97.0,
					},
				},
			},
			wantLen: 1,
			wantSev: SeverityCritical,
		},
		{
			name: "multiple quotas mixed severities",
			results: ScanResults{
				Quotas: []kube.QuotaIssue{
					{QuotaName: "q1", Resource: "cpu", UsedPercent: 82.0, Used: "820m", Hard: "1"},
					{QuotaName: "q2", Resource: "pods", UsedPercent: 96.0, Used: "48", Hard: "50"},
				},
			},
			wantLen: 2,
		},
	}

	c := QuotaClassifier{}
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
			if f.Category != CategoryQuotaExhausted {
				t.Errorf("category = %q, want %q", f.Category, CategoryQuotaExhausted)
			}
			if tt.wantSev != "" && f.Severity != tt.wantSev {
				t.Errorf("severity = %q, want %q", f.Severity, tt.wantSev)
			}
			if f.DetailFields["quota_name"] == "" {
				t.Error("DetailFields missing quota_name")
			}
			if f.DetailFields["resource"] == "" {
				t.Error("DetailFields missing resource")
			}
		})
	}
}
