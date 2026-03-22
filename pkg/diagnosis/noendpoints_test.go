package diagnosis

import (
	"testing"

	"github.com/vishukamble/klarity/pkg/kube"
)

func TestNoEndpointsClassifier(t *testing.T) {
	tests := []struct {
		name        string
		results     ScanResults
		wantLen     int
		wantPodName string
	}{
		{
			name:    "empty input",
			results: ScanResults{},
			wantLen: 0,
		},
		{
			name: "single service with no endpoints",
			results: ScanResults{
				EnvName:    "prod",
				ClusterCtx: "prod-us-east",
				Services: []kube.ServiceIssue{
					{
						Namespace:   "default",
						ServiceName: "api-svc",
						Selector:    map[string]string{"app": "api", "tier": "backend"},
					},
				},
			},
			wantLen:     1,
			wantPodName: "api-svc",
		},
		{
			name: "multiple services with no endpoints",
			results: ScanResults{
				Services: []kube.ServiceIssue{
					{Namespace: "ns1", ServiceName: "svc-a", Selector: map[string]string{"app": "a"}},
					{Namespace: "ns2", ServiceName: "svc-b", Selector: map[string]string{"app": "b"}},
				},
			},
			wantLen: 2,
		},
	}

	c := NoEndpointsClassifier{}
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
			if f.Category != CategoryNoEndpoints {
				t.Errorf("category = %q, want %q", f.Category, CategoryNoEndpoints)
			}
			if f.Severity != SeverityWarning {
				t.Errorf("severity = %q, want %q", f.Severity, SeverityWarning)
			}
			if f.DetailFields["selector"] == "" {
				t.Error("DetailFields missing selector")
			}
			if tt.wantPodName != "" && f.PodName != tt.wantPodName {
				t.Errorf("PodName = %q, want %q", f.PodName, tt.wantPodName)
			}
		})
	}
}
