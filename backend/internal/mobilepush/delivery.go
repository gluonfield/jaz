package mobilepush

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/wins/jaz/backend/internal/bots"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

type contentState struct {
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type alert struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// process consumes stored conversation events, never provider reasoning or
// streamed assistant fragments. Bot messages are delivered only by SendMessage.
func (s *Service) process(ctx context.Context, event sessionevents.Event) error {
	if !s.Enabled() {
		return nil
	}
	event.NormalizePayload()
	if event.Type == sessionevents.TypeRoomMessage && (event.RoomMessage == nil || event.RoomMessage.Speaker != "bot") {
		return nil
	}
	session, err := s.store.LoadSession(event.SessionID)
	if err != nil {
		return err
	}
	if session.Archived {
		return nil
	}
	if session.SourceType == storage.SourceBotMember {
		membership, err := s.store.LoadMembershipByThread(session.ID)
		if err != nil {
			return err
		}
		session, err = s.store.LoadSession(membership.GroupID)
		if err != nil {
			return err
		}
		event = sessionevents.Event{SessionID: session.ID, Type: sessionevents.TypeSession}
	}
	if session.SourceType == storage.SourceBot {
		if record, err := s.store.LoadBot(session.ID); err == nil && record.Kind == bots.KindGroup {
			working, _ := bots.GroupState(s.store, record)
			session.Status = storage.StatusIdle
			if len(working) > 0 {
				session.Status = storage.StatusRunning
			}
		}
	}
	state, relevant := progress(session, event)
	if !relevant {
		return nil
	}
	if state.Status == "working" && s.awaitsAnswer(session.ID) {
		state = contentState{"waiting", "Needs your answer"}
	}
	notification, key := notificationFor(session, event)
	return s.deliver(ctx, session, state, notification, key, event.Permission)
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

func notificationFor(session storage.Session, event sessionevents.Event) (*alert, string) {
	visible := session.SourceType == storage.SourceBot || session.SourceType == "" && session.ParentID == ""
	if !visible {
		return nil, ""
	}
	title := session.Title
	if title == "" {
		title = session.Slug
	}
	notification := &alert{Title: bounded(title, 100)}
	key := fmt.Sprintf("%s:%d", event.Type, event.Seq)
	switch event.Type {
	case sessionevents.TypeRoomMessage:
		notification.Body = bounded(event.RoomMessage.Text, 300)
		if notification.Body == "" {
			notification.Body = "New message"
		}
	case "permission_request":
		if event.Permission == nil {
			return nil, ""
		}
		key = "question:" + event.Permission.ID
		notification.Body = "Your answer is needed"
		for _, question := range event.Permission.Questions {
			if !question.IsSecret {
				notification.Body = bounded(question.Question, 300)
				break
			}
		}
	case sessionevents.TypeTurnFinished:
		if event.ACP == nil {
			return nil, ""
		}
		if event.ACP.State == "failed" {
			key = "failed:" + session.UpdatedAt.UTC().Format(time.RFC3339Nano)
			notification.Body = "Work stopped with an error. Open the conversation for details."
			return notification, key
		}
		if session.SourceType != "" || event.ACP.State != "idle" || event.ACP.StopReason != "end_turn" || session.CompletedAt.IsZero() {
			return nil, ""
		}
		key = "completed:" + session.CompletedAt.UTC().Format(time.RFC3339Nano)
		notification.Body = "Your reply is ready"
	default:
		return nil, ""
	}
	return notification, key
}

func (s *Service) deliver(ctx context.Context, session storage.Session, state contentState, notification *alert, key string, permission *sessionevents.ACPPermission) error {
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
		if notification != nil && device.Token != "" {
			if err := s.sendAlert(ctx, setting.Key, device.Registration, session, *notification, key, permission); err != nil {
				failures = append(failures, err)
			}
		}
		for _, activity := range device.Activities {
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
			if err == nil && ended || errors.Is(err, ErrInvalidToken) {
				failures = append(failures, s.Unregister(setting.Key, activity.ID))
			} else if err != nil {
				failures = append(failures, err)
			} else {
				failures = append(failures, s.update(setting.Key, func(device *deviceRegistration) {
					for index := range device.Activities {
						if device.Activities[index].ID == activity.ID && device.Activities[index].Token == activity.Token {
							device.Activities[index].LastTimestamp = timestamp
							device.Activities[index].LastState = state
						}
					}
				}))
			}
		}
	}
	return errors.Join(failures...)
}

func (s *Service) refresh(ctx context.Context) error {
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

func (s *Service) sendAlert(ctx context.Context, deviceID string, registration Registration, session storage.Session, notification alert, key string, permission *sessionevents.ACPPermission) error {
	deliveryID := deviceID + ":" + session.ID
	encodedKey, _ := json.Marshal(key)
	previous, err := s.store.LoadSetting("mobile_delivered", deliveryID)
	if err == nil && string(previous.Value) == string(encodedKey) {
		return nil
	}
	if err != nil && !errors.Is(err, storage.ErrSettingNotFound) {
		return err
	}
	kind := "chat"
	if session.SourceType == storage.SourceBot {
		kind = "bot"
		if bot, err := s.store.LoadBot(session.ID); err == nil && bot.Kind == bots.KindGroup {
			kind = "group"
		}
	}
	category := "JAZ_REPLY"
	if permission != nil {
		for _, question := range permission.Questions {
			if question.IsSecret {
				category = "JAZ_OPEN"
				break
			}
		}
	}
	payload := map[string]any{
		"aps": map[string]any{
			"alert":     notification,
			"sound":     "default",
			"thread-id": session.ID,
			"category":  category,
		},
		"session_id":      session.ID,
		"kind":            kind,
		"installation_id": registration.InstallationID,
	}
	if permission != nil {
		payload["request_id"] = permission.ID
	}
	if err := s.sender.Send(ctx, registration.Token, registration.Environment, "alert", digest(session.ID+key), payload); err != nil {
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
