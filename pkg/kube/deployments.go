package kube

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// DeploymentIssue describes an unhealthy deployment.
type DeploymentIssue struct {
	Namespace           string
	DeploymentName      string
	DesiredReplicas     int32
	ReadyReplicas       int32
	UnavailableReplicas int32
}

// ListUnhealthyDeployments returns deployments in namespace whose ready
// replicas fall below their desired count.
func ListUnhealthyDeployments(ctx context.Context, cs kubernetes.Interface, namespace string) ([]DeploymentIssue, error) {
	deps, err := cs.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing deployments in %q: %w", namespace, err)
	}

	var issues []DeploymentIssue
	for _, dep := range deps.Items {
		desired := int32(1)
		if dep.Spec.Replicas != nil {
			desired = *dep.Spec.Replicas
		}
		if dep.Status.UnavailableReplicas > 0 || dep.Status.ReadyReplicas < desired {
			issues = append(issues, DeploymentIssue{
				Namespace:           dep.Namespace,
				DeploymentName:      dep.Name,
				DesiredReplicas:     desired,
				ReadyReplicas:       dep.Status.ReadyReplicas,
				UnavailableReplicas: dep.Status.UnavailableReplicas,
			})
		}
	}
	return issues, nil
}
