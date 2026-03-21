package diagnosis

import (
	"testing"
	"time"

	"github.com/vishukamble/klarity/pkg/kube"
)

func TestCronJobClassifier(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		results ScanResults
		wantLen int
	}{
		{
			name:    "empty input",
			results: ScanResults{},
			wantLen: 0,
		},
		{
			name: "single suspended cronjob with last schedule",
			results: ScanResults{
				EnvName:    "prod",
				ClusterCtx: "prod-1",
				CronJobs: []kube.CronJobIssue{
					{
						Namespace:    "batch",
						CronJobName:  "nightly-backup",
						Schedule:     "0 2 * * *",
						Suspended:    true,
						LastSchedule: &now,
					},
				},
			},
			wantLen: 1,
		},
		{
			name: "suspended cronjob never scheduled",
			results: ScanResults{
				CronJobs: []kube.CronJobIssue{
					{
						Namespace:   "batch",
						CronJobName: "new-job",
						Schedule:    "*/5 * * * *",
						Suspended:   true,
					},
				},
			},
			wantLen: 1,
		},
		{
			name: "multiple suspended cronjobs",
			results: ScanResults{
				CronJobs: []kube.CronJobIssue{
					{CronJobName: "cj-a", Schedule: "0 * * * *", Suspended: true},
					{CronJobName: "cj-b", Schedule: "0 0 * * *", Suspended: true, LastSchedule: &now},
				},
			},
			wantLen: 2,
		},
	}

	c := CronJobClassifier{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.Classify(tt.results)
			if len(got) != tt.wantLen {
				t.Fatalf("got %d findings, want %d", len(got), tt.wantLen)
			}
			if tt.wantLen == 0 {
				return
			}
			f := got[0]
			if f.Category != CategoryCronJobSuspended {
				t.Errorf("category = %q, want %q", f.Category, CategoryCronJobSuspended)
			}
			if f.Severity != SeverityInfo {
				t.Errorf("severity = %q, want %q", f.Severity, SeverityInfo)
			}
			if f.DetailFields["schedule"] == "" {
				t.Error("DetailFields missing schedule")
			}
			if f.DetailFields["last_schedule"] == "" {
				t.Error("DetailFields missing last_schedule")
			}
			// Verify "never" vs RFC3339 in last_schedule.
			cj := tt.results.CronJobs[0]
			if cj.LastSchedule == nil && f.DetailFields["last_schedule"] != "never" {
				t.Errorf("last_schedule = %q, want \"never\"", f.DetailFields["last_schedule"])
			}
			if cj.LastSchedule != nil && f.DetailFields["last_schedule"] == "never" {
				t.Errorf("last_schedule should not be \"never\" when LastSchedule is set")
			}
		})
	}
}
