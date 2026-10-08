package bots

import (
	"context"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

const (
	KindBot   = "bot"
	KindGroup = "group"
)

var (
	shapes = []string{"circle", "blob", "squircle", "pill", "triangle", "hex", "cloud", "drop"}
	colors = []string{"white", "brown", "red", "orange", "amber", "green", "teal", "blue", "purple", "pink", "gray"}
)

type Avatar struct {
	Shape string `json:"shape"`
	Color string `json:"color"`
}

type Bot struct {
	ID              string    `json:"id"`
	Kind            string    `json:"kind"`
	Name            string    `json:"name"`
	Avatar          Avatar    `json:"avatar"`
	Pinned          int       `json:"pinned,omitempty"`
	Unread          bool      `json:"unread"`
	Status          string    `json:"status"`
	Preview         string    `json:"preview,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
	Agent           string    `json:"agent,omitempty"`
	Model           string    `json:"model,omitempty"`
	ReasoningEffort string    `json:"reasoning_effort,omitempty"`
	Members         []string  `json:"members,omitempty"`
	// Working lists the members taking a turn in a group.
	Working []string `json:"working,omitempty"`
}

type CreateBot struct {
	Name   string  `json:"name"`
	Avatar *Avatar `json:"avatar,omitempty"`
	Agent  string  `json:"agent,omitempty"`
	Model  string  `json:"model,omitempty"`
}

type UpdateBot struct {
	Name    *string   `json:"name,omitempty"`
	Avatar  *Avatar   `json:"avatar,omitempty"`
	Members *[]string `json:"members,omitempty"`
	// Agent moves the bot to another agent, which starts a fresh native session.
	Agent *string `json:"agent,omitempty"`
	// Model picks the model the bot's agent runs with, together with
	// ReasoningEffort.
	Model           *string `json:"model,omitempty"`
	ReasoningEffort string  `json:"reasoning_effort,omitempty"`
}

// Threads is the agent runtime bot threads run on.
type Threads interface {
	CreateSession(context.Context, acp.SpawnRequest) (storage.Session, error)
	StartInternalTurnWhenIdle(context.Context, acp.InternalTurnRequest) (acp.Job, error)
	SteerInternal(ctx context.Context, session, message string) (acp.Job, error)
	Wait(context.Context, acp.WaitRequest) (acp.Job, error)
	SwitchAgent(ctx context.Context, sessionID, agent string) error
	SetModel(ctx context.Context, sessionID, model, effort string) error
}

type TurnQueue interface {
	QueueInternalTurn(context.Context, string, storage.QueuedMessage) error
}

// Attachments finds the files uploaded to a thread by their ids.
type Attachments interface {
	ResolveAttachments(threadID string, ids []string) ([]storage.Attachment, error)
}

// Store keeps bot records and the threads they live in.
type Store interface {
	PromptStore
	SaveBot(storage.BotRecord) error
	ListBots() ([]storage.BotRecord, error)
	PinBots(ids []string) error
	CreateSession(storage.CreateSession) (storage.Session, error)
	AppendTurnReply(id, message string) error
	ListSessions(storage.SessionFilter) ([]storage.Session, error)
	UpdateSessionTitle(id, title string) error
	SetArchived(id string, archived bool) error
	LoadSessionEvents(id string) ([]sessionevents.Event, error)
	AppendSessionEvents(id string, events ...sessionevents.Event) error
	LoadLatestSessionEvent(id, eventType string) (sessionevents.Event, bool, error)
	SaveMembership(storage.BotMembership) error
	LoadMembership(groupID, botID string) (storage.BotMembership, error)
	ListMemberships() ([]storage.BotMembership, error)
}

// PromptStore is what a bot's identity prompt is read from.
type PromptStore interface {
	LoadBot(threadID string) (storage.BotRecord, error)
	LoadSession(string) (storage.Session, error)
	LoadMembershipByThread(threadID string) (storage.BotMembership, error)
}

// Routines is the loop service seen as a bot's routines.
type Routines interface {
	List() ([]loops.Loop, error)
	Update(string, loops.UpdateLoop) (loops.Loop, error)
	Delete(string) error
	OnBoard(loops.Loop) bool
}

type Publisher interface {
	Publish(sessionevents.Event)
}
