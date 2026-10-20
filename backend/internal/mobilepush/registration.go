package mobilepush

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

var ErrInvalidRegistration = errors.New("invalid push registration")

const namespace = "mobile_push"

type Store interface {
	storage.SettingsStorage
	ListDevices() ([]storage.Device, error)
	LoadSession(string) (storage.Session, error)
	LoadBot(string) (storage.BotRecord, error)
	LoadMembership(string, string) (storage.BotMembership, error)
	LoadMembershipByThread(string) (storage.BotMembership, error)
}

type Registration struct {
	Token          string `json:"token"`
	Environment    string `json:"environment"`
	InstallationID string `json:"installation_id,omitempty"`
}

type Activity struct {
	Registration
	ID            string       `json:"id"`
	SessionID     string       `json:"session_id"`
	ExpiresAt     time.Time    `json:"expires_at"`
	LastTimestamp int64        `json:"last_timestamp,omitempty"`
	LastState     contentState `json:"last_state"`
}

type deviceRegistration struct {
	Registration
	Activities []Activity `json:"activities,omitempty"`
}

type Service struct {
	store        Store
	sender       Sender
	awaitsAnswer func(string) bool
	changed      chan struct{}
	mu           sync.Mutex
}

func NewService(store Store, sender Sender, awaitsAnswer func(string) bool) *Service {
	return &Service{store: store, sender: sender, awaitsAnswer: awaitsAnswer, changed: make(chan struct{}, 1)}
}

func (s *Service) Enabled() bool {
	return s.sender.Enabled()
}

func validate(input Registration) error {
	if input.Environment != "development" && input.Environment != "production" {
		return fmt.Errorf("%w: unknown environment", ErrInvalidRegistration)
	}
	if len(input.Token) < 32 || len(input.Token) > 1024 {
		return fmt.Errorf("%w: invalid token length", ErrInvalidRegistration)
	}
	if len(input.InstallationID) > 128 {
		return fmt.Errorf("%w: installation identity is too long", ErrInvalidRegistration)
	}
	if _, err := hex.DecodeString(input.Token); err != nil {
		return fmt.Errorf("%w: token must be hexadecimal", ErrInvalidRegistration)
	}
	return nil
}

func (s *Service) Register(deviceID string, input Registration) error {
	input.Token = strings.ToLower(strings.TrimSpace(input.Token))
	input.Environment = strings.TrimSpace(input.Environment)
	input.InstallationID = strings.TrimSpace(input.InstallationID)
	if input.InstallationID == "" {
		return fmt.Errorf("%w: installation identity is required", ErrInvalidRegistration)
	}
	if err := validate(input); err != nil {
		return err
	}
	return s.update(deviceID, func(device *deviceRegistration) {
		device.Registration = input
	})
}

func (s *Service) RegisterActivity(deviceID string, input Activity) error {
	input.Token = strings.ToLower(strings.TrimSpace(input.Token))
	input.Environment = strings.TrimSpace(input.Environment)
	if err := validate(input.Registration); err != nil {
		return err
	}
	if input.ID == "" || len(input.ID) > 200 || input.SessionID == "" {
		return ErrInvalidRegistration
	}
	session, err := s.store.LoadSession(input.SessionID)
	if err != nil {
		return fmt.Errorf("%w: conversation not found", ErrInvalidRegistration)
	}
	input.SessionID = session.ID
	input.ExpiresAt = time.Now().Add(8 * time.Hour)
	input.LastTimestamp = 0
	input.LastState = contentState{}
	err = s.update(deviceID, func(device *deviceRegistration) {
		next := make([]Activity, 0, len(device.Activities)+1)
		for _, activity := range device.Activities {
			if activity.ID == input.ID {
				input.LastTimestamp = activity.LastTimestamp
				input.LastState = activity.LastState
			}
			if activity.ID != input.ID && activity.ExpiresAt.After(time.Now()) {
				next = append(next, activity)
			}
		}
		if len(next) >= 16 {
			next = next[len(next)-15:]
		}
		device.Activities = append(next, input)
	})
	if err == nil {
		select {
		case s.changed <- struct{}{}:
		default:
		}
	}
	return err
}

func (s *Service) Unregister(deviceID, activityID string) error {
	if activityID == "" {
		return s.update(deviceID, func(device *deviceRegistration) {
			device.Registration = Registration{}
		})
	}
	return s.update(deviceID, func(device *deviceRegistration) {
		next := device.Activities[:0]
		for _, activity := range device.Activities {
			if activity.ID != activityID {
				next = append(next, activity)
			}
		}
		device.Activities = next
	})
}

func (s *Service) update(deviceID string, change func(*deviceRegistration)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var device deviceRegistration
	setting, err := s.store.LoadSetting(namespace, deviceID)
	if err == nil {
		if err := json.Unmarshal(setting.Value, &device); err != nil {
			return err
		}
	} else if !errors.Is(err, storage.ErrSettingNotFound) {
		return err
	}
	change(&device)
	value, err := json.Marshal(device)
	if err != nil {
		return err
	}
	_, err = s.store.SaveSetting(namespace, deviceID, value)
	return err
}
