package kube

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// StatefulSetIssue describes a StatefulSet whose ready replica count falls
// below its desired count.
type StatefulSetIssue struct {
	Namespace       string
	StatefulSetName string
	Replicas        int32
	ReadyReplicas   int32
}

// ListUnhealthyStatefulSets returns StatefulSets in namespace whose
// readyReplicas is less than the desired replicas count.
func ListUnhealthyStatefulSets(ctx context.Context, cs kubernetes.Interface, namespace string) ([]StatefulSetIssue, error) {
	stsList, err := cs.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing statefulsets in %q: %w", namespace, err)
	}

	var issues []StatefulSetIssue
	for _, sts := range stsList.Items {
		desired := int32(1)
		if sts.Spec.Replicas != nil {
			desired = *sts.Spec.Replicas
		}
		if sts.Status.ReadyReplicas < desired {
			issues = append(issues, StatefulSetIssue{
				Namespace:       sts.Namespace,
				StatefulSetName: sts.Name,
				Replicas:        desired,
				ReadyReplicas:   sts.Status.ReadyReplicas,
			})
		}
	}
	return issues, nil
}
