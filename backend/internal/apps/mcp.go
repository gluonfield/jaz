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

type PrepareDraftArgs struct {
	Title        string              `json:"title"`
	Description  string              `json:"description,omitempty"`
	Kind         string              `json:"kind,omitempty"`
	Entry        string              `json:"entry,omitempty"`
	HTML         string              `json:"html,omitempty"`
	Markdown     string              `json:"markdown,omitempty"`
	Files        map[string]string   `json:"files,omitempty"`
	Paths        []string            `json:"paths,omitempty"`
	Capabilities []string            `json:"capabilities,omitempty"`
	AllowedTools []string            `json:"allowed_tools,omitempty"`
	TargetAppID  string              `json:"target_app_id,omitempty"`
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
		Name:        "prepare_app_draft",
		Description: "Pack inline HTML/Markdown or workspace files into a validated jaz-app/1 preflight draft for explicit user confirmation and publishing.",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, args PrepareDraftArgs) (*mcpsdk.CallToolResult, any, error) {
		sessionID := mcpsession.SessionID(req)
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
		if len(args.Paths) > 0 {
			if sessionDir == "" {
				return nil, nil, errors.New("paths requires an active session workspace directory")
			}
			for _, p := range args.Paths {
				cleanRel, err := appbundle.ValidatePath(strings.TrimSpace(p))
				if err != nil {
					return nil, nil, fmt.Errorf("invalid workspace path %q: %w", p, err)
				}
				fullPath, err := pathsafe.Resolve(sessionDir, cleanRel)
				if err != nil {
					return nil, nil, fmt.Errorf("workspace path %q escapes directory", p)
				}
				info, err := os.Lstat(fullPath)
				if err != nil {
					return nil, nil, fmt.Errorf("stat workspace file %q: %w", cleanRel, err)
				}
				if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
					return nil, nil, fmt.Errorf("%w: %s", appbundle.ErrSymlinkBlocked, cleanRel)
				}
				if info.Size() > appbundle.MaxFileBytes {
					return nil, nil, fmt.Errorf("%w: %s", appbundle.ErrFileTooLarge, cleanRel)
				}
				data, err := os.ReadFile(filepath.Clean(fullPath))
				if err != nil {
					return nil, nil, fmt.Errorf("read workspace file %q: %w", cleanRel, err)
				}
				rawFiles[cleanRel] = data
			}
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

		bundle, err := appbundle.PackFiles(appbundle.Manifest{
			Format:       appbundle.Format,
			Title:        args.Title,
			Description:  args.Description,
			Kind:         kind,
			Entry:        entry,
			Capabilities: args.Capabilities,
			AllowedTools: args.AllowedTools,
			Icon:         args.Icon,
		}, rawFiles)
		if err != nil {
			return nil, nil, err
		}

		draft, err := svc.StageDraft(ctx, bundle, args.TargetAppID, sessionID)
		if err != nil {
			return nil, nil, err
		}
		payload := map[string]any{
			"draft_id":                   draft.ID,
			"bundle_sha256":              draft.BundleSHA256,
			"target_app_id":              draft.TargetAppID,
			"title":                      draft.Title,
			"kind":                       draft.Kind,
			"entry_path":                 draft.EntryPath,
			"size_bytes":                 draft.SizeBytes,
			"file_count":                 draft.FileCount,
			"files":                      draft.Files,
			"capabilities":               draft.Capabilities,
			"allowed_tools":              draft.AllowedTools,
			"external_urls":              draft.ExternalURLs,
			"warnings":                   draft.Warnings,
			"expires_at":                 draft.ExpiresAt,
			"requires_user_confirmation": true,
		}
		encoded, _ := json.Marshal(payload)
		return &mcpsdk.CallToolResult{
			Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: string(encoded)}},
		}, payload, nil
	})
}
