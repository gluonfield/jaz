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
	var eventRows []eventdb.ListSessionEventPageRow
	eventsMore := false
	if request.HistoryRevision == 0 || request.BeforeEventSeq > 0 {
		eventRows, eventsMore, err = loadTranscriptEventRows(ctx, events, id, request.BeforeEventSeq)
		if err != nil {
			return storage.TranscriptPage{}, err
		}
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
		messageStart := int64(0)
		if messageHasEarlier {
			messageStart = boundaries[request.Turns-1].Seq
		}
		baseStart := messageStart
		messageRows, loadErr := messages.ListMessageRangeSizes(ctx, messagedb.ListMessageRangeSizesParams{
			ThreadID: id, StartSeq: messageStart, BeforeSeq: request.BeforeMessageSeq,
		})
		if loadErr != nil {
			return storage.TranscriptPage{}, loadErr
		}
		eventOwner := int64(0)
		if len(eventRows) > 0 {
			owner, ownerErr := messages.LatestUserMessageBeforeEvent(ctx, messagedb.LatestUserMessageBeforeEventParams{
				ThreadID: id, CreatedAtMs: eventRows[len(eventRows)-1].CreatedAtMs,
			})
			if ownerErr != nil && ownerErr != sql.ErrNoRows {
				return storage.TranscriptPage{}, ownerErr
			}
			if ownerErr == nil {
				eventOwner = owner.Seq
			}
		}
		alignedStart := eventAlignedMessageStart(messageRows, messageStart, eventOwner)
		eventAligned := alignedStart > messageStart
		var truncated bool
		messageStart, truncated, err = boundedTranscriptMessageStart(messageRows, alignedStart)
		messageHasEarlier = messageHasEarlier || truncated || messageStart > baseStart
		if err == nil {
			err = loadTranscriptMessages(ctx, messages, id, messageStart, request.BeforeMessageSeq, &page)
		}
		if err != nil {
			return storage.TranscriptPage{}, fmt.Errorf("load transcript cursor: %w", err)
		}
		if eventAligned && messageStart > 0 {
			startAt, loadErr := messages.GetMessageTime(ctx, messagedb.GetMessageTimeParams{
				ThreadID: id, Seq: messageStart,
			})
			if loadErr != nil {
				return storage.TranscriptPage{}, loadErr
			}
			for len(eventRows) > 1 && eventRows[len(eventRows)-1].CreatedAtMs < startAt {
				eventRows = eventRows[:len(eventRows)-1]
				eventsMore = true
			}
		}
		if messageHasEarlier {
			page.BeforeMessageSeq = messageStart
		} else {
			page.BeforeMessageSeq = 0
		}
	}
	page.Events = make([]sessionevents.Event, 0, len(eventRows))
	for i := len(eventRows) - 1; i >= 0; i-- {
		row := eventRows[i]
		event, decodeErr := eventFromDBFields(row.ThreadID, row.Seq, row.ProjectionKey, row.ProjectionOp, row.Type, row.Content, row.Acp, row.Plan, row.Permission, row.CreatedAtMs)
		if decodeErr != nil {
			return storage.TranscriptPage{}, decodeErr
		}
		page.Events = append(page.Events, event)
	}
	if eventsMore && len(page.Events) > 0 {
		page.BeforeEventSeq = page.Events[0].Seq
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

func loadTranscriptEventRows(
	ctx context.Context,
	events *eventdb.Queries,
	id string,
	beforeSeq int64,
) ([]eventdb.ListSessionEventPageRow, bool, error) {
	sizes, err := events.ListSessionEventPageSizes(ctx, eventdb.ListSessionEventPageSizesParams{
		ThreadID: id, BeforeSeq: beforeSeq, LimitCount: transcriptEventPageRows + 1,
	})
	if err != nil {
		return nil, false, err
	}
	selected := 0
	selectedBytes := int64(0)
	for selected < len(sizes) && selected < transcriptEventPageRows {
		if sizes[selected].Bytes > transcriptEventPageBytes {
			return nil, false, fmt.Errorf("session event %d is %d bytes; limit is %d", sizes[selected].Seq, sizes[selected].Bytes, transcriptEventPageBytes)
		}
		if selectedBytes+sizes[selected].Bytes > transcriptEventPageBytes {
			break
		}
		selectedBytes += sizes[selected].Bytes
		selected++
	}
	rows, err := events.ListSessionEventPage(ctx, eventdb.ListSessionEventPageParams{
		ThreadID: id, BeforeSeq: beforeSeq, LimitCount: int64(selected),
	})
	return rows, len(sizes) > selected, err
}

// ACP events replace assistant message rows for some turns. Return the earliest
// suffix whose turns preceding the loaded event owner all have assistant rows.
func eventAlignedMessageStart(rows []messagedb.ListMessageRangeSizesRow, startSeq, eventOwnerSeq int64) int64 {
	if eventOwnerSeq > startSeq {
		openUser := int64(0)
		answered := false
		ownerSeen := false
		for i := len(rows) - 1; i >= 0; i-- {
			row := rows[i]
			if row.Seq > eventOwnerSeq {
				break
			}
			switch row.Role {
			case "user":
				if openUser > 0 && !answered {
					startSeq = row.Seq
				}
				openUser = row.Seq
				answered = false
				ownerSeen = row.Seq == eventOwnerSeq
			case "assistant":
				answered = openUser > 0
			}
			if ownerSeen {
				break
			}
		}
		if !ownerSeen && openUser > 0 && openUser < eventOwnerSeq && !answered {
			startSeq = eventOwnerSeq
		}
	}
	return startSeq
}

func boundedTranscriptMessageStart(rows []messagedb.ListMessageRangeSizesRow, startSeq int64) (int64, bool, error) {
	bytes := int64(0)
	acceptedStart := int64(0)
	for _, row := range rows {
		if row.Seq < startSeq {
			break
		}
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
