package drive

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchTranslatesExistingQueriesAndSharedDriveResults(t *testing.T) {
	cases := []struct {
		input string
		query string
	}{
		{"title contains 'parentId owner title'", "name contains 'parentId owner title'"},
		{"parentId = 'shared-folder' and owner != 'me'", "'shared-folder' in parents and not ('me' in owners)"},
		{"(title = 'O\\'Brien' or fullText contains 'a\\\\b') and sharedWithMe = false", "( name = 'O\\'Brien' or fullText contains 'a\\\\b' ) and not sharedWithMe"},
		{"parentId != 'root' and sharedWithMe != false", "not ('root' in parents) and sharedWithMe"},
		{"modifiedTime >= '2026-01-01T00:00:00Z' and mimeType = 'application/pdf'", "modifiedTime >= '2026-01-01T00:00:00Z' and mimeType = 'application/pdf'"},
	}
	for _, test := range cases {
		t.Run(test.input, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				if q.Get("q") != "trashed = false and ("+test.query+")" || q.Get("supportsAllDrives") != "true" || q.Get("includeItemsFromAllDrives") != "true" || q.Get("pageToken") != "next" {
					t.Errorf("query = %v", q)
				}
				_, _ = io.WriteString(w, `{"nextPageToken":"after","files":[{"id":"file","name":"Report","parents":["shared-folder"],"mimeType":"text/plain","size":"1024","webViewLink":"https://drive.google.com/file/d/file/view","owners":[{"emailAddress":"owner@example.com"}],"capabilities":{"canAddChildren":false},"description":"`+strings.Repeat("ė", 1200)+`"}]}`)
			}))
			defer upstream.Close()
			client := APIClient{HTTP: upstream.Client(), BaseURL: upstream.URL}
			out, err := client.Search(context.Background(), SearchInput{Query: test.input, ListOptions: ListOptions{PageToken: "next", SnippetVerbosity: "BRIEF"}})
			if err != nil || len(out.Files) != 1 || out.NextPageToken != "after" {
				t.Fatalf("search = %+v, %v", out, err)
			}
			file := out.Files[0]
			if file.Title != "Report" || file.FileSize != "1024" || file.ParentID != "shared-folder" || file.Owner != "owner@example.com" || len([]rune(file.ContentSnippet)) != 1000 || file.CanAddChildren == nil || *file.CanAddChildren {
				t.Fatalf("metadata mapping = %+v", file)
			}
		})
	}
}

func TestReadNativeDocumentExportsTextAndPaginatesComments(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/drive/v3/files/doc":
			_, _ = io.WriteString(w, `{"id":"doc","name":"Plan","mimeType":"application/vnd.google-apps.document","webViewLink":"https://docs.google.com/document/d/doc/edit"}`)
		case "/drive/v3/files/doc/export":
			if r.URL.Query().Get("mimeType") != "text/plain" {
				t.Errorf("export = %v", r.URL.Query())
			}
			_, _ = io.WriteString(w, "A plan with actual content.")
		case "/drive/v3/files/doc/comments":
			if r.URL.Query().Get("pageToken") == "second" {
				_, _ = io.WriteString(w, `{"comments":[{"id":"c2","content":"Resolved","resolved":true}]}`)
			} else {
				_, _ = io.WriteString(w, `{"nextPageToken":"second","comments":[{"id":"c1","content":"Review this","author":{"displayName":"Alex"},"replies":[{"id":"r1","content":"Agreed"}]}]}`)
			}
		default:
			t.Errorf("unexpected request %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	client := APIClient{HTTP: upstream.Client(), BaseURL: upstream.URL}
	out, err := client.Read(context.Background(), ReadInput{FileID: "doc", IncludeComments: true})
	if err != nil || out.FileContent != "A plan with actual content." || out.Title != "Plan" || len(out.CommentThreads) != 2 {
		t.Fatalf("read = %+v, %v", out, err)
	}
	if out.CommentThreads[0].HeadPost.AuthorName != "Alex" || out.CommentThreads[0].Replies[0].Content != "Agreed" || out.CommentThreads[1].Status != "RESOLVED" {
		t.Fatalf("comments = %+v", out.CommentThreads)
	}
}

func TestDownloadBinaryRevisionAndReadUnsupportedFormat(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/drive/v3/files/pdf":
			_, _ = io.WriteString(w, `{"id":"pdf","name":"Drawing","mimeType":"application/pdf"}`)
		case "/drive/v3/files/pdf/revisions/older":
			if r.URL.Query().Get("alt") != "media" {
				t.Errorf("revision is not a content request: %v", r.URL)
			}
			_, _ = w.Write([]byte{'P', 'D', 'F', 0, 255})
		default:
			t.Errorf("unexpected request %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	client := APIClient{HTTP: upstream.Client(), BaseURL: upstream.URL}
	body, mimeType, err := client.Download(context.Background(), DownloadInput{FileID: "pdf", RevisionID: "older"})
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	data, err := io.ReadAll(body)
	if err != nil || string(data) != string([]byte{'P', 'D', 'F', 0, 255}) || mimeType != "application/pdf" {
		t.Fatalf("download = %v, %q, %v", data, mimeType, err)
	}
	out, err := client.Read(context.Background(), ReadInput{FileID: "pdf", IncludeComments: true})
	if err != nil || !out.TextFormattingNotSupported || !out.CommentsNotSupported || out.FileContent != "" {
		t.Fatalf("unsupported read = %+v, %v", out, err)
	}
}

func TestCopyUsesWritableOriginalParentOrMyDrive(t *testing.T) {
	for _, parentAccess := range []string{"writable", "readonly", "hidden"} {
		t.Run(parentAccess, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/drive/v3/files/original":
					_, _ = io.WriteString(w, `{"id":"original","name":"Original","parents":["folder"]}`)
				case "/drive/v3/files/folder":
					if parentAccess == "hidden" {
						http.Error(w, "hidden folder", http.StatusNotFound)
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"capabilities": map[string]bool{"canAddChildren": parentAccess == "writable"}})
				case "/drive/v3/files/original/copy":
					var metadata fileMetadata
					if err := json.NewDecoder(r.Body).Decode(&metadata); err != nil {
						t.Error(err)
					}
					wantParent := "folder"
					if parentAccess != "writable" {
						wantParent = "root"
					}
					if metadata.Name != "Copy of Original" || len(metadata.Parents) != 1 || metadata.Parents[0] != wantParent || r.URL.Query().Get("supportsAllDrives") != "true" {
						t.Errorf("copy request = %+v, %v", metadata, r.URL)
					}
					_, _ = io.WriteString(w, `{"id":"copy","name":"Copy of Original"}`)
				default:
					t.Errorf("unexpected request %s", r.URL)
					http.NotFound(w, r)
				}
			}))
			defer upstream.Close()
			client := APIClient{HTTP: upstream.Client(), BaseURL: upstream.URL}
			out, err := client.Copy(context.Background(), CopyInput{FileID: "original"})
			if err != nil || out.ID != "copy" {
				t.Fatalf("copy = %+v, %v", out, err)
			}
		})
	}
}

func TestPermissionsPaginatesAndPreservesGoogleErrors(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "denied") {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"error":{"message":"No permission to access this file"}}`)
			return
		}
		if r.URL.Query().Get("supportsAllDrives") != "true" {
			t.Errorf("missing shared-drive support: %v", r.URL)
		}
		if r.URL.Query().Get("pageToken") == "second" {
			_, _ = io.WriteString(w, `{"permissions":[{"type":"domain","role":"reader","displayName":"example.com"}]}`)
		} else {
			_, _ = io.WriteString(w, `{"nextPageToken":"second","permissions":[{"type":"user","role":"owner","emailAddress":"owner@example.com"}]}`)
		}
	}))
	defer upstream.Close()
	client := APIClient{HTTP: upstream.Client(), BaseURL: upstream.URL}
	out, err := client.Permissions(context.Background(), IDInput{FileID: "file"})
	if err != nil || len(out.Permissions) != 2 || out.Permissions[1].Type != "domain" {
		t.Fatalf("permissions = %+v, %v", out, err)
	}
	_, err = client.Metadata(context.Background(), FileInput{FileID: "denied"})
	var apiError APIError
	if !errors.As(err, &apiError) || apiError.StatusCode != 403 || !strings.Contains(err.Error(), "No permission") {
		t.Fatalf("error = %v", err)
	}
}

func TestRecentFilesAndNativeSpreadsheetDownloads(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/drive/v3/files":
			if r.URL.Query().Get("orderBy") != "modifiedByMeTime desc" || r.URL.Query().Get("q") != "trashed = false" {
				t.Errorf("recent query = %v", r.URL.Query())
			}
			_, _ = io.WriteString(w, `{"files":[{"id":"sheet","name":"Costs","mimeType":"application/vnd.google-apps.spreadsheet"}]}`)
		case "/drive/v3/files/sheet":
			_, _ = io.WriteString(w, `{"id":"sheet","name":"Costs","mimeType":"application/vnd.google-apps.spreadsheet"}`)
		case "/drive/v3/files/sheet/export":
			if r.URL.Query().Get("mimeType") == "text/csv" {
				_, _ = io.WriteString(w, "item,cost\nbolt,1\n")
			} else if r.URL.Query().Get("mimeType") == "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
				_, _ = w.Write([]byte{'P', 'K', 3, 4})
			} else if r.URL.Query().Get("mimeType") == "application/pdf" {
				_, _ = io.WriteString(w, "%PDF")
			} else {
				t.Errorf("invalid export type %v", r.URL)
			}
		default:
			t.Errorf("unexpected request %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	client := APIClient{HTTP: upstream.Client(), BaseURL: upstream.URL}
	list, err := client.Recent(context.Background(), RecentInput{OrderBy: "lastModifiedByMe"})
	if err != nil || len(list.Files) != 1 {
		t.Fatalf("recent = %+v, %v", list, err)
	}
	read, err := client.Read(context.Background(), ReadInput{FileID: "sheet"})
	if err != nil || !strings.HasPrefix(read.FileContent, "First worksheet only") || !strings.Contains(read.FileContent, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet") || !strings.HasSuffix(read.FileContent, "item,cost\nbolt,1\n") {
		t.Fatalf("spreadsheet text must disclose the missing worksheets: %+v, %v", read, err)
	}
	for _, format := range []string{"", "text/csv", "application/pdf"} {
		body, mimeType, err := client.Download(context.Background(), DownloadInput{FileID: "sheet", ExportMIMEType: format})
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(body)
		body.Close()
		want := string([]byte{'P', 'K', 3, 4})
		wantType := "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
		if format == "text/csv" {
			want = "item,cost\nbolt,1\n"
			wantType = format
		} else if format == "application/pdf" {
			want = "%PDF"
			wantType = format
		}
		if err != nil || string(data) != want || mimeType != wantType {
			t.Fatalf("export = %q, %q, %v", data, mimeType, err)
		}
	}
}

func TestNativeRevisionUsesItsOwnExportLink(t *testing.T) {
	requests := 0
	client := APIClient{HTTP: &http.Client{Transport: driveTransportFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		content := ""
		switch {
		case r.URL.Path == "/drive/v3/files/doc":
			content = `{"mimeType":"application/vnd.google-apps.document"}`
		case r.URL.Path == "/drive/v3/files/doc/revisions/version":
			content = `{"exportLinks":{"text/plain":"https://docs.google.com/document/export?revision=version"}}`
		case r.URL.Host == "docs.google.com" && r.URL.Query().Get("revision") == "version":
			content = "historical content"
		default:
			t.Errorf("unexpected request %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(content))}, nil
	})}}
	body, mimeType, err := client.Download(context.Background(), DownloadInput{FileID: "doc", RevisionID: "version"})
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	content, err := io.ReadAll(body)
	if err != nil || string(content) != "historical content" || mimeType != "text/plain" || requests != 3 {
		t.Fatalf("revision download = %q, %q, %d requests, %v", content, mimeType, requests, err)
	}
}

func TestUploadRejectsMissingAndForeignSessionLocations(t *testing.T) {
	for _, location := range []string{"", "https://another.example/upload", "https://user:password@www.googleapis.com/upload"} {
		t.Run(location, func(t *testing.T) {
			puts := 0
			client := APIClient{HTTP: &http.Client{Transport: driveTransportFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method == http.MethodPut {
					puts++
				}
				header := make(http.Header)
				header.Set("Location", location)
				return &http.Response{StatusCode: http.StatusOK, Request: r, Header: header, Body: io.NopCloser(strings.NewReader(`{"id":"unexpected"}`))}, nil
			})}}
			_, err := client.Create(context.Background(), CreateInput{Title: "file", ContentMIMEType: "text/plain", DisableConversionToGoogleType: true}, strings.NewReader("content"))
			if err == nil || !strings.Contains(err.Error(), "invalid upload location") || puts != 0 {
				t.Fatalf("invalid session location accepted: %q, %d uploads, %v", location, puts, err)
			}
		})
	}
}

type driveTransportFunc func(*http.Request) (*http.Response, error)

func (f driveTransportFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
