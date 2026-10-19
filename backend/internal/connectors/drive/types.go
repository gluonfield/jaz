package drive

type File struct {
	ID               string `json:"id,omitempty"`
	Title            string `json:"title,omitempty"`
	MIMEType         string `json:"mimeType,omitempty"`
	ParentID         string `json:"parentId,omitempty"`
	FileSize         string `json:"fileSize,omitempty"`
	FileExtension    string `json:"fileExtension,omitempty"`
	Description      string `json:"description,omitempty"`
	ContentSnippet   string `json:"contentSnippet,omitempty"`
	ViewURL          string `json:"viewUrl,omitempty"`
	Owner            string `json:"owner,omitempty"`
	CreatedTime      string `json:"createdTime,omitempty"`
	ModifiedTime     string `json:"modifiedTime,omitempty"`
	ViewedByMeTime   string `json:"viewedByMeTime,omitempty"`
	SharedWithMeTime string `json:"sharedWithMeTime,omitempty"`
	CanAddChildren   *bool  `json:"canAddChildren,omitempty"`
}

type FileList struct {
	Files         []File `json:"files"`
	NextPageToken string `json:"nextPageToken,omitempty"`
}

type ListOptions struct {
	PageSize               int    `json:"pageSize,omitempty"`
	PageToken              string `json:"pageToken,omitempty"`
	ExcludeContentSnippets bool   `json:"excludeContentSnippets,omitempty"`
	SnippetVerbosity       string `json:"snippetVerbosity,omitempty"`
}

type SearchInput struct {
	ListOptions
	Query string `json:"query,omitempty"`
}

type RecentInput struct {
	ListOptions
	OrderBy string `json:"orderBy,omitempty"`
}

type FileInput struct {
	FileID                 string `json:"fileId" jsonschema:"Exact Drive file ID from discovery or supplied by the user"`
	ExcludeContentSnippets bool   `json:"excludeContentSnippets,omitempty"`
	SnippetVerbosity       string `json:"snippetVerbosity,omitempty"`
}

type IDInput struct {
	FileID string `json:"fileId" jsonschema:"Exact Drive file ID from discovery or supplied by the user"`
}

type CopyInput struct {
	FileID   string `json:"fileId"`
	Title    string `json:"title,omitempty"`
	ParentID string `json:"parentId,omitempty"`
}

type CreateInput struct {
	Title                         string  `json:"title"`
	ParentID                      string  `json:"parentId,omitempty"`
	ContentMIMEType               string  `json:"contentMimeType,omitempty"`
	MIMEType                      string  `json:"mimeType,omitempty" jsonschema:"Deprecated alias for contentMimeType"`
	TextContent                   *string `json:"textContent,omitempty"`
	Base64Content                 *string `json:"base64Content,omitempty"`
	Content                       *string `json:"content,omitempty" jsonschema:"Deprecated alias for base64Content"`
	FilePath                      string  `json:"filePath,omitempty" jsonschema:"Absolute file path accessible to the Jaz server or agent workspace; uploads directly without base64"`
	DisableConversionToGoogleType bool    `json:"disableConversionToGoogleType,omitempty"`
}

type DownloadInput struct {
	FileID         string `json:"fileId"`
	ExportMIMEType string `json:"exportMimeType,omitempty"`
	RevisionID     string `json:"revisionId,omitempty"`
	FilePath       string `json:"filePath,omitempty" jsonschema:"Optional absolute destination in the server/agent workspace; streams bytes to a new file instead of returning inline base64"`
}

type ReadInput struct {
	FileID          string `json:"fileId"`
	IncludeComments bool   `json:"includeComments,omitempty"`
}

type Permission struct {
	Type         string `json:"type,omitempty"`
	Role         string `json:"role,omitempty"`
	EmailAddress string `json:"emailAddress,omitempty"`
	DisplayName  string `json:"displayName,omitempty"`
	View         string `json:"view,omitempty"`
}

type Permissions struct {
	Permissions []Permission `json:"permissions"`
}

type Post struct {
	PostID       string `json:"postId,omitempty"`
	AuthorName   string `json:"authorName,omitempty"`
	Content      string `json:"content,omitempty"`
	ModifiedTime string `json:"modifiedTime,omitempty"`
}

type CommentThread struct {
	CommentID string `json:"commentId,omitempty"`
	HeadPost  Post   `json:"headPost"`
	Replies   []Post `json:"replies,omitempty"`
	Status    string `json:"status,omitempty"`
}

type FileContent struct {
	Title                      string          `json:"title,omitempty"`
	ViewURL                    string          `json:"viewUrl,omitempty"`
	FileContent                string          `json:"fileContent,omitempty"`
	CommentThreads             []CommentThread `json:"commentThreads,omitempty"`
	CommentsNotSupported       bool            `json:"commentsNotSupported,omitempty"`
	TextFormattingNotSupported bool            `json:"textFormattingNotSupported,omitempty"`
}
