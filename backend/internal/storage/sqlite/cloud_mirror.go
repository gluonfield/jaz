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
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/settingsdb"
)

const (
	cloudAccountSecretID        = "cloud:account:token"
	cloudSettingsNamespace      = "cloud"
	activeWorkspaceIDSettingKey = "active_workspace_id"
)

var _ storage.CloudMirrorStore = (*Store)(nil)

func (s *Store) ListCloudAccounts(ctx context.Context) ([]storage.CloudAccount, error) {
	rows, err := cloudmirror.New(s.db).ListCloudAccounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]storage.CloudAccount, 0, len(rows))
	for _, row := range rows {
		acc, err := s.cloudAccountFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, acc)
	}
	return out, nil
}

func (s *Store) LoadCloudAccount(ctx context.Context, id string) (storage.CloudAccount, bool, error) {
	row, err := cloudmirror.New(s.db).GetCloudAccount(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.CloudAccount{}, false, nil
	}
	if err != nil {
		return storage.CloudAccount{}, false, err
	}
	acc, err := s.cloudAccountFromRow(row)
	if err != nil {
		return storage.CloudAccount{}, false, err
	}
	return acc, true, nil
}

type cloudAccountSecretEnvelope struct {
	V            int    `json:"v"`
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

func encodeCloudAccountSecret(token, refreshToken string) string {
	if refreshToken == "" {
		return token
	}
	raw, err := json.Marshal(cloudAccountSecretEnvelope{
		V:            2,
		Token:        token,
		RefreshToken: refreshToken,
	})
	if err != nil {
		return token
	}
	return string(raw)
}

func decodeCloudAccountSecret(raw string) (string, string) {
	if strings.HasPrefix(raw, "{") {
		var env cloudAccountSecretEnvelope
		if json.Unmarshal([]byte(raw), &env) == nil && env.V == 2 {
			return env.Token, env.RefreshToken
		}
	}
	return raw, ""
}

func (s *Store) cloudAccountFromRow(row cloudmirror.CloudAccount) (storage.CloudAccount, error) {
	var token, refreshToken string
	if row.TokenEncrypted != "" {
		raw, err := s.decryptSecret(cloudAccountSecretID, row.TokenEncrypted)
		if err != nil {
			return storage.CloudAccount{}, err
		}
		token, refreshToken = decodeCloudAccountSecret(string(raw))
	}
	caps := json.RawMessage(row.CapabilitiesJson)
	if len(caps) == 0 {
		caps = json.RawMessage(`{}`)
	}
	return storage.CloudAccount{
		ID:               row.ID,
		BaseURL:          row.BaseUrl,
		Token:            token,
		RefreshToken:     refreshToken,
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
	}, nil
}

func (s *Store) SaveCloudAccount(ctx context.Context, account storage.CloudAccount) error {
	params, err := s.cloudAccountParams(account)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return cloudmirror.New(s.db).UpsertCloudAccount(ctx, params)
}

func (s *Store) cloudAccountParams(account storage.CloudAccount) (cloudmirror.UpsertCloudAccountParams, error) {
	var encrypted string
	if account.Token != "" || account.RefreshToken != "" {
		var err error
		payload := encodeCloudAccountSecret(account.Token, account.RefreshToken)
		encrypted, err = s.encryptSecret(cloudAccountSecretID, []byte(payload))
		if err != nil {
			return cloudmirror.UpsertCloudAccountParams{}, err
		}
	}
	caps := strings.TrimSpace(string(account.CapabilitiesJSON))
	if caps == "" {
		caps = "{}"
	}
	return cloudmirror.UpsertCloudAccountParams{
		ID:               account.ID,
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
	}, nil
}

func (s *Store) UpdateCloudSyncState(ctx context.Context, accountID string, lastSyncAt time.Time, lastError string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return cloudmirror.New(s.db).UpdateCloudAccountSyncState(ctx, cloudmirror.UpdateCloudAccountSyncStateParams{
		ID:           accountID,
		LastSyncAtMs: timeToMs(lastSyncAt),
		LastError:    lastError,
	})
}

func (s *Store) ApplyCloudWorkspaceChange(ctx context.Context, change storage.CloudWorkspaceChange) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := cloudmirror.New(s.db).WithTx(tx)
	if change.SignIn != nil {
		params, err := s.cloudAccountParams(*change.SignIn)
		if err != nil {
			return err
		}
		if err := q.UpsertCloudAccount(ctx, params); err != nil {
			return err
		}
	}
	for _, id := range change.RemoveIDs {
		if err := deleteCloudAccount(ctx, q, id); err != nil {
			return err
		}
	}
	if change.ActiveID != "" {
		now := timeToMs(time.Now())
		encoded, _ := json.Marshal(change.ActiveID)
		err := settingsdb.New(tx).UpsertSetting(ctx, settingsdb.UpsertSettingParams{
			Namespace:   cloudSettingsNamespace,
			Key:         activeWorkspaceIDSettingKey,
			ValueJson:   string(encoded),
			CreatedAtMs: now,
			UpdatedAtMs: now,
		})
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func deleteCloudAccount(ctx context.Context, q *cloudmirror.Queries, accountID string) error {
	for _, fn := range []func(context.Context, string) error{
		q.DeleteCloudThreadEventsByAccount,
		q.DeleteCloudThreadsByAccount,
		q.DeleteCloudSpacesByAccount,
		q.DeleteCloudInstructionsByAccount,
		q.DeleteCloudUsageEventsByAccount,
		q.DeleteCloudAccount,
	} {
		if err := fn(ctx, accountID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) LoadActiveWorkspaceID(_ context.Context) (string, error) {
	setting, err := s.LoadSetting(cloudSettingsNamespace, activeWorkspaceIDSettingKey)
	if errors.Is(err, storage.ErrSettingNotFound) {
		return storage.LocalWorkspaceID, nil
	}
	if err != nil {
		return "", err
	}
	var id string
	if err := json.Unmarshal(setting.Value, &id); err != nil {
		return storage.LocalWorkspaceID, nil
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return storage.LocalWorkspaceID, nil
	}
	return id, nil
}

func (s *Store) ReplaceCloudSpaces(ctx context.Context, accountID string, spaces []storage.CloudSpace) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := cloudmirror.New(s.db).WithTx(tx)
	if err := q.DeleteCloudSpacesByAccount(ctx, accountID); err != nil {
		return err
	}
	for _, sp := range spaces {
		if err := q.UpsertCloudSpace(ctx, cloudmirror.UpsertCloudSpaceParams{
			AccountID:   accountID,
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

func (s *Store) UpsertCloudSpace(ctx context.Context, accountID string, sp storage.CloudSpace) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return cloudmirror.New(s.db).UpsertCloudSpace(ctx, cloudmirror.UpsertCloudSpaceParams{
		AccountID:   accountID,
		ID:          sp.ID,
		OrgID:       sp.OrgID,
		Name:        sp.Name,
		CreatedBy:   sp.CreatedBy,
		CreatedAtMs: timeToMs(sp.CreatedAt),
		UpdatedAtMs: timeToMs(sp.UpdatedAt),
	})
}

func (s *Store) ListCloudSpaces(ctx context.Context, accountID string) ([]storage.CloudSpace, error) {
	rows, err := cloudmirror.New(s.db).ListCloudSpaces(ctx, accountID)
	if err != nil {
		return nil, err
	}
	out := make([]storage.CloudSpace, 0, len(rows))
	for _, row := range rows {
		out = append(out, storage.CloudSpace{
			AccountID: row.AccountID,
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

func (s *Store) SyncCloudThreads(ctx context.Context, accountID string, threads []storage.CloudThread) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := cloudmirror.New(s.db).WithTx(tx)
	existingIDs, err := q.ListAllCloudThreadIDs(ctx, accountID)
	if err != nil {
		return err
	}
	keep := make(map[string]struct{}, len(threads))
	for _, th := range threads {
		keep[th.ID] = struct{}{}
		if err := q.UpsertCloudThread(ctx, cloudThreadUpsertParams(accountID, th)); err != nil {
			return err
		}
	}
	for _, id := range existingIDs {
		if _, ok := keep[id]; !ok {
			if err := q.DeleteCloudThread(ctx, cloudmirror.DeleteCloudThreadParams{
				AccountID: accountID,
				ID:        id,
			}); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (s *Store) UpsertCloudThread(ctx context.Context, accountID string, th storage.CloudThread) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return cloudmirror.New(s.db).UpsertCloudThread(ctx, cloudThreadUpsertParams(accountID, th))
}

func (s *Store) GetCloudThread(ctx context.Context, accountID, id string) (storage.CloudThread, bool, error) {
	row, err := cloudmirror.New(s.db).GetCloudThread(ctx, cloudmirror.GetCloudThreadParams{
		AccountID: accountID,
		ID:        id,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return storage.CloudThread{}, false, nil
	}
	if err != nil {
		return storage.CloudThread{}, false, err
	}
	return cloudThreadFromRow(row), true, nil
}

func (s *Store) ListCloudThreads(ctx context.Context, accountID string, archived bool) ([]storage.CloudThread, error) {
	rows, err := cloudmirror.New(s.db).ListCloudThreads(ctx, cloudmirror.ListCloudThreadsParams{
		AccountID: accountID,
		Archived:  boolInt(archived),
	})
	if err != nil {
		return nil, err
	}
	out := make([]storage.CloudThread, 0, len(rows))
	for _, row := range rows {
		out = append(out, cloudThreadFromRow(row))
	}
	return out, nil
}

func (s *Store) UpdateCloudThreadUserState(ctx context.Context, accountID, id string, patch storage.CloudThreadUserStatePatch) (storage.CloudThread, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.CloudThread{}, err
	}
	defer tx.Rollback()
	q := cloudmirror.New(s.db).WithTx(tx)
	row, err := q.GetCloudThread(ctx, cloudmirror.GetCloudThreadParams{
		AccountID: accountID,
		ID:        id,
	})
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
		AccountID:              accountID,
		ID:                     id,
		Pinned:                 boolInt(th.Pinned),
		Archived:               boolInt(th.Archived),
		PersonalSourcesEnabled: boolInt(th.PersonalSourcesEnabled),
		Unread:                 boolInt(th.Unread),
	}); err != nil {
		return storage.CloudThread{}, err
	}
	return th, tx.Commit()
}

func (s *Store) UpdateCloudThreadStatus(ctx context.Context, accountID, id, status string, updatedAt time.Time) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return cloudmirror.New(s.db).UpdateCloudThreadStatus(ctx, cloudmirror.UpdateCloudThreadStatusParams{
		AccountID:   accountID,
		ID:          id,
		Status:      status,
		UpdatedAtMs: timeToMs(updatedAt),
	})
}

func (s *Store) DeleteCloudThread(ctx context.Context, accountID, id string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return cloudmirror.New(s.db).DeleteCloudThread(ctx, cloudmirror.DeleteCloudThreadParams{
		AccountID: accountID,
		ID:        id,
	})
}

func (s *Store) AppendCloudThreadEvents(ctx context.Context, accountID, threadID string, events []storage.CloudThreadEvent) error {
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
			AccountID:   accountID,
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
			AccountID: accountID,
			ID:        threadID,
			LastSeq:   maxSeq,
		}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListCloudThreadEvents(ctx context.Context, accountID, threadID string, afterSeq int64) ([]storage.CloudThreadEvent, error) {
	rows, err := cloudmirror.New(s.db).ListCloudThreadEventsAfter(ctx, cloudmirror.ListCloudThreadEventsAfterParams{
		AccountID: accountID,
		ThreadID:  threadID,
		AfterSeq:  afterSeq,
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

func (s *Store) UpsertCloudInstruction(ctx context.Context, accountID string, doc storage.CloudInstruction) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return cloudmirror.New(s.db).UpsertCloudInstruction(ctx, cloudmirror.UpsertCloudInstructionParams{
		AccountID:   accountID,
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

func (s *Store) GetCloudInstruction(ctx context.Context, accountID, orgID, scope, scopeID, kind string) (storage.CloudInstruction, bool, error) {
	row, err := cloudmirror.New(s.db).GetCloudInstruction(ctx, cloudmirror.GetCloudInstructionParams{
		AccountID: accountID,
		OrgID:     orgID,
		Scope:     scope,
		ScopeID:   scopeID,
		Kind:      kind,
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

func (s *Store) UpsertCloudUsageEvents(ctx context.Context, accountID string, events []storage.CloudUsageEvent) error {
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
			AccountID:         accountID,
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

func (s *Store) MaxCloudUsageEventID(ctx context.Context, accountID string) (int64, error) {
	raw, err := cloudmirror.New(s.db).MaxCloudUsageEventID(ctx, accountID)
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

func cloudThreadUpsertParams(accountID string, th storage.CloudThread) cloudmirror.UpsertCloudThreadParams {
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
		AccountID:              accountID,
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
		PersonalSourcesEnabled: boolInt(th.PersonalSourcesEnabled),
		Pinned:                 boolInt(th.Pinned),
		Archived:               boolInt(th.Archived),
		Unread:                 boolInt(th.Unread),
		LastSeq:                th.LastSeq,
		CreatedAtMs:            timeToMs(th.CreatedAt),
		UpdatedAtMs:            timeToMs(th.UpdatedAt),
	}
}

func cloudThreadFromRow(row cloudmirror.CloudThread) storage.CloudThread {
	return storage.CloudThread{
		AccountID:              row.AccountID,
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
