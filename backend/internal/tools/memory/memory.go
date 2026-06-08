package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/wins/jaz/backend/internal/helpers"
	"github.com/wins/jaz/backend/internal/tools"
	"github.com/wins/jazmem/pkg/jazmem"
)

type SearchTool struct {
	Memory *jazmem.Memory
}

type GetTool struct {
	Memory *jazmem.Memory
}

type searchInput struct {
	Query string `json:"query" jsonschema_description:"Question or topic to answer from jazmem memory."`
}

type getInput struct {
	Slug string `json:"slug" jsonschema_description:"Markdown page slug, for example people/alice or inbox/2026-06-08-note."`
}

func (t *SearchTool) Definition() tools.Definition {
	return tools.Function("mem_search", "Search jazmem and synthesize an evidence-grounded answer with citations and gaps. Use this as the default memory search tool.", true, helpers.GenerateSchema[searchInput]())
}

func (t *SearchTool) Execute(ctx context.Context, inputs map[string]any) (tools.Result, error) {
	req, err := helpers.DecodeMap[searchInput](inputs)
	if err != nil {
		return tools.Result{}, err
	}
	result, err := t.Memory.AgenticSearch(ctx, req.Query, jazmem.AgenticOptions{})
	if err != nil {
		return tools.Result{}, err
	}
	return tools.JSONResult(result)
}

func (t *GetTool) Definition() tools.Definition {
	return tools.Function("mem_get", "Read a jazmem markdown page by slug. Returns raw markdown content; if missing, returns similar slug suggestions.", true, helpers.GenerateSchema[getInput]())
}

func (t *GetTool) Execute(ctx context.Context, inputs map[string]any) (tools.Result, error) {
	req, err := helpers.DecodeMap[getInput](inputs)
	if err != nil {
		return tools.Result{}, err
	}
	page, err := t.Memory.GetPage(ctx, req.Slug)
	if err != nil {
		var notFound *jazmem.NotFoundError
		if errors.As(err, &notFound) {
			return tools.Result{Content: notFoundText(notFound)}, nil
		}
		return tools.Result{}, err
	}
	return tools.Result{Content: page.Raw}, nil
}

func notFoundText(notFound *jazmem.NotFoundError) string {
	var b strings.Builder
	b.WriteString(notFound.Error())
	if len(notFound.Suggestions) == 0 {
		return b.String()
	}
	b.WriteString("\nsuggestions:")
	for _, suggestion := range notFound.Suggestions {
		if suggestion.Title == "" {
			fmt.Fprintf(&b, "\n- %s", suggestion.Slug)
			continue
		}
		fmt.Fprintf(&b, "\n- %s (%s)", suggestion.Slug, suggestion.Title)
	}
	return b.String()
}
