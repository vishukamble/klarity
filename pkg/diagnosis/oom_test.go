package diagnosis

import (
	"testing"

	"github.com/vishukamble/klarity/pkg/kube"
)

func TestOOMClassifier(t *testing.T) {
	tests := []struct {
		name     string
		results  ScanResults
		wantLen  int
		wantSev  Severity
		wantPod  string
	}{
		{
			name:    "empty input",
			results: ScanResults{},
			wantLen: 0,
		},
		{
			name: "no OOMKilled pods",
			results: ScanResults{
				Pods: []kube.PodIssue{
					{PodName: "web-abc", ContainerName: "web", Reason: "CrashLoopBackOff"},
				},
			},
			wantLen: 0,
		},
		{
			name: "single OOMKilled container",
			results: ScanResults{
				EnvName:    "prod",
				ClusterCtx: "prod-us-east",
				Pods: []kube.PodIssue{
					{
						Namespace:     "default",
						PodName:       "worker-abc",
						ContainerName: "worker",
						Reason:        "OOMKilled",
						Image:         "worker:v1.2",
						RestartCount:  5,
					},
				},
			},
			wantLen: 1,
			wantSev: SeverityCritical,
			wantPod: "worker-abc",
		},
		{
			name: "multiple OOMKilled containers",
			results: ScanResults{
				Pods: []kube.PodIssue{
					{PodName: "pod-a", ContainerName: "c1", Reason: "OOMKilled"},
					{PodName: "pod-b", ContainerName: "c2", Reason: "CrashLoopBackOff"},
					{PodName: "pod-c", ContainerName: "c3", Reason: "OOMKilled"},
				},
			},
			wantLen: 2,
		},
		{
			name: "OOMKilled finding has correct detail fields",
			results: ScanResults{
				Pods: []kube.PodIssue{
					{
						PodName:       "api-xyz",
						ContainerName: "api",
						Reason:        "OOMKilled",
						Image:         "api:latest",
						RestartCount:  3,
					},
				},
			},
			wantLen: 1,
			wantPod: "api-xyz",
		},
	}

	c := OOMClassifier{}
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
			if f.Category != CategoryOOMKilled {
				t.Errorf("category = %q, want %q", f.Category, CategoryOOMKilled)
			}
			if tt.wantSev != "" && f.Severity != tt.wantSev {
				t.Errorf("severity = %q, want %q", f.Severity, tt.wantSev)
			}
			if tt.wantPod != "" && f.PodName != tt.wantPod {
				t.Errorf("pod = %q, want %q", f.PodName, tt.wantPod)
			}
			if f.DetailFields["restart_count"] == "" {
				t.Error("DetailFields missing restart_count")
			}
			// Only assert image when the test set one.
			for _, p := range tt.results.Pods {
				if p.Reason == "OOMKilled" && p.Image != "" {
					if f.DetailFields["image"] == "" {
						t.Error("DetailFields missing image")
					}
					break
				}
			}
		})
	}
}
