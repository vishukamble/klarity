package kube

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// NodeIssue describes an unhealthy condition on a cluster node.
type NodeIssue struct {
	Name      string
	Condition string // NotReady | MemoryPressure | DiskPressure | PIDPressure | NetworkUnavailable
	Message   string // from condition.Message
	Since     time.Duration
}

// unhealthyNodeConditions maps condition types to the status value that
// indicates a problem. Ready=False/Unknown is unhealthy; all pressure/
// network conditions are unhealthy when True.
var unhealthyNodeConditions = map[corev1.NodeConditionType]corev1.ConditionStatus{
	corev1.NodeMemoryPressure:     corev1.ConditionTrue,
	corev1.NodeDiskPressure:       corev1.ConditionTrue,
	corev1.NodePIDPressure:        corev1.ConditionTrue,
	corev1.NodeNetworkUnavailable: corev1.ConditionTrue,
}

// ListUnhealthyNodes returns nodes that have at least one unhealthy condition.
// This is a cluster-wide call (not namespaced).
func ListUnhealthyNodes(ctx context.Context, cs kubernetes.Interface) ([]NodeIssue, error) {
	nodes, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}

	now := time.Now()
	var issues []NodeIssue
	for _, node := range nodes.Items {
		for _, cond := range node.Status.Conditions {
			if issue := checkNodeCondition(node.Name, cond, now); issue != nil {
				issues = append(issues, *issue)
			}
		}
	}
	return issues, nil
}

func checkNodeCondition(nodeName string, cond corev1.NodeCondition, now time.Time) *NodeIssue {
	// Ready condition: unhealthy when False or Unknown.
	if cond.Type == corev1.NodeReady {
		if cond.Status == corev1.ConditionFalse || cond.Status == corev1.ConditionUnknown {
			return &NodeIssue{
				Name:      nodeName,
				Condition: "NotReady",
				Message:   cond.Message,
				Since:     now.Sub(cond.LastTransitionTime.Time),
			}
		}
		return nil
	}

	// Pressure / network conditions: unhealthy when True.
	if badStatus, ok := unhealthyNodeConditions[cond.Type]; ok && cond.Status == badStatus {
		return &NodeIssue{
			Name:      nodeName,
			Condition: string(cond.Type),
			Message:   cond.Message,
			Since:     now.Sub(cond.LastTransitionTime.Time),
		}
	}
	return nil
}
