package memory

import (
	"context"
	"errors"
	"time"

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

type FileTool struct {
	Memory *jazmem.Memory
}

type ReindexTool struct {
	Memory *jazmem.Memory
}

type DreamTool struct {
	Memory *jazmem.Memory
}

type LinkHygieneTool struct {
	Memory *jazmem.Memory
}

type searchInput struct {
	Query string `json:"query" jsonschema_description:"Full-text memory search query."`
	Limit int    `json:"limit,omitempty" jsonschema_description:"Maximum results to return. Defaults to 10, capped at 50."`
}

type getInput struct {
	Slug string `json:"slug" jsonschema_description:"Markdown page slug, for example people/alice or inbox/2026-06-08-note."`
}

type fileInput struct {
	Slug string `json:"slug" jsonschema_description:"Markdown page slug, for example people/alice. Returns the absolute markdown file path."`
}

type reindexInput struct{}

type dreamInput struct {
	Date string `json:"date,omitempty" jsonschema_description:"Optional dream date in YYYY-MM-DD format. Defaults to today."`
}

type linkHygieneInput struct{}

func (t *SearchTool) Definition() tools.Definition {
	return tools.Function("mem_search", "Search canonical markdown memory using the rebuildable SQLite FTS index. Returns compact ranked chunks; it does not synthesize an answer.", true, helpers.GenerateSchema[searchInput]())
}

func (t *SearchTool) Execute(ctx context.Context, inputs map[string]any) (tools.Result, error) {
	req, err := helpers.DecodeMap[searchInput](inputs)
	if err != nil {
		return tools.Result{}, err
	}
	results, err := t.Memory.Retrieve(ctx, req.Query, jazmem.SearchOptions{Limit: req.Limit})
	if err != nil {
		return tools.Result{}, err
	}
	return tools.JSONResult(results)
}

func (t *GetTool) Definition() tools.Definition {
	return tools.Function("mem_get", "Read a canonical markdown memory page by slug.", true, helpers.GenerateSchema[getInput]())
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
			return tools.JSONResult(map[string]any{
				"error":       notFound.Error(),
				"suggestions": notFound.Suggestions,
			})
		}
		return tools.Result{}, err
	}
	return tools.JSONResult(page)
}

func (t *FileTool) Definition() tools.Definition {
	return tools.Function("mem_file", "Resolve a canonical markdown memory slug to its absolute file path before reading or editing the raw markdown file.", true, helpers.GenerateSchema[fileInput]())
}

func (t *FileTool) Execute(ctx context.Context, inputs map[string]any) (tools.Result, error) {
	req, err := helpers.DecodeMap[fileInput](inputs)
	if err != nil {
		return tools.Result{}, err
	}
	page, err := t.Memory.GetPage(ctx, req.Slug)
	if err != nil {
		var notFound *jazmem.NotFoundError
		if errors.As(err, &notFound) {
			return tools.JSONResult(map[string]any{
				"error":       notFound.Error(),
				"suggestions": notFound.Suggestions,
			})
		}
		return tools.Result{}, err
	}
	return tools.JSONResult(jazmem.PageRef{Slug: page.Slug, Path: page.Path})
}

func (t *ReindexTool) Definition() tools.Definition {
	return tools.Function("mem_reindex", "Rebuild the SQLite memory index from canonical markdown files.", true, helpers.GenerateSchema[reindexInput]())
}

func (t *ReindexTool) Execute(ctx context.Context, inputs map[string]any) (tools.Result, error) {
	if _, err := helpers.DecodeMap[reindexInput](inputs); err != nil {
		return tools.Result{}, err
	}
	report, err := t.Memory.Reindex(ctx, jazmem.ReindexOptions{})
	if err != nil {
		return tools.Result{}, err
	}
	return tools.JSONResult(report)
}

func (t *DreamTool) Definition() tools.Definition {
	return tools.Function("mem_dream", "Run deterministic jazmem dream consolidation scaffolding and write a dream run markdown page.", true, helpers.GenerateSchema[dreamInput]())
}

func (t *DreamTool) Execute(ctx context.Context, inputs map[string]any) (tools.Result, error) {
	req, err := helpers.DecodeMap[dreamInput](inputs)
	if err != nil {
		return tools.Result{}, err
	}
	var date time.Time
	if req.Date != "" {
		date, err = time.Parse("2006-01-02", req.Date)
		if err != nil {
			return tools.Result{}, err
		}
	}
	report, err := t.Memory.Dream(ctx, jazmem.DreamOptions{Date: date})
	if err != nil {
		return tools.Result{}, err
	}
	return tools.JSONResult(report)
}

func (t *LinkHygieneTool) Definition() tools.Definition {
	return tools.Function("mem_link_hygiene", "Scan derived memory links and write relationship proposals to a review markdown page. It does not edit canonical entity pages.", true, helpers.GenerateSchema[linkHygieneInput]())
}

func (t *LinkHygieneTool) Execute(ctx context.Context, inputs map[string]any) (tools.Result, error) {
	if _, err := helpers.DecodeMap[linkHygieneInput](inputs); err != nil {
		return tools.Result{}, err
	}
	report, err := t.Memory.LinkHygiene(ctx)
	if err != nil {
		return tools.Result{}, err
	}
	return tools.JSONResult(report)
}
