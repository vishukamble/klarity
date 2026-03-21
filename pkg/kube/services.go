package kube

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ServiceIssue describes a service whose selector matches no ready endpoints.
type ServiceIssue struct {
	Namespace   string
	ServiceName string
	Selector    map[string]string
}

// ListServicesWithNoEndpoints returns services in namespace that have a
// non-empty selector but no ready endpoint addresses.
func ListServicesWithNoEndpoints(ctx context.Context, cs kubernetes.Interface, namespace string) ([]ServiceIssue, error) {
	svcs, err := cs.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing services in %q: %w", namespace, err)
	}

	var issues []ServiceIssue
	for _, svc := range svcs.Items {
		// Headless services and ExternalName services have no selector or
		// don't route to pods — skip them.
		if len(svc.Spec.Selector) == 0 {
			continue
		}

		// The Endpoints object has the same name as the Service.
		ep, err := cs.CoreV1().Endpoints(namespace).Get(ctx, svc.Name, metav1.GetOptions{})
		if err != nil {
			// If the Endpoints object doesn't exist at all, that's an issue.
			issues = append(issues, ServiceIssue{
				Namespace:   svc.Namespace,
				ServiceName: svc.Name,
				Selector:    svc.Spec.Selector,
			})
			continue
		}

		// Check if any subset has at least one ready address.
		hasReady := false
		for _, subset := range ep.Subsets {
			if len(subset.Addresses) > 0 {
				hasReady = true
				break
			}
		}
		if !hasReady {
			issues = append(issues, ServiceIssue{
				Namespace:   svc.Namespace,
				ServiceName: svc.Name,
				Selector:    svc.Spec.Selector,
			})
		}
	}
	return issues, nil
}
