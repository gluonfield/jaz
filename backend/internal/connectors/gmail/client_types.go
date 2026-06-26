package gmail

type IDType string

const (
	IDTypeMessage IDType = "message"
	IDTypeThread  IDType = "thread"
)

func (t IDType) valid() bool {
	return t == "" || t == IDTypeMessage || t == IDTypeThread
}

type Profile struct {
	EmailAddress  string `json:"emailAddress"`
	MessagesTotal int64  `json:"messagesTotal"`
	ThreadsTotal  int64  `json:"threadsTotal"`
	HistoryID     string `json:"historyId"`
}

type SearchThreadsRequest struct {
	Query      string
	MaxResults int
}

type SearchThreadsResponse struct {
	Threads            []Thread `json:"threads"`
	NextPageToken      string   `json:"next_page_token,omitempty"`
	ResultSizeEstimate int64    `json:"result_size_estimate,omitempty"`
}

type ReadThreadRequest struct {
	ID          string
	IDType      IDType
	MaxMessages int
}

type ThreadContent struct {
	ID        string           `json:"id"`
	HistoryID string           `json:"history_id,omitempty"`
	Snippet   string           `json:"snippet,omitempty"`
	Messages  []MessageContent `json:"messages,omitempty"`
}

type MessageContent struct {
	Message  Message `json:"message"`
	BodyText string  `json:"body_text,omitempty"`
	BodyHTML string  `json:"body_html,omitempty"`
}

type ComposeMessageRequest struct {
	ThreadID   string
	To         []string
	Cc         []string
	Bcc        []string
	Subject    string
	BodyText   string
	InReplyTo  string
	References string
}

type DraftContent struct {
	Draft    Draft  `json:"draft"`
	BodyText string `json:"body_text,omitempty"`
	BodyHTML string `json:"body_html,omitempty"`
}

type ListDraftsRequest struct {
	Query      string
	MaxResults int
}

type ListDraftsResponse struct {
	Drafts             []Draft `json:"drafts"`
	NextPageToken      string  `json:"next_page_token,omitempty"`
	ResultSizeEstimate int64   `json:"result_size_estimate,omitempty"`
}
