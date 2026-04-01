package diagnosis

import (
	"testing"

	"github.com/vishukamble/klarity/pkg/kube"
)

func TestImagePullClassifier(t *testing.T) {
	tests := []struct {
		name        string
		results     ScanResults
		wantLen     int
		wantSubtype string
	}{
		{
			name:    "empty input",
			results: ScanResults{},
			wantLen: 0,
		},
		{
			name: "non-imagepull pod ignored",
			results: ScanResults{
				Pods: []kube.PodIssue{
					{PodName: "x", Reason: "OOMKilled"},
					{PodName: "y", Reason: "CrashLoopBackOff"},
				},
			},
			wantLen: 0,
		},
		{
			name: "ImagePullBackOff auth error",
			results: ScanResults{
				Pods: []kube.PodIssue{
					{
						PodName:       "api",
						ContainerName: "api",
						Reason:        "ImagePullBackOff",
						Image:         "private.registry/api:v2",
						Message:       "unauthorized: authentication required",
					},
				},
			},
			wantLen:     1,
			wantSubtype: string(imagePullAuth),
		},
		{
			name: "ErrImagePull tag not found",
			results: ScanResults{
				Pods: []kube.PodIssue{
					{
						PodName:       "web",
						ContainerName: "web",
						Reason:        "ErrImagePull",
						Image:         "nginx:notexist",
						Message:       "manifest unknown: manifest does not exist",
					},
				},
			},
			wantLen:     1,
			wantSubtype: string(imagePullTag),
		},
		{
			name: "ImagePullBackOff registry unreachable",
			results: ScanResults{
				Pods: []kube.PodIssue{
					{
						PodName:       "svc",
						ContainerName: "svc",
						Reason:        "ImagePullBackOff",
						Image:         "registry.internal/svc:latest",
						Message:       "connection refused: dial tcp 10.0.0.1:443",
					},
				},
			},
			wantLen:     1,
			wantSubtype: string(imagePullRegistry),
		},
		{
			// InvalidImageName is intentionally NOT handled by ImagePullClassifier —
			// ContainerErrorClassifier provides a more specific diagnosis. Expect 0.
			name: "InvalidImageName not handled here",
			results: ScanResults{
				Pods: []kube.PodIssue{
					{
						PodName:       "bad",
						ContainerName: "c",
						Reason:        "InvalidImageName",
						Image:         "BAD IMAGE NAME",
						Message:       "",
					},
				},
			},
			wantLen: 0,
		},
		{
			name: "multiple failures mixed",
			results: ScanResults{
				Pods: []kube.PodIssue{
					{PodName: "a", Reason: "ImagePullBackOff", Image: "img:a"},
					{PodName: "b", Reason: "OOMKilled"},
					{PodName: "c", Reason: "ErrImagePull", Image: "img:c"},
				},
			},
			wantLen: 2,
		},
	}

	c := ImagePullClassifier{}
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
			if f.Category != CategoryImagePull {
				t.Errorf("category = %q, want %q", f.Category, CategoryImagePull)
			}
			if f.Severity != SeverityCritical {
				t.Errorf("severity = %q, want Critical", f.Severity)
			}
			if tt.wantSubtype != "" && f.DetailFields["subtype"] != tt.wantSubtype {
				t.Errorf("subtype = %q, want %q", f.DetailFields["subtype"], tt.wantSubtype)
			}
		})
	}
}

func TestClassifyImagePullMessage(t *testing.T) {
	tests := []struct {
		msg  string
		want imagePullSubtype
	}{
		{"unauthorized: authentication required", imagePullAuth},
		{"403 Forbidden", imagePullAuth},
		{"manifest unknown", imagePullTag},
		{"tag does not exist", imagePullTag},
		{"connection refused", imagePullRegistry},
		{"i/o timeout", imagePullRegistry},
		{"no route to host", imagePullRegistry},
		{"", imagePullUnknown},
		{"some other error", imagePullUnknown},
	}
	for _, tt := range tests {
		got := classifyImagePullMessage(tt.msg)
		if got != tt.want {
			t.Errorf("classifyImagePullMessage(%q) = %q, want %q", tt.msg, got, tt.want)
		}
	}
}
