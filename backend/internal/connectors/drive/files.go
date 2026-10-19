package drive

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

const fileFields = "id,name,mimeType,parents,size,fileExtension,description,webViewLink,owners(emailAddress),createdTime,modifiedTime,viewedByMeTime,sharedWithMeTime,capabilities(canAddChildren),exportLinks"

type apiFile struct {
	File
	Name        string            `json:"name"`
	Parents     []string          `json:"parents"`
	Size        string            `json:"size"`
	WebViewLink string            `json:"webViewLink"`
	ExportLinks map[string]string `json:"exportLinks"`
	Owners      []struct {
		Email string `json:"emailAddress"`
	} `json:"owners"`
	Capabilities struct {
		CanAddChildren *bool `json:"canAddChildren"`
	} `json:"capabilities"`
}

func (f apiFile) view(excludeSnippets bool, verbosity string) File {
	out := f.File
	out.Title = f.Name
	out.FileSize = f.Size
	out.ViewURL = f.WebViewLink
	out.CanAddChildren = f.Capabilities.CanAddChildren
	if len(f.Parents) > 0 {
		out.ParentID = f.Parents[0]
	}
	if len(f.Owners) > 0 {
		out.Owner = f.Owners[0].Email
	}
	if !excludeSnippets {
		limit := 5000
		switch verbosity {
		case "BRIEF":
			limit = 1000
		case "MEDIUM":
			limit = 2500
		case "MAX_ALLOWED":
			limit = 20000
		}
		runes := []rune(f.Description)
		out.ContentSnippet = string(runes[:min(len(runes), limit)])
	}
	return out
}

func fileQuery() url.Values {
	return url.Values{"fields": {fileFields}, "supportsAllDrives": {"true"}}
}

func (c APIClient) metadata(ctx context.Context, id string) (apiFile, error) {
	path, err := filePath(id)
	if err != nil {
		return apiFile{}, err
	}
	var out apiFile
	err = c.json(ctx, http.MethodGet, path, fileQuery(), nil, &out)
	return out, err
}

func (c APIClient) Metadata(ctx context.Context, input FileInput) (File, error) {
	file, err := c.metadata(ctx, input.FileID)
	return file.view(input.ExcludeContentSnippets, input.SnippetVerbosity), err
}

func (c APIClient) Search(ctx context.Context, input SearchInput) (FileList, error) {
	query, err := searchQuery(input.Query)
	if err != nil {
		return FileList{}, err
	}
	return c.list(ctx, query, "", input.ListOptions)
}

func (c APIClient) Recent(ctx context.Context, input RecentInput) (FileList, error) {
	order := "recency desc"
	switch input.OrderBy {
	case "lastModified":
		order = "modifiedTime desc"
	case "lastModifiedByMe":
		order = "modifiedByMeTime desc"
	}
	return c.list(ctx, "trashed = false", order, input.ListOptions)
}

func (c APIClient) list(ctx context.Context, query, order string, input ListOptions) (FileList, error) {
	pageSize := input.PageSize
	if pageSize <= 0 {
		pageSize = 10
	}
	q := url.Values{
		"q":                         {query},
		"fields":                    {"nextPageToken,files(" + fileFields + ")"},
		"pageSize":                  {strconv.Itoa(min(pageSize, 1000))},
		"supportsAllDrives":         {"true"},
		"includeItemsFromAllDrives": {"true"},
	}
	if input.PageToken != "" {
		q.Set("pageToken", input.PageToken)
	}
	if order != "" {
		q.Set("orderBy", order)
	}
	var list struct {
		Files         []apiFile `json:"files"`
		NextPageToken string    `json:"nextPageToken"`
	}
	if err := c.json(ctx, http.MethodGet, "/drive/v3/files", q, nil, &list); err != nil {
		return FileList{}, err
	}
	out := FileList{Files: make([]File, 0, len(list.Files)), NextPageToken: list.NextPageToken}
	for _, file := range list.Files {
		out.Files = append(out.Files, file.view(input.ExcludeContentSnippets, input.SnippetVerbosity))
	}
	return out, nil
}

func (c APIClient) Copy(ctx context.Context, input CopyInput) (File, error) {
	path, err := filePath(input.FileID)
	if err != nil {
		return File{}, err
	}
	name := input.Title
	parent := input.ParentID
	if name == "" || parent == "" {
		original, err := c.metadata(ctx, input.FileID)
		if err != nil {
			return File{}, err
		}
		if name == "" {
			name = "Copy of " + original.Name
		}
		if parent == "" && len(original.Parents) > 0 {
			folder, err := c.metadata(ctx, original.Parents[0])
			var denied APIError
			if err != nil && !(errors.As(err, &denied) && (denied.StatusCode == http.StatusForbidden || denied.StatusCode == http.StatusNotFound)) {
				return File{}, err
			}
			parent = original.Parents[0]
			if err != nil || (folder.Capabilities.CanAddChildren != nil && !*folder.Capabilities.CanAddChildren) {
				parent = "root"
			}
		}
	}
	body := fileMetadata{Name: name}
	if parent != "" {
		body.Parents = []string{parent}
	}
	var out apiFile
	err = c.json(ctx, http.MethodPost, path+"/copy", fileQuery(), body, &out)
	return out.view(true, ""), err
}

func (c APIClient) Permissions(ctx context.Context, input IDInput) (Permissions, error) {
	path, err := filePath(input.FileID)
	if err != nil {
		return Permissions{}, err
	}
	q := url.Values{"supportsAllDrives": {"true"}, "fields": {"nextPageToken,permissions(type,role,emailAddress,displayName,view)"}, "pageSize": {"100"}}
	out := Permissions{Permissions: []Permission{}}
	for {
		var page struct {
			Permissions
			NextPageToken string `json:"nextPageToken"`
		}
		if err := c.json(ctx, http.MethodGet, path+"/permissions", q, nil, &page); err != nil {
			return Permissions{}, err
		}
		out.Permissions = append(out.Permissions, page.Permissions.Permissions...)
		if page.NextPageToken == "" {
			return out, nil
		}
		if page.NextPageToken == q.Get("pageToken") {
			return Permissions{}, fmt.Errorf("Google Drive repeated a permissions page token")
		}
		q.Set("pageToken", page.NextPageToken)
	}
}
