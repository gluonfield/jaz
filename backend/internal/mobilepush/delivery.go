package mobilepush

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/wins/jaz/backend/internal/bots"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

type contentState struct {
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type apsAlert struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"`
	Body     string `json:"body"`
}

// process consumes stored conversation events, never provider reasoning or
// streamed assistant fragments. Desktops listening get its alert; with Apple
// push configured, paired iPhones get the alert and the progress.
func (s *Service) process(ctx context.Context, event sessionevents.Event) error {
	if !s.Enabled() && !s.listening() {
		return nil
	}
	event.NormalizePayload()
	if event.Type == sessionevents.TypeRoomMessage && (event.RoomMessage == nil || event.RoomMessage.Speaker != "bot") {
		return nil
	}
	session, err := s.store.LoadSession(event.SessionID)
	if err != nil || session.Archived {
		return err
	}
	alert, err := s.classify(session, event)
	if err != nil {
		return err
	}
	if alert != nil {
		s.broadcast(*alert)
	}
	if !s.Enabled() {
		return nil
	}
	session, state, relevant, err := s.liveState(session, event)
	if err != nil || alert == nil && !relevant {
		return err
	}
	return s.deliver(ctx, session, state, relevant, alert)
}

// liveState is the progress a Live Activity shows for the chat an event
// belongs to: a member's thread reports as its group, which works while any
// member does.
func (s *Service) liveState(session storage.Session, event sessionevents.Event) (storage.Session, contentState, bool, error) {
	room, _, err := s.room(session)
	if err != nil {
		return room, contentState{}, false, err
	}
	if room.ID != session.ID {
		event = sessionevents.Event{SessionID: room.ID, Type: sessionevents.TypeSession}
	}
	if room.SourceType == storage.SourceBot {
		if record, err := s.store.LoadBot(room.ID); err == nil && record.Kind == bots.KindGroup {
			working, _ := bots.GroupState(s.store, record)
			room.Status = storage.StatusIdle
			if len(working) > 0 {
				room.Status = storage.StatusRunning
			}
		}
	}
	state, relevant := progress(room, event)
	if state.Status == "working" && s.awaitsAnswer(room.ID) {
		state = contentState{"waiting", "Needs your answer"}
	}
	return room, state, relevant, nil
}

func progress(session storage.Session, event sessionevents.Event) (contentState, bool) {
	switch event.Type {
	case "permission_request":
		if event.Permission != nil {
			return contentState{"waiting", "Needs your answer"}, true
		}
	case "permission_response":
		return contentState{"working", "Working"}, true
	case "plan_update", "proposed_plan":
		if event.Plan != nil {
			for _, item := range event.Plan.Plan {
				if item.Status == "in_progress" {
					return contentState{"working", bounded(item.Content, 160)}, true
				}
			}
		}
	case sessionevents.TypeRoomMessage:
		if session.Status != storage.StatusRunning {
			return contentState{"finished", "Reply ready"}, true
		}
		return contentState{"working", "Working"}, true
	case "acp", sessionevents.TypeTurnFinished:
		if event.ACP == nil {
			break
		}
		switch event.ACP.State {
		case "starting", "running":
			return contentState{"working", "Working"}, true
		case "failed":
			return contentState{"error", "Work stopped with an error"}, true
		case "cancelled":
			return contentState{"finished", "Stopped"}, true
		case "idle":
			if session.Status == storage.StatusRunning {
				return contentState{"working", "Working"}, true
			}
			return contentState{"finished", "Finished"}, true
		}
	case sessionevents.TypeSession:
		switch session.Status {
		case storage.StatusRunning:
			return contentState{"working", "Working"}, true
		case storage.StatusInterrupted:
			return contentState{"waiting", "Work paused"}, true
		case storage.StatusError:
			return contentState{"error", "Work stopped"}, true
		default:
			return contentState{"finished", "Finished"}, true
		}
	}
	return contentState{}, false
}

func (s *Service) deliver(ctx context.Context, session storage.Session, state contentState, relevant bool, alert *Alert) error {
	devices, err := s.store.ListDevices()
	if err != nil {
		return err
	}
	approved := make(map[string]bool, len(devices))
	for _, device := range devices {
		approved[device.ID] = device.Status == storage.DeviceStatusApproved
	}
	registrations, err := s.store.ListSettings(namespace)
	if err != nil {
		return err
	}
	var failures []error
	for _, setting := range registrations {
		if !approved[setting.Key] {
			continue
		}
		var device deviceRegistration
		if err := json.Unmarshal(setting.Value, &device); err != nil {
			failures = append(failures, err)
			continue
		}
		if alert != nil && device.Token != "" {
			failures = append(failures, s.sendAlert(ctx, setting.Key, device.Registration, *alert))
		}
		if relevant {
			failures = append(failures, s.track(ctx, setting.Key, device.Activities, session, state))
		}
	}
	return errors.Join(failures...)
}

// track moves the Live Activities a device follows session with to state.
func (s *Service) track(ctx context.Context, deviceID string, activities []Activity, session storage.Session, state contentState) error {
	var failures []error
	for _, activity := range activities {
		if activity.SessionID != session.ID || !activity.ExpiresAt.After(time.Now()) {
			continue
		}
		ended := state.Status == "finished" || state.Status == "error"
		now := time.Now().Unix()
		if !ended && activity.LastState == state && now-activity.LastTimestamp < 60 {
			continue
		}
		timestamp := max(now, activity.LastTimestamp+1)
		aps := map[string]any{
			"timestamp":     timestamp,
			"event":         "update",
			"content-state": state,
			"stale-date":    timestamp + 180,
		}
		if ended {
			aps["event"] = "end"
			aps["dismissal-date"] = time.Now().Add(time.Minute).Unix()
		}
		err := s.sender.Send(ctx, activity.Token, activity.Environment, "liveactivity", digest(activity.ID), map[string]any{"aps": aps})
		if err != nil && !errors.Is(err, ErrInvalidToken) {
			failures = append(failures, err)
			continue
		}
		remove := ended || errors.Is(err, ErrInvalidToken)
		failures = append(failures, s.update(deviceID, func(device *deviceRegistration) {
			next := device.Activities[:0]
			for _, current := range device.Activities {
				if current.ID == activity.ID && current.Token == activity.Token {
					if remove {
						continue
					}
					current.LastTimestamp = timestamp
					current.LastState = state
				}
				next = append(next, current)
			}
			device.Activities = next
		}))
	}
	return errors.Join(failures...)
}

func (s *Service) refresh(ctx context.Context) error {
	if !s.Enabled() {
		return nil
	}
	settings, err := s.store.ListSettings(namespace)
	if err != nil {
		return err
	}
	ids := make(map[string]bool)
	for _, setting := range settings {
		var device deviceRegistration
		if err := json.Unmarshal(setting.Value, &device); err != nil {
			return err
		}
		for _, activity := range device.Activities {
			if activity.ExpiresAt.After(time.Now()) {
				ids[activity.SessionID] = true
			}
		}
	}
	var failures []error
	for id := range ids {
		failures = append(failures, s.process(ctx, sessionevents.Event{SessionID: id, Type: sessionevents.TypeSession}))
	}
	return errors.Join(failures...)
}

func (s *Service) sendAlert(ctx context.Context, deviceID string, registration Registration, alert Alert) error {
	deliveryID := deviceID + ":" + alert.SessionID
	encodedKey, _ := json.Marshal(alert.Key)
	previous, err := s.store.LoadSetting("mobile_delivered", deliveryID)
	if err == nil && string(previous.Value) == string(encodedKey) {
		return nil
	}
	if err != nil && !errors.Is(err, storage.ErrSettingNotFound) {
		return err
	}
	category := "JAZ_REPLY"
	if alert.secret {
		category = "JAZ_OPEN"
	}
	shown := apsAlert{Title: cmp.Or(alert.Title, alert.Name), Body: alert.Body}
	if alert.Title != "" {
		shown.Subtitle = alert.Name
	}
	payload := map[string]any{
		"aps": map[string]any{
			"alert":     shown,
			"sound":     "default",
			"thread-id": alert.SessionID,
			"category":  category,
		},
		"session_id":      alert.SessionID,
		"kind":            alert.Kind,
		"installation_id": registration.InstallationID,
	}
	if alert.RequestID != "" {
		payload["request_id"] = alert.RequestID
	}
	if err := s.sender.Send(ctx, registration.Token, registration.Environment, "alert", digest(alert.SessionID+alert.Key), payload); err != nil {
		if errors.Is(err, ErrInvalidToken) {
			return s.update(deviceID, func(device *deviceRegistration) {
				if device.Token == registration.Token {
					device.Token = ""
				}
			})
		}
		return err
	}
	_, err = s.store.SaveSetting("mobile_delivered", deliveryID, encodedKey)
	return err
}

func bounded(value string, count int) string {
	runes := []rune(value)
	return string(runes[:min(len(runes), count)])
}

func digest(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}
