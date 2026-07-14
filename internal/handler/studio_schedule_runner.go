package handler

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"portfolio-backend/internal/model"
)

func (runner *StudioExecutionRunner) enqueueDueSchedules(ctx context.Context, now time.Time) error {
	slot := now.UTC().Format("200601021504")
	if runner.lastScheduleSlot == slot {
		return nil
	}
	workflows, err := runner.service.Studio.ListWorkflows(ctx)
	if err != nil {
		return err
	}
	runner.lastScheduleSlot = slot
	for index := range workflows {
		workflow := &workflows[index]
		if workflow.Status != "active" || workflow.Definition == nil {
			continue
		}
		for _, node := range workflow.Definition.Nodes {
			if node.Type != "schedule" || node.Kind != "trigger" {
				continue
			}
			scheduledAt, due := studioScheduleOccurrence(node.Config, now)
			if !due {
				continue
			}
			misfire, _ := node.Config["misfirePolicy"].(string)
			if misfire != "run-once" && !scheduledAt.Truncate(time.Minute).Equal(now.Truncate(time.Minute)) {
				continue
			}
			compiled, compileErr := compileStudioGraph(workflow.Definition, node.ID, studioGraphModeFull, "")
			if compileErr != nil {
				continue
			}
			path := make([]model.StudioExecutionPathNode, 0, len(compiled.Nodes))
			for _, pathNode := range compiled.Nodes {
				path = append(path, model.StudioExecutionPathNode{ID: pathNode.ID, Type: pathNode.Type, Label: pathNode.Label})
			}
			localSlot := scheduledAt.UTC().Format("200601021504")
			if locationName, ok := node.Config["timezone"].(string); ok {
				if location, locationErr := time.LoadLocation(locationName); locationErr == nil {
					localSlot = scheduledAt.In(location).Format("200601021504")
				}
			}
			_, enqueueErr := runner.service.Studio.EnqueueGraphExecution(ctx, model.StudioGraphExecutionInput{
				WorkflowID: workflow.ID, WorkflowName: workflow.Name, WorkflowUpdatedAt: workflow.UpdatedAt,
				TriggerNodeID: node.ID, Mode: "full", Source: "schedule",
				SourceKey:    workflow.ID + ":schedule:" + node.ID + ":" + localSlot,
				InitialInput: []map[string]any{}, Path: path,
			})
			if enqueueErr != nil {
				return fmt.Errorf("enqueue workflow %s schedule %s: %w", workflow.ID, node.ID, enqueueErr)
			}
		}
	}
	return nil
}

func studioScheduleDue(config map[string]any, now time.Time) bool {
	occurrence, ok := studioScheduleOccurrence(config, now)
	return ok && occurrence.Truncate(time.Minute).Equal(now.Truncate(time.Minute))
}

func studioScheduleOccurrence(config map[string]any, now time.Time) (time.Time, bool) {
	enabled, _ := config["enabled"].(bool)
	locationName, _ := config["timezone"].(string)
	location, err := time.LoadLocation(locationName)
	if !enabled || err != nil {
		return time.Time{}, false
	}
	local := now.In(location).Truncate(time.Minute)
	mode, _ := config["mode"].(string)
	switch mode {
	case "interval":
		minutes, ok := configNumber(config["intervalMinutes"])
		if !ok || minutes < 1 {
			return time.Time{}, false
		}
		minuteCount := local.Unix() / 60
		return time.Unix((minuteCount-minuteCount%int64(minutes))*60, 0).In(location), true
	case "daily":
		clock, _ := config["time"].(string)
		scheduled, parseErr := time.ParseInLocation("2006-01-02 15:04", local.Format("2006-01-02 ")+clock, location)
		if parseErr != nil {
			return time.Time{}, false
		}
		if scheduled.After(local) {
			scheduled = scheduled.AddDate(0, 0, -1)
		}
		return scheduled, true
	case "weekly":
		clock, _ := config["time"].(string)
		for daysBack := 0; daysBack < 8; daysBack++ {
			candidateDay := local.AddDate(0, 0, -daysBack)
			if !studioScheduleHasWeekday(config["daysOfWeek"], int(candidateDay.Weekday())) {
				continue
			}
			scheduled, parseErr := time.ParseInLocation("2006-01-02 15:04", candidateDay.Format("2006-01-02 ")+clock, location)
			if parseErr == nil && !scheduled.After(local) {
				return scheduled, true
			}
		}
		return time.Time{}, false
	case "cron":
		expression, _ := config["cronExpression"].(string)
		return studioLatestCronOccurrence(expression, local)
	default:
		return time.Time{}, false
	}
}

func studioLatestCronOccurrence(expression string, local time.Time) (time.Time, bool) {
	fields := strings.Fields(expression)
	if len(fields) != 5 || !validCronExpression(expression) {
		return time.Time{}, false
	}
	for daysBack := 0; daysBack <= 366; daysBack++ {
		day := local.AddDate(0, 0, -daysBack)
		if !studioCronFieldMatches(fields[3], int(day.Month()), 1, 12) {
			continue
		}
		dayOfMonth := studioCronFieldMatches(fields[2], day.Day(), 1, 31)
		dayOfWeek := studioCronFieldMatches(fields[4], int(day.Weekday()), 0, 6)
		dayMatches := dayOfMonth && dayOfWeek
		if fields[2] != "*" && fields[4] != "*" {
			dayMatches = dayOfMonth || dayOfWeek
		}
		if !dayMatches {
			continue
		}
		maxHour := 23
		if daysBack == 0 {
			maxHour = local.Hour()
		}
		for hour := maxHour; hour >= 0; hour-- {
			if !studioCronFieldMatches(fields[1], hour, 0, 23) {
				continue
			}
			maxMinute := 59
			if daysBack == 0 && hour == local.Hour() {
				maxMinute = local.Minute()
			}
			for minute := maxMinute; minute >= 0; minute-- {
				if studioCronFieldMatches(fields[0], minute, 0, 59) {
					return time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, local.Location()), true
				}
			}
		}
	}
	return time.Time{}, false
}

func studioScheduleHasWeekday(raw any, weekday int) bool {
	values, ok := raw.([]any)
	if !ok {
		if typed, typedOK := raw.([]int); typedOK {
			for _, value := range typed {
				if value == weekday {
					return true
				}
			}
		}
		return false
	}
	for _, value := range values {
		if number, valid := configNumber(value); valid && int(number) == weekday {
			return true
		}
	}
	return false
}

func studioCronMatches(expression string, value time.Time) bool {
	fields := strings.Fields(expression)
	if len(fields) != 5 || !validCronExpression(expression) {
		return false
	}
	minute := studioCronFieldMatches(fields[0], value.Minute(), 0, 59)
	hour := studioCronFieldMatches(fields[1], value.Hour(), 0, 23)
	month := studioCronFieldMatches(fields[3], int(value.Month()), 1, 12)
	dayOfMonth := studioCronFieldMatches(fields[2], value.Day(), 1, 31)
	dayOfWeek := studioCronFieldMatches(fields[4], int(value.Weekday()), 0, 6)
	dayMatches := dayOfMonth && dayOfWeek
	if fields[2] != "*" && fields[4] != "*" {
		dayMatches = dayOfMonth || dayOfWeek
	}
	return minute && hour && month && dayMatches
}

func studioCronFieldMatches(field string, value, minimum, maximum int) bool {
	for _, item := range strings.Split(field, ",") {
		parts := strings.Split(item, "/")
		step := 1
		if len(parts) == 2 {
			step, _ = strconv.Atoi(parts[1])
		}
		base := parts[0]
		start, end := minimum, maximum
		switch {
		case base == "*":
		case strings.Contains(base, "-"):
			rangeParts := strings.Split(base, "-")
			start, _ = strconv.Atoi(rangeParts[0])
			end, _ = strconv.Atoi(rangeParts[1])
		case base != "":
			start, _ = strconv.Atoi(base)
			if len(parts) == 1 {
				end = start
			}
		}
		if value >= start && value <= end && (value-start)%step == 0 {
			return true
		}
	}
	return false
}
