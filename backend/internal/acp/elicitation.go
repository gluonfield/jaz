package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/gluonfield/acp-transport/jsonrpc"
	"github.com/wins/jaz/backend/internal/sessionevents"
)

type elicitationAnswerField struct {
	Field       string
	CustomField string
	Multi       bool
	Options     map[string]bool
}

func (m *Manager) createElicitation(ctx context.Context, raw json.RawMessage) (json.RawMessage, *jsonrpc.Error) {
	var req acpschema.CreateElicitationRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, jsonrpc.InvalidParams("invalid elicitation/create", map[string]any{"error": err.Error()})
	}
	if req.SessionID == nil {
		return nil, jsonrpc.InvalidParams("missing acp session", nil)
	}
	sessionID := string(*req.SessionID)
	job := m.jobByACP(sessionID)
	if job == nil {
		return nil, jsonrpc.InvalidParams("unknown acp session", nil)
	}
	if req.Mode != "form" {
		return jsonrpc.EncodeResult(acpschema.CreateElicitationResponse{Action: "decline"})
	}
	questions, fields := elicitationQuestions(req.Message, req.RequestedSchema)
	if len(questions) == 0 {
		return jsonrpc.EncodeResult(acpschema.CreateElicitationResponse{Action: "decline"})
	}

	toolCallID := ""
	if req.ToolCallID != nil {
		toolCallID = string(*req.ToolCallID)
	}
	permission := sessionevents.ACPPermission{
		ID:         fmt.Sprintf("perm-%d", atomicAddPermission(&m.permissionSeq)),
		SessionID:  sessionID,
		Title:      "Clarifying questions",
		ToolCallID: toolCallID,
		Questions:  questions,
		Status:     "pending",
	}
	pending := &pendingPermission{
		sessionID:     job.ID,
		request:       permission,
		encodeAnswers: elicitationAnswerEncoder(fields),
		answer:        make(chan string, 1),
	}
	if !m.registerPendingPermission(job, pending) {
		return jsonrpc.EncodeResult(acpschema.CreateElicitationResponse{Action: "cancel"})
	}

	m.setJobPermission(job, permission)
	m.publishPermission(job, permission, "permission_request")

	select {
	case raw := <-pending.answer:
		if raw == "" {
			return jsonrpc.EncodeResult(acpschema.CreateElicitationResponse{Action: "cancel"})
		}
		return jsonrpc.EncodeResult(json.RawMessage(raw))
	case <-ctx.Done():
		m.removePendingPermission(permission.ID)
		m.removeJobPermission(job, permission.ID)
		permission.Status = "cancelled"
		m.publishPermission(job, permission, "permission_response")
		return jsonrpc.EncodeResult(acpschema.CreateElicitationResponse{Action: "cancel"})
	}
}

func elicitationQuestions(message string, schema *acpschema.ElicitationSchema) ([]sessionevents.ACPQuestion, map[string]elicitationAnswerField) {
	if schema == nil {
		return nil, nil
	}
	keys := make([]string, 0, len(schema.Properties))
	for key := range schema.Properties {
		if strings.HasSuffix(key, "_custom") {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	questions := make([]sessionevents.ACPQuestion, 0, len(keys))
	fields := make(map[string]elicitationAnswerField, len(keys))
	for _, key := range keys {
		property := schema.Properties[key]
		options := elicitationOptions(property)
		questionText := firstNonEmpty(strings.TrimSpace(property.Description), strings.TrimSpace(property.Title), strings.TrimSpace(message), key)
		if len(keys) == 1 {
			questionText = firstNonEmpty(strings.TrimSpace(message), questionText)
		}
		if questionText == "" {
			continue
		}
		optionSet := make(map[string]bool, len(options))
		for _, option := range options {
			optionSet[option.Label] = true
		}
		customKey := key + "_custom"
		_, hasCustom := schema.Properties[customKey]
		customField := ""
		if hasCustom {
			customField = customKey
		}
		questions = append(questions, sessionevents.ACPQuestion{
			ID:       key,
			Header:   strings.TrimSpace(property.Title),
			Question: questionText,
			IsOther:  hasCustom || len(options) == 0,
			Options:  options,
		})
		fields[key] = elicitationAnswerField{
			Field:       key,
			CustomField: customField,
			Multi:       property.Type == "array",
			Options:     optionSet,
		}
	}
	return questions, fields
}

func elicitationOptions(property acpschema.ElicitationPropertySchema) []sessionevents.ACPQuestionOption {
	rawOptions := property.OneOf
	if len(rawOptions) == 0 {
		rawOptions = enumOptions(property.Enum)
	}
	if len(rawOptions) == 0 && property.Items != nil {
		rawOptions = property.Items.AnyOf
		if len(rawOptions) == 0 {
			rawOptions = enumOptions(property.Items.Enum)
		}
	}
	out := make([]sessionevents.ACPQuestionOption, 0, len(rawOptions))
	for _, option := range rawOptions {
		label := strings.TrimSpace(firstNonEmpty(option.Const, option.Title))
		if label == "" {
			continue
		}
		out = append(out, sessionevents.ACPQuestionOption{
			Label:       label,
			Description: elicitationOptionDescription(option),
		})
	}
	return out
}

func enumOptions(values []string) []acpschema.EnumOption {
	options := make([]acpschema.EnumOption, 0, len(values))
	for _, value := range values {
		options = append(options, acpschema.EnumOption{Const: value, Title: value})
	}
	return options
}

func elicitationOptionDescription(option acpschema.EnumOption) string {
	if value, ok := option.Meta["_claude/askUserQuestionOption"].(map[string]any); ok {
		if description, ok := value["description"].(string); ok {
			return strings.TrimSpace(description)
		}
	}
	if option.Title == "" || option.Title == option.Const {
		return ""
	}
	prefix := option.Const + " — "
	if strings.HasPrefix(option.Title, prefix) {
		return strings.TrimSpace(strings.TrimPrefix(option.Title, prefix))
	}
	return strings.TrimSpace(option.Title)
}

func encodeElicitationResponse(fields map[string]elicitationAnswerField, answers map[string]InteractiveAnswerValue) (string, error) {
	content := map[string]acpschema.ElicitationContentValue{}
	for id, answer := range answers {
		field, ok := fields[id]
		if !ok {
			continue
		}
		values := trimmedAnswers(answer.Answers)
		if len(values) == 0 {
			continue
		}
		if field.Multi {
			raw, err := elicitationContentValue(values)
			if err != nil {
				return "", err
			}
			content[field.Field] = raw
			continue
		}
		value := values[0]
		if field.CustomField != "" && !field.Options[value] {
			raw, err := elicitationContentValue(value)
			if err != nil {
				return "", err
			}
			content[field.CustomField] = raw
			continue
		}
		raw, err := elicitationContentValue(value)
		if err != nil {
			return "", err
		}
		content[field.Field] = raw
	}
	raw, err := json.Marshal(acpschema.CreateElicitationResponse{Action: "accept", Content: content})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func elicitationAnswerEncoder(fields map[string]elicitationAnswerField) answerEncoder {
	return func(answers map[string]InteractiveAnswerValue) (string, error) {
		return encodeElicitationResponse(fields, answers)
	}
}

func elicitationContentValue(value any) (acpschema.ElicitationContentValue, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return acpschema.ElicitationContentValue(raw), nil
}
