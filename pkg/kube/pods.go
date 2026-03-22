package kube

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// PodIssue describes a single unhealthy signal from a pod or container.
// One pod can produce multiple PodIssues (e.g., CrashLoopBackOff + OOMKilled).
type PodIssue struct {
	Namespace        string
	PodName          string
	ContainerName    string    // empty for pod-level issues (Pending)
	Reason           string    // CrashLoopBackOff | ImagePullBackOff | OOMKilled | Pending | …
	Phase            string    // Running | Pending | Failed | Unknown
	Image            string
	RestartCount     int32
	Message          string    // from container waiting/terminated message
	PendingSince     time.Time // populated when Reason == "Pending"
	LogSummary       string    // one-line log extract; set by scan loop after FEAT-18/19; empty until then
	VolumeClaimNames []string  // PVC names referenced by pod spec volumes; populated for Pending pods
}

// unhealthyWaitingReasons are container waiting reasons that indicate a problem.
var unhealthyWaitingReasons = map[string]bool{
	"CrashLoopBackOff":           true,
	"ImagePullBackOff":           true,
	"ErrImagePull":               true,
	"CreateContainerConfigError": true,
	"InvalidImageName":           true,
	"CreateContainerError":       true,
	"RunContainerError":          true,
}

// ListUnhealthyPods returns all pods in namespace that have an unhealthy
// signal. Completed (Succeeded) pods are skipped.
func ListUnhealthyPods(ctx context.Context, cs kubernetes.Interface, namespace string) ([]PodIssue, error) {
	pods, err := cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing pods in %q: %w", namespace, err)
	}

	var issues []PodIssue
	for _, pod := range pods.Items {
		// Completed pods are healthy by definition.
		if pod.Status.Phase == corev1.PodSucceeded {
			continue
		}

		// Evicted — pod was evicted by kubelet due to node pressure.
		if pod.Status.Phase == corev1.PodFailed && pod.Status.Reason == "Evicted" {
			issues = append(issues, PodIssue{
				Namespace: pod.Namespace,
				PodName:   pod.Name,
				Reason:    "Evicted",
				Phase:     string(pod.Status.Phase),
				Message:   pod.Status.Message,
			})
			continue
		}

		// Pending — pod hasn't been scheduled yet or containers haven't started.
		if pod.Status.Phase == corev1.PodPending {
			issues = append(issues, PodIssue{
				Namespace:        pod.Namespace,
				PodName:          pod.Name,
				Reason:           "Pending",
				Phase:            string(pod.Status.Phase),
				Message:          pendingMessage(pod),
				PendingSince:     pod.CreationTimestamp.Time,
				VolumeClaimNames: extractVolumeClaimNames(pod),
			})
			// Don't also inspect empty container statuses for pending pods.
			continue
		}

		issues = append(issues, inspectContainerStatuses(pod, pod.Status.InitContainerStatuses)...)
		issues = append(issues, inspectContainerStatuses(pod, pod.Status.ContainerStatuses)...)
	}
	return issues, nil
}

// extractVolumeClaimNames returns the PVC claim names referenced by the pod's volumes.
func extractVolumeClaimNames(pod corev1.Pod) []string {
	var names []string
	for _, vol := range pod.Spec.Volumes {
		if vol.PersistentVolumeClaim != nil && vol.PersistentVolumeClaim.ClaimName != "" {
			names = append(names, vol.PersistentVolumeClaim.ClaimName)
		}
	}
	return names
}

// pendingMessage extracts a scheduling reason from pod conditions.
// It returns the message from the first PodScheduled=False condition, or the
// first condition with a non-empty message, or empty string if none found.
func pendingMessage(pod corev1.Pod) string {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionFalse {
			if cond.Message != "" {
				return cond.Message
			}
			if cond.Reason != "" {
				return cond.Reason
			}
		}
	}
	// Fallback: any condition with a message.
	for _, cond := range pod.Status.Conditions {
		if cond.Message != "" {
			return cond.Message
		}
	}
	return ""
}

// inspectContainerStatuses extracts PodIssues from a slice of ContainerStatus
// entries belonging to pod.
func inspectContainerStatuses(pod corev1.Pod, statuses []corev1.ContainerStatus) []PodIssue {
	var issues []PodIssue
	for _, cs := range statuses {
		// Unhealthy waiting state (CrashLoopBackOff, ImagePullBackOff, …)
		if cs.State.Waiting != nil && unhealthyWaitingReasons[cs.State.Waiting.Reason] {
			issues = append(issues, PodIssue{
				Namespace:     pod.Namespace,
				PodName:       pod.Name,
				ContainerName: cs.Name,
				Reason:        cs.State.Waiting.Reason,
				Phase:         string(pod.Status.Phase),
				Image:         cs.Image,
				RestartCount:  cs.RestartCount,
				Message:       cs.State.Waiting.Message,
			})
		}

		// OOMKilled surfaces in lastTerminationState even when the container
		// is currently in CrashLoopBackOff — we emit it as a separate signal
		// so the OOM classifier can pick it up independently.
		if cs.LastTerminationState.Terminated != nil &&
			cs.LastTerminationState.Terminated.Reason == "OOMKilled" {
			issues = append(issues, PodIssue{
				Namespace:     pod.Namespace,
				PodName:       pod.Name,
				ContainerName: cs.Name,
				Reason:        "OOMKilled",
				Phase:         string(pod.Status.Phase),
				Image:         cs.Image,
				RestartCount:  cs.RestartCount,
			})
		}
	}
	return issues
}
