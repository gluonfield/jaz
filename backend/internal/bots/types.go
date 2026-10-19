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
	// mainName is what the user's main bot is called when Jaz creates it.
	mainName = "Jaz"
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
	Prompt          string    `json:"prompt,omitempty"`
	Avatar          Avatar    `json:"avatar"`
	Pinned          int       `json:"pinned,omitempty"`
	Main            bool      `json:"main,omitempty"`
	Unread          bool      `json:"unread"`
	Status          string    `json:"status"`
	Preview         string    `json:"preview,omitempty"`
	CompletedAt     time.Time `json:"completed_at,omitzero"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	Agent           string    `json:"agent,omitempty"`
	Model           string    `json:"model,omitempty"`
	ReasoningEffort string    `json:"reasoning_effort,omitempty"`
	Members         []string  `json:"members,omitempty"`
	// Working lists the members taking a turn in a group.
	Working []string `json:"working,omitempty"`
	// Waiting is set while the bot's thread holds a question or plan decision
	// for the user.
	Waiting bool `json:"waiting,omitempty"`
	// Thinking is set while the bot's turn is under way and has called no tool
	// yet.
	Thinking bool `json:"thinking,omitempty"`
	// Away is set while the bot's routines wait for the user to come back.
	Away bool `json:"away,omitempty"`
	// The bot's context fill as its agent last reported it; the fill is
	// unknown while either is zero.
	ContextTokens       int64 `json:"context_tokens,omitempty"`
	ContextWindowTokens int64 `json:"context_window_tokens,omitempty"`
}

type CreateBot struct {
	Name   string  `json:"name"`
	Prompt string  `json:"prompt,omitempty"`
	Avatar *Avatar `json:"avatar,omitempty"`
	Agent  string  `json:"agent,omitempty"`
	Model  string  `json:"model,omitempty"`
	// StoredModel marks Model as saved earlier, as an adopted routine's is.
	StoredModel bool `json:"-"`
	Main        bool `json:"-"`
}

type UpdateBot struct {
	Name    *string   `json:"name,omitempty"`
	Prompt  *string   `json:"prompt,omitempty"`
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
	SteerInternal(ctx context.Context, session, message string) (acp.Job, error)
	SwitchAgent(ctx context.Context, sessionID, agent string) error
	SetModel(ctx context.Context, sessionID, model, effort string) error
	AwaitsAnswer(sessionID string) bool
	Thinking(sessionID string) bool
}

type TurnQueue interface {
	QueueInternalTurn(context.Context, storage.TurnDelivery) error
}

// Attachments finds the files uploaded to a thread by their ids.
type Attachments interface {
	ResolveAttachments(threadID string, ids []string) ([]storage.Attachment, error)
}

// Store keeps bot records and the threads they live in.
type Store interface {
	PromptStore
	CommitmentStore
	SaveBot(storage.BotRecord) error
	ListBots() ([]storage.BotRecord, error)
	PinBots(ids []string) error
	CreateSession(storage.CreateSession) (storage.Session, error)
	AppendTurnReply(id, message string, done []int64) error
	ListSessions(storage.SessionFilter) ([]storage.Session, error)
	UpdateSessionTitle(id, title string) error
	SetArchived(id string, archived bool) error
	SetThreadUnread(id string, unread bool) error
	LoadSessionEvents(id string) ([]sessionevents.Event, error)
	AppendSessionEvents(id string, events ...sessionevents.Event) error
	LoadLatestSessionEvent(id, eventType string) (sessionevents.Event, bool, error)
	LoadBotActivity(id string, since time.Time) ([]sessionevents.Event, error)
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
	Runs(loopID string, limit int) ([]loops.Run, error)
	Create(loops.CreateLoop) (loops.Loop, error)
	Update(string, loops.UpdateLoop) (loops.Loop, error)
	Delete(string) error
	OnBoard(loops.Loop) bool
	HeldBots() (map[string]bool, error)
}

type Publisher interface {
	Publish(sessionevents.Event)
}

// EventStore keeps a thread's events.
type EventStore interface {
	AppendSessionEvents(id string, events ...sessionevents.Event) error
}
