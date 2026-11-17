package acp

import (
	"context"
	"fmt"
	"strings"

	"github.com/wins/jaz/backend/internal/sessionevents"
)

func questionAnswers(questions []sessionevents.ACPQuestion, answers map[string]InteractiveAnswerValue) (map[string]InteractiveAnswerValue, error) {
	byID := make(map[string]sessionevents.ACPQuestion, len(questions))
	for _, question := range questions {
		byID[question.ID] = question
	}
	normalized := make(map[string]InteractiveAnswerValue, len(answers))
	for id, answer := range answers {
		question, exists := byID[id]
		if !exists {
			return nil, fmt.Errorf("unknown question: %s", id)
		}
		values := trimmedAnswers(answer.Answers)
		if len(values) == 0 {
			continue
		}
		if !question.MultiSelect && len(values) != 1 {
			return nil, fmt.Errorf("question %s requires one answer", id)
		}
		normalized[id] = InteractiveAnswerValue{Answers: values}
	}
	return normalized, nil
}

func (m *Manager) answerSavedQuestion(ctx context.Context, req InteractiveAnswer) error {
	question, err := m.savedQuestion(req.Session, req.RequestID)
	if err != nil {
		return err
	}
	if question.SessionID != "" && question.SessionID != req.Session {
		question, err = m.savedQuestion(question.SessionID, req.RequestID)
		if err != nil {
			return err
		}
	}
	if question.Status == "selected" {
		return nil
	}
	if !sessionevents.QuestionOpen(question) || req.Answers == nil {
		return fmt.Errorf("question is no longer open: %s", req.RequestID)
	}
	answers, err := questionAnswers(question.Questions, req.Answers)
	if err != nil {
		return err
	}
	owner := question.SessionID
	if owner == "" {
		owner = req.Session
	}
	var text strings.Builder
	text.WriteString("Answers to your questions:\n")
	question.Answers = make(map[string][]string, len(question.Questions))
	for _, field := range question.Questions {
		values := answers[field.ID].Answers
		question.Answers[field.ID] = values
		text.WriteString("\n" + field.Question + "\n")
		if len(values) == 0 {
			text.WriteString("Skipped.\n")
		} else {
			text.WriteString(strings.Join(values, "\n") + "\n")
		}
	}
	if _, err := m.Input(ctx, SendRequest{Session: owner, Message: text.String(), FromUser: true, PlanRequested: req.PlanRequested, ParentVisible: req.ParentVisible || req.Session != owner, questionAnswer: question.ID}); err != nil {
		return err
	}
	question.Status = "selected"
	question.SelectedOptionID = "answered"
	events := []sessionevents.Event{{SessionID: owner, Type: "permission_response", Permission: &question}}
	if req.Session != owner {
		events = append(events, sessionevents.Event{SessionID: req.Session, Type: "permission_response", Permission: &question})
	}
	if err := m.recordAndPublishEventListDirect(events); err != nil {
		return err
	}
	m.permissionMu.Lock()
	delete(m.pendingPermission, req.RequestID)
	m.permissionMu.Unlock()
	if job := m.jobByID(owner); job != nil {
		m.removeJobPermission(job, req.RequestID)
	}
	return nil
}

func (m *Manager) savedQuestion(session, id string) (sessionevents.ACPPermission, error) {
	events, err := m.store.LoadSessionEvents(session)
	if err != nil {
		return sessionevents.ACPPermission{}, err
	}
	for _, question := range sessionevents.Questions(events) {
		if question.ID == id {
			return question, nil
		}
	}
	return sessionevents.ACPPermission{}, fmt.Errorf("question not found in this thread: %s", id)
}
