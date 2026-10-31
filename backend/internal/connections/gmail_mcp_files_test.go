package connections

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	"github.com/wins/jaz/backend/internal/filepathx"
	"github.com/wins/jaz/backend/pkg/integrations"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

func TestGmailMCPFileAttachments(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "résumé \"2026\".pdf")
	data := bytes.Repeat([]byte{0, 1, 2, 128, 255}, 100)
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(root, "empty.txt")
	if err := os.WriteFile(empty, nil, 0600); err != nil {
		t.Fatal(err)
	}
	body := "Hello π\nWith a second line."
	old := []byte("existing bytes")
	inline := []byte{137, 80, 78, 71}
	b64 := base64.RawURLEncoding.EncodeToString
	mutations := 0
	attachmentReads := 0
	failRead := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer work-access" {
			t.Errorf("wrong account authorization: %q", r.Header.Get("Authorization"))
			http.Error(w, "wrong account", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/messages/original":
			_, _ = io.WriteString(w, `{"id":"original","threadId":"thread"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/threads/thread":
			_, _ = io.WriteString(w, `{"id":"thread","messages":[{"id":"original","threadId":"thread","payload":{"headers":[{"name":"From","value":"sender@example.com"},{"name":"To","value":"work@example.com"},{"name":"Subject","value":"Topic"},{"name":"Message-ID","value":"<original@example.com>"}]}}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/drafts/draft":
			_, _ = io.WriteString(w, `{"id":"draft","message":{"id":"current","threadId":"thread","payload":{"mimeType":"multipart/mixed","headers":[{"name":"To","value":"sender@example.com"},{"name":"Subject","value":"Topic"},{"name":"In-Reply-To","value":"<original@example.com>"},{"name":"References","value":"<original@example.com>"}],"parts":[{"mimeType":"multipart/alternative","parts":[{"mimeType":"text/plain","body":{"data":"`+b64([]byte("Original body"))+`"}},{"mimeType":"text/html","body":{"data":"`+b64([]byte(`<p>Original body<img src="cid:logo"></p>`))+`"}}]},{"mimeType":"application/pdf","filename":"old.pdf","body":{"attachmentId":"old"}},{"mimeType":"image/png","filename":"logo.png","headers":[{"name":"Content-ID","value":"<logo>"},{"name":"Content-Disposition","value":"inline"}],"body":{"data":"`+b64(inline)+`"}},{"mimeType":"text/plain","filename":"notes.txt","body":{"data":"`+b64([]byte("Not the email body"))+`"}}]}}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/messages/current/attachments/old":
			attachmentReads++
			if failRead {
				http.Error(w, "unavailable", http.StatusBadGateway)
				return
			}
			_, _ = io.WriteString(w, `{"data":"`+b64(old)+`","size":14}`)
		case (r.Method == http.MethodPost && r.URL.Path == "/gmail/v1/users/me/drafts") || (r.Method == http.MethodPut && r.URL.Path == "/gmail/v1/users/me/drafts/draft"):
			mutations++
			var payload struct {
				Message struct {
					Raw      string `json:"raw"`
					ThreadID string `json:"threadId"`
				} `json:"message"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Error(err)
				return
			}
			raw, err := base64.RawURLEncoding.DecodeString(payload.Message.Raw)
			if err != nil {
				t.Error(err)
				return
			}
			message, err := mail.ReadMessage(bytes.NewReader(raw))
			if err != nil {
				t.Error(err)
				return
			}
			parts := readGmailMIME(t, textproto.MIMEHeader(message.Header), message.Body)
			wantCount := 3
			if r.Method == http.MethodPut {
				wantCount = 7
				if mutations == 4 {
					wantCount = 6
				}
				if mutations >= 5 {
					wantCount = 5
				}
				assertGmailPart(t, parts, "old.pdf", old)
				logo := assertGmailPart(t, parts, "logo.png", inline)
				if logo.header.Get("Content-ID") != "<logo>" || !strings.HasPrefix(logo.header.Get("Content-Disposition"), "inline;") {
					t.Errorf("inline metadata = %v", logo.header)
				}
				assertGmailPart(t, parts, "notes.txt", []byte("Not the email body"))
			}
			if len(parts) != wantCount {
				t.Errorf("MIME leaves = %d, want %d", len(parts), wantCount)
			}
			if mutations < 5 {
				assertGmailPart(t, parts, filepath.Base(file), data)
				assertGmailPart(t, parts, "empty.txt", nil)
			}
			if r.Method == http.MethodPut && (mutations == 3 || mutations >= 5) {
				if string(parts[0].data) != "Original body" || !bytes.Contains(parts[1].data, []byte("cid:logo")) {
					t.Errorf("existing bodies lost: %q", parts[:2])
				}
			} else if string(parts[0].data) != strings.ReplaceAll(body, "\n", "\r\n") {
				t.Errorf("body = %q", parts[0].data)
			}
			if mutations > 1 && (payload.Message.ThreadID != "thread" || message.Header.Get("In-Reply-To") != "<original@example.com>" || message.Header.Get("References") != "<original@example.com>") {
				t.Errorf("threading lost: %s %v", payload.Message.ThreadID, message.Header)
			}
			_, _ = io.WriteString(w, `{"id":"draft","message":{"id":"saved","threadId":"thread"}}`)
		default:
			t.Errorf("unexpected Gmail request: %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	store := &testConnectionStore{tokens: map[string]integrationoauth.Token{}, connections: []integrations.Connection{}}
	for _, alias := range []string{"personal", "work"} {
		id := "gmail:" + alias
		store.tokens[id] = integrationoauth.Token{AccessToken: alias + "-access", TokenType: "Bearer", Expiry: time.Now().Add(time.Hour)}
		store.connections = append(store.connections, integrations.Connection{ID: id, Provider: gmailconnector.ProviderID, AccountID: alias + "@example.com", Alias: alias})
	}
	tools := newTestGmailMCPTools(t, store)
	tools.apiBaseURL = upstream.URL
	server := mcp.NewServer(&mcp.Implementation{Name: "jaz", Version: "1"}, nil)
	tools.AddTo(server)
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "1"}, nil).Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	call := func(name string, input map[string]any, wantError bool) *mcp.CallToolResult {
		t.Helper()
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: input})
		if err != nil {
			t.Fatal(err)
		}
		if result.IsError != wantError {
			t.Fatalf("%s error=%v, want %v: %v", name, result.IsError, wantError, result.Content)
		}
		return result
	}
	paths := []string{file, filepathx.FileURI(empty)}
	for _, account := range []string{"", "work"} {
		result := call(gmailconnector.ToolCreateDraft, map[string]any{"account": account, "to": []string{"sender@example.com"}, "body_text": body, "attachment_paths": paths}, false)
		var output GmailDraftOutput
		encoded, _ := json.Marshal(result.StructuredContent)
		if err := json.Unmarshal(encoded, &output); err != nil {
			t.Fatal(err)
		}
		if account == "" && (!output.AccountRequired || mutations != 0) {
			t.Fatalf("ambiguous account mutated Gmail: %+v", output)
		}
		if account != "" && (output.AccountID != "work@example.com" || output.Draft.ID != "draft") {
			t.Fatalf("wrong account output: %+v", output)
		}
	}
	call(gmailconnector.ToolCreateReply, map[string]any{"account": "work@example.com", "id": "original", "body_text": body, "attachment_paths": paths}, false)
	call(gmailconnector.ToolUpdateDraft, map[string]any{"account": "gmail:work", "id": "draft", "subject": "Edited", "attachment_paths": paths}, false)
	call(gmailconnector.ToolUpdateDraft, map[string]any{"account": "work", "id": "draft", "body_text": body, "attachment_paths": paths}, false)
	call(gmailconnector.ToolUpdateDraft, map[string]any{"account": "work", "id": "draft", "attachment_paths": []string{}}, false)
	call(gmailconnector.ToolUpdateDraft, map[string]any{"account": "work", "id": "draft", "subject": "Still attached"}, false)
	if mutations != 6 || attachmentReads != 4 {
		t.Fatalf("requests: mutations=%d attachmentReads=%d", mutations, attachmentReads)
	}
	before := mutations
	for _, path := range []string{"relative.txt", "", root, filepath.Join(root, "missing")} {
		call(gmailconnector.ToolCreateDraft, map[string]any{"account": "work", "to": []string{"sender@example.com"}, "body_text": body, "attachment_paths": []string{path}}, true)
	}
	large := filepath.Join(root, "large.bin")
	f, err := os.Create(large)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(gmailconnector.MaxAttachmentBytes + 1); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	call(gmailconnector.ToolCreateDraft, map[string]any{"account": "work", "to": []string{"sender@example.com"}, "body_text": body, "attachment_paths": []string{large}}, true)
	if err := os.Truncate(large, gmailconnector.MaxAttachmentBytes); err != nil {
		t.Fatal(err)
	}
	call(gmailconnector.ToolUpdateDraft, map[string]any{"account": "work", "id": "draft", "attachment_paths": []string{large}}, true)
	failRead = true
	call(gmailconnector.ToolUpdateDraft, map[string]any{"account": "work", "id": "draft", "subject": "Cannot drop files"}, true)
	if mutations != before {
		t.Fatalf("failed attachment preparation mutated draft")
	}
}

type gmailMIMELeaf struct {
	header textproto.MIMEHeader
	data   []byte
}

func readGmailMIME(t *testing.T, header textproto.MIMEHeader, body io.Reader) []gmailMIMELeaf {
	t.Helper()
	mediaType, params, err := mime.ParseMediaType(header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(mediaType, "multipart/") {
		reader := multipart.NewReader(body, params["boundary"])
		var leaves []gmailMIMELeaf
		for {
			part, err := reader.NextRawPart()
			if err == io.EOF {
				return leaves
			}
			if err != nil {
				t.Fatal(err)
			}
			leaves = append(leaves, readGmailMIME(t, part.Header, part)...)
		}
	}
	switch header.Get("Content-Transfer-Encoding") {
	case "base64":
		body = base64.NewDecoder(base64.StdEncoding, body)
	case "quoted-printable":
		body = quotedprintable.NewReader(body)
	default:
		t.Fatalf("unexpected transfer encoding: %v", header)
	}
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	return []gmailMIMELeaf{{header: header, data: data}}
}

func assertGmailPart(t *testing.T, parts []gmailMIMELeaf, filename string, data []byte) gmailMIMELeaf {
	t.Helper()
	for _, part := range parts {
		_, params, err := mime.ParseMediaType(part.header.Get("Content-Disposition"))
		if err == nil && params["filename"] == filename {
			if !bytes.Equal(part.data, data) {
				t.Errorf("attachment %q bytes changed", filename)
			}
			return part
		}
	}
	t.Fatalf("missing attachment %q", filename)
	return gmailMIMELeaf{}
}

func TestGmailMCPUpdateRejectsUndownloadableAttachmentContainers(t *testing.T) {
	for _, header := range []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}{
		{Name: "Content-Disposition", Value: "attachment"},
		{Name: "Content-ID", Value: "<attached-container>"},
	} {
		t.Run(header.Name, func(t *testing.T) {
			reads := 0
			writes := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/gmail/v1/users/me/drafts/draft" {
					writes++
					http.Error(w, "unexpected mutation", http.StatusBadRequest)
					return
				}
				reads++
				encoded, _ := json.Marshal(header)
				_, _ = io.WriteString(w, `{"id":"draft","message":{"id":"current","payload":{"mimeType":"multipart/mixed","headers":[{"name":"To","value":"recipient@example.com"}],"parts":[{"mimeType":"text/plain","body":{"data":"Ym9keQ"}},{"mimeType":"multipart/mixed","headers":[`+string(encoded)+`],"parts":[{"mimeType":"text/plain","body":{"data":"YXR0YWNoZWQ"}}]}]}}}`)
			}))
			defer upstream.Close()
			tools := newTestGmailMCPTools(t, &testConnectionStore{
				tokens:      map[string]integrationoauth.Token{"gmail:work": {AccessToken: "access", TokenType: "Bearer", Expiry: time.Now().Add(time.Hour)}},
				connections: []integrations.Connection{{ID: "gmail:work", Provider: gmailconnector.ProviderID, AccountID: "work@example.com", Alias: "work"}},
			})
			tools.apiBaseURL = upstream.URL
			subject := "Keep attachments"
			_, _, err := tools.UpdateDraft(context.Background(), nil, GmailUpdateDraftInput{Account: "work", ID: "draft", Subject: &subject})
			if err == nil || !strings.Contains(err.Error(), "attachment container without downloadable content") || writes != 0 || reads != 1 {
				t.Fatalf("update: err=%v reads=%d writes=%d", err, reads, writes)
			}
		})
	}
}
