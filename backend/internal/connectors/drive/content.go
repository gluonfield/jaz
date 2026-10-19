package drive

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"
)

const maxReadBytes = 10 << 20

func (c APIClient) Download(ctx context.Context, input DownloadInput) (io.ReadCloser, string, error) {
	file, err := c.metadata(ctx, input.FileID)
	if err != nil {
		return nil, "", err
	}
	return c.download(ctx, input, file)
}

func (c APIClient) download(ctx context.Context, input DownloadInput, file apiFile) (io.ReadCloser, string, error) {
	path, err := filePath(input.FileID)
	if err != nil {
		return nil, "", err
	}
	q := url.Values{"alt": {"media"}}
	mimeType := file.MIMEType
	if strings.HasPrefix(mimeType, "application/vnd.google-apps.") {
		exportType := input.ExportMIMEType
		if exportType == "" {
			exportType = "text/plain"
			if mimeType == "application/vnd.google-apps.spreadsheet" {
				exportType = "text/csv"
			}
		}
		if input.RevisionID != "" {
			var revision struct {
				ExportLinks map[string]string `json:"exportLinks"`
			}
			if err := c.json(ctx, http.MethodGet, path+"/revisions/"+url.PathEscape(input.RevisionID), url.Values{"fields": {"exportLinks"}}, nil, &revision); err != nil {
				return nil, "", err
			}
			link, err := url.Parse(revision.ExportLinks[exportType])
			if err != nil || link == nil || link.Scheme != "https" || !(link.Hostname() == "www.googleapis.com" || link.Hostname() == "docs.google.com") || link.User != nil {
				return nil, "", fmt.Errorf("revision has no supported export link for %s", exportType)
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, link.String(), nil)
			if err != nil {
				return nil, "", err
			}
			resp, err := c.HTTP.Do(req)
			if err != nil {
				return nil, "", err
			}
			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				return nil, "", APIError{StatusCode: resp.StatusCode, Message: resp.Status}
			}
			return resp.Body, exportType, nil
		}
		path += "/export"
		q = url.Values{"mimeType": {exportType}}
		mimeType = exportType
	} else if input.RevisionID != "" {
		path += "/revisions/" + url.PathEscape(input.RevisionID)
	} else {
		q.Set("supportsAllDrives", "true")
	}
	resp, err := c.request(ctx, http.MethodGet, path, q, nil, "")
	if err != nil {
		return nil, "", err
	}
	return resp.Body, mimeType, nil
}

func (c APIClient) Read(ctx context.Context, input ReadInput) (FileContent, error) {
	file, err := c.metadata(ctx, input.FileID)
	if err != nil {
		return FileContent{}, err
	}
	out := FileContent{Title: file.Name, ViewURL: file.WebViewLink}
	native := file.MIMEType == "application/vnd.google-apps.document" || file.MIMEType == "application/vnd.google-apps.spreadsheet" || file.MIMEType == "application/vnd.google-apps.presentation"
	if native || strings.HasPrefix(file.MIMEType, "text/") || file.MIMEType == "application/json" || file.MIMEType == "application/xml" {
		body, _, err := c.download(ctx, DownloadInput{FileID: input.FileID}, file)
		if err != nil {
			return FileContent{}, err
		}
		defer body.Close()
		data, err := io.ReadAll(io.LimitReader(body, maxReadBytes+1))
		if err != nil {
			return FileContent{}, err
		}
		if len(data) > maxReadBytes {
			return FileContent{}, fmt.Errorf("file text exceeds 10 MiB; use download_file_content")
		}
		if !utf8.Valid(data) {
			out.TextFormattingNotSupported = true
		} else {
			out.FileContent = string(data)
		}
	} else {
		out.TextFormattingNotSupported = true
	}
	if input.IncludeComments {
		if native {
			out.CommentThreads, err = c.comments(ctx, input.FileID)
		} else {
			out.CommentsNotSupported = true
		}
	}
	return out, err
}

type apiPost struct {
	ID           string `json:"id"`
	Content      string `json:"content"`
	ModifiedTime string `json:"modifiedTime"`
	Author       struct {
		DisplayName string `json:"displayName"`
	} `json:"author"`
}

func (p apiPost) post() Post {
	return Post{PostID: p.ID, Content: p.Content, ModifiedTime: p.ModifiedTime, AuthorName: p.Author.DisplayName}
}

func (c APIClient) comments(ctx context.Context, id string) ([]CommentThread, error) {
	path, err := filePath(id)
	if err != nil {
		return nil, err
	}
	q := url.Values{"fields": {"nextPageToken,comments(id,content,modifiedTime,author(displayName),resolved,replies(id,content,modifiedTime,author(displayName)))"}, "pageSize": {"100"}}
	var out []CommentThread
	for {
		var page struct {
			Comments []struct {
				apiPost
				Resolved bool      `json:"resolved"`
				Replies  []apiPost `json:"replies"`
			} `json:"comments"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := c.json(ctx, http.MethodGet, path+"/comments", q, nil, &page); err != nil {
			return nil, err
		}
		for _, comment := range page.Comments {
			thread := CommentThread{CommentID: comment.ID, HeadPost: comment.post(), Status: "OPEN"}
			if comment.Resolved {
				thread.Status = "RESOLVED"
			}
			for _, reply := range comment.Replies {
				thread.Replies = append(thread.Replies, reply.post())
			}
			out = append(out, thread)
		}
		if page.NextPageToken == "" {
			return out, nil
		}
		if page.NextPageToken == q.Get("pageToken") {
			return nil, fmt.Errorf("Google Drive repeated a comments page token")
		}
		q.Set("pageToken", page.NextPageToken)
	}
}
