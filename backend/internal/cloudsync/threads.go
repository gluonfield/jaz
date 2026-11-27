package cloudsync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

func (s *Service) ListSessions(ctx context.Context, archived bool) ([]storage.Session, error) {
	acc, _, ok := s.activeAccountWithGen()
	if !ok {
		return nil, nil
	}
	threads, err := s.store.ListCloudThreads(ctx, acc.ID, archived)
	if err != nil {
		return nil, err
	}
	out := make([]storage.Session, 0, len(threads))
	for _, th := range threads {
		out = append(out, th.ToSession())
	}
	return out, nil
}

func (s *Service) GetSession(ctx context.Context, id string) (storage.Session, bool, error) {
	if err := validatePathSegment(id); err != nil {
		return storage.Session{}, false, nil
	}
	acc, gen, connected := s.activeAccountWithGen()
	if !connected {
		return storage.Session{}, false, nil
	}
	th, ok, err := s.store.GetCloudThread(ctx, acc.ID, id)
	if err != nil {
		return storage.Session{}, false, err
	}
	if ok {
		return th.ToSession(), true, nil
	}
	if acc.Token == "" {
		return storage.Session{}, false, nil
	}
	remote, err := withAccountRefresh(ctx, s, acc, func(token string) (storage.CloudThread, error) {
		return s.client.getThread(ctx, acc.BaseURL, token, id)
	})
	if err != nil {
		return storage.Session{}, false, nil
	}
	remote.AccountID = acc.ID
	if s.isGeneration(gen) {
		_ = s.store.UpsertCloudThread(ctx, acc.ID, remote)
	}
	return remote.ToSession(), true, nil
}

func (s *Service) CreateThread(ctx context.Context, req CreateThreadRequest) (storage.Session, error) {
	acc, gen, ok := s.activeAccountWithGen()
	if !ok || acc.Token == "" {
		return storage.Session{}, ErrNotConnected
	}
	created, err := withAccountRefresh(ctx, s, acc, func(token string) (storage.CloudThread, error) {
		return s.client.createThread(ctx, acc.BaseURL, token, req)
	})
	if err != nil {
		return storage.Session{}, err
	}
	created.AccountID = acc.ID
	if s.isGeneration(gen) {
		if err := s.store.UpsertCloudThread(ctx, acc.ID, created); err != nil {
			return storage.Session{}, err
		}
	}
	return created.ToSession(), nil
}

func (s *Service) LoadTranscript(ctx context.Context, id string) (ProjectedTranscript, bool, error) {
	if err := validatePathSegment(id); err != nil {
		return ProjectedTranscript{}, false, nil
	}
	acc, gen, connected := s.activeAccountWithGen()
	if !connected {
		return ProjectedTranscript{}, false, nil
	}
	th, ok, err := s.store.GetCloudThread(ctx, acc.ID, id)
	if err != nil {
		return ProjectedTranscript{}, false, err
	}
	fetchedRemote := false
	if !ok {
		if acc.Token == "" {
			return ProjectedTranscript{}, false, nil
		}
		remote, fetchErr := withAccountRefresh(ctx, s, acc, func(token string) (storage.CloudThread, error) {
			return s.client.getThread(ctx, acc.BaseURL, token, id)
		})
		if fetchErr != nil {
			return ProjectedTranscript{}, false, nil
		}
		remote.AccountID = acc.ID
		th = remote
		fetchedRemote = true
		if s.isGeneration(gen) {
			_ = s.store.UpsertCloudThread(ctx, acc.ID, th)
		}
	}
	if acc.Token != "" {
		var (
			remote    storage.CloudThread
			remoteErr error
			fresh     []storage.CloudThreadEvent
			freshErr  error
			wg        sync.WaitGroup
		)
		if !fetchedRemote {
			wg.Add(1)
			go func() {
				defer wg.Done()
				remote, remoteErr = withAccountRefresh(ctx, s, acc, func(token string) (storage.CloudThread, error) {
					return s.client.getThread(ctx, acc.BaseURL, token, th.ID)
				})
			}()
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			fresh, freshErr = withAccountRefresh(ctx, s, acc, func(token string) ([]storage.CloudThreadEvent, error) {
				return s.client.fetchEventsOnce(ctx, acc.BaseURL, token, th.ID, th.LastSeq)
			})
		}()
		wg.Wait()
		if !fetchedRemote && remoteErr == nil {
			remote.AccountID = acc.ID
			remote.LastSeq = max(remote.LastSeq, th.LastSeq)
			th = remote
			if s.isGeneration(gen) {
				_ = s.store.UpsertCloudThread(ctx, acc.ID, th)
			}
		}
		if freshErr == nil && len(fresh) > 0 && s.isGeneration(gen) {
			_ = s.store.AppendCloudThreadEvents(ctx, acc.ID, th.ID, fresh)
		}
	}
	events, err := s.store.ListCloudThreadEvents(ctx, acc.ID, th.ID, 0)
	if err != nil {
		return ProjectedTranscript{}, true, err
	}
	projected := ProjectTranscript(th, events)
	if projected.Session.Status != th.ToSession().Status && s.isGeneration(gen) {
		_ = s.store.UpdateCloudThreadStatus(ctx, acc.ID, th.ID, projected.Session.Status, projected.Session.UpdatedAt)
	}
	return projected, true, nil
}

func (s *Service) StreamEvents(ctx context.Context, threadID string, afterSeq int64, emit func(sessionevents.Event)) error {
	if err := validatePathSegment(threadID); err != nil {
		return err
	}
	acc, gen, ok := s.activeAccountWithGen()
	if !ok || acc.Token == "" {
		return ErrNotConnected
	}
	projector := NewStreamProjector(threadID)
	allCached, err := s.store.ListCloudThreadEvents(ctx, acc.ID, threadID, 0)
	if err != nil {
		return err
	}
	lastSeq := afterSeq
	for _, ev := range allCached {
		if ev.Seq <= afterSeq {
			// Seed streaming assistant buffer for mid-turn reconnects without re-emitting.
			_ = projector.Convert(ev)
			continue
		}
		if ev.Seq > lastSeq {
			lastSeq = ev.Seq
		}
		emit(projector.Convert(ev))
	}
	_, err = withAccountRefresh(ctx, s, acc, func(token string) (struct{}, error) {
		return struct{}{}, s.client.streamEvents(ctx, acc.BaseURL, token, threadID, lastSeq, func(ev storage.CloudThreadEvent) error {
			if ev.Seq > 0 && ev.Seq <= lastSeq {
				return nil
			}
			if ev.Seq > lastSeq {
				lastSeq = ev.Seq
			}
			if s.isGeneration(gen) {
				_ = s.store.AppendCloudThreadEvents(ctx, acc.ID, threadID, []storage.CloudThreadEvent{ev})
				if term := cloudTerminalState(ev.Type); term != "" {
					status := storage.StatusIdle
					if term == "failed" {
						status = storage.StatusError
					} else if term == "cancelled" {
						status = storage.StatusInterrupted
					}
					_ = s.store.UpdateCloudThreadStatus(ctx, acc.ID, threadID, status, ev.CreatedAt)
					s.TriggerSync()
				} else if isCloudUserEvent(ev.Type) || isCloudAssistantDelta(ev.Type) || isCloudThinkingDelta(ev.Type) || isCloudToolEvent(ev.Type) {
					_ = s.store.UpdateCloudThreadStatus(ctx, acc.ID, threadID, storage.StatusRunning, ev.CreatedAt)
				}
			}
			emit(projector.Convert(ev))
			return nil
		})
	})
	return err
}

func (s *Service) CreateTurn(ctx context.Context, threadID string, body json.RawMessage) (json.RawMessage, int, error) {
	if err := validatePathSegment(threadID); err != nil {
		return nil, 0, err
	}
	acc, gen, ok := s.activeAccountWithGen()
	if !ok || acc.Token == "" {
		return nil, 0, ErrNotConnected
	}
	raw, code, err := s.doAccountJSON(ctx, acc, http.MethodPost, "/v1/threads/"+url.PathEscape(threadID)+"/turns", body)
	if err != nil {
		return nil, code, err
	}
	if s.isGeneration(gen) {
		_ = s.store.UpdateCloudThreadStatus(ctx, acc.ID, threadID, storage.StatusRunning, time.Now().UTC())
	}
	return raw, code, nil
}

func (s *Service) CancelThread(ctx context.Context, threadID string) (json.RawMessage, int, error) {
	if err := validatePathSegment(threadID); err != nil {
		return nil, 0, err
	}
	acc, gen, ok := s.activeAccountWithGen()
	if !ok || acc.Token == "" {
		return nil, 0, ErrNotConnected
	}
	raw, code, err := s.doAccountJSON(ctx, acc, http.MethodPost, "/v1/threads/"+url.PathEscape(threadID)+"/cancel", nil)
	if err != nil {
		return nil, code, err
	}
	if s.isGeneration(gen) {
		_ = s.store.UpdateCloudThreadStatus(ctx, acc.ID, threadID, storage.StatusIdle, time.Now().UTC())
		s.TriggerSync()
	}
	return raw, code, nil
}

func (s *Service) SteerThread(ctx context.Context, threadID, text, mode string) (json.RawMessage, int, error) {
	if err := validatePathSegment(threadID); err != nil {
		return nil, 0, err
	}
	acc, gen, ok := s.activeAccountWithGen()
	if !ok || acc.Token == "" {
		return nil, 0, ErrNotConnected
	}
	payload := map[string]any{
		"text": text,
	}
	if trimmedMode := strings.TrimSpace(mode); trimmedMode != "" {
		payload["mode"] = trimmedMode
	}
	raw, code, err := s.doAccountJSON(ctx, acc, http.MethodPost, "/v1/threads/"+url.PathEscape(threadID)+"/steer", payload)
	if err != nil {
		return nil, code, err
	}
	if s.isGeneration(gen) {
		_ = s.store.UpdateCloudThreadStatus(ctx, acc.ID, threadID, storage.StatusRunning, time.Now().UTC())
	}
	return raw, code, nil
}

func (s *Service) UpdateUserState(ctx context.Context, threadID string, patch storage.CloudThreadUserStatePatch) (storage.Session, json.RawMessage, error) {
	if err := validatePathSegment(threadID); err != nil {
		return storage.Session{}, nil, err
	}
	acc, gen, ok := s.activeAccountWithGen()
	if !ok || acc.Token == "" {
		return storage.Session{}, nil, ErrNotConnected
	}
	payload := map[string]any{}
	if patch.Pinned != nil {
		payload["pinned"] = *patch.Pinned
	}
	if patch.Archived != nil {
		payload["archived"] = *patch.Archived
	}
	if patch.PersonalSourcesEnabled != nil {
		payload["personal_sources_enabled"] = *patch.PersonalSourcesEnabled
	}
	if patch.Unread != nil && !*patch.Unread {
		if cur, found, err := s.store.GetCloudThread(ctx, acc.ID, threadID); err == nil && found && cur.LastSeq > 0 {
			payload["last_read_seq"] = cur.LastSeq
		}
	}
	raw, _, err := s.doAccountJSON(ctx, acc, http.MethodPut, "/v1/threads/"+url.PathEscape(threadID)+"/user-state", payload)
	if err != nil {
		return storage.Session{}, nil, err
	}
	if !s.isGeneration(gen) {
		return storage.Session{}, raw, nil
	}
	updated, err := s.store.UpdateCloudThreadUserState(ctx, acc.ID, threadID, patch)
	if err != nil {
		return storage.Session{}, raw, nil
	}
	return updated.ToSession(), raw, nil
}

func (s *Service) UpdateThread(ctx context.Context, threadID string, body json.RawMessage) (json.RawMessage, int, error) {
	if err := validatePathSegment(threadID); err != nil {
		return nil, 0, err
	}
	acc, gen, ok := s.activeAccountWithGen()
	if !ok || acc.Token == "" {
		return nil, 0, ErrNotConnected
	}
	raw, status, err := s.doAccountJSON(ctx, acc, http.MethodPatch, "/v1/threads/"+url.PathEscape(threadID), body)
	if err != nil {
		return nil, status, err
	}
	var updated storage.CloudThread
	if json.Unmarshal(raw, &updated) == nil && updated.ID != "" && s.isGeneration(gen) {
		updated.AccountID = acc.ID
		_ = s.store.UpsertCloudThread(ctx, acc.ID, updated)
	}
	return raw, status, nil
}

func (s *Service) DeleteThread(ctx context.Context, threadID string) (json.RawMessage, int, error) {
	if err := validatePathSegment(threadID); err != nil {
		return nil, 0, err
	}
	acc, gen, ok := s.activeAccountWithGen()
	if !ok || acc.Token == "" {
		return nil, 0, ErrNotConnected
	}
	raw, status, err := s.doAccountJSON(ctx, acc, http.MethodDelete, "/v1/threads/"+url.PathEscape(threadID), nil)
	if err != nil {
		return nil, status, err
	}
	if s.isGeneration(gen) {
		_ = s.store.DeleteCloudThread(ctx, acc.ID, threadID)
	}
	return raw, status, nil
}
