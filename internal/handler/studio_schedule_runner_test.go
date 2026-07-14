package handler

import (
	"testing"
	"time"
)

func TestStudioScheduleDue(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 15, 9, 30, 0, 0, time.FixedZone("ICT", 7*60*60))
	cases := []struct {
		name   string
		config map[string]any
		want   bool
	}{
		{"daily", map[string]any{"enabled": true, "mode": "daily", "timezone": "Asia/Bangkok", "time": "09:30"}, true},
		{"weekly", map[string]any{"enabled": true, "mode": "weekly", "timezone": "Asia/Bangkok", "time": "09:30", "daysOfWeek": []int{3}}, true},
		{"cron", map[string]any{"enabled": true, "mode": "cron", "timezone": "Asia/Bangkok", "cronExpression": "*/15 9 * * 3"}, true},
		{"disabled", map[string]any{"enabled": false, "mode": "daily", "timezone": "Asia/Bangkok", "time": "09:30"}, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := studioScheduleDue(test.config, now); got != test.want {
				t.Fatalf("studioScheduleDue()=%v want=%v", got, test.want)
			}
		})
	}

	missed := map[string]any{"enabled": true, "mode": "daily", "timezone": "Asia/Bangkok", "time": "09:29", "misfirePolicy": "run-once"}
	occurrence, ok := studioScheduleOccurrence(missed, now)
	if !ok || occurrence.In(now.Location()).Format("15:04") != "09:29" || studioScheduleDue(missed, now) {
		t.Fatalf("missed run occurrence was not recovered correctly: %v %v", occurrence, ok)
	}
}

func TestStudioCronMatchesRangeStepAndDayOR(t *testing.T) {
	t.Parallel()
	wednesday := time.Date(2026, 7, 15, 9, 30, 0, 0, time.UTC)
	if !studioCronMatches("15-45/15 9 1 * 3", wednesday) {
		t.Fatal("cron range/step or day-of-month/day-of-week OR semantics did not match")
	}
	if studioCronMatches("0 10 * * *", wednesday) {
		t.Fatal("non-matching cron expression matched")
	}
}
