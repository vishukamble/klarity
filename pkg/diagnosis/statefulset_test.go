package diagnosis

import (
	"testing"

	"github.com/vishukamble/klarity/pkg/kube"
)

func TestStatefulSetClassifier(t *testing.T) {
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
			name: "single degraded statefulset",
			results: ScanResults{
				EnvName:    "prod",
				ClusterCtx: "prod-1",
				StatefulSets: []kube.StatefulSetIssue{
					{
						Namespace:       "default",
						StatefulSetName: "redis",
						Replicas:        3,
						ReadyReplicas:   1,
					},
				},
			},
			wantLen: 1,
		},
		{
			name: "multiple degraded statefulsets",
			results: ScanResults{
				StatefulSets: []kube.StatefulSetIssue{
					{StatefulSetName: "redis", Replicas: 3, ReadyReplicas: 2},
					{StatefulSetName: "zookeeper", Replicas: 5, ReadyReplicas: 3},
				},
			},
			wantLen: 2,
		},
	}

	c := StatefulSetClassifier{}
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
			if f.Category != CategoryStatefulSetDegraded {
				t.Errorf("category = %q, want %q", f.Category, CategoryStatefulSetDegraded)
			}
			if f.Severity != SeverityWarning {
				t.Errorf("severity = %q, want %q", f.Severity, SeverityWarning)
			}
			if f.DetailFields["replicas"] == "" {
				t.Error("DetailFields missing replicas")
			}
			if f.DetailFields["ready_replicas"] == "" {
				t.Error("DetailFields missing ready_replicas")
			}
		})
	}
}
