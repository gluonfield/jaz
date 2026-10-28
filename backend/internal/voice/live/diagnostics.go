package live

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
)

type Diagnostic struct {
	ID              string    `json:"id"`
	CallID          string    `json:"call_id"`
	Stage           string    `json:"stage"`
	RequestID       string    `json:"request_id,omitempty"`
	UserTurnID      string    `json:"user_turn_id,omitempty"`
	EventID         string    `json:"event_id,omitempty"`
	SourceSeq       int64     `json:"source_seq,omitempty"`
	At              time.Time `json:"at"`
	ElapsedMS       float64   `json:"elapsed_ms"`
	ProviderStartMS *float64  `json:"provider_start_ms,omitempty"`
	ProviderEndMS   *float64  `json:"provider_end_ms,omitempty"`
	Detail          string    `json:"detail,omitempty"`
}

func (s *Transcript) Record(sessionID string, records []Diagnostic) error {
	if len(records) == 0 || len(records) > 32 {
		return fmt.Errorf("%w: expected 1–32 voice diagnostics", ErrInput)
	}
	if _, err := s.store.LoadSession(sessionID); err != nil {
		return err
	}
	events := make([]sessionevents.Event, 0, len(records))
	for _, record := range records {
		if record.ID == "" || record.CallID == "" || record.Stage == "" || record.At.IsZero() || record.ElapsedMS < 0 || record.SourceSeq < 0 || len(record.ID) > 128 || len(record.CallID) > 128 || len(record.Stage) > 64 || len(record.RequestID) > 128 || len(record.UserTurnID) > 128 || len(record.EventID) > 128 || len(record.Detail) > 512 {
			return fmt.Errorf("%w: invalid voice diagnostic", ErrInput)
		}
		data, err := json.Marshal(record)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrInput, err)
		}
		events = append(events, sessionevents.Event{
			SessionID: sessionID, Type: sessionevents.TypeVoiceDiagnostic, Content: string(data),
			ProjectionKey: sessionevents.TypeVoiceDiagnostic + ":" + record.CallID + ":" + record.ID,
			ProjectionOp:  sessionevents.ProjectionReplace,
		})
	}
	return s.append(sessionID, events)
}
