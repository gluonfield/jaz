package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
	devicequeries "github.com/wins/jaz/backend/internal/storage/sqlite/generated/devices"
)

func (s *Store) CreateDevicePairing(input storage.CreateDevicePairing) (storage.DevicePairing, error) {
	profile, err := json.Marshal(input.Device)
	if err != nil {
		return storage.DevicePairing{}, err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storage.DevicePairing{}, err
	}
	defer tx.Rollback()
	q := devicequeries.New(s.db).WithTx(tx)
	if err := q.PrunePairingRequests(ctx, timeToMs(input.Policy.RetainAfter)); err != nil {
		return storage.DevicePairing{}, err
	}
	if err := q.PrunePairingDevices(ctx, timeToMs(input.Policy.RetainAfter)); err != nil {
		return storage.DevicePairing{}, err
	}
	admission, err := q.PairingAdmission(ctx, devicequeries.PairingAdmissionParams{RecentAfterMs: timeToMs(input.Policy.RecentAfter), AtMs: timeToMs(input.CreatedAt)})
	if err != nil {
		return storage.DevicePairing{}, err
	}
	if admission.Recent >= int64(input.Policy.RecentLimit) || admission.Pending >= int64(input.Policy.PendingLimit) {
		return storage.DevicePairing{}, storage.ErrPairingLimit
	}
	d := input.Device
	if err := q.SavePairingDevice(ctx, devicequeries.SavePairingDeviceParams{
		ID: d.ID, Name: d.Name, Kind: d.Kind, PublicKey: d.PublicKey,
		Platform: d.Platform, DeviceFamily: d.Family, ModelIdentifier: d.Model,
		CreatedAtMs: timeToMs(d.CreatedAt), LastSeenIp: d.LastSeenIP,
		UserAgent: d.UserAgent, AppVersion: d.AppVersion,
	}); err != nil {
		return storage.DevicePairing{}, err
	}
	if err := q.InvalidateDevicePairings(ctx, devicequeries.InvalidateDevicePairingsParams{DeviceID: d.ID, AtMs: timeToMs(input.CreatedAt)}); err != nil {
		return storage.DevicePairing{}, err
	}
	if err := q.CreatePairingRequest(ctx, devicequeries.CreatePairingRequestParams{
		ID: input.ID, DeviceID: d.ID, SecretHash: input.SecretHash,
		CreatedAtMs: timeToMs(input.CreatedAt), ExpiresAtMs: timeToMs(input.ExpiresAt), ProfileJson: string(profile),
	}); err != nil {
		return storage.DevicePairing{}, err
	}
	if err := tx.Commit(); err != nil {
		return storage.DevicePairing{}, err
	}
	pairing, _, err := s.LoadDevicePairing(input.ID)
	return pairing, err
}

func (s *Store) LoadDevicePairing(id string) (storage.DevicePairing, string, error) {
	row, err := devicequeries.New(s.db).GetPairingRequest(context.Background(), id)
	if err != nil {
		return storage.DevicePairing{}, "", deviceError(err)
	}
	pairing, err := pairingFromDB(row)
	return pairing, row.SecretHash, err
}

func (s *Store) ListDevicePairings(after time.Time) ([]storage.DevicePairing, error) {
	rows, err := devicequeries.New(s.db).ListPairingRequests(context.Background(), timeToMs(after))
	if err != nil {
		return nil, err
	}
	out := make([]storage.DevicePairing, 0, len(rows))
	for _, row := range rows {
		pairing, err := pairingFromDB(row)
		if err != nil {
			return nil, err
		}
		out = append(out, pairing)
	}
	return out, nil
}

func (s *Store) ApproveDevicePairing(id string, at time.Time) (storage.DevicePairing, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	changed, err := devicequeries.New(s.db).ApprovePairingRequest(context.Background(), devicequeries.ApprovePairingRequestParams{ID: id, ApprovedAtMs: timeToMs(at)})
	if err != nil {
		return storage.DevicePairing{}, err
	}
	if changed == 0 {
		return storage.DevicePairing{}, fmt.Errorf("pairing request not pending: %s", id)
	}
	pairing, _, err := s.LoadDevicePairing(id)
	return pairing, err
}

func (s *Store) RejectDevicePairing(id string, at time.Time) (storage.DevicePairing, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	changed, err := devicequeries.New(s.db).RejectPairingRequest(context.Background(), devicequeries.RejectPairingRequestParams{ID: id, RejectedAtMs: timeToMs(at)})
	if err != nil {
		return storage.DevicePairing{}, err
	}
	if changed == 0 {
		return storage.DevicePairing{}, fmt.Errorf("pairing request not pending: %s", id)
	}
	pairing, _, err := s.LoadDevicePairing(id)
	return pairing, err
}

func pairingFromDB(row devicequeries.DevicePairingRequest) (storage.DevicePairing, error) {
	var device storage.Device
	if err := json.Unmarshal([]byte(row.ProfileJson), &device); err != nil {
		return storage.DevicePairing{}, err
	}
	return storage.DevicePairing{
		ID: row.ID, DeviceID: row.DeviceID, Status: row.Status, Device: device,
		CreatedAt: msToTime(row.CreatedAtMs), ExpiresAt: msToTime(row.ExpiresAtMs),
		ApprovedAt: msToTime(row.ApprovedAtMs), RejectedAt: msToTime(row.RejectedAtMs),
	}, nil
}

func (s *Store) ClaimDevicePairing(id, secretHash, tokenHash string, at time.Time) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := devicequeries.New(s.db).WithTx(tx)
	row, err := q.GetPairingRequest(ctx, id)
	if err != nil {
		return err
	}
	pairing, err := pairingFromDB(row)
	if err != nil {
		return err
	}
	changed, err := q.ClaimPairingRequest(ctx, devicequeries.ClaimPairingRequestParams{ID: id, SecretHash: secretHash, AtMs: timeToMs(at)})
	if err != nil {
		return err
	}
	if changed != 1 {
		return fmt.Errorf("pairing is unavailable")
	}
	d := pairing.Device
	if err := q.SaveApprovedDevice(ctx, approvedDeviceParams(storage.SaveApprovedDevice{
		ID: d.ID, Name: d.Name, Kind: d.Kind, PublicKey: d.PublicKey,
		Platform: d.Platform, Family: d.Family, Model: d.Model, TokenHash: tokenHash,
		CreatedAt: d.CreatedAt, ApprovedAt: at, LastSeenAt: at,
		LastSeenIP: d.LastSeenIP, UserAgent: d.UserAgent, AppVersion: d.AppVersion,
	})); err != nil {
		return err
	}
	if err := q.InvalidateDevicePairings(ctx, devicequeries.InvalidateDevicePairingsParams{DeviceID: d.ID, AtMs: timeToMs(at)}); err != nil {
		return err
	}
	return tx.Commit()
}
