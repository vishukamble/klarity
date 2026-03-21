package diagnosis

import (
	"testing"

	"github.com/vishukamble/klarity/pkg/kube"
)

func TestPVCClassifier(t *testing.T) {
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
			name: "single pending PVC",
			results: ScanResults{
				EnvName:    "prod",
				ClusterCtx: "prod-1",
				PVCs: []kube.PVCIssue{
					{
						Namespace:    "default",
						PVCName:      "data-vol",
						StorageClass: "gp3",
						Capacity:     "10Gi",
						Phase:        "Pending",
					},
				},
			},
			wantLen: 1,
		},
		{
			name: "PVC with empty storage class and capacity",
			results: ScanResults{
				PVCs: []kube.PVCIssue{
					{
						Namespace: "ns1",
						PVCName:   "orphan-pvc",
					},
				},
			},
			wantLen: 1,
		},
		{
			name: "multiple pending PVCs",
			results: ScanResults{
				PVCs: []kube.PVCIssue{
					{Namespace: "ns1", PVCName: "pvc-a", StorageClass: "ssd", Capacity: "5Gi"},
					{Namespace: "ns2", PVCName: "pvc-b", StorageClass: "hdd", Capacity: "100Gi"},
				},
			},
			wantLen: 2,
		},
	}

	c := PVCClassifier{}
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
			if f.Category != CategoryPVCPending {
				t.Errorf("category = %q, want %q", f.Category, CategoryPVCPending)
			}
			if f.Severity != SeverityWarning {
				t.Errorf("severity = %q, want %q", f.Severity, SeverityWarning)
			}
			if f.DetailFields["storage_class"] == "" {
				t.Error("DetailFields missing storage_class")
			}
			if f.DetailFields["capacity"] == "" {
				t.Error("DetailFields missing capacity")
			}
		})
	}
}
