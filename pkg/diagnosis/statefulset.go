package diagnosis

import (
	"fmt"
)

// StatefulSetClassifier finds StatefulSets whose ready replicas are below desired.
type StatefulSetClassifier struct{}

func (StatefulSetClassifier) Classify(results ScanResults) []Finding {
	var findings []Finding
	for _, sts := range results.StatefulSets {
		oneLiner := fmt.Sprintf("StatefulSet %s %d/%d ready", sts.StatefulSetName, sts.ReadyReplicas, sts.Replicas)

		findings = append(findings, Finding{
			Category:   CategoryStatefulSetDegraded,
			Severity:   SeverityWarning,
			EnvName:    results.EnvName,
			ClusterCtx: results.ClusterCtx,
			Namespace:  sts.Namespace,
			OneLiner:   oneLiner,
			DetailFields: map[string]string{
				"replicas":       fmt.Sprintf("%d", sts.Replicas),
				"ready_replicas": fmt.Sprintf("%d", sts.ReadyReplicas),
			},
		})
	}
	return findings
}
