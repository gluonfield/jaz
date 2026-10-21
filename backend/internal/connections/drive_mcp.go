package connections

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/connectors/drive"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

func NewDriveMCPServer(store integrationoauth.Store, connectionID string) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "google_drive", Version: "1"}, nil)
	addDriveTool(server, store, connectionID, "search_files", "Search Drive files with structured queries. Use title, parentId, owner, sharedWithMe, mimeType, timestamps and boolean clauses. Supports shared drives. Optional snippets are excerpts of file descriptions.", true, drive.APIClient.Search)
	addDriveTool(server, store, connectionID, "list_recent_files", "List recent Drive files, including shared drives. orderBy: recency (default), lastModified, lastModifiedByMe. Supports pagination. Optional snippets are excerpts of file descriptions.", true, drive.APIClient.Recent)
	addDriveTool(server, store, connectionID, "get_file_metadata", "Get metadata for an exact Drive file ID. Optional snippets are excerpts of its description.", true, drive.APIClient.Metadata)
	addDriveTool(server, store, connectionID, "get_file_permissions", "List a Drive file's sharing permissions, including shared-drive access.", true, drive.APIClient.Permissions)
	addDriveTool(server, store, connectionID, "copy_file", "Copy a Drive file. Optional title defaults to Copy of the original name. Optional parentId defaults to the original folder if writable, otherwise My Drive.", false, drive.APIClient.Copy)
	addDriveTool(server, store, connectionID, "create_file", "Create a file or folder, or upload content to Drive. Prefer filePath for an existing file in the server/agent workspace. Otherwise use textContent or base64Content. contentMimeType is required for uploads; use application/vnd.google-apps.folder for folders. Supported uploads convert to Google formats unless disableConversionToGoogleType is true.", false, createDriveFile)
	addDriveTool(server, store, connectionID, "read_file_content", "Read text from an exact Drive file ID discovered by search_files/list_recent_files or supplied by the user. Supports Google Docs, Slides and UTF-8 text. Sheets return the first worksheet only; use download_file_content with exportMimeType application/vnd.openxmlformats-officedocument.spreadsheetml.sheet for all worksheets. Optionally include comments on Google documents. Binary formats return textFormattingNotSupported; use download_file_content to retrieve them.", true, drive.APIClient.Read)
	addDriveTool(server, store, connectionID, "download_file_content", "Download a Drive file as base64, or stream it to a new server/agent-workspace file with filePath. Google Docs and Slides export to text by default; Sheets export to XLSX preserving all worksheets. exportMimeType selects another format; Sheets CSV/TSV exports contain only the first worksheet. revisionId selects a historical version.", false,
		func(client drive.APIClient, ctx context.Context, in drive.DownloadInput) (map[string]string, error) {
			body, mimeType, err := client.Download(ctx, in)
			if err != nil {
				return nil, err
			}
			defer body.Close()
			if in.FilePath != "" {
				if !filepath.IsAbs(in.FilePath) {
					return nil, fmt.Errorf("filePath must be an absolute server path")
				}
				file, err := os.OpenFile(in.FilePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
				if err != nil {
					return nil, err
				}
				_, copyErr := io.Copy(file, body)
				closeErr := file.Close()
				if err := errors.Join(copyErr, closeErr); err != nil {
					_ = os.Remove(in.FilePath)
					return nil, err
				}
				return map[string]string{"filePath": in.FilePath, "mimeType": mimeType}, nil
			}
			data, err := io.ReadAll(io.LimitReader(body, (32<<20)+1))
			if err != nil {
				return nil, err
			}
			if len(data) > 32<<20 {
				return nil, fmt.Errorf("file exceeds the 32 MiB inline download limit; provide filePath to stream it to disk")
			}
			return map[string]string{"base64Content": base64.StdEncoding.EncodeToString(data), "mimeType": mimeType}, nil
		})
	return server
}

func addDriveTool[In, Out any](server *mcp.Server, store integrationoauth.Store, connectionID, name, description string, readOnly bool, call func(drive.APIClient, context.Context, In) (Out, error)) {
	mcp.AddTool(server, &mcp.Tool{
		Name: name, Description: description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: readOnly, DestructiveHint: new(false)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
		var zero Out
		client, err := (integrationoauth.Refresher{Store: store}).Client(ctx, connectionID)
		if err != nil {
			return nil, zero, err
		}
		client.Timeout = 2 * time.Minute
		out, err := call(drive.APIClient{HTTP: client}, ctx, input)
		return nil, out, err
	})
}

func createDriveFile(client drive.APIClient, ctx context.Context, input drive.CreateInput) (drive.File, error) {
	input.Title = strings.TrimSpace(input.Title)
	sources := 0
	for _, provided := range []bool{input.FilePath != "", input.TextContent != nil, input.Base64Content != nil, input.Content != nil} {
		if provided {
			sources++
		}
	}
	if sources > 1 {
		return drive.File{}, fmt.Errorf("provide only one of filePath, textContent, base64Content or content")
	}
	var body io.ReadSeeker
	switch {
	case input.FilePath != "":
		if !filepath.IsAbs(input.FilePath) {
			return drive.File{}, fmt.Errorf("filePath must be an absolute path accessible to the Jaz server")
		}
		file, err := os.Open(input.FilePath)
		if err != nil {
			return drive.File{}, fmt.Errorf("open server file: %w", err)
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			return drive.File{}, err
		}
		if !info.Mode().IsRegular() {
			return drive.File{}, fmt.Errorf("filePath must identify a regular file")
		}
		body = file
	case input.TextContent != nil:
		body = strings.NewReader(*input.TextContent)
	case input.Base64Content != nil || input.Content != nil:
		value := input.Base64Content
		if value == nil {
			value = input.Content
		}
		data, err := base64.StdEncoding.DecodeString(*value)
		if err != nil {
			return drive.File{}, fmt.Errorf("invalid base64 content: %w", err)
		}
		body = bytes.NewReader(data)
	}
	return client.Create(ctx, input, body)
}
