package diagnosis

import (
	"fmt"
	"sort"
	"strings"
)

// NoEndpointsClassifier finds services that have a selector but no ready endpoints.
type NoEndpointsClassifier struct{}

func (NoEndpointsClassifier) Classify(results ScanResults) []Finding {
	var findings []Finding
	for _, s := range results.Services {
		// Build a sorted "key=val,key=val" selector string.
		var parts []string
		for k, v := range s.Selector {
			parts = append(parts, fmt.Sprintf("%s=%s", k, v))
		}
		sort.Strings(parts)
		selectorStr := strings.Join(parts, ",")

		oneLiner := fmt.Sprintf("Service %s has no ready endpoints", s.ServiceName)

		findings = append(findings, Finding{
			Category:   CategoryNoEndpoints,
			Severity:   SeverityWarning,
			EnvName:    results.EnvName,
			ClusterCtx: results.ClusterCtx,
			Namespace:  s.Namespace,
			OneLiner:   oneLiner,
			DetailFields: map[string]string{
				"selector": selectorStr,
			},
		})
	}
	return findings
}
