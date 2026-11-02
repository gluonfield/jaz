package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	modernsqlite "modernc.org/sqlite"
)

func TestBotChatPageBoundsMetadataReads(t *testing.T) {
	for _, count := range []int{10_000, 100_000} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			store, err := New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			bot, err := store.CreateSession(storage.CreateSession{SourceType: storage.SourceBot})
			if err != nil {
				t.Fatal(err)
			}
			events := []sessionevents.Event{{Type: "room_message", RoomMessage: &sessionevents.RoomMessageEvent{Speaker: "bot", Text: "Historical reply"}}}
			for i := range count {
				events = append(events, sessionevents.Event{Type: "acp", ACP: &sessionevents.ACPEvent{ID: fmt.Sprint(i), ParentID: bot.ID, State: "idle"}})
			}
			if err := store.AppendSessionEvents(bot.ID, events...); err != nil {
				t.Fatal(err)
			}
			stored, err := store.LoadSessionEvents(bot.ID)
			if err != nil || len(stored) != count+1 {
				t.Fatalf("workload stored %d of %d records: %v", len(stored), count+1, err)
			}
			bytes := 0
			for _, event := range stored {
				bytes += len(event.StorageContent())
				if event.ACP != nil {
					raw, err := json.Marshal(event.ACP)
					if err != nil {
						t.Fatal(err)
					}
					bytes += len(raw)
				}
			}
			if bytes >= storage.MaxTranscriptEventBytes {
				t.Fatal("workload does not fit the byte budget")
			}
			measured := &metadataReadConnector{dsn: filepath.Join(store.RootDir(), "jaz.sqlite")}
			if err := store.db.Close(); err != nil {
				t.Fatal(err)
			}
			store.db = sql.OpenDB(measured)
			page, err := store.LoadTranscriptPage(t.Context(), bot.ID, storage.TranscriptPageRequest{Turns: 14, ChatOnly: true})
			if err != nil {
				t.Fatal(err)
			}
			maxHeaders := 0
			for _, rows := range measured.headerRows {
				maxHeaders = max(maxHeaders, *rows)
			}
			t.Logf("history=%d payload_bytes=%d size_headers=%d returned_events=%d cursor=%d", count+1, bytes, maxHeaders, len(page.Events), page.BeforeEventSeq)
			if maxHeaders == 0 || maxHeaders > transcriptEventPageRows+1 || len(page.Events) > transcriptEventPageRows {
				t.Fatal("metadata tail bypassed the per-page read bound")
			}
			if len(measured.headerPlans) != 1 || !strings.Contains(measured.headerPlans[0], "USING INDEX idx_session_events_chat ") {
				t.Fatalf("size headers scanned unindexed history: %v", measured.headerPlans)
			}
			if len(measured.targetPlans) != 1 || !strings.Contains(measured.targetPlans[0], "USING INDEX idx_session_events_chat_entries ") {
				t.Fatalf("initial target scanned unindexed history: %v", measured.targetPlans)
			}
			if len(measured.boundaryPlans) != 1 || !strings.Contains(measured.boundaryPlans[0], "USING INDEX idx_session_events_chat_entries ") {
				t.Fatalf("conversation boundary scanned unindexed history: %v", measured.boundaryPlans)
			}
			if page.HistoryTargetEventSeq != 1 {
				t.Fatal("initial conversation target did not find the existing reply")
			}
			if !page.HasEarlier || page.BeforeEventSeq != int64(count+2-len(page.Events)) || page.LatestEventSeq != int64(count+1) {
				t.Fatal("bounded metadata page lost its continuation")
			}
		})
	}
}

type metadataReadConnector struct {
	dsn           string
	headerRows    []*int
	headerPlans   []string
	boundaryPlans []string
	targetPlans   []string
}

func (c *metadataReadConnector) Driver() driver.Driver { return &modernsqlite.Driver{} }

func (c *metadataReadConnector) Connect(context.Context) (driver.Conn, error) {
	conn, err := c.Driver().Open(c.dsn)
	if err != nil {
		return nil, err
	}
	return &metadataReadConn{Conn: conn, reads: c}, nil
}

type metadataReadConn struct {
	driver.Conn
	reads *metadataReadConnector
}

func (c *metadataReadConn) BeginTx(ctx context.Context, options driver.TxOptions) (driver.Tx, error) {
	return c.Conn.(driver.ConnBeginTx).BeginTx(ctx, options)
}

func (c *metadataReadConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	plan, err := c.Conn.(driver.QueryerContext).QueryContext(ctx, "EXPLAIN QUERY PLAN "+query, args)
	if err != nil {
		return nil, err
	}
	values := make([]driver.Value, 4)
	var details []string
	for err = plan.Next(values); err == nil; err = plan.Next(values) {
		details = append(details, values[3].(string))
	}
	closeErr := plan.Close()
	if err != io.EOF {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	rows, err := c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
	if err != nil {
		return nil, err
	}
	columns := rows.Columns()
	if len(columns) == 1 && columns[0] == "seq" && len(args) == 2 {
		c.reads.targetPlans = append(c.reads.targetPlans, strings.Join(details, "\n"))
	}
	if len(columns) == 1 && columns[0] == "seq" && len(args) == 3 {
		c.reads.boundaryPlans = append(c.reads.boundaryPlans, strings.Join(details, "\n"))
	}
	if len(columns) != 2 || columns[0] != "seq" || columns[1] != "bytes" {
		return rows, nil
	}
	c.reads.headerPlans = append(c.reads.headerPlans, strings.Join(details, "\n"))
	count := new(int)
	c.reads.headerRows = append(c.reads.headerRows, count)
	return &metadataReadRows{Rows: rows, count: count}, nil
}

type metadataReadRows struct {
	driver.Rows
	count *int
}

func (r *metadataReadRows) Next(values []driver.Value) error {
	err := r.Rows.Next(values)
	if err == nil {
		*r.count++
	}
	return err
}
