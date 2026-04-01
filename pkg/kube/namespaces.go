package kube

import (
	"context"
	"fmt"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/vishukamble/klarity/pkg/config"
)

// ResolveNamespaces returns the concrete list of namespace names to scan for
// the given cluster, honoring the NamespaceFilter mode:
//
//   - include — return filter.Include verbatim (no API call)
//   - all     — list all namespaces via API, subtract filter.Exclude;
//     if filter.Exclude is empty, subtract defaultExclude instead
//   - exclude — list all namespaces via API, subtract filter.Exclude
//
// defaultExclude is applied only when mode == "all" and the cluster has no
// explicit exclude list (filter.Exclude is empty). Pass nil to disable.
func ResolveNamespaces(ctx context.Context, cs kubernetes.Interface, filter config.NamespaceFilter, defaultExclude []string) ([]string, error) {
	switch filter.Mode {
	case config.NamespaceModeInclude:
		return filter.Include, nil

	case config.NamespaceModeAll, config.NamespaceModeExclude:
		nsList, err := cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("listing namespaces: %w", err)
		}

		// For mode=all with no explicit exclude, apply defaultExclude from settings.
		excludeList := filter.Exclude
		if filter.Mode == config.NamespaceModeAll && len(excludeList) == 0 {
			excludeList = defaultExclude
		}

		exclude := make(map[string]bool, len(excludeList))
		for _, ns := range excludeList {
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

// MatchNamespaces filters candidates by a set of glob patterns using
// filepath.Match semantics (* matches any sequence, ? matches one character).
// Exact names (no wildcard characters) are treated as literal matches.
// Results are deduplicated and ordered by candidate position, not pattern order.
// A pattern that matches no candidate is silently ignored (no error).
// Returns an error only if a pattern is syntactically invalid.
// If patterns is empty, candidates is returned unchanged (passthrough).
func MatchNamespaces(patterns []string, candidates []string) ([]string, error) {
	if len(patterns) == 0 {
		return candidates, nil
	}

	seen := make(map[string]bool, len(candidates))
	var result []string

	for _, candidate := range candidates {
		for _, pattern := range patterns {
			matched, err := filepath.Match(pattern, candidate)
			if err != nil {
				return nil, fmt.Errorf("invalid namespace pattern %q: %w", pattern, err)
			}
			if matched && !seen[candidate] {
				seen[candidate] = true
				result = append(result, candidate)
				break
			}
		}
	}
	return result, nil
}
