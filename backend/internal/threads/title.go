package threads

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

type TitleStore interface {
	UpdateSessionTitleFromAgent(id, title string) (storage.Session, bool, error)
}

type TitleTools struct {
	Store  TitleStore
	Events *sessionevents.Bus
}

type TitleInput struct {
	Title string `json:"title" jsonschema:"The title to set for this thread."`
}

type TitleResult struct {
	Title                string `json:"title"`
	ManualTitlePreserved bool   `json:"manual_title_preserved"`
}

func (t *TitleTools) AddTo(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_thread_title",
		Description: "Set this thread's title once its topic is known and a skill or the user requests a specific title; preserves titles manually set by the user.",
	}, t.Set)
}

func (t *TitleTools) Set(ctx context.Context, req *mcp.CallToolRequest, input TitleInput) (*mcp.CallToolResult, TitleResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, TitleResult{}, err
	}
	id := mcpsession.SessionID(req)
	if id == "" {
		return nil, TitleResult{}, fmt.Errorf("set_thread_title requires a current Jaz thread")
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return nil, TitleResult{}, fmt.Errorf("title is required")
	}
	session, updated, err := t.Store.UpdateSessionTitleFromAgent(id, title)
	if err != nil {
		return nil, TitleResult{}, err
	}
	if updated {
		t.Events.Publish(sessionevents.Event{SessionID: session.ID, Type: sessionevents.TypeSession})
	}
	return nil, TitleResult{Title: session.Title, ManualTitlePreserved: session.ManualTitle}, nil
}
