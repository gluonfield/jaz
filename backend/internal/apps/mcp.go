package apps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/appbundle"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/pathsafe"
	"github.com/wins/jaz/backend/internal/storage"
)

type AppToolArgs struct {
	Title        string              `json:"title" jsonschema:"app name shown in the Apps library"`
	Description  string              `json:"description,omitempty" jsonschema:"one sentence on what the app does"`
	Kind         string              `json:"kind,omitempty" jsonschema:"artifact for HTML or SVG (default), document for Markdown"`
	Entry        string              `json:"entry,omitempty" jsonschema:"entry file of a multi-file app; defaults to index.html, or index.md for a document"`
	HTML         string              `json:"html,omitempty" jsonschema:"the artifact: raw SVG, HTML fragment, or full self-contained HTML document, exactly as show_artifact takes it"`
	Markdown     string              `json:"markdown,omitempty" jsonschema:"the document body, instead of html"`
	Files        map[string]string   `json:"files,omitempty" jsonschema:"a multi-file app as relative path to text content, instead of html"`
	Paths        []string            `json:"paths,omitempty" jsonschema:"files in this thread's working directory to bundle, by relative path, instead of inline content"`
	Capabilities []string            `json:"capabilities,omitempty" jsonschema:"only what the app uses. Saved at once: storage:personal, storage:shared. Held for the user's approval: ai:complete, tools:call, desktop:window, desktop:notifications, desktop:shortcuts, desktop:tray, desktop:always_on_top, desktop:fs, desktop:full_access"`
	AllowedTools []string            `json:"allowed_tools,omitempty" jsonschema:"MCP tools the app may call; requires the tools:call capability"`
	TargetAppID  string              `json:"target_app_id,omitempty" jsonschema:"app_id of an app published earlier, to publish this as its next version"`
	Icon         *appbundle.IconSpec `json:"icon,omitempty"`
}

func DefaultKindAndEntry(rawKind, rawEntry string, hasHTML, hasMarkdown bool) (string, string) {
	kind := strings.ToLower(strings.TrimSpace(rawKind))
	if kind == "" {
		if hasMarkdown && !hasHTML {
			kind = appbundle.KindDocument
		} else {
			kind = appbundle.KindArtifact
		}
	}
	entry := strings.TrimSpace(rawEntry)
	if entry == "" {
		if kind == appbundle.KindDocument {
			entry = "index.md"
		} else {
			entry = "index.html"
		}
	}
	return kind, entry
}

type sessionLoader interface {
	LoadSession(ref string) (storage.Session, error)
}

func AddMCPTools(server *mcpsdk.Server, svc *Service, sessions sessionLoader) {
	if server == nil || svc == nil {
		return
	}
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "publish_app",
		Title:       "Publish app",
		Description: "Publishes an artifact or a Markdown document as an app in the user's private Jaz Apps library, where they reopen, install and share it. Use it when the user asks for an app, or asks to save, keep, publish, share or install an artifact or document. Pass html (the same code show_artifact takes), markdown, or workspace file paths. The app is saved at once and Jaz shows its card with Show App and Install App. An app that declares AI, MCP tool or desktop capabilities is held on that card until the user approves them. Pass target_app_id to publish a new version.",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, args AppToolArgs) (*mcpsdk.CallToolResult, any, error) {
		sessionID := mcpsession.SessionID(req)
		bundle, err := packToolBundle(args, sessionID, sessions)
		if err != nil {
			return nil, nil, err
		}
		if appbundle.RequiresPublishApproval(bundle.Manifest.Capabilities) {
			draft, err := svc.StageDraft(ctx, bundle, args.TargetAppID, sessionID)
			if err != nil {
				return nil, nil, err
			}
			return toolResult(map[string]any{
				"status":        "awaiting_user_approval",
				"note":          "Not saved yet. Jaz shows a card where the user approves these capabilities; tell them it is waiting there.",
				"draft_id":      draft.ID,
				"bundle_sha256": draft.BundleSHA256,
				"target_app_id": draft.TargetAppID,
				"title":         draft.Title,
				"kind":          draft.Kind,
				"capabilities":  draft.Capabilities,
				"allowed_tools": draft.AllowedTools,
				"expires_at":    draft.ExpiresAt,
			})
		}
		app, _, _, err := svc.PublishBundle(ctx, bundle, args.TargetAppID, sessionID)
		if err != nil {
			return nil, nil, err
		}
		return toolResult(map[string]any{
			"status":     "saved",
			"note":       "Saved to the user's Apps. Jaz shows its card with Show App and Install App; do not repeat the app inline or ask the user to confirm.",
			"app_id":     app.ID,
			"title":      app.Title,
			"kind":       app.Kind,
			"version":    app.CurrentVersion,
			"visibility": app.Visibility,
		})
	})
}

func toolResult(payload map[string]any) (*mcpsdk.CallToolResult, any, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: string(encoded)}},
	}, payload, nil
}

func packToolBundle(args AppToolArgs, sessionID string, sessions sessionLoader) (appbundle.Bundle, error) {
	var sessionDir string
	if sessionID != "" && sessions != nil {
		if sess, err := sessions.LoadSession(sessionID); err == nil && sess.RuntimeRef != nil {
			sessionDir = sess.RuntimeRef.Cwd
		}
	}

	kind, entry := DefaultKindAndEntry(
		args.Kind,
		args.Entry,
		strings.TrimSpace(args.HTML) != "",
		strings.TrimSpace(args.Markdown) != "",
	)

	rawFiles := make(map[string][]byte)
	for rel, content := range args.Files {
		rawFiles[rel] = []byte(content)
	}
	if len(args.Paths) > 0 && sessionDir == "" {
		return appbundle.Bundle{}, errors.New("paths requires an active session workspace directory")
	}
	for _, p := range args.Paths {
		cleanRel, err := appbundle.ValidatePath(strings.TrimSpace(p))
		if err != nil {
			return appbundle.Bundle{}, fmt.Errorf("invalid workspace path %q: %w", p, err)
		}
		fullPath, err := pathsafe.Resolve(sessionDir, cleanRel)
		if err != nil {
			return appbundle.Bundle{}, fmt.Errorf("workspace path %q escapes directory", p)
		}
		info, err := os.Lstat(fullPath)
		if err != nil {
			return appbundle.Bundle{}, fmt.Errorf("stat workspace file %q: %w", cleanRel, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return appbundle.Bundle{}, fmt.Errorf("%w: %s", appbundle.ErrSymlinkBlocked, cleanRel)
		}
		if info.Size() > appbundle.MaxFileBytes {
			return appbundle.Bundle{}, fmt.Errorf("%w: %s", appbundle.ErrFileTooLarge, cleanRel)
		}
		data, err := os.ReadFile(filepath.Clean(fullPath))
		if err != nil {
			return appbundle.Bundle{}, fmt.Errorf("read workspace file %q: %w", cleanRel, err)
		}
		rawFiles[cleanRel] = data
	}
	if len(rawFiles) == 0 {
		inline := args.HTML
		if strings.TrimSpace(inline) == "" {
			inline = args.Markdown
		}
		if strings.TrimSpace(inline) != "" {
			rawFiles[entry] = []byte(inline)
		}
	}

	return appbundle.PackFiles(appbundle.Manifest{
		Format:       appbundle.Format,
		Title:        args.Title,
		Description:  args.Description,
		Kind:         kind,
		Entry:        entry,
		Capabilities: args.Capabilities,
		AllowedTools: args.AllowedTools,
		Icon:         args.Icon,
	}, rawFiles)
}
