package connections

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/connectors/drive"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

func TestDriveMCPUploadsContentSourcesAndConvertsNativeFiles(t *testing.T) {
	data := bytes.Repeat([]byte{0, 255, 23, 42}, 800000)
	file := filepath.Join(t.TempDir(), "report.pdf")
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	session := driveTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch r.URL.Path {
		case "/drive/v3/about":
			_, _ = io.WriteString(w, `{"importFormats":{"text/plain":["application/vnd.google-apps.document"]}}`)
		case "/upload/drive/v3/files":
			_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil {
				t.Error(err)
				return
			}
			reader := multipart.NewReader(r.Body, params["boundary"])
			part, err := reader.NextPart()
			if err != nil {
				t.Error(err)
				return
			}
			var metadata struct {
				Name     string   `json:"name"`
				MIMEType string   `json:"mimeType"`
				Parents  []string `json:"parents"`
			}
			if err := json.NewDecoder(part).Decode(&metadata); err != nil {
				t.Error(err)
			}
			part, err = reader.NextPart()
			if err != nil {
				t.Error(err)
				return
			}
			content, err := io.ReadAll(part)
			if err != nil {
				t.Error(err)
			}
			if metadata.Name == "empty" {
				if len(content) != 0 || metadata.MIMEType != "application/vnd.google-apps.document" {
					t.Errorf("empty native conversion: %d bytes, %q", len(content), metadata.MIMEType)
				}
			} else if !bytes.Equal(content, data) || metadata.MIMEType != "application/pdf" {
				t.Errorf("binary upload mismatch: %d bytes, %q", len(content), metadata.MIMEType)
			}
			if len(metadata.Parents) != 1 || metadata.Parents[0] != "shared-folder" || r.URL.Query().Get("supportsAllDrives") != "true" {
				t.Errorf("shared-drive upload = %+v, %v", metadata, r.URL)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "uploaded", "name": metadata.Name, "mimeType": metadata.MIMEType})
		case "/drive/v3/files":
			var metadata map[string]any
			if err := json.NewDecoder(r.Body).Decode(&metadata); err != nil {
				t.Error(err)
			}
			if metadata["mimeType"] != "application/vnd.google-apps.folder" {
				t.Errorf("folder metadata = %v", metadata)
			}
			_, _ = io.WriteString(w, `{"id":"folder","name":"Folder","mimeType":"application/vnd.google-apps.folder"}`)
		default:
			t.Errorf("unexpected request %s", r.URL)
			http.NotFound(w, r)
		}
	})
	for _, source := range []map[string]any{{"filePath": file}, {"base64Content": base64.StdEncoding.EncodeToString(data)}, {"content": base64.StdEncoding.EncodeToString(data)}} {
		source["title"] = "PDF"
		source["parentId"] = "shared-folder"
		source["contentMimeType"] = "application/pdf"
		source["disableConversionToGoogleType"] = true
		var out drive.File
		callDriveTool(t, session, "create_file", source, &out)
		if out.ID != "uploaded" || out.Title != "PDF" {
			t.Fatalf("upload = %+v", out)
		}
	}
	var native drive.File
	callDriveTool(t, session, "create_file", map[string]any{"title": "empty", "parentId": "shared-folder", "textContent": "", "contentMimeType": "text/plain"}, &native)
	if native.MIMEType != "application/vnd.google-apps.document" {
		t.Fatalf("native conversion = %+v", native)
	}
	var folder drive.File
	callDriveTool(t, session, "create_file", map[string]any{"title": "Folder", "mimeType": "application/vnd.google-apps.folder"}, &folder)
	if folder.ID != "folder" {
		t.Fatalf("folder = %+v", folder)
	}
	before := calls.Load()
	for _, input := range []map[string]any{
		{"title": "conflict", "textContent": "hello", "filePath": file, "contentMimeType": "text/plain"},
		{"title": "bad base64", "base64Content": "not base64", "contentMimeType": "application/pdf"},
		{"title": "missing MIME", "textContent": "hello"},
		{"title": "relative path", "filePath": "report.pdf", "contentMimeType": "application/pdf"},
		{"title": "directory", "filePath": filepath.Dir(file), "contentMimeType": "application/pdf"},
	} {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "create_file", Arguments: input})
		if err != nil || !result.IsError {
			t.Fatalf("invalid upload = %+v, %v", result, err)
		}
	}
	if calls.Load() != before {
		t.Fatal("invalid uploads reached Google")
	}
}

func TestDriveMCPDownloadReturnsOriginalBinaryAndExportType(t *testing.T) {
	data := []byte{'P', 'D', 'F', 0, 255}
	session := driveTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("alt") == "media" {
			_, _ = w.Write(data)
			return
		}
		_, _ = io.WriteString(w, `{"id":"pdf","name":"Drawing","mimeType":"application/pdf"}`)
	})
	var out map[string]string
	callDriveTool(t, session, "download_file_content", map[string]any{"fileId": "pdf"}, &out)
	decoded, err := base64.StdEncoding.DecodeString(out["base64Content"])
	if err != nil || !bytes.Equal(decoded, data) || out["mimeType"] != "application/pdf" {
		t.Fatalf("download = %v, %v", out, err)
	}
	destination := filepath.Join(t.TempDir(), "download.pdf")
	callDriveTool(t, session, "download_file_content", map[string]any{"fileId": "pdf", "filePath": destination}, &out)
	saved, err := os.ReadFile(destination)
	if err != nil || !bytes.Equal(saved, data) || out["filePath"] != destination {
		t.Fatalf("streamed download = %v, %v", out, err)
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "download_file_content", Arguments: map[string]any{"fileId": "pdf", "filePath": destination}})
	if err != nil || !result.IsError {
		t.Fatalf("existing destination was accepted: %+v, %v", result, err)
	}
	saved, err = os.ReadFile(destination)
	if err != nil || !bytes.Equal(saved, data) {
		t.Fatal("existing destination was modified")
	}
}

func driveTestSession(t *testing.T, handler http.HandlerFunc) *mcp.ClientSession {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access" {
			t.Error("Drive request lost its account credential")
		}
		handler(w, r)
	}))
	t.Cleanup(upstream.Close)
	base := http.DefaultTransport
	target, _ := url.Parse(upstream.URL)
	http.DefaultTransport = driveRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "www.googleapis.com" {
			return nil, fmt.Errorf("unexpected upstream %s", req.URL.Host)
		}
		copy := req.Clone(req.Context())
		copy.URL.Scheme = target.Scheme
		copy.URL.Host = target.Host
		return base.RoundTrip(copy)
	})
	t.Cleanup(func() {
		http.DefaultTransport = base
	})
	store := &testConnectionStore{tokens: map[string]integrationoauth.Token{"drive": {AccessToken: "access", TokenType: "Bearer", Expiry: time.Now().Add(time.Hour)}}}
	server := NewDriveMCPServer(store, "drive")
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = serverSession.Close()
	})
	client := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = session.Close()
	})
	return session
}

type driveRoundTripper func(*http.Request) (*http.Response, error)

func (f driveRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func callDriveTool(t *testing.T, session *mcp.ClientSession, name string, arguments map[string]any, output any) {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("%s failed: %v, %s", name, err, toolText(result))
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, output); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
}

func TestDriveMCPRetainsToolInputsAndReadOnlyAnnotations(t *testing.T) {
	session := driveTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("discovery should not call Google: %s", r.URL)
	})
	list, err := session.ListTools(context.Background(), nil)
	if err != nil || len(list.Tools) != 8 {
		t.Fatalf("tools = %+v, %v", list, err)
	}
	inputs := map[string]string{
		"copy_file": "fileId,parentId,title", "create_file": "base64Content,content,contentMimeType,disableConversionToGoogleType,filePath,mimeType,parentId,textContent,title",
		"download_file_content": "exportMimeType,fileId,filePath,revisionId", "get_file_metadata": "excludeContentSnippets,fileId,snippetVerbosity",
		"get_file_permissions": "fileId", "list_recent_files": "excludeContentSnippets,orderBy,pageSize,pageToken,snippetVerbosity",
		"read_file_content": "fileId,includeComments", "search_files": "excludeContentSnippets,pageSize,pageToken,query,snippetVerbosity",
	}
	for _, tool := range list.Tools {
		schema, _ := json.Marshal(tool.InputSchema)
		var parsed struct {
			Properties map[string]any `json:"properties"`
		}
		if err := json.Unmarshal(schema, &parsed); err != nil {
			t.Fatal(err)
		}
		want := strings.Split(inputs[tool.Name], ",")
		if len(parsed.Properties) != len(want) {
			t.Fatalf("%s schema = %s", tool.Name, schema)
		}
		for _, name := range want {
			if _, ok := parsed.Properties[name]; !ok {
				t.Fatalf("%s lost input %s", tool.Name, name)
			}
		}
		readOnly := tool.Name != "copy_file" && tool.Name != "create_file"
		if tool.Annotations == nil || tool.Annotations.ReadOnlyHint != readOnly {
			t.Fatalf("%s read-only annotation = %+v", tool.Name, tool.Annotations)
		}
	}
}
