package diagnosis

import (
	"fmt"
	"strings"
)

// NodeClassifier converts unhealthy node conditions into findings.
type NodeClassifier struct{}

func (NodeClassifier) Classify(results ScanResults) []Finding {
	var findings []Finding
	for _, n := range results.Nodes {
		why := classifyNodeCondition(n.Condition, n.Message)

		findings = append(findings, Finding{
			Category: CategoryNodeIssue,
			Severity: SeverityCritical,
			EnvName:  results.EnvName,
			ClusterCtx: results.ClusterCtx,
			OneLiner: why,
			DetailFields: map[string]string{
				"node_name": n.Name,
				"condition": n.Condition,
			},
		})
	}
	return findings
}

func classifyNodeCondition(condition, message string) string {
	lower := strings.ToLower(message)

	switch condition {
	case "NotReady":
		if strings.Contains(lower, "kubelet stopped") {
			return "kubelet not posting status — node may be down or unreachable"
		}
		if strings.Contains(lower, "container runtime") {
			return "container runtime unresponsive — check containerd/docker on node"
		}
		return fmt.Sprintf("Node not ready — check: kubectl describe node <name>")
	case "MemoryPressure":
		return "Memory pressure — evictions likely, check node memory usage"
	case "DiskPressure":
		return "Disk pressure — pods may be evicted, clean logs or expand disk"
	case "PIDPressure":
		return "PID pressure — too many processes, check for runaway containers"
	case "NetworkUnavailable":
		return "Network unavailable — CNI plugin may be down on this node"
	default:
		return fmt.Sprintf("Node condition %s — check node status", condition)
	}
}
