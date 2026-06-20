package loops

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

func NormalizeCreate(input CreateLoop, now time.Time) (CreateLoop, time.Time, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Prompt = strings.TrimSpace(input.Prompt)
	input.Status = normalizeStatus(input.Status)
	input.Runtime = normalizeRuntime(input.Runtime)
	if input.Status != StatusActive && input.Status != StatusPaused {
		return input, time.Time{}, fmt.Errorf("unsupported loop status %q", input.Status)
	}
	if input.Runtime != RuntimeACP {
		return input, time.Time{}, fmt.Errorf("unsupported loop runtime %q", input.Runtime)
	}
	// An empty agent is left empty on purpose: the ACP manager resolves the
	// canonical default at spawn time, so loops never pin a specific agent.
	input.ACPAgent = strings.TrimSpace(input.ACPAgent)
	effort, err := normalizeReasoningEffort(input.ReasoningEffort)
	if err != nil {
		return input, time.Time{}, err
	}
	input.ReasoningEffort = effort
	input.ModelProvider = strings.TrimSpace(input.ModelProvider)
	input.Model = strings.TrimSpace(input.Model)
	input.Directory = strings.TrimSpace(input.Directory)
	if input.Name == "" {
		input.Name = titleFromPrompt(input.Prompt)
	}
	if input.Prompt == "" {
		return input, time.Time{}, fmt.Errorf("prompt is required")
	}
	schedule, next, err := NormalizeSchedule(input.Schedule, now)
	if err != nil {
		return input, time.Time{}, err
	}
	input.Schedule = schedule
	return input, next, nil
}

func NormalizeUpdate(current Loop, input UpdateLoop, now time.Time) (Loop, bool, error) {
	next := current
	if input.Name != nil {
		next.Name = strings.TrimSpace(*input.Name)
	}
	if input.Prompt != nil {
		next.Prompt = strings.TrimSpace(*input.Prompt)
	}
	if input.Status != nil {
		next.Status = normalizeStatus(*input.Status)
	}
	if input.Runtime != nil {
		next.Runtime = normalizeRuntime(*input.Runtime)
	}
	if input.ACPAgent != nil {
		next.ACPAgent = strings.TrimSpace(*input.ACPAgent)
	}
	if input.ReasoningEffort != nil {
		effort, err := normalizeReasoningEffort(*input.ReasoningEffort)
		if err != nil {
			return next, false, err
		}
		next.ReasoningEffort = effort
	}
	if input.ModelProvider != nil {
		next.ModelProvider = strings.TrimSpace(*input.ModelProvider)
	}
	if input.Model != nil {
		next.Model = strings.TrimSpace(*input.Model)
	}
	if input.Directory != nil {
		next.Directory = strings.TrimSpace(*input.Directory)
	}
	if next.Status != StatusActive && next.Status != StatusPaused {
		return next, false, fmt.Errorf("unsupported loop status %q", next.Status)
	}
	if next.Runtime != RuntimeACP {
		return next, false, fmt.Errorf("unsupported loop runtime %q", next.Runtime)
	}
	if next.Name == "" {
		next.Name = titleFromPrompt(next.Prompt)
	}
	if next.Prompt == "" {
		return next, false, fmt.Errorf("prompt is required")
	}
	reschedule := input.Reschedule
	if input.Schedule != nil {
		schedule, _, err := NormalizeSchedule(*input.Schedule, now)
		if err != nil {
			return next, false, err
		}
		next.Schedule = schedule
		reschedule = true
	}
	if next.Status == StatusActive && next.NextRunAt.IsZero() {
		reschedule = true
	}
	if reschedule {
		base := now
		if !input.RescheduleAt.IsZero() {
			base = input.RescheduleAt
		}
		nextRun, err := NextRun(next.Schedule, base)
		if err != nil {
			return next, false, err
		}
		next.NextRunAt = nextRun
	}
	return next, reschedule, nil
}

func NormalizeSchedule(schedule Schedule, now time.Time) (Schedule, time.Time, error) {
	schedule.Kind = strings.TrimSpace(schedule.Kind)
	if schedule.Kind == "" {
		schedule.Kind = ScheduleCron
	}
	if schedule.Kind != ScheduleCron {
		return schedule, time.Time{}, fmt.Errorf("unsupported schedule kind %q", schedule.Kind)
	}
	schedule.Expr = strings.TrimSpace(schedule.Expr)
	if schedule.Expr == "" {
		return schedule, time.Time{}, fmt.Errorf("cron expression is required")
	}
	loc, tz, err := scheduleLocation(schedule.Timezone)
	if err != nil {
		return schedule, time.Time{}, err
	}
	schedule.Timezone = tz
	parsed, err := cronParser.Parse(schedule.Expr)
	if err != nil {
		return schedule, time.Time{}, fmt.Errorf("invalid cron expression: %w", err)
	}
	next := parsed.Next(now.In(loc)).UTC()
	if next.IsZero() {
		return schedule, time.Time{}, fmt.Errorf("cron expression produced no next run")
	}
	return schedule, next, nil
}

func NextRun(schedule Schedule, after time.Time) (time.Time, error) {
	normalized, next, err := NormalizeSchedule(schedule, after)
	if err != nil {
		return time.Time{}, err
	}
	_ = normalized
	return next, nil
}

func scheduleLocation(name string) (*time.Location, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		if time.Local != nil {
			return time.Local, time.Local.String(), nil
		}
		return time.UTC, "UTC", nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, "", fmt.Errorf("invalid timezone %q: %w", name, err)
	}
	return loc, name, nil
}

func normalizeStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", StatusActive:
		return StatusActive
	case StatusPaused:
		return StatusPaused
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

func normalizeReasoningEffort(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "none":
		return "", nil
	case "minimal", "low", "medium", "high", "xhigh":
		return strings.ToLower(strings.TrimSpace(value)), nil
	default:
		return "", fmt.Errorf("unknown reasoning effort %q; valid values are none, minimal, low, medium, high, xhigh", value)
	}
}

func normalizeRuntime(runtime string) string {
	switch strings.ToLower(strings.TrimSpace(runtime)) {
	case "", RuntimeACP:
		return RuntimeACP
	default:
		return strings.ToLower(strings.TrimSpace(runtime))
	}
}

// Inline mentions arrive as markdown-style links ("[$skill](/path)",
// "[@rel/path](</abs path>)"); titles keep just the visible token.
var mentionLink = regexp.MustCompile(`\[([$@][^\]\n]+)\]\([^)\n]+\)`)

func titleFromPrompt(prompt string) string {
	prompt = mentionLink.ReplaceAllString(prompt, "$1")
	words := strings.Fields(prompt)
	if len(words) == 0 {
		return "Loop"
	}
	if len(words) > 6 {
		words = words[:6]
	}
	title := strings.Join(words, " ")
	title = strings.Trim(title, " \t\r\n.,!?;:")
	if len(title) > 64 {
		title = strings.TrimSpace(title[:64])
	}
	if title == "" {
		return "Loop"
	}
	return title
}
