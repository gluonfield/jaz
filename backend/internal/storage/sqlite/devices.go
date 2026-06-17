package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
	devicequeries "github.com/wins/jaz/backend/internal/storage/sqlite/generated/devices"
)

func (s *Store) ListDevices() ([]storage.Device, error) {
	s.mu.Lock()
	rows, err := devicequeries.New(s.db).ListDevices(context.Background())
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	out := make([]storage.Device, 0, len(rows))
	for _, row := range rows {
		out = append(out, deviceFromListRow(row))
	}
	return out, nil
}

func (s *Store) CountApprovedDevices() (int, error) {
	s.mu.Lock()
	count, err := devicequeries.New(s.db).CountApprovedDevices(context.Background())
	s.mu.Unlock()
	return int(count), err
}

func (s *Store) LoadDeviceByTokenHash(hash string) (storage.Device, error) {
	s.mu.Lock()
	row, err := devicequeries.New(s.db).GetDeviceByTokenHash(context.Background(), hash)
	s.mu.Unlock()
	if err != nil {
		return storage.Device{}, deviceError(err)
	}
	return deviceFromTokenRow(row), nil
}

func (s *Store) LoadDevice(id string) (storage.Device, error) {
	s.mu.Lock()
	row, err := devicequeries.New(s.db).GetDevice(context.Background(), id)
	s.mu.Unlock()
	if err != nil {
		return storage.Device{}, deviceError(err)
	}
	return deviceFromGetRow(row), nil
}

func (s *Store) CreateDevice(input storage.CreateDevice) (storage.Device, error) {
	s.mu.Lock()
	err := devicequeries.New(s.db).CreateDevice(context.Background(), devicequeries.CreateDeviceParams{
		ID:              input.ID,
		Name:            input.Name,
		Kind:            input.Kind,
		Status:          input.Status,
		PublicKey:       input.PublicKey,
		Platform:        input.Platform,
		DeviceFamily:    input.Family,
		ModelIdentifier: input.Model,
		TokenHash:       input.TokenHash,
		CreatedAtMs:     timeToMs(input.CreatedAt),
		ApprovedAtMs:    optionalTimeToMs(input.ApprovedAt),
		LastSeenAtMs:    optionalTimeToMs(input.LastSeenAt),
		LastSeenIp:      input.LastSeenIP,
		UserAgent:       input.UserAgent,
		AppVersion:      input.AppVersion,
	})
	s.mu.Unlock()
	if err != nil {
		return storage.Device{}, err
	}
	return s.LoadDevice(input.ID)
}

func (s *Store) SavePairingDevice(input storage.SavePairingDevice) (storage.Device, error) {
	s.mu.Lock()
	err := devicequeries.New(s.db).SavePairingDevice(context.Background(), devicequeries.SavePairingDeviceParams{
		ID:              input.ID,
		Name:            input.Name,
		Kind:            input.Kind,
		PublicKey:       input.PublicKey,
		Platform:        input.Platform,
		DeviceFamily:    input.Family,
		ModelIdentifier: input.Model,
		TokenHash:       input.TokenHash,
		CreatedAtMs:     timeToMs(input.CreatedAt),
		LastSeenIp:      input.LastSeenIP,
		UserAgent:       input.UserAgent,
		AppVersion:      input.AppVersion,
	})
	s.mu.Unlock()
	if err != nil {
		return storage.Device{}, err
	}
	return s.LoadDevice(input.ID)
}

func (s *Store) UpdateDeviceSeen(id, ip, userAgent string, at time.Time) error {
	s.mu.Lock()
	_, err := devicequeries.New(s.db).UpdateDeviceSeen(context.Background(), devicequeries.UpdateDeviceSeenParams{
		ID:           id,
		LastSeenAtMs: timeToMs(at),
		LastSeenIp:   ip,
		UserAgent:    userAgent,
	})
	s.mu.Unlock()
	return err
}

func (s *Store) ApproveDevice(id string, at time.Time) (storage.Device, error) {
	s.mu.Lock()
	changed, err := devicequeries.New(s.db).ApproveDevice(context.Background(), devicequeries.ApproveDeviceParams{
		ID:           id,
		TokenHash:    "",
		ApprovedAtMs: timeToMs(at),
	})
	s.mu.Unlock()
	if err != nil {
		return storage.Device{}, err
	}
	if changed == 0 {
		return storage.Device{}, fmt.Errorf("device not found: %s", id)
	}
	return s.LoadDevice(id)
}

func (s *Store) RevokeDevice(id string, at time.Time) (storage.Device, error) {
	s.mu.Lock()
	changed, err := devicequeries.New(s.db).RevokeDevice(context.Background(), devicequeries.RevokeDeviceParams{
		ID:          id,
		RevokedAtMs: timeToMs(at),
	})
	s.mu.Unlock()
	if err != nil {
		return storage.Device{}, err
	}
	if changed == 0 {
		return storage.Device{}, fmt.Errorf("device not found: %s", id)
	}
	return s.LoadDevice(id)
}

func (s *Store) RenameDevice(id, name string) (storage.Device, error) {
	s.mu.Lock()
	changed, err := devicequeries.New(s.db).RenameDevice(context.Background(), devicequeries.RenameDeviceParams{
		ID:   id,
		Name: name,
	})
	s.mu.Unlock()
	if err != nil {
		return storage.Device{}, err
	}
	if changed == 0 {
		return storage.Device{}, fmt.Errorf("device not found: %s", id)
	}
	return s.LoadDevice(id)
}

func (s *Store) CreateDevicePairing(input storage.CreateDevicePairing) (storage.DevicePairing, error) {
	s.mu.Lock()
	err := devicequeries.New(s.db).CreatePairingRequest(context.Background(), devicequeries.CreatePairingRequestParams{
		ID:          input.ID,
		DeviceID:    input.DeviceID,
		SecretHash:  input.SecretHash,
		Status:      input.Status,
		CreatedAtMs: timeToMs(input.CreatedAt),
		ExpiresAtMs: timeToMs(input.ExpiresAt),
	})
	s.mu.Unlock()
	if err != nil {
		return storage.DevicePairing{}, err
	}
	pairing, _, err := s.LoadDevicePairing(input.ID)
	return pairing, err
}

func (s *Store) LoadDevicePairing(id string) (storage.DevicePairing, string, error) {
	s.mu.Lock()
	row, err := devicequeries.New(s.db).GetPairingRequest(context.Background(), id)
	s.mu.Unlock()
	if err != nil {
		return storage.DevicePairing{}, "", deviceError(err)
	}
	return pairingFromDB(row), row.SecretHash, nil
}

func (s *Store) ListDevicePairings() ([]storage.DevicePairing, error) {
	s.mu.Lock()
	rows, err := devicequeries.New(s.db).ListPairingRequests(context.Background())
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	out := make([]storage.DevicePairing, 0, len(rows))
	for _, row := range rows {
		out = append(out, pairingFromListDB(row))
	}
	return out, nil
}

func (s *Store) ApproveDevicePairing(id string, at time.Time) (storage.DevicePairing, error) {
	s.mu.Lock()
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		s.mu.Unlock()
		return storage.DevicePairing{}, err
	}
	q := devicequeries.New(s.db).WithTx(tx)
	pairing, err := q.GetPairingRequest(context.Background(), id)
	if err != nil {
		_ = tx.Rollback()
		s.mu.Unlock()
		return storage.DevicePairing{}, deviceError(err)
	}
	if pairing.Status != storage.PairingStatusPending {
		_ = tx.Rollback()
		s.mu.Unlock()
		return storage.DevicePairing{}, fmt.Errorf("pairing request not pending: %s", id)
	}
	approvedAt := timeToMs(at)
	if changed, err := q.ApproveDevice(context.Background(), devicequeries.ApproveDeviceParams{ID: pairing.DeviceID, TokenHash: pairing.SecretHash, ApprovedAtMs: approvedAt}); err != nil {
		_ = tx.Rollback()
		s.mu.Unlock()
		return storage.DevicePairing{}, err
	} else if changed == 0 {
		_ = tx.Rollback()
		s.mu.Unlock()
		return storage.DevicePairing{}, fmt.Errorf("device not found: %s", pairing.DeviceID)
	}
	if changed, err := q.ApprovePairingRequest(context.Background(), devicequeries.ApprovePairingRequestParams{ID: id, ApprovedAtMs: approvedAt}); err != nil {
		_ = tx.Rollback()
		s.mu.Unlock()
		return storage.DevicePairing{}, err
	} else if changed == 0 {
		_ = tx.Rollback()
		s.mu.Unlock()
		return storage.DevicePairing{}, fmt.Errorf("pairing request not pending: %s", id)
	}
	if err := tx.Commit(); err != nil {
		s.mu.Unlock()
		return storage.DevicePairing{}, err
	}
	s.mu.Unlock()
	loaded, _, err := s.LoadDevicePairing(id)
	return loaded, err
}

func (s *Store) RejectDevicePairing(id string, at time.Time) (storage.DevicePairing, error) {
	s.mu.Lock()
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		s.mu.Unlock()
		return storage.DevicePairing{}, err
	}
	q := devicequeries.New(s.db).WithTx(tx)
	pairing, err := q.GetPairingRequest(context.Background(), id)
	if err != nil {
		_ = tx.Rollback()
		s.mu.Unlock()
		return storage.DevicePairing{}, deviceError(err)
	}
	if pairing.Status != storage.PairingStatusPending {
		_ = tx.Rollback()
		s.mu.Unlock()
		return storage.DevicePairing{}, fmt.Errorf("pairing request not pending: %s", id)
	}
	rejectedAt := timeToMs(at)
	if changed, err := q.RejectPairingRequest(context.Background(), devicequeries.RejectPairingRequestParams{ID: id, RejectedAtMs: rejectedAt}); err != nil {
		_ = tx.Rollback()
		s.mu.Unlock()
		return storage.DevicePairing{}, err
	} else if changed == 0 {
		_ = tx.Rollback()
		s.mu.Unlock()
		return storage.DevicePairing{}, fmt.Errorf("pairing request not pending: %s", id)
	}
	if pairing.DeviceStatus == storage.DeviceStatusPending {
		if _, err := q.RevokeDevice(context.Background(), devicequeries.RevokeDeviceParams{ID: pairing.DeviceID, RevokedAtMs: rejectedAt}); err != nil {
			_ = tx.Rollback()
			s.mu.Unlock()
			return storage.DevicePairing{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		s.mu.Unlock()
		return storage.DevicePairing{}, err
	}
	s.mu.Unlock()
	loaded, _, err := s.LoadDevicePairing(id)
	return loaded, err
}

func deviceFromListRow(row devicequeries.ListDevicesRow) storage.Device {
	return deviceFromRecord(deviceRecord{
		ID:           row.ID,
		Name:         row.Name,
		Kind:         row.Kind,
		Status:       row.Status,
		PublicKey:    row.PublicKey,
		Platform:     row.Platform,
		Family:       row.DeviceFamily,
		Model:        row.ModelIdentifier,
		CreatedAtMs:  row.CreatedAtMs,
		ApprovedAtMs: row.ApprovedAtMs,
		RevokedAtMs:  row.RevokedAtMs,
		LastSeenAtMs: row.LastSeenAtMs,
		LastSeenIP:   row.LastSeenIp,
		UserAgent:    row.UserAgent,
		AppVersion:   row.AppVersion,
	})
}

func deviceFromGetRow(row devicequeries.GetDeviceRow) storage.Device {
	return deviceFromRecord(deviceRecord{
		ID:           row.ID,
		Name:         row.Name,
		Kind:         row.Kind,
		Status:       row.Status,
		PublicKey:    row.PublicKey,
		Platform:     row.Platform,
		Family:       row.DeviceFamily,
		Model:        row.ModelIdentifier,
		CreatedAtMs:  row.CreatedAtMs,
		ApprovedAtMs: row.ApprovedAtMs,
		RevokedAtMs:  row.RevokedAtMs,
		LastSeenAtMs: row.LastSeenAtMs,
		LastSeenIP:   row.LastSeenIp,
		UserAgent:    row.UserAgent,
		AppVersion:   row.AppVersion,
	})
}

func deviceFromTokenRow(row devicequeries.GetDeviceByTokenHashRow) storage.Device {
	return deviceFromRecord(deviceRecord{
		ID:           row.ID,
		Name:         row.Name,
		Kind:         row.Kind,
		Status:       row.Status,
		PublicKey:    row.PublicKey,
		Platform:     row.Platform,
		Family:       row.DeviceFamily,
		Model:        row.ModelIdentifier,
		CreatedAtMs:  row.CreatedAtMs,
		ApprovedAtMs: row.ApprovedAtMs,
		RevokedAtMs:  row.RevokedAtMs,
		LastSeenAtMs: row.LastSeenAtMs,
		LastSeenIP:   row.LastSeenIp,
		UserAgent:    row.UserAgent,
		AppVersion:   row.AppVersion,
	})
}

func pairingFromDB(row devicequeries.GetPairingRequestRow) storage.DevicePairing {
	return storage.DevicePairing{
		ID:         row.ID,
		DeviceID:   row.DeviceID,
		Status:     row.Status,
		CreatedAt:  msToTime(row.CreatedAtMs),
		ExpiresAt:  msToTime(row.ExpiresAtMs),
		ApprovedAt: msToTime(row.ApprovedAtMs),
		RejectedAt: msToTime(row.RejectedAtMs),
		Device:     deviceFromPairingRow(row),
	}
}

func pairingFromListDB(row devicequeries.ListPairingRequestsRow) storage.DevicePairing {
	return storage.DevicePairing{
		ID:         row.ID,
		DeviceID:   row.DeviceID,
		Status:     row.Status,
		CreatedAt:  msToTime(row.CreatedAtMs),
		ExpiresAt:  msToTime(row.ExpiresAtMs),
		ApprovedAt: msToTime(row.ApprovedAtMs),
		RejectedAt: msToTime(row.RejectedAtMs),
		Device:     deviceFromPairingListRow(row),
	}
}

type deviceRecord struct {
	ID           string
	Name         string
	Kind         string
	Status       string
	PublicKey    string
	Platform     string
	Family       string
	Model        string
	CreatedAtMs  int64
	ApprovedAtMs int64
	RevokedAtMs  int64
	LastSeenAtMs int64
	LastSeenIP   string
	UserAgent    string
	AppVersion   string
}

func deviceFromRecord(row deviceRecord) storage.Device {
	return storage.Device{
		ID:         row.ID,
		Name:       row.Name,
		Kind:       row.Kind,
		Status:     row.Status,
		PublicKey:  row.PublicKey,
		Platform:   row.Platform,
		Family:     row.Family,
		Model:      row.Model,
		CreatedAt:  msToTime(row.CreatedAtMs),
		ApprovedAt: msToTime(row.ApprovedAtMs),
		RevokedAt:  msToTime(row.RevokedAtMs),
		LastSeenAt: msToTime(row.LastSeenAtMs),
		LastSeenIP: row.LastSeenIP,
		UserAgent:  row.UserAgent,
		AppVersion: row.AppVersion,
	}
}

func deviceFromPairingListRow(row devicequeries.ListPairingRequestsRow) storage.Device {
	return deviceFromRecord(deviceRecord{
		ID:           row.DeviceDbID,
		Name:         row.DeviceName,
		Kind:         row.DeviceKind,
		Status:       row.DeviceStatus,
		PublicKey:    row.DevicePublicKey,
		Platform:     row.DevicePlatform,
		Family:       row.DeviceFamily,
		Model:        row.DeviceModelIdentifier,
		CreatedAtMs:  row.DeviceCreatedAtMs,
		ApprovedAtMs: row.DeviceApprovedAtMs,
		RevokedAtMs:  row.DeviceRevokedAtMs,
		LastSeenAtMs: row.DeviceLastSeenAtMs,
		LastSeenIP:   row.DeviceLastSeenIp,
		UserAgent:    row.DeviceUserAgent,
		AppVersion:   row.DeviceAppVersion,
	})
}

func deviceFromPairingRow(row devicequeries.GetPairingRequestRow) storage.Device {
	return deviceFromRecord(deviceRecord{
		ID:           row.DeviceDbID,
		Name:         row.DeviceName,
		Kind:         row.DeviceKind,
		Status:       row.DeviceStatus,
		PublicKey:    row.DevicePublicKey,
		Platform:     row.DevicePlatform,
		Family:       row.DeviceFamily,
		Model:        row.DeviceModelIdentifier,
		CreatedAtMs:  row.DeviceCreatedAtMs,
		ApprovedAtMs: row.DeviceApprovedAtMs,
		RevokedAtMs:  row.DeviceRevokedAtMs,
		LastSeenAtMs: row.DeviceLastSeenAtMs,
		LastSeenIP:   row.DeviceLastSeenIp,
		UserAgent:    row.DeviceUserAgent,
		AppVersion:   row.DeviceAppVersion,
	})
}

func deviceError(err error) error {
	if err == sql.ErrNoRows {
		return fmt.Errorf("device not found")
	}
	return err
}

func optionalTimeToMs(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}
