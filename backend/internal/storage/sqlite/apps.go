package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
	appsdb "github.com/wins/jaz/backend/internal/storage/sqlite/generated/apps"
	boardsdb "github.com/wins/jaz/backend/internal/storage/sqlite/generated/boards"
)

func marshalJSONString(v any, fallback string) string {
	b, err := json.Marshal(v)
	if err != nil || len(b) == 0 || string(b) == "null" {
		return fallback
	}
	return string(b)
}

func unmarshalStringSlice(raw string) []string {
	var out []string
	_ = json.Unmarshal([]byte(raw), &out)
	if out == nil {
		return []string{}
	}
	return out
}

func unmarshalExternalURLs(raw string) []storage.AppExternalURL {
	var out []storage.AppExternalURL
	_ = json.Unmarshal([]byte(raw), &out)
	if out == nil {
		return []storage.AppExternalURL{}
	}
	return out
}

func unmarshalBundleFiles(raw string) []storage.AppBundleFile {
	var out []storage.AppBundleFile
	_ = json.Unmarshal([]byte(raw), &out)
	if out == nil {
		return []storage.AppBundleFile{}
	}
	return out
}

func rawJSONOrNil(raw string) json.RawMessage {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "{}" || trimmed == "null" {
		return nil
	}
	return json.RawMessage(trimmed)
}

func appFromRow(
	id, workspaceID, sourceThreadID, title, description, iconJSON, kind, visibility string,
	currentVersion int64, status string, createdAtMs, updatedAtMs int64, loopID string,
	bundleSHA, entryPath, capsJSON, toolsJSON string,
) storage.App {
	return storage.App{
		ID:             id,
		LoopID:         loopID,
		WorkspaceID:    firstNonEmpty(workspaceID, storage.LocalWorkspaceID),
		OwnerUserID:    storage.AppKVLocalOwnerID,
		SourceThreadID: sourceThreadID,
		Title:          title,
		Description:    description,
		IconJSON:       rawJSONOrNil(iconJSON),
		Kind:           kind,
		Visibility:     visibility,
		CurrentVersion: int(currentVersion),
		Status:         status,
		BundleSHA256:   bundleSHA,
		EntryPath:      entryPath,
		Capabilities:   unmarshalStringSlice(capsJSON),
		AllowedTools:   unmarshalStringSlice(toolsJSON),
		IsOwner:        true,
		CanEdit:        true,
		CreatedAt:      msToTime(createdAtMs),
		UpdatedAt:      msToTime(updatedAtMs),
	}
}

func appVersionFromRow(row appsdb.AppVersion) storage.AppVersion {
	return storage.AppVersion{
		AppID:        row.AppID,
		Version:      int(row.Version),
		BundleSHA256: row.BundleSha256,
		EntryPath:    row.EntryPath,
		StorageDir:   row.StorageDir,
		SizeBytes:    row.SizeBytes,
		FileCount:    int(row.FileCount),
		ManifestJSON: rawJSONOrNil(row.ManifestJson),
		Capabilities: unmarshalStringSlice(row.CapabilitiesJson),
		AllowedTools: unmarshalStringSlice(row.AllowedToolsJson),
		ExternalURLs: unmarshalExternalURLs(row.ExternalUrlsJson),
		Files:        unmarshalBundleFiles(row.FilesJson),
		CreatedBy:    storage.AppKVLocalOwnerID,
		CreatedAt:    msToTime(row.CreatedAtMs),
	}
}

func appDraftFromRow(row appsdb.AppDraft) storage.AppDraft {
	var consumed *time.Time
	if row.ConsumedAtMs.Valid {
		t := msToTime(row.ConsumedAtMs.Int64)
		consumed = &t
	}
	return storage.AppDraft{
		ID:             row.ID,
		WorkspaceID:    firstNonEmpty(row.WorkspaceID, storage.LocalWorkspaceID),
		TargetAppID:    row.TargetAppID,
		SourceThreadID: row.SourceThreadID,
		Title:          row.Title,
		Description:    row.Description,
		IconJSON:       rawJSONOrNil(row.IconJson),
		Kind:           row.Kind,
		BundleSHA256:   row.BundleSha256,
		EntryPath:      row.EntryPath,
		StorageDir:     row.StorageDir,
		SizeBytes:      row.SizeBytes,
		FileCount:      int(row.FileCount),
		ManifestJSON:   rawJSONOrNil(row.ManifestJson),
		Capabilities:   unmarshalStringSlice(row.CapabilitiesJson),
		AllowedTools:   unmarshalStringSlice(row.AllowedToolsJson),
		ExternalURLs:   unmarshalExternalURLs(row.ExternalUrlsJson),
		Warnings:       unmarshalStringSlice(row.WarningsJson),
		Files:          unmarshalBundleFiles(row.FilesJson),
		ExpiresAt:      msToTime(row.ExpiresAtMs),
		ConsumedAt:     consumed,
		CreatedAt:      msToTime(row.CreatedAtMs),
	}
}

func (s *Store) CreateApp(ctx context.Context, app storage.App) (storage.App, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.App{}, err
	}
	defer func() { _ = tx.Rollback() }()
	nowMs := timeToMs(time.Now().UTC())
	if err := appsdb.New(tx).InsertApp(ctx, appsdb.InsertAppParams{
		ID:          app.ID,
		WorkspaceID: firstNonEmpty(app.WorkspaceID, storage.LocalWorkspaceID),
		Title:       app.Title,
		IconJson:    "{}",
		Kind:        app.Kind,
		Visibility:  storage.AppVisibilityPrivate,
		CreatedAtMs: nowMs,
		UpdatedAtMs: nowMs,
	}); err != nil {
		return storage.App{}, err
	}
	if app.LoopID != "" {
		if err := boardsdb.New(tx).InsertLoopApp(ctx, boardsdb.InsertLoopAppParams{LoopID: app.LoopID, AppID: app.ID}); err != nil {
			return storage.App{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return storage.App{}, err
	}
	return s.GetApp(ctx, app.ID)
}

func (s *Store) CreateAppDraft(ctx context.Context, draft storage.AppDraft) (storage.AppDraft, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	now := time.Now().UTC()
	if draft.CreatedAt.IsZero() {
		draft.CreatedAt = now
	}
	ws := firstNonEmpty(draft.WorkspaceID, storage.LocalWorkspaceID)
	iconStr := "{}"
	if len(draft.IconJSON) > 0 {
		iconStr = string(draft.IconJSON)
	}
	manifestStr := "{}"
	if len(draft.ManifestJSON) > 0 {
		manifestStr = string(draft.ManifestJSON)
	}
	err := appsdb.New(s.db).InsertAppDraft(ctx, appsdb.InsertAppDraftParams{
		ID:               draft.ID,
		WorkspaceID:      ws,
		TargetAppID:      draft.TargetAppID,
		SourceThreadID:   draft.SourceThreadID,
		Title:            draft.Title,
		Description:      draft.Description,
		IconJson:         iconStr,
		Kind:             draft.Kind,
		BundleSha256:     draft.BundleSHA256,
		EntryPath:        draft.EntryPath,
		StorageDir:       draft.StorageDir,
		SizeBytes:        draft.SizeBytes,
		FileCount:        int64(draft.FileCount),
		ManifestJson:     manifestStr,
		CapabilitiesJson: marshalJSONString(draft.Capabilities, "[]"),
		AllowedToolsJson: marshalJSONString(draft.AllowedTools, "[]"),
		ExternalUrlsJson: marshalJSONString(draft.ExternalURLs, "[]"),
		WarningsJson:     marshalJSONString(draft.Warnings, "[]"),
		FilesJson:        marshalJSONString(draft.Files, "[]"),
		ExpiresAtMs:      timeToMs(draft.ExpiresAt),
		CreatedAtMs:      timeToMs(draft.CreatedAt),
	})
	if err != nil {
		return storage.AppDraft{}, err
	}
	draft.WorkspaceID = ws
	return draft, nil
}

func (s *Store) GetAppDraft(ctx context.Context, draftID string) (storage.AppDraft, error) {
	row, err := appsdb.New(s.db).GetAppDraft(ctx, draftID)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.AppDraft{}, storage.ErrAppNotFound
	}
	if err != nil {
		return storage.AppDraft{}, err
	}
	return appDraftFromRow(row), nil
}

func (s *Store) ReserveNextAppVersion(ctx context.Context, appID string) (bool, int, error) {
	q := appsdb.New(s.db)
	row, err := q.GetAppVersionMax(ctx, appID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, 1, nil
	}
	if err != nil {
		return false, 0, err
	}
	if row.Status != storage.AppStatusActive {
		return false, 0, storage.ErrAppNotFound
	}
	maxVer, err := q.MaxAppVersionRow(ctx, appID)
	if err != nil {
		return false, 0, err
	}
	next := int(maxVer) + 1
	if int(row.CurrentVersion)+1 > next {
		next = int(row.CurrentVersion) + 1
	}
	return true, next, nil
}

func (s *Store) PublishAppFromDraft(ctx context.Context, params storage.PublishAppDraftParams) (storage.App, storage.AppVersion, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.App{}, storage.AppVersion{}, err
	}
	defer func() { _ = tx.Rollback() }()

	q := appsdb.New(tx)
	draftRow, err := q.GetAppDraft(ctx, params.DraftID)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.App{}, storage.AppVersion{}, storage.ErrAppNotFound
	}
	if err != nil {
		return storage.App{}, storage.AppVersion{}, err
	}
	now := time.Now().UTC()
	nowMs := timeToMs(now)
	if draftRow.ConsumedAtMs.Valid || nowMs >= draftRow.ExpiresAtMs {
		return storage.App{}, storage.AppVersion{}, storage.ErrAppDraftConsumed
	}
	if draftRow.BundleSha256 != params.ExpectedSHA256 {
		return storage.App{}, storage.AppVersion{}, storage.ErrAppDraftMismatch
	}
	consumedRows, err := q.ConsumeAppDraft(ctx, appsdb.ConsumeAppDraftParams{
		ConsumedAtMs: sql.NullInt64{Int64: nowMs, Valid: true},
		ID:           params.DraftID,
		NowMs:        nowMs,
		BundleSha256: params.ExpectedSHA256,
	})
	if err != nil {
		return storage.App{}, storage.AppVersion{}, err
	}
	if consumedRows == 0 {
		return storage.App{}, storage.AppVersion{}, storage.ErrAppDraftConsumed
	}

	title := strings.TrimSpace(params.Title)
	if title == "" {
		title = draftRow.Title
	}
	desc := strings.TrimSpace(params.Description)
	if desc == "" {
		desc = draftRow.Description
	}
	vis := firstNonEmpty(params.Visibility, storage.AppVisibilityPrivate)
	ws := firstNonEmpty(params.WorkspaceID, storage.LocalWorkspaceID)

	if params.IsNew {
		if err := q.InsertApp(ctx, appsdb.InsertAppParams{
			ID:             params.AppID,
			WorkspaceID:    ws,
			SourceThreadID: draftRow.SourceThreadID,
			Title:          title,
			Description:    desc,
			IconJson:       draftRow.IconJson,
			Kind:           draftRow.Kind,
			Visibility:     vis,
			CurrentVersion: int64(params.Version),
			CreatedAtMs:    nowMs,
			UpdatedAtMs:    nowMs,
		}); err != nil {
			return storage.App{}, storage.AppVersion{}, err
		}
	} else {
		existing, err := q.GetApp(ctx, params.AppID)
		if errors.Is(err, sql.ErrNoRows) {
			return storage.App{}, storage.AppVersion{}, storage.ErrAppNotFound
		}
		if err != nil {
			return storage.App{}, storage.AppVersion{}, err
		}
		if existing.Kind == storage.AppKindBoard {
			return storage.App{}, storage.AppVersion{}, storage.ErrAppConflict
		}
		if params.Visibility == "" {
			vis = existing.Visibility
		}
		if err := q.UpdateAppPublished(ctx, appsdb.UpdateAppPublishedParams{
			Title:          title,
			Description:    desc,
			IconJson:       draftRow.IconJson,
			Kind:           draftRow.Kind,
			Visibility:     vis,
			CurrentVersion: int64(params.Version),
			UpdatedAtMs:    nowMs,
			ID:             params.AppID,
		}); err != nil {
			return storage.App{}, storage.AppVersion{}, err
		}
	}

	if err := q.InsertAppVersion(ctx, appsdb.InsertAppVersionParams{
		AppID:            params.AppID,
		Version:          int64(params.Version),
		BundleSha256:     draftRow.BundleSha256,
		EntryPath:        draftRow.EntryPath,
		StorageDir:       params.StorageDir,
		SizeBytes:        draftRow.SizeBytes,
		FileCount:        draftRow.FileCount,
		ManifestJson:     draftRow.ManifestJson,
		CapabilitiesJson: draftRow.CapabilitiesJson,
		AllowedToolsJson: draftRow.AllowedToolsJson,
		ExternalUrlsJson: draftRow.ExternalUrlsJson,
		FilesJson:        draftRow.FilesJson,
		CreatedAtMs:      nowMs,
	}); err != nil {
		return storage.App{}, storage.AppVersion{}, err
	}

	appRow, err := q.GetApp(ctx, params.AppID)
	if err != nil {
		return storage.App{}, storage.AppVersion{}, err
	}
	verRow, err := q.GetAppVersion(ctx, appsdb.GetAppVersionParams{
		AppID:   params.AppID,
		Version: int64(params.Version),
	})
	if err != nil {
		return storage.App{}, storage.AppVersion{}, err
	}
	if err := tx.Commit(); err != nil {
		return storage.App{}, storage.AppVersion{}, err
	}
	return appFromRow(
		appRow.ID, appRow.WorkspaceID, appRow.SourceThreadID, appRow.Title, appRow.Description,
		appRow.IconJson, appRow.Kind, appRow.Visibility, appRow.CurrentVersion, appRow.Status,
		appRow.CreatedAtMs, appRow.UpdatedAtMs, appRow.LoopID, appRow.BundleSha256, appRow.EntryPath,
		appRow.CapabilitiesJson, appRow.AllowedToolsJson,
	), appVersionFromRow(verRow), nil
}

func (s *Store) ListApps(ctx context.Context, workspaceID string, filter storage.AppListFilter) ([]storage.App, error) {
	rows, err := appsdb.New(s.db).ListApps(ctx, appsdb.ListAppsParams{
		FilterWorkspaceID:    workspaceID,
		FilterSourceThreadID: filter.SourceThreadID,
		FilterKind:           filter.Kind,
		FilterQuery:          filter.Query,
	})
	if err != nil {
		return nil, err
	}
	out := make([]storage.App, 0, len(rows))
	for _, r := range rows {
		out = append(out, appFromRow(
			r.ID, r.WorkspaceID, r.SourceThreadID, r.Title, r.Description,
			r.IconJson, r.Kind, r.Visibility, r.CurrentVersion, r.Status,
			r.CreatedAtMs, r.UpdatedAtMs, r.LoopID, r.BundleSha256, r.EntryPath,
			r.CapabilitiesJson, r.AllowedToolsJson,
		))
	}
	return out, nil
}

func (s *Store) GetApp(ctx context.Context, appID string) (storage.App, error) {
	r, err := appsdb.New(s.db).GetApp(ctx, appID)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.App{}, storage.ErrAppNotFound
	}
	if err != nil {
		return storage.App{}, err
	}
	return appFromRow(
		r.ID, r.WorkspaceID, r.SourceThreadID, r.Title, r.Description,
		r.IconJson, r.Kind, r.Visibility, r.CurrentVersion, r.Status,
		r.CreatedAtMs, r.UpdatedAtMs, r.LoopID, r.BundleSha256, r.EntryPath,
		r.CapabilitiesJson, r.AllowedToolsJson,
	), nil
}

func (s *Store) GetAppByLoop(ctx context.Context, loopID string) (storage.App, error) {
	id, err := appsdb.New(s.db).GetAppIDByLoop(ctx, loopID)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.App{}, storage.ErrAppNotFound
	}
	if err != nil {
		return storage.App{}, err
	}
	return s.GetApp(ctx, id)
}

func (s *Store) GetAppVersion(ctx context.Context, appID string, version int) (storage.AppVersion, error) {
	row, err := appsdb.New(s.db).GetAppVersion(ctx, appsdb.GetAppVersionParams{
		AppID:   appID,
		Version: int64(version),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return storage.AppVersion{}, storage.ErrAppNotFound
	}
	if err != nil {
		return storage.AppVersion{}, err
	}
	return appVersionFromRow(row), nil
}

func (s *Store) ListAppVersions(ctx context.Context, appID string) ([]storage.AppVersion, error) {
	rows, err := appsdb.New(s.db).ListAppVersions(ctx, appID)
	if err != nil {
		return nil, err
	}
	out := make([]storage.AppVersion, 0, len(rows))
	for _, r := range rows {
		out = append(out, appVersionFromRow(r))
	}
	return out, nil
}

func (s *Store) UpdateApp(ctx context.Context, appID string, patch storage.AppPatch) (storage.App, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	q := appsdb.New(s.db)
	cur, err := q.GetApp(ctx, appID)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.App{}, storage.ErrAppNotFound
	}
	if err != nil {
		return storage.App{}, err
	}
	title := cur.Title
	if patch.Title != nil {
		title = *patch.Title
	}
	desc := cur.Description
	if patch.Description != nil {
		desc = *patch.Description
	}
	iconStr := cur.IconJson
	if patch.IconJSON != nil {
		iconStr = string(*patch.IconJSON)
	}
	vis := cur.Visibility
	if patch.Visibility != nil {
		vis = *patch.Visibility
	}
	if err := q.UpdateAppMeta(ctx, appsdb.UpdateAppMetaParams{
		Title:       title,
		Description: desc,
		IconJson:    iconStr,
		Visibility:  vis,
		UpdatedAtMs: timeToMs(time.Now().UTC()),
		ID:          appID,
	}); err != nil {
		return storage.App{}, err
	}
	return s.GetApp(ctx, appID)
}

func (s *Store) RollbackAppVersion(ctx context.Context, appID string, targetVersion int) (storage.App, storage.AppVersion, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	q := appsdb.New(s.db)
	if _, err := q.GetApp(ctx, appID); errors.Is(err, sql.ErrNoRows) {
		return storage.App{}, storage.AppVersion{}, storage.ErrAppNotFound
	} else if err != nil {
		return storage.App{}, storage.AppVersion{}, err
	}
	verRow, err := q.GetAppVersion(ctx, appsdb.GetAppVersionParams{
		AppID:   appID,
		Version: int64(targetVersion),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return storage.App{}, storage.AppVersion{}, storage.ErrAppNotFound
	}
	if err != nil {
		return storage.App{}, storage.AppVersion{}, err
	}
	if err := q.SetAppCurrentVersion(ctx, appsdb.SetAppCurrentVersionParams{
		CurrentVersion: int64(targetVersion),
		UpdatedAtMs:    timeToMs(time.Now().UTC()),
		ID:             appID,
	}); err != nil {
		return storage.App{}, storage.AppVersion{}, err
	}
	updated, err := s.GetApp(ctx, appID)
	if err != nil {
		return storage.App{}, storage.AppVersion{}, err
	}
	return updated, appVersionFromRow(verRow), nil
}

func (s *Store) MarkAppDeleting(ctx context.Context, appID string) ([]string, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	q := appsdb.New(s.db)
	affected, err := q.MarkAppDeleting(ctx, appsdb.MarkAppDeletingParams{
		UpdatedAtMs: timeToMs(time.Now().UTC()),
		ID:          appID,
	})
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, storage.ErrAppNotFound
	}
	return q.ListAppVersionStorageDirs(ctx, appID)
}

func (s *Store) FinalizeDeleteApp(ctx context.Context, appID string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	q := appsdb.New(tx)
	if err := q.DeleteAppDraftsForApp(ctx, appID); err != nil {
		return err
	}
	if err := q.DeleteAppRow(ctx, appID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListAppKV(ctx context.Context, appID, scopeType, scopeOwnerID, prefix string) ([]storage.AppKVEntry, int64, error) {
	q := appsdb.New(s.db)
	rows, err := q.ListAppKV(ctx, appsdb.ListAppKVParams{
		AppID:        appID,
		ScopeType:    scopeType,
		ScopeOwnerID: scopeOwnerID,
		Prefix:       prefix,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := q.SumAppKVBytes(ctx, appID)
	if err != nil {
		return nil, 0, err
	}
	out := make([]storage.AppKVEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, storage.AppKVEntry{
			AppID:        r.AppID,
			Scope:        r.ScopeType,
			ScopeOwnerID: r.ScopeOwnerID,
			Key:          r.Key,
			Value:        r.ValueText,
			Revision:     r.Revision,
			SizeBytes:    r.SizeBytes,
			UpdatedBy:    r.UpdatedBy,
			UpdatedAt:    msToTime(r.UpdatedAtMs),
		})
	}
	return out, total, nil
}

func (s *Store) GetAppKV(ctx context.Context, appID, scopeType, scopeOwnerID, key string) (storage.AppKVEntry, error) {
	r, err := appsdb.New(s.db).GetAppKV(ctx, appsdb.GetAppKVParams{
		AppID:        appID,
		ScopeType:    scopeType,
		ScopeOwnerID: scopeOwnerID,
		Key:          key,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return storage.AppKVEntry{}, storage.ErrAppNotFound
	}
	if err != nil {
		return storage.AppKVEntry{}, err
	}
	return storage.AppKVEntry{
		AppID:        r.AppID,
		Scope:        r.ScopeType,
		ScopeOwnerID: r.ScopeOwnerID,
		Key:          r.Key,
		Value:        r.ValueText,
		Revision:     r.Revision,
		SizeBytes:    r.SizeBytes,
		UpdatedBy:    r.UpdatedBy,
		UpdatedAt:    msToTime(r.UpdatedAtMs),
	}, nil
}

func (s *Store) PutAppKV(ctx context.Context, appID, scopeType, scopeOwnerID, key, value, updatedBy string, expectedRevision *int64, quotaBytes int64) (storage.AppKVEntry, int64, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.AppKVEntry{}, 0, err
	}
	defer func() { _ = tx.Rollback() }()
	q := appsdb.New(tx)

	var (
		curRev   int64
		curBytes int64
	)
	existing, err := q.GetAppKV(ctx, appsdb.GetAppKVParams{
		AppID:        appID,
		ScopeType:    scopeType,
		ScopeOwnerID: scopeOwnerID,
		Key:          key,
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return storage.AppKVEntry{}, 0, err
	}
	if err == nil {
		curRev = existing.Revision
		curBytes = existing.SizeBytes
	}
	if expectedRevision != nil && *expectedRevision != curRev {
		return storage.AppKVEntry{}, curRev, storage.ErrAppKVConflict
	}

	newSize := int64(len(key) + len(value))
	totalBytes, err := q.SumAppKVBytes(ctx, appID)
	if err != nil {
		return storage.AppKVEntry{}, 0, err
	}
	if quotaBytes > 0 && (totalBytes-curBytes+newSize) > quotaBytes {
		return storage.AppKVEntry{}, curRev, storage.ErrAppKVQuotaExceeded
	}

	nextRev := curRev + 1
	now := time.Now().UTC()
	if err := q.UpsertAppKV(ctx, appsdb.UpsertAppKVParams{
		AppID:        appID,
		ScopeType:    scopeType,
		ScopeOwnerID: scopeOwnerID,
		Key:          key,
		ValueText:    value,
		Revision:     nextRev,
		SizeBytes:    newSize,
		UpdatedBy:    updatedBy,
		UpdatedAtMs:  timeToMs(now),
	}); err != nil {
		return storage.AppKVEntry{}, 0, err
	}
	if err := tx.Commit(); err != nil {
		return storage.AppKVEntry{}, 0, err
	}
	return storage.AppKVEntry{
		AppID:        appID,
		Scope:        scopeType,
		ScopeOwnerID: scopeOwnerID,
		Key:          key,
		Value:        value,
		Revision:     nextRev,
		SizeBytes:    newSize,
		UpdatedBy:    updatedBy,
		UpdatedAt:    now,
	}, nextRev, nil
}

func (s *Store) DeleteAppKV(ctx context.Context, appID, scopeType, scopeOwnerID, key string, expectedRevision *int64) (int64, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	q := appsdb.New(tx)

	existing, err := q.GetAppKV(ctx, appsdb.GetAppKVParams{
		AppID:        appID,
		ScopeType:    scopeType,
		ScopeOwnerID: scopeOwnerID,
		Key:          key,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return 0, storage.ErrAppNotFound
	}
	if err != nil {
		return 0, err
	}
	if expectedRevision != nil && *expectedRevision != existing.Revision {
		return existing.Revision, storage.ErrAppKVConflict
	}
	if err := q.DeleteAppKV(ctx, appsdb.DeleteAppKVParams{
		AppID:        appID,
		ScopeType:    scopeType,
		ScopeOwnerID: scopeOwnerID,
		Key:          key,
	}); err != nil {
		return 0, err
	}
	return existing.Revision, tx.Commit()
}
