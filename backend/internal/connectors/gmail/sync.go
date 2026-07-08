package gmail

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/wins/jaz/backend/pkg/integrations"
)

const (
	CursorKindSync      = "gmail.sync"
	SyncMessagePageSize = 25

	labelImportant = "IMPORTANT"
	labelStarred   = "STARRED"
)

// noiseCategories are Gmail's automated inbox categories excluded from memory
// sync: promotions/social/forums/updates are marketing and machine mail, while
// primary/personal correspondence carries no category label. IMPORTANT or
// STARRED always override the exclusion so flagged mail is never dropped.
var noiseCategories = []string{
	"CATEGORY_PROMOTIONS",
	"CATEGORY_SOCIAL",
	"CATEGORY_FORUMS",
	"CATEGORY_UPDATES",
}

func keepMessage(labels []string) bool {
	drop := false
	for _, label := range labels {
		if label == labelImportant || label == labelStarred {
			return true
		}
		if slices.Contains(noiseCategories, label) {
			drop = true
		}
	}
	return !drop
}

type SyncCursor struct {
	BackfillPageToken string `json:"backfill_page_token,omitempty"`
	BackfillComplete  bool   `json:"backfill_complete,omitempty"`
	HistoryID         string `json:"history_id,omitempty"`
	HistoryPageToken  string `json:"history_page_token,omitempty"`
}

func EncodeSyncCursor(cursor SyncCursor) (integrations.Cursor, error) {
	value, err := json.Marshal(cursor)
	if err != nil {
		return integrations.Cursor{}, err
	}
	return integrations.Cursor{Kind: CursorKindSync, Value: value}, nil
}

func DecodeSyncCursor(cursor integrations.Cursor) (SyncCursor, error) {
	if cursor.Empty() {
		return SyncCursor{}, nil
	}
	if cursor.Kind != CursorKindSync {
		return SyncCursor{}, errors.New("gmail sync cursor kind mismatch")
	}
	var out SyncCursor
	if len(cursor.Value) == 0 {
		return out, nil
	}
	return out, json.Unmarshal(cursor.Value, &out)
}

func (c APIClient) Observe(ctx context.Context, req integrations.ObserveRequest) (integrations.ObserveResult, error) {
	cursor, err := DecodeSyncCursor(req.Cursor)
	if err != nil {
		return integrations.ObserveResult{}, err
	}
	if req.Mode == integrations.ObserveModeBackfill || !cursor.BackfillComplete {
		return c.backfillMessages(ctx, req.Connection, cursor, req.Since)
	}
	return c.incrementalMessages(ctx, req.Connection, cursor)
}

func (c APIClient) backfillMessages(ctx context.Context, connection integrations.Connection, cursor SyncCursor, since time.Time) (integrations.ObserveResult, error) {
	q := url.Values{}
	q.Set("maxResults", strconv.Itoa(SyncMessagePageSize))
	if cursor.BackfillPageToken != "" {
		q.Set("pageToken", cursor.BackfillPageToken)
	}
	if !since.IsZero() {
		q.Set("q", "after:"+since.UTC().Format("2006/01/02"))
	}
	var list messageList
	if err := c.get(ctx, "gmail/v1/users/me/messages", q, &list); err != nil {
		return integrations.ObserveResult{}, err
	}
	records, err := c.messageRecords(ctx, connection, messageIDs(list.Messages))
	if err != nil {
		return integrations.ObserveResult{}, err
	}
	cursor.BackfillPageToken = list.NextPageToken
	if list.NextPageToken == "" {
		profile, err := c.Profile(ctx)
		if err != nil {
			return integrations.ObserveResult{}, err
		}
		cursor.BackfillComplete = true
		cursor.HistoryID = profile.HistoryID
		cursor.HistoryPageToken = ""
	}
	next, err := EncodeSyncCursor(cursor)
	if err != nil {
		return integrations.ObserveResult{}, err
	}
	return integrations.ObserveResult{Records: records, Cursor: next}, nil
}

func (c APIClient) incrementalMessages(ctx context.Context, connection integrations.Connection, cursor SyncCursor) (integrations.ObserveResult, error) {
	if cursor.HistoryID == "" {
		cursor.BackfillComplete = false
		next, err := EncodeSyncCursor(cursor)
		return integrations.ObserveResult{Cursor: next}, err
	}
	q := url.Values{}
	q.Set("startHistoryId", cursor.HistoryID)
	q.Set("historyTypes", "messageAdded")
	q.Set("maxResults", strconv.Itoa(SyncMessagePageSize))
	if cursor.HistoryPageToken != "" {
		q.Set("pageToken", cursor.HistoryPageToken)
	}
	var list historyList
	if err := c.get(ctx, "gmail/v1/users/me/history", q, &list); err != nil {
		if isAPIStatus(err, 404) {
			cursor = SyncCursor{}
			next, encodeErr := EncodeSyncCursor(cursor)
			if encodeErr != nil {
				return integrations.ObserveResult{}, encodeErr
			}
			return integrations.ObserveResult{Cursor: next}, nil
		}
		return integrations.ObserveResult{}, err
	}
	records, err := c.messageRecords(ctx, connection, historyMessageIDs(list.History))
	if err != nil {
		return integrations.ObserveResult{}, err
	}
	cursor.HistoryPageToken = list.NextPageToken
	if list.NextPageToken == "" && list.HistoryID != "" {
		cursor.HistoryID = list.HistoryID
	}
	next, err := EncodeSyncCursor(cursor)
	if err != nil {
		return integrations.ObserveResult{}, err
	}
	return integrations.ObserveResult{Records: records, Cursor: next}, nil
}

func (c APIClient) messageRecords(ctx context.Context, connection integrations.Connection, ids []string) ([]integrations.Record, error) {
	records := make([]integrations.Record, 0, len(ids))
	receivedAt := time.Now().UTC()
	for _, id := range ids {
		content, err := c.messageContent(ctx, id)
		if isAPIStatus(err, 404) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !keepMessage(content.Message.LabelIDs) {
			continue
		}
		record, err := MessageContentRecord(connection, content, receivedAt)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func isAPIStatus(err error, status int) bool {
	var apiErr APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == status
}

func (c APIClient) messageContent(ctx context.Context, id string) (MessageContent, error) {
	raw, err := c.message(ctx, id, "full")
	if err != nil {
		return MessageContent{}, err
	}
	return messageContentFromAPI(raw), nil
}

func messageIDs(messages []messageRef) []string {
	out := make([]string, 0, len(messages))
	for _, message := range messages {
		if message.ID != "" {
			out = append(out, message.ID)
		}
	}
	return out
}

func historyMessageIDs(history []historyEntry) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, entry := range history {
		for _, added := range entry.MessagesAdded {
			id := added.Message.ID
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	return out
}
