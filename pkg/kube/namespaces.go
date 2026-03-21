package kube

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/vishukamble/klarity/pkg/config"
)

// ResolveNamespaces returns the concrete list of namespace names to scan for
// the given cluster, honoring the NamespaceFilter mode:
//
//   - include — return filter.Include verbatim (no API call)
//   - all     — list all namespaces via API, subtract filter.Exclude
//   - exclude — list all namespaces via API, subtract filter.Exclude
func ResolveNamespaces(ctx context.Context, cs kubernetes.Interface, filter config.NamespaceFilter) ([]string, error) {
	switch filter.Mode {
	case config.NamespaceModeInclude:
		return filter.Include, nil

	case config.NamespaceModeAll, config.NamespaceModeExclude:
		nsList, err := cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("listing namespaces: %w", err)
		}
		exclude := make(map[string]bool, len(filter.Exclude))
		for _, ns := range filter.Exclude {
			exclude[ns] = true
		}
		var result []string
		for _, ns := range nsList.Items {
			if !exclude[ns.Name] {
				result = append(result, ns.Name)
			}
		}
		return result, nil

	default:
		return nil, fmt.Errorf("unknown namespace filter mode: %q", filter.Mode)
	}
}
