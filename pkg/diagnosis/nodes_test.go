package diagnosis

import (
	"strings"
	"testing"

	"github.com/vishukamble/klarity/pkg/kube"
)

func TestClassifyNodeCondition(t *testing.T) {
	tests := []struct {
		name      string
		condition string
		message   string
		wantPart  string
	}{
		{"NotReady kubelet stopped", "NotReady", "kubelet stopped posting node status", "kubelet not posting status"},
		{"NotReady container runtime", "NotReady", "container runtime not ready", "container runtime unresponsive"},
		{"NotReady generic", "NotReady", "some other reason", "Node not ready"},
		{"MemoryPressure", "MemoryPressure", "low memory", "Memory pressure"},
		{"DiskPressure", "DiskPressure", "disk full", "Disk pressure"},
		{"PIDPressure", "PIDPressure", "too many pids", "PID pressure"},
		{"NetworkUnavailable", "NetworkUnavailable", "cni down", "Network unavailable"},
		{"unknown condition", "SomethingElse", "weird", "Node condition SomethingElse"},
		{"empty message", "NotReady", "", "Node not ready"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyNodeCondition(tt.condition, tt.message)
			if !strings.Contains(got, tt.wantPart) {
				t.Errorf("classifyNodeCondition(%q, %q) = %q, want substring %q", tt.condition, tt.message, got, tt.wantPart)
			}
		})
	}
}

func TestNodeClassifier(t *testing.T) {
	c := NodeClassifier{}

	t.Run("empty input", func(t *testing.T) {
		got := c.Classify(ScanResults{})
		if len(got) != 0 {
			t.Errorf("want 0 findings, got %d", len(got))
		}
	})

	t.Run("unhealthy nodes", func(t *testing.T) {
		results := ScanResults{
			EnvName:    "prod",
			ClusterCtx: "prod-1",
			Nodes: []kube.NodeIssue{
				{Name: "node-1", Condition: "NotReady", Message: "kubelet stopped posting node status"},
				{Name: "node-2", Condition: "DiskPressure", Message: "disk full"},
			},
		}
		got := c.Classify(results)
		if len(got) != 2 {
			t.Fatalf("want 2 findings, got %d", len(got))
		}
		if got[0].Category != CategoryNodeIssue {
			t.Errorf("category = %q, want %q", got[0].Category, CategoryNodeIssue)
		}
		if got[0].Severity != SeverityCritical {
			t.Errorf("severity = %q, want %q", got[0].Severity, SeverityCritical)
		}
		if got[0].DetailFields["node_name"] != "node-1" {
			t.Errorf("node_name = %q, want node-1", got[0].DetailFields["node_name"])
		}
	})
}
