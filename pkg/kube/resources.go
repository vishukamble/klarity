package kube

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// QuotaIssue describes a resource quota whose usage meets or exceeds a
// reporting threshold (default 80%).
type QuotaIssue struct {
	Namespace   string
	QuotaName   string
	Resource    string  // e.g., "cpu", "memory", "pods"
	Used        string  // human-readable (e.g., "800m", "1Gi")
	Hard        string  // human-readable ceiling
	UsedPercent float64 // 0–100
}

// PVCIssue describes a PersistentVolumeClaim that is stuck in Pending.
type PVCIssue struct {
	Namespace    string
	PVCName      string
	StorageClass string // may be empty
	Capacity     string // requested capacity string
	Phase        string
}

// QuotaThreshold is the minimum usage percentage at which a quota is reported.
const QuotaThreshold = 80.0

// ListQuotaIssues returns ResourceQuota entries in namespace where at least
// one tracked resource has reached or exceeded QuotaThreshold percent.
func ListQuotaIssues(ctx context.Context, cs kubernetes.Interface, namespace string) ([]QuotaIssue, error) {
	quotas, err := cs.CoreV1().ResourceQuotas(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing resource quotas in %q: %w", namespace, err)
	}

	var issues []QuotaIssue
	for _, rq := range quotas.Items {
		for resource, hard := range rq.Status.Hard {
			used, ok := rq.Status.Used[resource]
			if !ok {
				continue
			}

			hardMilli := hard.MilliValue()
			if hardMilli == 0 {
				continue
			}
			usedMilli := used.MilliValue()
			pct := float64(usedMilli) / float64(hardMilli) * 100.0

			if pct >= QuotaThreshold {
				issues = append(issues, QuotaIssue{
					Namespace:   rq.Namespace,
					QuotaName:   rq.Name,
					Resource:    string(resource),
					Used:        used.String(),
					Hard:        hard.String(),
					UsedPercent: pct,
				})
			}
		}
	}
	return issues, nil
}

// ListPVCNames returns the names of all PersistentVolumeClaims in namespace.
func ListPVCNames(ctx context.Context, cs kubernetes.Interface, namespace string) ([]string, error) {
	pvcs, err := cs.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing PVC names in %q: %w", namespace, err)
	}
	names := make([]string, len(pvcs.Items))
	for i, pvc := range pvcs.Items {
		names[i] = pvc.Name
	}
	return names, nil
}

// ListPendingPVCs returns PersistentVolumeClaims in namespace that are stuck
// in Pending phase (storage has not been provisioned).
func ListPendingPVCs(ctx context.Context, cs kubernetes.Interface, namespace string) ([]PVCIssue, error) {
	pvcs, err := cs.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing PVCs in %q: %w", namespace, err)
	}

	var issues []PVCIssue
	for _, pvc := range pvcs.Items {
		if pvc.Status.Phase != corev1.ClaimPending {
			continue
		}

		sc := ""
		if pvc.Spec.StorageClassName != nil {
			sc = *pvc.Spec.StorageClassName
		}

		capacity := ""
		if req, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
			capacity = req.String()
		}

		issues = append(issues, PVCIssue{
			Namespace:    pvc.Namespace,
			PVCName:      pvc.Name,
			StorageClass: sc,
			Capacity:     capacity,
			Phase:        string(pvc.Status.Phase),
		})
	}
	return issues, nil
}
