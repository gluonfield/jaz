package loops

import (
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
)

// LoopWriter is the slice of the loop service the coordinator needs; both
// *Service and the MCP service satisfy it.
type LoopWriter interface {
	Create(CreateLoop) (Loop, error)
	Update(string, UpdateLoop) (Loop, error)
}

// SessionEventAppender persists thread events; SessionEventPublisher streams
// them live. Together they let a create drop a "loop created" card into the
// thread that asked for it (same rails the visualize tools use).
type SessionEventAppender interface {
	AppendSessionEvents(id string, events ...sessionevents.Event) error
}

type SessionEventPublisher interface {
	Publish(event sessionevents.Event)
}

// CardSink persists and streams the loop_created card. Both fields optional.
type CardSink struct {
	Store SessionEventAppender
	Bus   SessionEventPublisher
}

// Coordinator is the single create/update-with-boards use case every surface
// (HTTP, MCP) runs through, so the steps never drift. Board ids are validated
// before the loop is written — a rejected request can't orphan a loop — and the
// loop_created card is announced on threadID when a sink is configured and the
// call came from a thread. Boards and Card are optional.
type Coordinator struct {
	Loops  LoopWriter
	Boards BoardService
	Card   CardSink
}

// Create validates boards, writes the loop, reconciles its board assignment,
// then announces it on threadID. threadID is empty for surfaces with no thread
// (e.g. the REST API), which simply skips the card.
func (c Coordinator) Create(in CreateLoop, boardIDs []string, threadID string) (Loop, error) {
	if err := c.validateBoards(boardIDs); err != nil {
		return Loop{}, err
	}
	loop, err := c.Loops.Create(in)
	if err != nil {
		return Loop{}, err
	}
	if len(boardIDs) > 0 {
		if err := c.assignBoards(loop, boardIDs); err != nil {
			return Loop{}, err
		}
	}
	c.announce(loop, boardIDs, threadID)
	return loop, nil
}

// Update writes the loop, then reassigns its boards only when boardIDs is
// non-nil (nil leaves assignments untouched; an empty slice clears them).
func (c Coordinator) Update(id string, in UpdateLoop, boardIDs *[]string) (Loop, error) {
	if boardIDs != nil {
		if err := c.validateBoards(*boardIDs); err != nil {
			return Loop{}, err
		}
	}
	loop, err := c.Loops.Update(id, in)
	if err != nil {
		return Loop{}, err
	}
	if boardIDs != nil {
		if err := c.assignBoards(loop, *boardIDs); err != nil {
			return Loop{}, err
		}
	}
	return loop, nil
}

func (c Coordinator) validateBoards(boardIDs []string) error {
	if c.Boards == nil || len(boardIDs) == 0 {
		return nil
	}
	return c.Boards.ValidateBoardIDs(boardIDs)
}

func (c Coordinator) assignBoards(loop Loop, boardIDs []string) error {
	if c.Boards == nil {
		return nil
	}
	return c.Boards.AssignLoopBoards(loop, boardIDs)
}

// announce emits the loop_created card. Best-effort: no thread, no sink, or a
// failed append simply skips the card without failing the create.
func (c Coordinator) announce(loop Loop, boardIDs []string, threadID string) {
	if c.Card.Store == nil || threadID == "" {
		return
	}
	events := []sessionevents.Event{{
		SessionID: threadID,
		Type:      sessionevents.TypeLoopCreated,
		LoopCreated: &sessionevents.LoopCreatedEvent{
			LoopID:    loop.ID,
			LoopName:  loop.Name,
			Schedule:  loop.Schedule.Expr,
			Timezone:  loop.Schedule.Timezone,
			NextRunAt: loop.NextRunAt,
			Agent:     loop.ACPAgent,
			Status:    loop.Status,
			Boards:    c.boardRefs(boardIDs),
		},
		At: time.Now().UTC(),
	}}
	if err := c.Card.Store.AppendSessionEvents(threadID, events...); err != nil {
		return
	}
	// AppendSessionEvents assigned Seq/At in place; publish the stored event.
	if c.Card.Bus != nil {
		c.Card.Bus.Publish(events[0])
	}
}

// boardRefs resolves the assigned board ids to id+name pairs for the card,
// preserving assignment order.
func (c Coordinator) boardRefs(boardIDs []string) []sessionevents.LoopBoardRef {
	if c.Boards == nil || len(boardIDs) == 0 {
		return nil
	}
	all, err := c.Boards.ListBoards()
	if err != nil {
		return nil
	}
	names := make(map[string]string, len(all))
	for _, board := range all {
		names[board.ID] = board.Name
	}
	refs := make([]sessionevents.LoopBoardRef, 0, len(boardIDs))
	for _, id := range boardIDs {
		refs = append(refs, sessionevents.LoopBoardRef{ID: id, Name: names[id]})
	}
	return refs
}
