package jazagent

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/agent"
	"github.com/wins/jaz/backend/internal/media"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/sessioncontext"
	"github.com/wins/jaz/backend/internal/storage"
)

const VoiceModeNote = "Voice mode: the user spoke this message aloud and your final reply will be read out by text-to-speech. Keep the final response to a few short conversational sentences of plain prose — no markdown, lists, headings, or code blocks. Using tools is fine."

const PlanModeNote = `<collaboration_mode># Plan Mode

You are in Plan Mode until this turn ends. Plan Mode is a collaboration mode for producing an approval-ready implementation plan, separate from tool permissions or autonomy.

## Rules

- You may read/search/inspect and run non-mutating checks that improve the plan.
- You must not edit files, apply patches, run codegen/formatters that rewrite tracked files, or otherwise execute the plan.
- If the user asks you to implement while still in Plan Mode, plan the implementation instead of doing it.
- A final plan must be decision-complete: another engineer or agent should be able to implement it without making design choices.

## Proposing The Plan

When you are ready to present the official plan, call update_plan with the proposed plan. In Plan Mode, update_plan creates the approval surface shown to the user. Do not duplicate the full plan in normal assistant text after calling the tool.

The proposed plan should include a clear title or summary, important API/interface/type changes, concrete implementation steps, tests/scenarios, and explicit assumptions/defaults where needed. Do not ask "should I proceed?" in the final output; the user can approve the plan from the client.
</collaboration_mode>`

const PlanUserInstruction = "Plan mode is enabled for this turn. Use the update_plan tool to present the proposed plan when it is ready for approval."

type Store interface {
	storage.MessageStore
}

type PromptSource interface {
	SystemPromptForWorkspace(string) (string, error)
}

type Runner struct {
	Agent   *agent.Agent
	Store   Store
	Prompts PromptSource
	Log     *log.Logger
}

type Request struct {
	Session       storage.Session
	Message       string
	Attachments   []storage.Attachment
	VoiceMode     bool
	PlanRequested bool
	AppendUser    bool
}

type TurnRequest struct {
	Messages         []provider.Message
	MediaRefs        map[string][]media.Ref
	Transient        []string
	userMessageIndex int
	displayUser      provider.Message
}

func (r TurnRequest) StorageSnapshot(messages []provider.Message) []provider.Message {
	if r.userMessageIndex < 0 || r.userMessageIndex >= len(messages) {
		return messages
	}
	out := append([]provider.Message(nil), messages...)
	out[r.userMessageIndex] = r.displayUser
	return out
}

func (r *Runner) Run(ctx context.Context, req Request) <-chan agent.StreamEvent {
	out := make(chan agent.StreamEvent)
	go func() {
		defer close(out)
		r.run(ctx, req, out)
	}()
	return out
}

func (r *Runner) run(ctx context.Context, req Request, out chan<- agent.StreamEvent) {
	if r.Agent == nil {
		emit(out, agent.StreamEvent{Type: agent.StreamError, Error: "agent is not configured"})
		emit(out, agent.StreamEvent{Type: agent.StreamDone})
		return
	}
	turn, err := BuildRequest(r.Store, r.Prompts, req)
	if err != nil {
		emit(out, agent.StreamEvent{Type: agent.StreamError, Error: err.Error()})
		emit(out, agent.StreamEvent{Type: agent.StreamDone})
		return
	}
	runCtx := sessioncontext.WithSessionID(ctx, req.Session.ID)
	if req.Session.RuntimeRef != nil && strings.TrimSpace(req.Session.RuntimeRef.Cwd) != "" {
		runCtx = sessioncontext.WithCWD(runCtx, req.Session.RuntimeRef.Cwd)
	}
	if req.PlanRequested {
		runCtx = sessioncontext.WithCollaborationMode(runCtx, sessioncontext.CollaborationModePlan)
	}
	for event := range r.Agent.Run(runCtx, provider.Request{
		Provider:        req.Session.ModelProvider,
		Model:           req.Session.Model,
		ReasoningEffort: req.Session.ReasoningEffort,
		Messages:        turn.Messages,
		MediaRefs:       turn.MediaRefs,
	}) {
		if len(event.Messages) > 0 {
			if err := SaveSnapshot(r.Store, req.Session.ID, turn, event); err != nil {
				if r.Log != nil {
					r.Log.Error("saving jaz agent turn messages failed", "session", req.Session.ID, "error", err)
				}
				emit(out, agent.StreamEvent{Type: agent.StreamError, Error: err.Error()})
				emit(out, agent.StreamEvent{Type: agent.StreamDone})
				return
			}
		}
		emit(out, event)
	}
}

func BuildRequest(store Store, prompts PromptSource, req Request) (TurnRequest, error) {
	if store == nil {
		return TurnRequest{}, fmt.Errorf("message store is not configured")
	}
	messages, err := store.LoadMessages(req.Session.ID)
	if err != nil {
		return TurnRequest{}, err
	}
	mediaRefs, err := MediaRefs(store, req.Session.ID)
	if err != nil {
		return TurnRequest{}, err
	}
	for len(messages) > 0 && messages[0].OfSystem != nil {
		messages = messages[1:]
	}
	displayUser := provider.UserMessage(req.Message)
	if !req.AppendUser {
		var ok bool
		messages, displayUser, ok = pullLastStoredUser(messages, req.Message)
		if !ok {
			displayUser = provider.UserMessage(req.Message)
		}
	}
	transient := make([]string, 0, 2)
	if prompts != nil {
		workspace := ""
		if req.Session.RuntimeRef != nil && strings.TrimSpace(req.Session.RuntimeRef.Cwd) != "" {
			workspace = req.Session.RuntimeRef.Cwd
		}
		prompt, err := prompts.SystemPromptForWorkspace(workspace)
		if err != nil {
			return TurnRequest{}, fmt.Errorf("build system prompt: %w", err)
		}
		if prompt := strings.TrimSpace(prompt); prompt != "" {
			messages = append([]provider.Message{provider.SystemMessage(prompt)}, messages...)
			transient = append(transient, prompt)
		}
	}
	if req.VoiceMode {
		messages = append(messages, provider.SystemMessage(VoiceModeNote))
		transient = append(transient, VoiceModeNote)
	}
	if req.PlanRequested {
		messages = append(messages, provider.DeveloperMessage(PlanModeNote))
		transient = append(transient, PlanModeNote)
	}
	userMessageIndex := len(messages)
	userPrompt := MessageWithAttachmentLinks(req.Message, req.Attachments)
	if req.PlanRequested {
		userPrompt = strings.TrimSpace(userPrompt + "\n\n" + PlanUserInstruction)
	}
	messages = append(messages, provider.UserMessage(userPrompt))
	if req.AppendUser {
		if err := storage.AppendUserMessage(store, req.Session.ID, req.Message, req.Attachments); err != nil {
			return TurnRequest{}, err
		}
	}
	return TurnRequest{
		Messages:         messages,
		MediaRefs:        mediaRefs,
		Transient:        transient,
		userMessageIndex: userMessageIndex,
		displayUser:      displayUser,
	}, nil
}

func SaveSnapshot(store Store, sessionID string, turn TurnRequest, event agent.StreamEvent) error {
	toSave, reasoning := StripTransientSystem(turn.StorageSnapshot(event.Messages), event.ReasoningByMessage, turn.Transient)
	if store, ok := store.(mediaReasoningMessageStore); ok && (len(reasoning) > 0 || len(event.MediaRefs) > 0) {
		return store.SaveMessagesWithReasoningAndMedia(sessionID, toSave, reasoning, event.MediaRefs)
	}
	if store, ok := store.(reasoningMessageStore); ok && len(reasoning) > 0 {
		return store.SaveMessagesWithReasoning(sessionID, toSave, reasoning)
	}
	return store.SaveMessages(sessionID, toSave)
}

func StripTransientSystem(messages []provider.Message, reasoning map[int]string, injected []string) ([]provider.Message, map[int]string) {
	if len(injected) == 0 {
		return messages, reasoning
	}
	drop := make(map[int]bool)
	for i, msg := range messages {
		if msg.OfSystem == nil && msg.OfDeveloper == nil {
			continue
		}
		if slices.Contains(injected, provider.MessageContent(msg)) {
			drop[i] = true
		}
	}
	if len(drop) == 0 {
		return messages, reasoning
	}
	out := make([]provider.Message, 0, len(messages)-len(drop))
	remapped := make(map[int]string, len(reasoning))
	for i, msg := range messages {
		if drop[i] {
			continue
		}
		if text, ok := reasoning[i]; ok {
			remapped[len(out)] = text
		}
		out = append(out, msg)
	}
	if len(reasoning) == 0 {
		return out, reasoning
	}
	return out, remapped
}

func MediaRefs(store Store, sessionID string) (map[string][]media.Ref, error) {
	recordStore, ok := store.(messageRecordStore)
	if !ok {
		return nil, nil
	}
	records, err := recordStore.LoadMessageRecords(sessionID)
	if err != nil {
		return nil, err
	}
	return storage.MediaRefsByToolCall(records), nil
}

func MessageWithAttachmentLinks(message string, attachments []storage.Attachment) string {
	if len(attachments) == 0 {
		return message
	}
	var b strings.Builder
	b.WriteString(message)
	b.WriteString("\n\nAttachments:\n")
	for _, attachment := range attachments {
		fmt.Fprintf(&b, "- %s: %s\n", attachment.Name, attachment.URI)
	}
	return strings.TrimRight(b.String(), "\n")
}

type messageRecordStore interface {
	LoadMessageRecords(string) ([]storage.Message, error)
}

type reasoningMessageStore interface {
	SaveMessagesWithReasoning(string, []provider.Message, map[int]string) error
}

type mediaReasoningMessageStore interface {
	SaveMessagesWithReasoningAndMedia(string, []provider.Message, map[int]string, map[string][]media.Ref) error
}

func pullLastStoredUser(messages []provider.Message, message string) ([]provider.Message, provider.Message, bool) {
	if len(messages) == 0 {
		return messages, provider.Message{}, false
	}
	last := messages[len(messages)-1]
	if provider.MessageRole(last) != "user" {
		return messages, provider.Message{}, false
	}
	if strings.TrimSpace(provider.MessageContent(last)) != strings.TrimSpace(message) {
		return messages, provider.Message{}, false
	}
	return messages[:len(messages)-1], last, true
}

func emit(out chan<- agent.StreamEvent, event agent.StreamEvent) {
	out <- event
}
