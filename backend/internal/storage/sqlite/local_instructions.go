package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/localinstructions"
)

var _ storage.LocalInstructionStore = (*Store)(nil)

func (s *Store) GetLocalInstruction(ctx context.Context, scope, scopeID, kind string) (storage.LocalInstruction, bool, error) {
	row, err := localinstructions.New(s.db).GetLocalInstruction(ctx, localinstructions.GetLocalInstructionParams{
		Scope:   scope,
		ScopeID: scopeID,
		Kind:    kind,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return storage.LocalInstruction{}, false, nil
	}
	if err != nil {
		return storage.LocalInstruction{}, false, err
	}
	return localInstructionFromRow(row), true, nil
}

func (s *Store) ImportLocalInstruction(ctx context.Context, doc storage.LocalInstruction) (storage.LocalInstruction, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.LocalInstruction{}, err
	}
	defer tx.Rollback()
	q := localinstructions.New(s.db).WithTx(tx)
	existing, err := q.GetLocalInstruction(ctx, localinstructions.GetLocalInstructionParams{
		Scope:   doc.Scope,
		ScopeID: doc.ScopeID,
		Kind:    doc.Kind,
	})
	if err == nil {
		return localInstructionFromRow(existing), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return storage.LocalInstruction{}, err
	}
	if err := q.UpsertLocalInstruction(ctx, localinstructions.UpsertLocalInstructionParams{
		Scope:       doc.Scope,
		ScopeID:     doc.ScopeID,
		Kind:        doc.Kind,
		BodyMd:      doc.BodyMD,
		Version:     doc.Version,
		UpdatedAtMs: timeToMs(doc.UpdatedAt),
	}); err != nil {
		return storage.LocalInstruction{}, err
	}
	if err := tx.Commit(); err != nil {
		return storage.LocalInstruction{}, err
	}
	return doc, nil
}

func (s *Store) SaveLocalInstruction(ctx context.Context, doc storage.LocalInstruction, expectedVersion string) (storage.LocalInstruction, bool, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.LocalInstruction{}, false, err
	}
	defer tx.Rollback()
	q := localinstructions.New(s.db).WithTx(tx)
	existing, err := q.GetLocalInstruction(ctx, localinstructions.GetLocalInstructionParams{
		Scope:   doc.Scope,
		ScopeID: doc.ScopeID,
		Kind:    doc.Kind,
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if expectedVersion != "" {
			return storage.LocalInstruction{Scope: doc.Scope, ScopeID: doc.ScopeID, Kind: doc.Kind}, true, nil
		}
	case err != nil:
		return storage.LocalInstruction{}, false, err
	case existing.Version != expectedVersion:
		return localInstructionFromRow(existing), true, nil
	}
	if err := q.UpsertLocalInstruction(ctx, localinstructions.UpsertLocalInstructionParams{
		Scope:       doc.Scope,
		ScopeID:     doc.ScopeID,
		Kind:        doc.Kind,
		BodyMd:      doc.BodyMD,
		Version:     doc.Version,
		UpdatedAtMs: timeToMs(doc.UpdatedAt),
	}); err != nil {
		return storage.LocalInstruction{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return storage.LocalInstruction{}, false, err
	}
	return doc, false, nil
}

func localInstructionFromRow(row localinstructions.LocalInstruction) storage.LocalInstruction {
	return storage.LocalInstruction{
		Scope:     row.Scope,
		ScopeID:   row.ScopeID,
		Kind:      row.Kind,
		BodyMD:    row.BodyMd,
		Version:   row.Version,
		UpdatedAt: msToTime(row.UpdatedAtMs),
	}
}
