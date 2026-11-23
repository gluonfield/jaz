package bots

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/storage"
)

type Config struct {
	Name      string
	Prompt    string
	Agent     string
	Model     string
	Schedules []ScheduleConfig
}

type ScheduleConfig struct {
	Name     string
	Prompt   string
	Schedule loops.Schedule
	Status   string
}

type declaration struct {
	input     CreateBot
	bot       Bot
	schedules []scheduleDeclaration
}

type scheduleDeclaration struct {
	input   loops.CreateLoop
	routine loops.Loop
}

func (s *Service) Declare(ctx context.Context, declared []Config) error {
	if len(declared) == 0 {
		return nil
	}
	bots, err := s.ListForWorkspace(storage.LocalWorkspaceID)
	if err != nil {
		return err
	}
	allRoutines, err := s.allRoutines()
	if err != nil {
		return err
	}
	routines := make([]loops.Loop, 0, len(allRoutines))
	for _, routine := range allRoutines {
		if normalizeWorkspaceID(routine.WorkspaceID) == storage.LocalWorkspaceID {
			routines = append(routines, routine)
		}
	}
	plan, err := planDeclarations(declared, bots, routines)
	if err != nil {
		return err
	}
	for _, item := range plan {
		item.input.WorkspaceID = storage.LocalWorkspaceID
		bot, err := s.declareBot(ctx, item.bot, item.input)
		if err != nil {
			return fmt.Errorf("configured bot %q: %w", item.input.Name, err)
		}
		for _, schedule := range item.schedules {
			schedule.input.BotID = bot.ID
			schedule.input.WorkspaceID = storage.LocalWorkspaceID
			if err := s.declareSchedule(schedule.routine, schedule.input); err != nil {
				return fmt.Errorf("configured bot %q, schedule %q: %w", item.input.Name, schedule.input.Name, err)
			}
		}
	}
	return nil
}

func planDeclarations(declared []Config, bots []Bot, routines []loops.Loop) ([]declaration, error) {
	result := make([]declaration, 0, len(declared))
	for _, input := range declared {
		input.Name = strings.TrimSpace(input.Name)
		input.Prompt = strings.TrimSpace(input.Prompt)
		input.Agent = acp.CanonicalAgentName(input.Agent)
		input.Model = strings.TrimSpace(input.Model)
		if input.Name == "" || slices.ContainsFunc(result, func(other declaration) bool { return strings.EqualFold(other.input.Name, input.Name) }) {
			return nil, fmt.Errorf("configured bot name %q is empty or repeated", input.Name)
		}
		item := declaration{input: CreateBot{Name: input.Name, Prompt: input.Prompt, Agent: input.Agent, Model: input.Model}}
		for _, bot := range bots {
			if !strings.EqualFold(bot.Name, input.Name) {
				continue
			}
			if item.bot.ID != "" || bot.Kind != KindBot {
				return nil, fmt.Errorf("configured bot %q: name matches a group or multiple bots", input.Name)
			}
			item.bot = bot
		}
		for _, schedule := range input.Schedules {
			schedule.Name = strings.TrimSpace(schedule.Name)
			if schedule.Name == "" || slices.ContainsFunc(item.schedules, func(other scheduleDeclaration) bool { return strings.EqualFold(other.input.Name, schedule.Name) }) {
				return nil, fmt.Errorf("configured bot %q: schedule name %q is empty or repeated", input.Name, schedule.Name)
			}
			prompt := strings.TrimSpace(schedule.Prompt)
			if prompt == "" {
				prompt = input.Prompt
			}
			routine, _, err := loops.NormalizeCreate(loops.CreateLoop{
				Name: schedule.Name, Prompt: prompt, Schedule: schedule.Schedule, Status: schedule.Status,
			}, time.Now())
			if err != nil {
				return nil, fmt.Errorf("configured bot %q, schedule %q: %w", input.Name, schedule.Name, err)
			}
			next := scheduleDeclaration{input: routine}
			for _, saved := range routines {
				if item.bot.ID == "" || saved.BotID != item.bot.ID || !strings.EqualFold(saved.Name, routine.Name) {
					continue
				}
				if next.routine.ID != "" {
					return nil, fmt.Errorf("configured bot %q, schedule %q: name matches multiple routines", input.Name, schedule.Name)
				}
				next.routine = saved
			}
			item.schedules = append(item.schedules, next)
		}
		result = append(result, item)
	}
	return result, nil
}

func (s *Service) declareBot(ctx context.Context, current Bot, input CreateBot) (Bot, error) {
	if current.ID == "" {
		return s.Create(ctx, input)
	}
	update := UpdateBot{}
	if current.Prompt != input.Prompt {
		update.Prompt = &input.Prompt
	}
	if input.Agent != "" && current.Agent != input.Agent {
		update.Agent = &input.Agent
	}
	if input.Model != "" && (current.Model != input.Model || update.Agent != nil) {
		update.Model = &input.Model
		if update.Agent == nil {
			update.ReasoningEffort = current.ReasoningEffort
		}
	}
	if update.Prompt != nil || update.Agent != nil || update.Model != nil {
		return s.Update(ctx, current.ID, update)
	}
	return current, nil
}

func (s *Service) declareSchedule(current loops.Loop, input loops.CreateLoop) error {
	if current.ID == "" {
		_, err := s.routines.Create(input)
		return err
	}
	update := loops.UpdateLoop{}
	if current.Prompt != input.Prompt {
		update.Prompt = &input.Prompt
	}
	if current.Schedule != input.Schedule {
		update.Schedule = &input.Schedule
	}
	if current.Status != input.Status {
		update.Status = &input.Status
	}
	if update.Prompt != nil || update.Schedule != nil || update.Status != nil {
		_, err := s.routines.Update(current.ID, update)
		return err
	}
	return nil
}
