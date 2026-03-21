package kube

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// DaemonSetIssue describes a DaemonSet that has fewer ready pods than desired,
// or has misscheduled pods.
type DaemonSetIssue struct {
	Namespace        string
	DaemonSetName    string
	Desired          int32
	Ready            int32
	Misscheduled     int32
}

// ListUnhealthyDaemonSets returns DaemonSets in namespace whose ready count
// does not match desiredNumberScheduled, or that have misscheduled pods.
func ListUnhealthyDaemonSets(ctx context.Context, cs kubernetes.Interface, namespace string) ([]DaemonSetIssue, error) {
	dsList, err := cs.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing daemonsets in %q: %w", namespace, err)
	}

	var issues []DaemonSetIssue
	for _, ds := range dsList.Items {
		desired := ds.Status.DesiredNumberScheduled
		ready := ds.Status.NumberReady
		misscheduled := ds.Status.NumberMisscheduled

		if ready < desired || misscheduled > 0 {
			issues = append(issues, DaemonSetIssue{
				Namespace:     ds.Namespace,
				DaemonSetName: ds.Name,
				Desired:       desired,
				Ready:         ready,
				Misscheduled:  misscheduled,
			})
		}
	}
	return issues, nil
}
