package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/cloudmirror"
)

const cloudAccountSecretID = "cloud:account:token"

var _ storage.CloudMirrorStore = (*Store)(nil)

func (s *Store) LoadCloudAccount(ctx context.Context) (storage.CloudAccount, bool, error) {
	row, err := cloudmirror.New(s.db).GetCloudAccount(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.CloudAccount{}, false, nil
	}
	if err != nil {
		return storage.CloudAccount{}, false, err
	}
	var token string
	if row.TokenEncrypted != "" {
		raw, err := s.decryptSecret(cloudAccountSecretID, row.TokenEncrypted)
		if err != nil {
			return storage.CloudAccount{}, false, err
		}
		token = string(raw)
	}
	caps := json.RawMessage(row.CapabilitiesJson)
	if len(caps) == 0 {
		caps = json.RawMessage(`{}`)
	}
	return storage.CloudAccount{
		BaseURL:          row.BaseUrl,
		Token:            token,
		OIDCLoginURL:     row.OidcLoginUrl,
		UserID:           row.UserID,
		OrgID:            row.OrgID,
		OrgName:          row.OrgName,
		Email:            row.Email,
		DisplayName:      row.DisplayName,
		Role:             row.Role,
		CapabilitiesJSON: caps,
		ConnectedAt:      msToTime(row.ConnectedAtMs),
		LastSyncAt:       msToTime(row.LastSyncAtMs),
		LastError:        row.LastError,
	}, true, nil
}

func (s *Store) SaveCloudAccount(ctx context.Context, account storage.CloudAccount) error {
	var encrypted string
	if account.Token != "" {
		var err error
		encrypted, err = s.encryptSecret(cloudAccountSecretID, []byte(account.Token))
		if err != nil {
			return err
		}
	}
	caps := strings.TrimSpace(string(account.CapabilitiesJSON))
	if caps == "" {
		caps = "{}"
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return cloudmirror.New(s.db).UpsertCloudAccount(ctx, cloudmirror.UpsertCloudAccountParams{
		BaseUrl:          account.BaseURL,
		TokenEncrypted:   encrypted,
		OidcLoginUrl:     account.OIDCLoginURL,
		UserID:           account.UserID,
		OrgID:            account.OrgID,
		OrgName:          account.OrgName,
		Email:            account.Email,
		DisplayName:      account.DisplayName,
		Role:             account.Role,
		CapabilitiesJson: caps,
		ConnectedAtMs:    timeToMs(account.ConnectedAt),
		LastSyncAtMs:     timeToMs(account.LastSyncAt),
		LastError:        account.LastError,
	})
}

func (s *Store) UpdateCloudSyncState(ctx context.Context, lastSyncAt time.Time, lastError string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return cloudmirror.New(s.db).UpdateCloudAccountSyncState(ctx, cloudmirror.UpdateCloudAccountSyncStateParams{
		LastSyncAtMs: timeToMs(lastSyncAt),
		LastError:    lastError,
	})
}

func (s *Store) ClearCloudMirror(ctx context.Context) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := cloudmirror.New(s.db).WithTx(tx)
	for _, fn := range []func(context.Context) error{
		q.DeleteAllCloudThreadEvents,
		q.DeleteAllCloudThreads,
		q.DeleteAllCloudSpaces,
		q.DeleteAllCloudInstructions,
		q.DeleteAllCloudUsageEvents,
		q.DeleteCloudAccount,
	} {
		if err := fn(ctx); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ReplaceCloudSpaces(ctx context.Context, spaces []storage.CloudSpace) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := cloudmirror.New(s.db).WithTx(tx)
	if err := q.DeleteAllCloudSpaces(ctx); err != nil {
		return err
	}
	for _, sp := range spaces {
		if err := q.UpsertCloudSpace(ctx, cloudmirror.UpsertCloudSpaceParams{
			ID:          sp.ID,
			OrgID:       sp.OrgID,
			Name:        sp.Name,
			CreatedBy:   sp.CreatedBy,
			CreatedAtMs: timeToMs(sp.CreatedAt),
			UpdatedAtMs: timeToMs(sp.UpdatedAt),
		}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) UpsertCloudSpace(ctx context.Context, sp storage.CloudSpace) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return cloudmirror.New(s.db).UpsertCloudSpace(ctx, cloudmirror.UpsertCloudSpaceParams{
		ID:          sp.ID,
		OrgID:       sp.OrgID,
		Name:        sp.Name,
		CreatedBy:   sp.CreatedBy,
		CreatedAtMs: timeToMs(sp.CreatedAt),
		UpdatedAtMs: timeToMs(sp.UpdatedAt),
	})
}

func (s *Store) ListCloudSpaces(ctx context.Context) ([]storage.CloudSpace, error) {
	rows, err := cloudmirror.New(s.db).ListCloudSpaces(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]storage.CloudSpace, 0, len(rows))
	for _, row := range rows {
		out = append(out, storage.CloudSpace{
			ID:        row.ID,
			OrgID:     row.OrgID,
			Name:      row.Name,
			CreatedBy: row.CreatedBy,
			CreatedAt: msToTime(row.CreatedAtMs),
			UpdatedAt: msToTime(row.UpdatedAtMs),
		})
	}
	return out, nil
}

func (s *Store) SyncCloudThreads(ctx context.Context, threads []storage.CloudThread) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := cloudmirror.New(s.db).WithTx(tx)
	existingIDs, err := q.ListAllCloudThreadIDs(ctx)
	if err != nil {
		return err
	}
	keep := make(map[string]struct{}, len(threads))
	for _, th := range threads {
		keep[th.ID] = struct{}{}
		if err := q.UpsertCloudThread(ctx, cloudThreadUpsertParams(th)); err != nil {
			return err
		}
	}
	for _, id := range existingIDs {
		if _, ok := keep[id]; !ok {
			if err := q.DeleteCloudThread(ctx, id); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (s *Store) UpsertCloudThread(ctx context.Context, th storage.CloudThread) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return cloudmirror.New(s.db).UpsertCloudThread(ctx, cloudThreadUpsertParams(th))
}

func (s *Store) GetCloudThread(ctx context.Context, id string) (storage.CloudThread, bool, error) {
	row, err := cloudmirror.New(s.db).GetCloudThread(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.CloudThread{}, false, nil
	}
	if err != nil {
		return storage.CloudThread{}, false, err
	}
	return cloudThreadFromRow(row), true, nil
}

func (s *Store) ListCloudThreads(ctx context.Context, archived bool) ([]storage.CloudThread, error) {
	rows, err := cloudmirror.New(s.db).ListCloudThreads(ctx, boolToInt64(archived))
	if err != nil {
		return nil, err
	}
	out := make([]storage.CloudThread, 0, len(rows))
	for _, row := range rows {
		out = append(out, cloudThreadFromRow(row))
	}
	return out, nil
}

func (s *Store) UpdateCloudThreadUserState(ctx context.Context, id string, patch storage.CloudThreadUserStatePatch) (storage.CloudThread, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.CloudThread{}, err
	}
	defer tx.Rollback()
	q := cloudmirror.New(s.db).WithTx(tx)
	row, err := q.GetCloudThread(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.CloudThread{}, storage.ErrSessionNotFound
	}
	if err != nil {
		return storage.CloudThread{}, err
	}
	th := cloudThreadFromRow(row)
	if patch.Pinned != nil {
		th.Pinned = *patch.Pinned
	}
	if patch.Archived != nil {
		th.Archived = *patch.Archived
	}
	if patch.PersonalSourcesEnabled != nil {
		th.PersonalSourcesEnabled = *patch.PersonalSourcesEnabled
	}
	if patch.Unread != nil {
		th.Unread = *patch.Unread
	}
	if err := q.UpdateCloudThreadUserState(ctx, cloudmirror.UpdateCloudThreadUserStateParams{
		ID:                     id,
		Pinned:                 boolToInt64(th.Pinned),
		Archived:               boolToInt64(th.Archived),
		PersonalSourcesEnabled: boolToInt64(th.PersonalSourcesEnabled),
		Unread:                 boolToInt64(th.Unread),
	}); err != nil {
		return storage.CloudThread{}, err
	}
	return th, tx.Commit()
}

func (s *Store) UpdateCloudThreadStatus(ctx context.Context, id, status string, updatedAt time.Time) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return cloudmirror.New(s.db).UpdateCloudThreadStatus(ctx, cloudmirror.UpdateCloudThreadStatusParams{
		ID:          id,
		Status:      status,
		UpdatedAtMs: timeToMs(updatedAt),
	})
}

func (s *Store) AppendCloudThreadEvents(ctx context.Context, threadID string, events []storage.CloudThreadEvent) error {
	if len(events) == 0 {
		return nil
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := cloudmirror.New(s.db).WithTx(tx)
	var maxSeq int64
	for _, ev := range events {
		payload := strings.TrimSpace(string(ev.PayloadJSON))
		if payload == "" {
			payload = "{}"
		}
		if err := q.UpsertCloudThreadEvent(ctx, cloudmirror.UpsertCloudThreadEventParams{
			ThreadID:    threadID,
			Seq:         ev.Seq,
			OrgID:       ev.OrgID,
			TurnEpoch:   ev.TurnEpoch,
			ActorUserID: ev.ActorUserID,
			Type:        ev.Type,
			PayloadJson: payload,
			CreatedAtMs: timeToMs(ev.CreatedAt),
		}); err != nil {
			return err
		}
		if ev.Seq > maxSeq {
			maxSeq = ev.Seq
		}
	}
	if maxSeq > 0 {
		if err := q.UpdateCloudThreadLastSeq(ctx, cloudmirror.UpdateCloudThreadLastSeqParams{
			ID:      threadID,
			LastSeq: maxSeq,
		}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListCloudThreadEvents(ctx context.Context, threadID string, afterSeq int64) ([]storage.CloudThreadEvent, error) {
	rows, err := cloudmirror.New(s.db).ListCloudThreadEventsAfter(ctx, cloudmirror.ListCloudThreadEventsAfterParams{
		ThreadID: threadID,
		AfterSeq: afterSeq,
	})
	if err != nil {
		return nil, err
	}
	out := make([]storage.CloudThreadEvent, 0, len(rows))
	for _, row := range rows {
		out = append(out, storage.CloudThreadEvent{
			ThreadID:    row.ThreadID,
			Seq:         row.Seq,
			OrgID:       row.OrgID,
			TurnEpoch:   row.TurnEpoch,
			ActorUserID: row.ActorUserID,
			Type:        row.Type,
			PayloadJSON: json.RawMessage(row.PayloadJson),
			CreatedAt:   msToTime(row.CreatedAtMs),
		})
	}
	return out, nil
}

func (s *Store) UpsertCloudInstruction(ctx context.Context, doc storage.CloudInstruction) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return cloudmirror.New(s.db).UpsertCloudInstruction(ctx, cloudmirror.UpsertCloudInstructionParams{
		OrgID:       doc.OrgID,
		Scope:       doc.Scope,
		ScopeID:     doc.ScopeID,
		Kind:        doc.Kind,
		BodyMd:      doc.BodyMD,
		Version:     doc.Version,
		UpdatedBy:   doc.UpdatedBy,
		UpdatedAtMs: timeToMs(doc.UpdatedAt),
	})
}

func (s *Store) GetCloudInstruction(ctx context.Context, orgID, scope, scopeID, kind string) (storage.CloudInstruction, bool, error) {
	row, err := cloudmirror.New(s.db).GetCloudInstruction(ctx, cloudmirror.GetCloudInstructionParams{
		OrgID:   orgID,
		Scope:   scope,
		ScopeID: scopeID,
		Kind:    kind,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return storage.CloudInstruction{}, false, nil
	}
	if err != nil {
		return storage.CloudInstruction{}, false, err
	}
	return storage.CloudInstruction{
		OrgID:     row.OrgID,
		Scope:     row.Scope,
		ScopeID:   row.ScopeID,
		Kind:      row.Kind,
		BodyMD:    row.BodyMd,
		Version:   row.Version,
		UpdatedBy: row.UpdatedBy,
		UpdatedAt: msToTime(row.UpdatedAtMs),
	}, true, nil
}

func (s *Store) UpsertCloudUsageEvents(ctx context.Context, events []storage.CloudUsageEvent) error {
	if len(events) == 0 {
		return nil
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := cloudmirror.New(s.db).WithTx(tx)
	for _, ev := range events {
		if err := q.UpsertCloudUsageEvent(ctx, cloudmirror.UpsertCloudUsageEventParams{
			ID:                ev.ID,
			OrgID:             ev.OrgID,
			UserID:            ev.UserID,
			ThreadID:          ev.ThreadID,
			ModelProfileID:    ev.ModelProfileID,
			Model:             ev.Model,
			InputTokens:       ev.InputTokens,
			CachedInputTokens: ev.CachedInputTokens,
			OutputTokens:      ev.OutputTokens,
			CostUsd:           ev.CostUSD,
			CreatedAtMs:       timeToMs(ev.CreatedAt),
		}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) MaxCloudUsageEventID(ctx context.Context) (int64, error) {
	raw, err := cloudmirror.New(s.db).MaxCloudUsageEventID(ctx)
	if err != nil {
		return 0, err
	}
	switch v := raw.(type) {
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case float64:
		return int64(v), nil
	default:
		return 0, nil
	}
}

func cloudThreadUpsertParams(th storage.CloudThread) cloudmirror.UpsertCloudThreadParams {
	ref := strings.TrimSpace(string(th.RuntimeRefJSON))
	if ref == "" {
		ref = "{}"
	}
	runID := th.RunID
	if runID == "" {
		runID = th.ID
	}
	visibility := th.Visibility
	if visibility == "" {
		if th.SpaceID != "" {
			visibility = "space"
		} else {
			visibility = "private"
		}
	}
	status := th.Status
	if status == "" {
		status = "idle"
	}
	return cloudmirror.UpsertCloudThreadParams{
		ID:                     th.ID,
		OrgID:                  th.OrgID,
		OwnerUserID:            th.OwnerUserID,
		SpaceID:                th.SpaceID,
		RunID:                  runID,
		Visibility:             visibility,
		SourceType:             th.SourceType,
		SourceID:               th.SourceID,
		Title:                  th.Title,
		Status:                 status,
		TurnEpoch:              th.TurnEpoch,
		ModelProfileID:         th.ModelProfileID,
		Model:                  th.Model,
		ReasoningEffort:        th.ReasoningEffort,
		ExecutorTarget:         th.ExecutorTarget,
		RuntimeRefJson:         ref,
		PersonalSourcesEnabled: boolToInt64(th.PersonalSourcesEnabled),
		Pinned:                 boolToInt64(th.Pinned),
		Archived:               boolToInt64(th.Archived),
		Unread:                 boolToInt64(th.Unread),
		LastSeq:                th.LastSeq,
		CreatedAtMs:            timeToMs(th.CreatedAt),
		UpdatedAtMs:            timeToMs(th.UpdatedAt),
	}
}

func cloudThreadFromRow(row cloudmirror.CloudThread) storage.CloudThread {
	return storage.CloudThread{
		ID:                     row.ID,
		OrgID:                  row.OrgID,
		OwnerUserID:            row.OwnerUserID,
		SpaceID:                row.SpaceID,
		RunID:                  row.RunID,
		Visibility:             row.Visibility,
		SourceType:             row.SourceType,
		SourceID:               row.SourceID,
		Title:                  row.Title,
		Status:                 row.Status,
		TurnEpoch:              row.TurnEpoch,
		ModelProfileID:         row.ModelProfileID,
		Model:                  row.Model,
		ReasoningEffort:        row.ReasoningEffort,
		ExecutorTarget:         row.ExecutorTarget,
		RuntimeRefJSON:         json.RawMessage(row.RuntimeRefJson),
		PersonalSourcesEnabled: row.PersonalSourcesEnabled != 0,
		Pinned:                 row.Pinned != 0,
		Archived:               row.Archived != 0,
		Unread:                 row.Unread != 0,
		LastSeq:                row.LastSeq,
		CreatedAt:              msToTime(row.CreatedAtMs),
		UpdatedAt:              msToTime(row.UpdatedAtMs),
	}
}

func boolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
