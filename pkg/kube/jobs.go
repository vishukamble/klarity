package kube

import (
	"context"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// JobIssue describes a Job with one or more failed pods.
type JobIssue struct {
	Namespace  string
	JobName    string
	Failed     int32
	Conditions []string // failure condition messages
}

// CronJobIssue describes a CronJob that is suspended.
type CronJobIssue struct {
	Namespace    string
	CronJobName  string
	Schedule     string
	Suspended    bool
	LastSchedule *time.Time // nil if never scheduled
}

// ListFailedJobs returns Jobs in namespace that have at least one failed pod.
// When excludeCompleted is true, jobs that have already completed (have a
// CompletionTime) are skipped even if they had intermediate failures.
func ListFailedJobs(ctx context.Context, cs kubernetes.Interface, namespace string, excludeCompleted bool) ([]JobIssue, error) {
	jobs, err := cs.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing jobs in %q: %w", namespace, err)
	}

	var issues []JobIssue
	for _, job := range jobs.Items {
		if job.Status.Failed == 0 {
			continue
		}
		if excludeCompleted && job.Status.CompletionTime != nil {
			continue
		}

		var conditions []string
		for _, c := range job.Status.Conditions {
			if c.Type == batchv1.JobFailed && c.Status == "True" {
				conditions = append(conditions, c.Message)
			}
		}

		issues = append(issues, JobIssue{
			Namespace:  job.Namespace,
			JobName:    job.Name,
			Failed:     job.Status.Failed,
			Conditions: conditions,
		})
	}
	return issues, nil
}

// ListSuspendedCronJobs returns CronJobs in namespace that are suspended.
func ListSuspendedCronJobs(ctx context.Context, cs kubernetes.Interface, namespace string) ([]CronJobIssue, error) {
	cjList, err := cs.BatchV1().CronJobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing cronjobs in %q: %w", namespace, err)
	}

	var issues []CronJobIssue
	for _, cj := range cjList.Items {
		if cj.Spec.Suspend == nil || !*cj.Spec.Suspend {
			continue
		}

		var lastSched *time.Time
		if cj.Status.LastScheduleTime != nil {
			t := cj.Status.LastScheduleTime.Time
			lastSched = &t
		}

		issues = append(issues, CronJobIssue{
			Namespace:    cj.Namespace,
			CronJobName:  cj.Name,
			Schedule:     cj.Spec.Schedule,
			Suspended:    true,
			LastSchedule: lastSched,
		})
	}
	return issues, nil
}
