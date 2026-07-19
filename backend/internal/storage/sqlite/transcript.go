package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/eventdb"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/messagedb"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/threaddb"
)

const (
	transcriptEventPageRows  = 256
	transcriptEventPageBytes = storage.MaxTranscriptEventBytes
)

func (s *Store) LoadTranscriptSession(ctx context.Context, ref string) (storage.Session, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return storage.Session{}, fmt.Errorf("session id or slug is required")
	}
	row, err := threaddb.New(s.db).GetSession(ctx, ref)
	if err == sql.ErrNoRows {
		return storage.Session{}, fmt.Errorf("%w: %s", storage.ErrSessionNotFound, ref)
	}
	if err != nil {
		return storage.Session{}, err
	}
	return sessionFromDB(row)
}

func (s *Store) LoadTranscriptPage(ctx context.Context, id string, request storage.TranscriptPageRequest) (storage.TranscriptPage, error) {
	if request.Turns <= 0 {
		return storage.TranscriptPage{}, fmt.Errorf("turn count must be positive")
	}
	if (request.BeforeMessageSeq > 0 || request.BeforeEventSeq > 0) && request.HistoryRevision <= 0 {
		return storage.TranscriptPage{}, fmt.Errorf("history revision is required with a transcript cursor")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return storage.TranscriptPage{}, err
	}
	defer tx.Rollback()

	messages := messagedb.New(tx)
	page := storage.TranscriptPage{}
	events := eventdb.New(tx)
	page.HistoryRevision, err = threaddb.New(tx).GetTranscriptRevision(ctx, id)
	if err != nil {
		return storage.TranscriptPage{}, err
	}
	if request.HistoryRevision > 0 && request.HistoryRevision != page.HistoryRevision {
		return storage.TranscriptPage{}, storage.ErrTranscriptChanged
	}
	messageHasEarlier := false
	if request.HistoryRevision == 0 || request.BeforeMessageSeq > 0 {
		boundaries, loadErr := messages.ListUserMessageBoundaries(ctx, messagedb.ListUserMessageBoundariesParams{
			ThreadID: id, BeforeSeq: request.BeforeMessageSeq, LimitCount: int64(request.Turns + 1),
		})
		if loadErr != nil {
			return storage.TranscriptPage{}, loadErr
		}
		messageHasEarlier = len(boundaries) > request.Turns
		if messageHasEarlier {
			page.BeforeMessageSeq = boundaries[request.Turns-1].Seq
		}
		var truncated bool
		page.BeforeMessageSeq, truncated, err = boundedTranscriptMessages(
			ctx, messages, id, page.BeforeMessageSeq, request.BeforeMessageSeq,
		)
		messageHasEarlier = messageHasEarlier || truncated
		if err == nil {
			err = loadTranscriptMessages(ctx, messages, id, page.BeforeMessageSeq, request.BeforeMessageSeq, &page)
		}
		if err != nil {
			return storage.TranscriptPage{}, fmt.Errorf("load transcript cursor: %w", err)
		}
	}

	eventsMore := false
	if request.HistoryRevision == 0 || request.BeforeEventSeq > 0 {
		sizes, loadErr := events.ListSessionEventPageSizes(ctx, eventdb.ListSessionEventPageSizesParams{
			ThreadID: id, BeforeSeq: request.BeforeEventSeq, LimitCount: transcriptEventPageRows + 1,
		})
		if loadErr != nil {
			return storage.TranscriptPage{}, loadErr
		}
		selected := 0
		selectedBytes := int64(0)
		for selected < len(sizes) && selected < transcriptEventPageRows {
			if sizes[selected].Bytes > transcriptEventPageBytes {
				return storage.TranscriptPage{}, fmt.Errorf("session event %d is %d bytes; limit is %d", sizes[selected].Seq, sizes[selected].Bytes, transcriptEventPageBytes)
			}
			if selectedBytes+sizes[selected].Bytes > transcriptEventPageBytes {
				break
			}
			selectedBytes += sizes[selected].Bytes
			selected++
		}
		eventRows, loadErr := events.ListSessionEventPage(ctx, eventdb.ListSessionEventPageParams{
			ThreadID: id, BeforeSeq: request.BeforeEventSeq, LimitCount: int64(selected),
		})
		if loadErr != nil {
			return storage.TranscriptPage{}, loadErr
		}
		eventsMore = len(sizes) > selected
		page.Events = make([]sessionevents.Event, 0, len(eventRows))
		for i := len(eventRows) - 1; i >= 0; i-- {
			row := eventRows[i]
			event, decodeErr := eventFromDBFields(row.ThreadID, row.Seq, row.ProjectionKey, row.ProjectionOp, row.Type, row.Content, row.Acp, row.Plan, row.Permission, row.CreatedAtMs)
			if decodeErr != nil {
				return storage.TranscriptPage{}, decodeErr
			}
			page.Events = append(page.Events, event)
		}
		if eventsMore {
			page.BeforeEventSeq = page.Events[0].Seq
		}
	}
	page.HasEarlier = eventsMore || messageHasEarlier
	page.LatestEventSeq, err = events.LatestSessionEventSeq(ctx, id)
	if err != nil {
		return storage.TranscriptPage{}, err
	}
	if err := tx.Commit(); err != nil {
		return storage.TranscriptPage{}, err
	}
	return page, nil
}

func boundedTranscriptMessages(
	ctx context.Context,
	messages *messagedb.Queries,
	id string,
	startSeq int64,
	beforeSeq int64,
) (int64, bool, error) {
	rows, err := messages.ListMessageRangeSizes(ctx, messagedb.ListMessageRangeSizesParams{
		ThreadID: id, StartSeq: startSeq, BeforeSeq: beforeSeq,
	})
	if err != nil {
		return 0, false, err
	}
	bytes := int64(0)
	acceptedStart := int64(0)
	for _, row := range rows {
		if row.Bytes > storage.MaxTranscriptMessageBytes {
			return 0, false, fmt.Errorf("message %d is %d bytes; limit is %d", row.Seq, row.Bytes, storage.MaxTranscriptMessageBytes)
		}
		if bytes+row.Bytes > storage.MaxTranscriptMessageBytes {
			if acceptedStart == 0 {
				return 0, false, fmt.Errorf("latest transcript turn exceeds %d bytes", storage.MaxTranscriptMessageBytes)
			}
			return acceptedStart, true, nil
		}
		bytes += row.Bytes
		if row.Role == "user" {
			acceptedStart = row.Seq
		}
	}
	return startSeq, false, nil
}

func loadTranscriptMessages(ctx context.Context, messages *messagedb.Queries, id string, startSeq, beforeSeq int64, page *storage.TranscriptPage) error {
	rows, err := messages.ListMessagesByThreadRange(ctx, messagedb.ListMessagesByThreadRangeParams{
		ThreadID: id, StartSeq: startSeq, BeforeSeq: beforeSeq,
	})
	if err != nil {
		return err
	}
	page.Messages = make([]storage.Message, 0, len(rows))
	for _, row := range rows {
		blocks, decodeErr := unmarshalBlocks(row.Blocks)
		if decodeErr != nil {
			return decodeErr
		}
		page.Messages = append(page.Messages, storage.Message{
			ThreadID: row.ThreadID, Seq: row.Seq, Role: row.Role, Content: row.Content,
			Reasoning: row.Reasoning.String, Blocks: blocks, CreatedAt: msToTime(row.CreatedAtMs),
		})
	}
	return nil
}
