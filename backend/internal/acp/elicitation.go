package acp

import (
	"context"
	"encoding/json"
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
		ID:         newPermissionID(),
		SessionID:  sessionID,
		Title:      "Clarifying questions",
		ToolCallID: toolCallID,
		Questions:  questions,
		Status:     "pending",
	}
	answer := m.awaitPermissionAnswer(ctx, job, permission, nil)
	if answer.Answers == nil {
		return jsonrpc.EncodeResult(acpschema.CreateElicitationResponse{Action: "cancel"})
	}
	rawAnswer, err := encodeElicitationResponse(fields, answer.Answers)
	if err != nil {
		return nil, jsonrpc.InternalError("encode elicitation answers", map[string]any{"error": err.Error()})
	}
	return jsonrpc.EncodeResult(json.RawMessage(rawAnswer))
}

func elicitationQuestions(message string, schema *acpschema.ElicitationSchema) ([]sessionevents.ACPQuestion, map[string]elicitationAnswerField) {
	if schema == nil {
		return nil, nil
	}
	parsed := make(map[string]elicitationField, len(schema.Properties))
	customFields := map[string]string{}
	keys := make([]string, 0, len(schema.Properties))
	for key, property := range schema.Properties {
		field := readElicitationField(property, message)
		if field.note {
			if field.noteFor != "" {
				customFields[field.noteFor] = key
			}
			continue
		}
		parsed[key] = field
		keys = append(keys, key)
	}
	sort.Strings(keys)
	filtered := keys[:0]
	for _, key := range keys {
		if base, ok := companionOtherBase(key, schema.Properties, parsed, customFields); ok {
			customFields[base] = key
			if baseField := parsed[base]; baseField.header != "" || baseField.question != "" {
				baseField.message = ""
				parsed[base] = baseField
			}
			delete(parsed, key)
			continue
		}
		filtered = append(filtered, key)
	}
	keys = filtered

	questions := make([]sessionevents.ACPQuestion, 0, len(keys))
	fields := make(map[string]elicitationAnswerField, len(keys))
	for _, key := range keys {
		field := parsed[key]
		property := schema.Properties[key]
		options := elicitationOptions(property)
		questionText := firstNonEmpty(field.question, field.header, field.message, key)
		if len(keys) == 1 {
			questionText = firstNonEmpty(field.message, questionText)
		}
		if questionText == "" {
			continue
		}
		optionSet := make(map[string]bool, len(options))
		for _, option := range options {
			optionSet[option.Label] = true
		}
		customField := customFields[key]
		questions = append(questions, sessionevents.ACPQuestion{
			ID:          key,
			Header:      field.header,
			Question:    questionText,
			IsOther:     customField != "" || len(options) == 0,
			IsSecret:    field.secret,
			MultiSelect: property.Type == "array",
			Options:     options,
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

func companionOtherBase(
	key string,
	properties map[string]acpschema.ElicitationPropertySchema,
	parsed map[string]elicitationField,
	customFields map[string]string,
) (string, bool) {
	base, ok := strings.CutSuffix(key, "_other")
	if !ok || base == "" || customFields[base] != "" {
		return "", false
	}
	if _, exists := parsed[base]; !exists {
		return "", false
	}
	prop := properties[key]
	if (prop.Type != "" && prop.Type != "string") || len(elicitationOptions(prop)) > 0 {
		return "", false
	}
	if len(elicitationOptions(properties[base])) == 0 {
		return "", false
	}
	return base, true
}

type elicitationField struct {
	header, question, message string
	note                      bool
	noteFor                   string
	secret                    bool
}

// readElicitationField reads one requested property in its agent's dialect.
// Claude titles a field with its header, asks a lone question in the form
// message and marks free text with _meta.jetbrains.air.customAnswer.
// Codex titles a field with the question, describes it with the header, sends a
// boilerplate form message and marks note fields in _meta.codex.
func readElicitationField(property acpschema.ElicitationPropertySchema, message string) elicitationField {
	field := elicitationField{
		header:   strings.TrimSpace(property.Title),
		question: strings.TrimSpace(property.Description),
		message:  strings.TrimSpace(message),
	}
	jetbrains, _ := property.Meta["jetbrains"].(map[string]any)
	air, _ := jetbrains["air"].(map[string]any)
	if custom, ok := air["customAnswer"].(map[string]any); ok {
		field.note = true
		field.noteFor, _ = custom["questionId"].(string)
		return field
	}
	codex, ok := property.Meta[codexMetaKey].(map[string]any)
	if !ok {
		return field
	}
	if codex["role"] == "user_note" {
		field.note = true
		field.noteFor, _ = codex["questionId"].(string)
		return field
	}
	field.header, field.question, field.message = field.question, field.header, ""
	field.secret = codex["isSecret"] == true
	return field
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
	if description := strings.TrimSpace(option.Description); description != "" {
		return description
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
		if field.CustomField != "" {
			choices := make([]string, 0, len(values))
			var custom []string
			for _, value := range values {
				if field.Options[value] {
					choices = append(choices, value)
				} else {
					custom = append(custom, value)
				}
			}
			if len(custom) > 0 {
				raw, err := elicitationContentValue(strings.Join(custom, "\n"))
				if err != nil {
					return "", err
				}
				content[field.CustomField] = raw
			}
			values = choices
			if len(values) == 0 {
				continue
			}
		}
		var value any = values[0]
		if field.Multi {
			value = values
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

func elicitationContentValue(value any) (acpschema.ElicitationContentValue, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return acpschema.ElicitationContentValue(raw), nil
}
