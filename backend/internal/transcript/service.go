package transcript

import (
	"context"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

type Store interface {
	storage.TranscriptPageReader
}

type LiveReader interface {
	HydrationJobs([]string) map[string]acp.HydrationView
}

type Service struct {
	store Store
	live  LiveReader
}

type View struct {
	Session          storage.Session
	Page             storage.TranscriptPage
	Snapshot         *acp.Job
	Children         []acp.Job
	ChildPermissions []sessionevents.ACPPermission
	Meta             map[string]Metadata
}

func NewService(store Store, live LiveReader) *Service {
	return &Service{store: store, live: live}
}

func (s *Service) Load(ctx context.Context, ref string, request storage.TranscriptPageRequest) (View, error) {
	session, err := s.store.LoadTranscriptSession(ctx, ref)
	if err != nil {
		return View{}, err
	}
	page, err := s.store.LoadTranscriptPage(ctx, session.ID, request)
	if err != nil {
		return View{}, err
	}
	if request.BeforeMessageSeq == 0 && request.BeforeEventSeq == 0 {
		clearPendingSteer(&session, page.Messages)
	}
	page.Events = sessionevents.CompactTranscript(storage.GoalDisplayEvents(page.Events))
	related, err := s.store.LoadTranscriptSessions(ctx, session.ID, referencedSessionIDs(page.Events, session.ID))
	if err != nil {
		return View{}, err
	}
	active := s.activeJobs(session.ID, related.Children)
	children, permissions := childSnapshots(session.ID, page.Events, related.Children, active)
	view := View{
		Session: session, Page: page, Children: children, ChildPermissions: permissions,
		Meta: metadata(page.Events, session, children, related.References),
	}
	if session.Runtime == storage.RuntimeACP {
		snapshot := sessionSnapshot(session, active)
		view.Snapshot = &snapshot
		if status := storage.SessionStatusForACPState(snapshot.State); session.Status == storage.StatusRunning && status != "" {
			view.Session.Status = status
		}
	}
	return view, nil
}

func (s *Service) activeJobs(sessionID string, children []storage.TranscriptSession) map[string]acp.HydrationView {
	ids := make([]string, 1, len(children)+1)
	ids[0] = sessionID
	for _, child := range children {
		ids = append(ids, child.ID)
	}
	return s.live.HydrationJobs(ids)
}

func referencedSessionIDs(events []sessionevents.Event, currentID string) []string {
	seen := map[string]struct{}{}
	ids := make([]string, 0)
	for _, event := range events {
		if event.ACP == nil || event.ACP.ID == "" || event.ACP.ID == currentID {
			continue
		}
		if _, ok := seen[event.ACP.ID]; ok {
			continue
		}
		seen[event.ACP.ID] = struct{}{}
		ids = append(ids, event.ACP.ID)
	}
	return ids
}

func clearPendingSteer(session *storage.Session, records []storage.Message) {
	if session.PendingSteer == nil || len(records) == 0 {
		return
	}
	last := records[len(records)-1]
	if last.Role == "user" && last.Content == session.PendingSteer.Text && !last.CreatedAt.Before(session.LastAttentionAt) {
		session.PendingSteer = nil
	}
}
