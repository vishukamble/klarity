package diagnosis

import (
	"fmt"
)

// HPAClassifier finds HPAs that are at their scaling ceiling or have a metric overshoot.
type HPAClassifier struct{}

func (HPAClassifier) Classify(results ScanResults) []Finding {
	var findings []Finding
	for _, h := range results.HPAs {
		if !h.AtCeiling && !h.ScalingLimited {
			continue
		}

		var oneLiner string
		var severity Severity

		switch {
		case h.AtCeiling && h.CurrentCPUPercent > 0 && h.TargetCPUPercent > 0:
			multiply := float64(h.CurrentCPUPercent) / float64(h.TargetCPUPercent)
			if multiply >= 2.0 {
				oneLiner = fmt.Sprintf(
					"%s at max replicas (%d/%d); CPU at %.1f× target (%d%% vs %d%%)",
					h.HPAName, h.CurrentReplicas, h.MaxReplicas,
					multiply, h.CurrentCPUPercent, h.TargetCPUPercent,
				)
			} else {
				oneLiner = fmt.Sprintf(
					"%s at max replicas (%d/%d); CPU %d%% vs target %d%%",
					h.HPAName, h.CurrentReplicas, h.MaxReplicas,
					h.CurrentCPUPercent, h.TargetCPUPercent,
				)
			}
			severity = SeverityCritical
		case h.AtCeiling:
			oneLiner = fmt.Sprintf(
				"%s at max replicas (%d/%d) — cannot scale further",
				h.HPAName, h.CurrentReplicas, h.MaxReplicas,
			)
			severity = SeverityCritical
		default: // ScalingLimited only
			oneLiner = fmt.Sprintf(
				"%s scaling limited (desired %d, current %d, max %d)",
				h.HPAName, h.DesiredReplicas, h.CurrentReplicas, h.MaxReplicas,
			)
			severity = SeverityWarning
		}

		detail := map[string]string{
			"hpa_name":         h.HPAName,
			"min_replicas":     fmt.Sprintf("%d", h.MinReplicas),
			"max_replicas":     fmt.Sprintf("%d", h.MaxReplicas),
			"current_replicas": fmt.Sprintf("%d", h.CurrentReplicas),
			"desired_replicas": fmt.Sprintf("%d", h.DesiredReplicas),
			"target_ref":       fmt.Sprintf("%s/%s", h.TargetKind, h.TargetRef),
		}
		if h.CurrentCPUPercent > 0 {
			detail["cpu_current_percent"] = fmt.Sprintf("%d", h.CurrentCPUPercent)
			detail["cpu_target_percent"] = fmt.Sprintf("%d", h.TargetCPUPercent)
		}
		if h.AtCeiling {
			detail["at_ceiling"] = "true"
		}
		if h.ScalingLimited {
			detail["scaling_limited"] = "true"
		}

		findings = append(findings, Finding{
			Category:   CategoryHPACeiling,
			Severity:   severity,
			EnvName:    results.EnvName,
			ClusterCtx: results.ClusterCtx,
			Namespace:  h.Namespace,
			OneLiner:   oneLiner,
			DetailFields: detail,
		})
	}
	return findings
}
