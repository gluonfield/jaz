package acp

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/wins/jaz/backend/internal/sessionevents"
)

type AskUserInput struct {
	Questions []UserQuestion `json:"questions" jsonschema:"One or more questions in display order."`
}

type UserQuestion struct {
	ID          string   `json:"id" jsonschema:"Unique identifier used to return this question's answer."`
	Question    string   `json:"question" jsonschema:"Complete self-contained question."`
	Options     []string `json:"options,omitempty" jsonschema:"Short plain-text answer choices. Provide them whenever useful so the user can select rather than type. The user can always type another answer."`
	MultiSelect bool     `json:"multi_select,omitempty" jsonschema:"Allow several choices, shown as checkboxes. Requires options. Otherwise choices are single-select radio buttons."`
}

type AskUserOutput struct {
	Answers   map[string]InteractiveAnswerValue `json:"answers"`
	Cancelled bool                              `json:"cancelled,omitempty"`
}

func (m *Manager) AskUser(ctx context.Context, sessionID string, input AskUserInput) (AskUserOutput, error) {
	if sessionID == "" {
		return AskUserOutput{}, fmt.Errorf("ask_user requires a current Jaz thread")
	}
	job := m.jobByID(sessionID)
	if job == nil {
		return AskUserOutput{}, fmt.Errorf("current Jaz thread is not active")
	}
	if len(input.Questions) == 0 {
		return AskUserOutput{}, fmt.Errorf("at least one question is required")
	}
	questions := make([]sessionevents.ACPQuestion, 0, len(input.Questions))
	multiSelectByID := make(map[string]bool, len(input.Questions))
	for _, question := range input.Questions {
		id := strings.TrimSpace(question.ID)
		text := strings.TrimSpace(question.Question)
		_, duplicate := multiSelectByID[id]
		if id == "" || text == "" || duplicate {
			return AskUserOutput{}, fmt.Errorf("questions require unique nonempty ids and nonempty text")
		}
		multiSelectByID[id] = question.MultiSelect
		if question.MultiSelect && len(question.Options) == 0 {
			return AskUserOutput{}, fmt.Errorf("question %s requires options for multiple selection", id)
		}
		options := make([]sessionevents.ACPQuestionOption, 0, len(question.Options))
		for _, label := range question.Options {
			option := sessionevents.ACPQuestionOption{Label: strings.TrimSpace(label)}
			if option.Label == "" || slices.Contains(options, option) {
				return AskUserOutput{}, fmt.Errorf("question %s requires unique nonempty options", id)
			}
			options = append(options, option)
		}
		questions = append(questions, sessionevents.ACPQuestion{
			ID: id, Question: text, IsOther: true, Options: options, MultiSelect: question.MultiSelect,
		})
	}
	prepare := func(answers map[string]InteractiveAnswerValue) (map[string]InteractiveAnswerValue, error) {
		normalized := make(map[string]InteractiveAnswerValue, len(answers))
		for id, answer := range answers {
			values := trimmedAnswers(answer.Answers)
			multiSelect, known := multiSelectByID[id]
			if !known {
				return nil, fmt.Errorf("unknown question: %s", id)
			}
			if len(values) == 0 {
				continue
			}
			if !multiSelect && len(values) != 1 {
				return nil, fmt.Errorf("question %s requires one answer", id)
			}
			normalized[id] = InteractiveAnswerValue{Answers: values}
		}
		return normalized, nil
	}
	answer := m.awaitPermissionAnswer(ctx, job, sessionevents.ACPPermission{
		ID: newPermissionID(), Title: "Questions", Questions: questions, Status: "pending",
	}, prepare)
	out := AskUserOutput{Answers: answer.Answers, Cancelled: answer.Answers == nil}
	if out.Cancelled {
		out.Answers = map[string]InteractiveAnswerValue{}
	}
	return out, nil
}
