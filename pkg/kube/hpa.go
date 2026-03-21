package kube

import (
	"context"
	"fmt"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// HPAIssue describes an HPA that is hitting its scaling ceiling or has a
// metric significantly above target.
type HPAIssue struct {
	Namespace         string
	HPAName           string
	TargetRef         string // e.g., "api-gateway"
	TargetKind        string // e.g., "Deployment"
	MinReplicas       int32
	MaxReplicas       int32
	CurrentReplicas   int32
	DesiredReplicas   int32
	AtCeiling         bool  // currentReplicas == maxReplicas
	ScalingLimited    bool  // ScalingLimited condition is True (HPA wants more but can't)
	CurrentCPUPercent int32 // 0 if no CPU metric
	TargetCPUPercent  int32 // 0 if no CPU metric
}

// ListUnhealthyHPAs returns HPAs that are at their maximum replica count or
// are otherwise scaling-limited.
func ListUnhealthyHPAs(ctx context.Context, cs kubernetes.Interface, namespace string) ([]HPAIssue, error) {
	hpas, err := cs.AutoscalingV2().HorizontalPodAutoscalers(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing HPAs in %q: %w", namespace, err)
	}

	var issues []HPAIssue
	for _, hpa := range hpas.Items {
		atCeiling := hpa.Status.CurrentReplicas >= hpa.Spec.MaxReplicas
		scalingLimited := hasScalingLimitedCondition(hpa.Status.Conditions)
		desiredGtMax := hpa.Status.DesiredReplicas > hpa.Spec.MaxReplicas

		if !atCeiling && !scalingLimited && !desiredGtMax {
			continue
		}

		minReplicas := int32(1)
		if hpa.Spec.MinReplicas != nil {
			minReplicas = *hpa.Spec.MinReplicas
		}

		issue := HPAIssue{
			Namespace:       hpa.Namespace,
			HPAName:         hpa.Name,
			TargetRef:       hpa.Spec.ScaleTargetRef.Name,
			TargetKind:      hpa.Spec.ScaleTargetRef.Kind,
			MinReplicas:     minReplicas,
			MaxReplicas:     hpa.Spec.MaxReplicas,
			CurrentReplicas: hpa.Status.CurrentReplicas,
			DesiredReplicas: hpa.Status.DesiredReplicas,
			AtCeiling:       atCeiling,
			ScalingLimited:  scalingLimited,
		}

		// Extract CPU utilization if present.
		issue.CurrentCPUPercent, issue.TargetCPUPercent = extractCPUMetrics(hpa)

		issues = append(issues, issue)
	}
	return issues, nil
}

// hasScalingLimitedCondition returns true if the HPA has a ScalingLimited
// condition set to True.
func hasScalingLimitedCondition(conditions []autoscalingv2.HorizontalPodAutoscalerCondition) bool {
	for _, c := range conditions {
		if c.Type == autoscalingv2.ScalingLimited && c.Status == "True" {
			return true
		}
	}
	return false
}

// extractCPUMetrics returns the current and target CPU utilisation percentages
// from the first Resource metric targeting CPU, or (0, 0) if none is present.
func extractCPUMetrics(hpa autoscalingv2.HorizontalPodAutoscaler) (current, target int32) {
	// Target from spec
	for _, m := range hpa.Spec.Metrics {
		if m.Type == autoscalingv2.ResourceMetricSourceType &&
			m.Resource != nil &&
			m.Resource.Name == "cpu" &&
			m.Resource.Target.AverageUtilization != nil {
			target = *m.Resource.Target.AverageUtilization
			break
		}
	}
	// Current from status
	for _, m := range hpa.Status.CurrentMetrics {
		if m.Type == autoscalingv2.ResourceMetricSourceType &&
			m.Resource != nil &&
			m.Resource.Current.AverageUtilization != nil {
			current = *m.Resource.Current.AverageUtilization
			break
		}
	}
	return current, target
}
