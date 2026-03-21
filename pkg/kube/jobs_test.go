package kube

import (
	"context"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// ── Job helpers ───────────────────────────────────────────────────────────────

func makeJob(name, ns string, failed int32, failedCondMsg string) *batchv1.Job {
	j := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Status:     batchv1.JobStatus{Failed: failed},
	}
	if failedCondMsg != "" {
		j.Status.Conditions = []batchv1.JobCondition{
			{
				Type:    batchv1.JobFailed,
				Status:  corev1.ConditionTrue,
				Message: failedCondMsg,
			},
		}
	}
	return j
}

func TestListFailedJobs_Healthy(t *testing.T) {
	cs := fake.NewSimpleClientset(makeJob("etl-job", "default", 0, ""))
	issues, err := ListFailedJobs(context.Background(), cs, "default", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected no issues, got %d", len(issues))
	}
}

func TestListFailedJobs_Failed(t *testing.T) {
	cs := fake.NewSimpleClientset(makeJob("etl-job", "default", 3, "BackoffLimitExceeded"))
	issues, err := ListFailedJobs(context.Background(), cs, "default", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("want 1 issue, got %d", len(issues))
	}
	if issues[0].Failed != 3 {
		t.Errorf("failed count: want 3, got %d", issues[0].Failed)
	}
	if len(issues[0].Conditions) != 1 || issues[0].Conditions[0] != "BackoffLimitExceeded" {
		t.Errorf("conditions: %v", issues[0].Conditions)
	}
}

func TestListFailedJobs_NoConditionMessage(t *testing.T) {
	// Failed pods but no Failed condition yet (still retrying)
	cs := fake.NewSimpleClientset(makeJob("retry-job", "default", 1, ""))
	issues, err := ListFailedJobs(context.Background(), cs, "default", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("want 1 issue, got %d", len(issues))
	}
	if len(issues[0].Conditions) != 0 {
		t.Errorf("expected no conditions, got %v", issues[0].Conditions)
	}
}

func TestListFailedJobs_Mixed(t *testing.T) {
	cs := fake.NewSimpleClientset(
		makeJob("ok-job", "default", 0, ""),
		makeJob("bad-job", "default", 5, "DeadlineExceeded"),
	)
	issues, err := ListFailedJobs(context.Background(), cs, "default", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 || issues[0].JobName != "bad-job" {
		t.Errorf("expected only bad-job, got %v", issues)
	}
}

func TestListFailedJobs_ExcludeCompleted(t *testing.T) {
	// A job that failed some pods but ultimately completed.
	completedJob := makeJob("completed-job", "default", 2, "")
	now := metav1.Now()
	completedJob.Status.CompletionTime = &now

	// A job that failed and hasn't completed.
	activeFailedJob := makeJob("active-failed", "default", 3, "BackoffLimitExceeded")

	cs := fake.NewSimpleClientset(completedJob, activeFailedJob)

	// With excludeCompleted=true, completed job should be skipped.
	issues, err := ListFailedJobs(context.Background(), cs, "default", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 || issues[0].JobName != "active-failed" {
		t.Errorf("excludeCompleted=true: expected only active-failed, got %v", issues)
	}

	// With excludeCompleted=false, both should appear.
	issues, err = ListFailedJobs(context.Background(), cs, "default", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 2 {
		t.Errorf("excludeCompleted=false: expected 2 issues, got %d", len(issues))
	}
}

// ── CronJob helpers ───────────────────────────────────────────────────────────

func makeCronJob(name, ns, schedule string, suspended bool, lastSched *time.Time) *batchv1.CronJob {
	susp := suspended
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: batchv1.CronJobSpec{
			Schedule: schedule,
			Suspend:  &susp,
		},
	}
	if lastSched != nil {
		cj.Status.LastScheduleTime = &metav1.Time{Time: *lastSched}
	}
	return cj
}

func TestListSuspendedCronJobs_Active(t *testing.T) {
	cs := fake.NewSimpleClientset(makeCronJob("backup", "default", "0 2 * * *", false, nil))
	issues, err := ListSuspendedCronJobs(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected no issues for active cronjob, got %d", len(issues))
	}
}

func TestListSuspendedCronJobs_Suspended(t *testing.T) {
	last := time.Now().Add(-2 * time.Hour)
	cs := fake.NewSimpleClientset(makeCronJob("report", "default", "0 6 * * *", true, &last))
	issues, err := ListSuspendedCronJobs(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("want 1 issue, got %d", len(issues))
	}
	if issues[0].CronJobName != "report" {
		t.Errorf("name: want report, got %s", issues[0].CronJobName)
	}
	if issues[0].Schedule != "0 6 * * *" {
		t.Errorf("schedule: want '0 6 * * *', got %s", issues[0].Schedule)
	}
	if issues[0].LastSchedule == nil {
		t.Error("LastSchedule should be set")
	}
}

func TestListSuspendedCronJobs_SuspendedNeverRan(t *testing.T) {
	cs := fake.NewSimpleClientset(makeCronJob("new-cj", "default", "*/5 * * * *", true, nil))
	issues, err := ListSuspendedCronJobs(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 || issues[0].LastSchedule != nil {
		t.Errorf("expected 1 issue with nil LastSchedule, got %v", issues)
	}
}

func TestListSuspendedCronJobs_Mixed(t *testing.T) {
	cs := fake.NewSimpleClientset(
		makeCronJob("active-cj", "default", "0 * * * *", false, nil),
		makeCronJob("paused-cj", "default", "0 * * * *", true, nil),
	)
	issues, err := ListSuspendedCronJobs(context.Background(), cs, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 || issues[0].CronJobName != "paused-cj" {
		t.Errorf("expected only paused-cj, got %v", issues)
	}
}
