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
	rows, err := devicequeries.New(s.db).ListDevices(context.Background())
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
	count, err := devicequeries.New(s.db).CountApprovedDevices(context.Background())
	return int(count), err
}

func (s *Store) LoadDeviceByTokenHash(hash string) (storage.Device, error) {
	row, err := devicequeries.New(s.db).GetDeviceByTokenHash(context.Background(), hash)
	if err != nil {
		return storage.Device{}, deviceError(err)
	}
	return deviceFromTokenRow(row), nil
}

func (s *Store) LoadDevice(id string) (storage.Device, error) {
	row, err := devicequeries.New(s.db).GetDevice(context.Background(), id)
	if err != nil {
		return storage.Device{}, deviceError(err)
	}
	return deviceFromGetRow(row), nil
}

func (s *Store) SaveApprovedDevice(input storage.SaveApprovedDevice) (storage.Device, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.Device{}, err
	}
	defer tx.Rollback()
	q := devicequeries.New(s.db).WithTx(tx)
	if err := q.SaveApprovedDevice(ctx, approvedDeviceParams(input)); err != nil {
		return storage.Device{}, err
	}
	if err := q.InvalidateDevicePairings(ctx, devicequeries.InvalidateDevicePairingsParams{DeviceID: input.ID, AtMs: timeToMs(input.ApprovedAt)}); err != nil {
		return storage.Device{}, err
	}
	if err := tx.Commit(); err != nil {
		return storage.Device{}, err
	}
	return s.LoadDevice(input.ID)
}

func approvedDeviceParams(input storage.SaveApprovedDevice) devicequeries.SaveApprovedDeviceParams {
	return devicequeries.SaveApprovedDeviceParams{
		ID:              input.ID,
		Name:            input.Name,
		Kind:            input.Kind,
		PublicKey:       input.PublicKey,
		Platform:        input.Platform,
		DeviceFamily:    input.Family,
		ModelIdentifier: input.Model,
		TokenHash:       input.TokenHash,
		CreatedAtMs:     timeToMs(input.CreatedAt),
		ApprovedAtMs:    timeToMs(input.ApprovedAt),
		LastSeenAtMs:    timeToMs(input.LastSeenAt),
		LastSeenIp:      input.LastSeenIP,
		UserAgent:       input.UserAgent,
		AppVersion:      input.AppVersion,
	}
}

func (s *Store) RevokeDevice(id string, at time.Time) (storage.Device, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.Device{}, err
	}
	defer tx.Rollback()
	q := devicequeries.New(s.db).WithTx(tx)
	changed, err := q.RevokeDevice(ctx, devicequeries.RevokeDeviceParams{ID: id, RevokedAtMs: timeToMs(at)})
	if err != nil {
		return storage.Device{}, err
	}
	if changed == 0 {
		return storage.Device{}, fmt.Errorf("device not found: %s", id)
	}
	if err := q.InvalidateDevicePairings(ctx, devicequeries.InvalidateDevicePairingsParams{DeviceID: id, AtMs: timeToMs(at)}); err != nil {
		return storage.Device{}, err
	}
	if err := tx.Commit(); err != nil {
		return storage.Device{}, err
	}
	return s.LoadDevice(id)
}

func (s *Store) UpdateDeviceSeen(id, ip, userAgent string, at time.Time) error {
	s.writeMu.Lock()
	_, err := devicequeries.New(s.db).UpdateDeviceSeen(context.Background(), devicequeries.UpdateDeviceSeenParams{
		ID:           id,
		LastSeenAtMs: timeToMs(at),
		LastSeenIp:   ip,
		UserAgent:    userAgent,
	})
	s.writeMu.Unlock()
	return err
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

func deviceError(err error) error {
	if err == sql.ErrNoRows {
		return fmt.Errorf("device not found")
	}
	return err
}
