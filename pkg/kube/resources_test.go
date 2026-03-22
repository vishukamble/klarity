package kube

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// ── ResourceQuota helpers ─────────────────────────────────────────────────────

func makeQuota(name, ns string, hard, used corev1.ResourceList) *corev1.ResourceQuota {
	return &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Status: corev1.ResourceQuotaStatus{
			Hard: hard,
			Used: used,
		},
	}
}

func TestListQuotaIssues_BelowThreshold(t *testing.T) {
	rq := makeQuota("compute", "default",
		corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("10")},
		corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")}, // 10%
	)
	cs := fake.NewSimpleClientset(rq)
	issues, err := ListQuotaIssues(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected no issues at 10%%, got %d", len(issues))
	}
}

func TestListQuotaIssues_AtThreshold(t *testing.T) {
	// 8/10 = 80% — exactly at threshold
	rq := makeQuota("compute", "default",
		corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("10")},
		corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("8")},
	)
	cs := fake.NewSimpleClientset(rq)
	issues, err := ListQuotaIssues(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("want 1 issue at 80%%, got %d", len(issues))
	}
	if issues[0].UsedPercent < 79.9 || issues[0].UsedPercent > 80.1 {
		t.Errorf("UsedPercent: want ~80, got %.1f", issues[0].UsedPercent)
	}
}

func TestListQuotaIssues_AtLimit(t *testing.T) {
	// 10/10 = 100%
	rq := makeQuota("compute", "default",
		corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("8Gi")},
		corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("8Gi")},
	)
	cs := fake.NewSimpleClientset(rq)
	issues, err := ListQuotaIssues(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("want 1 issue at 100%%, got %d", len(issues))
	}
}

func TestListQuotaIssues_MultipleResources(t *testing.T) {
	rq := makeQuota("mixed", "default",
		corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("10"),
			corev1.ResourceMemory: resource.MustParse("10Gi"),
		},
		corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),   // 10% — ok
			corev1.ResourceMemory: resource.MustParse("9Gi"), // 90% — issue
		},
	)
	cs := fake.NewSimpleClientset(rq)
	issues, err := ListQuotaIssues(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 || issues[0].Resource != "memory" {
		t.Errorf("expected 1 memory issue, got %v", issues)
	}
}

// ── PVC helpers ───────────────────────────────────────────────────────────────

func makePVC(name, ns, sc string, capacity string, phase corev1.PersistentVolumeClaimPhase) *corev1.PersistentVolumeClaim {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: corev1.PersistentVolumeClaimSpec{
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(capacity),
				},
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{Phase: phase},
	}
	if sc != "" {
		pvc.Spec.StorageClassName = &sc
	}
	return pvc
}

func TestListPendingPVCs_Pending(t *testing.T) {
	pvc := makePVC("data-pvc", "default", "standard", "50Gi", corev1.ClaimPending)
	cs := fake.NewSimpleClientset(pvc)
	issues, err := ListPendingPVCs(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 || issues[0].PVCName != "data-pvc" {
		t.Errorf("expected data-pvc issue, got %v", issues)
	}
	if issues[0].Capacity != "50Gi" {
		t.Errorf("capacity: want 50Gi, got %s", issues[0].Capacity)
	}
	if issues[0].StorageClass != "standard" {
		t.Errorf("storage class: want standard, got %s", issues[0].StorageClass)
	}
}

func TestListPendingPVCs_Bound(t *testing.T) {
	pvc := makePVC("ok-pvc", "default", "ssd", "10Gi", corev1.ClaimBound)
	cs := fake.NewSimpleClientset(pvc)
	issues, err := ListPendingPVCs(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("bound PVC should not be reported, got %d issues", len(issues))
	}
}

func TestListPendingPVCs_Mixed(t *testing.T) {
	cs := fake.NewSimpleClientset(
		makePVC("bound-pvc", "default", "ssd", "1Gi", corev1.ClaimBound),
		makePVC("stuck-pvc", "default", "slow", "100Gi", corev1.ClaimPending),
	)
	issues, err := ListPendingPVCs(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 || issues[0].PVCName != "stuck-pvc" {
		t.Errorf("expected only stuck-pvc, got %v", issues)
	}
}

// ── ListPVCNames tests ───────────────────────────────────────────────────────

func TestListPVCNames_Empty(t *testing.T) {
	cs := fake.NewSimpleClientset()
	names, err := ListPVCNames(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected empty, got %v", names)
	}
}

func TestListPVCNames_ReturnAll(t *testing.T) {
	cs := fake.NewSimpleClientset(
		makePVC("data-pvc", "payments", "ssd", "10Gi", corev1.ClaimBound),
		makePVC("logs-pvc", "payments", "hdd", "50Gi", corev1.ClaimPending),
		makePVC("other-pvc", "other-ns", "ssd", "5Gi", corev1.ClaimBound),
	)
	names, err := ListPVCNames(context.Background(), cs, "payments")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("want 2 PVC names, got %d: %v", len(names), names)
	}
	nameSet := map[string]bool{names[0]: true, names[1]: true}
	if !nameSet["data-pvc"] || !nameSet["logs-pvc"] {
		t.Errorf("expected data-pvc and logs-pvc, got %v", names)
	}
}
