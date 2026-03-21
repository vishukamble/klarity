package kube

import (
	"context"
	"testing"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func makeHPA(name, ns string, min, max, current, desired int32) *autoscalingv2.HorizontalPodAutoscaler {
	return &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			MinReplicas: &min,
			MaxReplicas: max,
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind: "Deployment",
				Name: name + "-deploy",
			},
		},
		Status: autoscalingv2.HorizontalPodAutoscalerStatus{
			CurrentReplicas: current,
			DesiredReplicas: desired,
		},
	}
}

func TestListUnhealthyHPAs_Healthy(t *testing.T) {
	// 3 current, max 10 — nowhere near ceiling
	cs := fake.NewSimpleClientset(makeHPA("api", "default", 1, 10, 3, 3))
	issues, err := ListUnhealthyHPAs(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected no issues, got %d", len(issues))
	}
}

func TestListUnhealthyHPAs_AtCeiling(t *testing.T) {
	// current == max == 20
	cs := fake.NewSimpleClientset(makeHPA("gateway", "default", 2, 20, 20, 20))
	issues, err := ListUnhealthyHPAs(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("want 1 issue, got %d", len(issues))
	}
	if !issues[0].AtCeiling {
		t.Error("AtCeiling should be true")
	}
	if issues[0].MaxReplicas != 20 {
		t.Errorf("max: want 20, got %d", issues[0].MaxReplicas)
	}
}

func TestListUnhealthyHPAs_ScalingLimited(t *testing.T) {
	hpa := makeHPA("svc", "default", 1, 10, 8, 10)
	hpa.Status.Conditions = []autoscalingv2.HorizontalPodAutoscalerCondition{
		{
			Type:   autoscalingv2.ScalingLimited,
			Status: "True",
		},
	}
	cs := fake.NewSimpleClientset(hpa)
	issues, err := ListUnhealthyHPAs(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 || !issues[0].ScalingLimited {
		t.Errorf("expected ScalingLimited issue, got %v", issues)
	}
}

func TestListUnhealthyHPAs_CPUMetrics(t *testing.T) {
	cpuTarget := int32(60)
	cpuCurrent := int32(89)
	hpa := makeHPA("api", "default", 1, 20, 20, 20)
	hpa.Spec.Metrics = []autoscalingv2.MetricSpec{
		{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceCPU,
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: &cpuTarget,
				},
			},
		},
	}
	hpa.Status.CurrentMetrics = []autoscalingv2.MetricStatus{
		{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricStatus{
				Name: corev1.ResourceCPU,
				Current: autoscalingv2.MetricValueStatus{
					AverageUtilization: &cpuCurrent,
				},
			},
		},
	}
	cs := fake.NewSimpleClientset(hpa)
	issues, err := ListUnhealthyHPAs(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("want 1 issue, got %d", len(issues))
	}
	if issues[0].CurrentCPUPercent != 89 {
		t.Errorf("CurrentCPUPercent: want 89, got %d", issues[0].CurrentCPUPercent)
	}
	if issues[0].TargetCPUPercent != 60 {
		t.Errorf("TargetCPUPercent: want 60, got %d", issues[0].TargetCPUPercent)
	}
}

func TestListUnhealthyHPAs_DesiredExceedsMax(t *testing.T) {
	// desired > max — HPA wants to scale beyond its max
	hpa := makeHPA("svc", "default", 1, 10, 10, 15)
	cs := fake.NewSimpleClientset(hpa)
	issues, err := ListUnhealthyHPAs(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("want 1 issue, got %d", len(issues))
	}
}
