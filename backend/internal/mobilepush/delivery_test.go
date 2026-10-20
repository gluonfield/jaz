package mobilepush

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

type capturedPush struct {
	token   string
	kind    string
	payload map[string]any
}

type capturingSender struct {
	pushes []capturedPush
	err    error
}

func (s *capturingSender) Enabled() bool {
	return true
}

func (s *capturingSender) Send(_ context.Context, token, _, kind, _ string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	s.pushes = append(s.pushes, capturedPush{token: token, kind: kind, payload: parsed})
	return s.err
}

func pushFixture(t *testing.T, source string) (*Service, *sqlitestore.Store, *capturingSender, storage.Session) {
	t.Helper()
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	session, err := store.CreateSession(storage.CreateSession{Title: "Research", SourceType: source, Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveApprovedDevice(storage.SaveApprovedDevice{
		ID: "phone", Name: "Phone", Kind: storage.DeviceKindMobile, TokenHash: "test-hash", CreatedAt: time.Now(), ApprovedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	sender := &capturingSender{}
	service := NewService(store, sender, func(string) bool {
		return false
	})
	if err := service.Register("phone", Registration{Token: strings.Repeat("ab", 32), Environment: "development", InstallationID: "installation"}); err != nil {
		t.Fatal(err)
	}
	return service, store, sender, session
}

func TestBotDeliveryUsesExplicitMessageAndIsNotRepeated(t *testing.T) {
	service, _, sender, session := pushFixture(t, storage.SourceBot)
	ctx := context.Background()
	for _, event := range []sessionevents.Event{
		{SessionID: session.ID, Type: sessionevents.TypeACPMessage, Content: "private narration"},
		{SessionID: session.ID, Type: "acp", ACP: &sessionevents.ACPEvent{State: "idle", StopReason: "end_turn"}},
		{SessionID: session.ID, Type: sessionevents.TypeRoomMessage, Seq: 12, RoomMessage: &sessionevents.RoomMessageEvent{Speaker: "user", Text: "Hello"}},
	} {
		if err := service.process(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	if len(sender.pushes) != 0 {
		t.Fatalf("private output produced notifications: %#v", sender.pushes)
	}
	message := sessionevents.Event{
		SessionID: session.ID, Type: sessionevents.TypeRoomMessage, Seq: 13,
		RoomMessage: &sessionevents.RoomMessageEvent{Speaker: "bot", Text: "Found three suppliers"},
	}
	for range 2 {
		if err := service.process(ctx, message); err != nil {
			t.Fatal(err)
		}
	}
	if len(sender.pushes) != 1 {
		t.Fatalf("expected one explicit delivery, got %d", len(sender.pushes))
	}
	payload := sender.pushes[0].payload
	aps := payload["aps"].(map[string]any)
	body := aps["alert"].(map[string]any)["body"]
	if body != "Found three suppliers" || payload["kind"] != "bot" || payload["session_id"] != session.ID || payload["installation_id"] != "installation" {
		t.Fatalf("notification lost conversation identity: %#v", payload)
	}
}

func TestChatCompletionFollowsPersistedTurnAndUpdatesLiveActivity(t *testing.T) {
	service, store, sender, session := pushFixture(t, "")
	if err := store.UpdateSessionStatus(session.ID, storage.StatusRunning, "", time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := service.RegisterActivity("phone", Activity{
		Registration: Registration{Token: strings.Repeat("cd", 32), Environment: "development"},
		ID:           "activity", SessionID: session.ID,
	}); err != nil {
		t.Fatal(err)
	}
	idle := sessionevents.Event{SessionID: session.ID, Type: "acp", ACP: &sessionevents.ACPEvent{State: "idle", StopReason: "end_turn"}}
	if err := service.process(context.Background(), idle); err != nil {
		t.Fatal(err)
	}
	if len(sender.pushes) != 1 || sender.pushes[0].kind != "liveactivity" {
		t.Fatalf("early idle generated a reply alert: %#v", sender.pushes)
	}
	aps := sender.pushes[0].payload["aps"].(map[string]any)
	if aps["event"] != "update" || aps["content-state"].(map[string]any)["status"] != "working" {
		t.Fatalf("early idle ended saved work: %#v", aps)
	}
	if err := store.UpdateSessionStatus(session.ID, storage.StatusIdle, "", time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteSession(session.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	idle.Type = sessionevents.TypeTurnFinished
	if err := service.process(context.Background(), idle); err != nil {
		t.Fatal(err)
	}
	if len(sender.pushes) != 3 || sender.pushes[1].kind != "alert" || sender.pushes[2].kind != "liveactivity" {
		t.Fatalf("saved completion did not alert and end progress: %#v", sender.pushes)
	}
	if sender.pushes[2].payload["aps"].(map[string]any)["event"] != "end" {
		t.Fatal("completed activity remains live")
	}
	restarted := NewService(store, sender, func(string) bool {
		return false
	})
	if err := restarted.process(context.Background(), idle); err != nil {
		t.Fatal(err)
	}
	if len(sender.pushes) != 3 {
		t.Fatal("restart duplicated completion or retained the ended activity")
	}
}

func TestQuestionsCarryReplyIdentityAndProtectSecretQuestions(t *testing.T) {
	service, _, sender, session := pushFixture(t, "")
	for index, secret := range []bool{false, true} {
		event := sessionevents.Event{
			SessionID: session.ID, Type: "permission_request",
			Permission: &sessionevents.ACPPermission{
				ID:        []string{"question", "secret"}[index],
				Questions: []sessionevents.ACPQuestion{{ID: "q", Question: "Which supplier?", IsSecret: secret}},
			},
		}
		if err := service.process(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}
	for index, category := range []string{"JAZ_REPLY", "JAZ_OPEN"} {
		payload := sender.pushes[index].payload
		aps := payload["aps"].(map[string]any)
		if aps["category"] != category || payload["request_id"] == nil {
			t.Fatalf("wrong reply affordance: %#v", payload)
		}
		if index == 1 && aps["alert"].(map[string]any)["body"] != "Your answer is needed" {
			t.Fatal("secret question exposed on Lock Screen")
		}
	}
}

func TestRevocationAndNotificationOptOutPreserveBoundary(t *testing.T) {
	service, store, sender, session := pushFixture(t, storage.SourceBot)
	if err := service.RegisterActivity("phone", Activity{
		Registration: Registration{Token: strings.Repeat("cd", 32), Environment: "development"},
		ID:           "activity", SessionID: session.ID,
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.Unregister("phone", ""); err != nil {
		t.Fatal(err)
	}
	event := sessionevents.Event{SessionID: session.ID, Type: "permission_request", Permission: &sessionevents.ACPPermission{ID: "question"}}
	if err := service.process(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if len(sender.pushes) != 1 || sender.pushes[0].kind != "liveactivity" {
		t.Fatalf("opting out of notifications removed progress: %#v", sender.pushes)
	}
	if _, err := store.RevokeDevice("phone", time.Now()); err != nil {
		t.Fatal(err)
	}
	event.Type = "permission_response"
	if err := service.process(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if len(sender.pushes) != 1 {
		t.Fatal("a revoked device received a push")
	}
}

func TestFailedDeliveryCanRetry(t *testing.T) {
	service, _, sender, session := pushFixture(t, storage.SourceBot)
	event := sessionevents.Event{SessionID: session.ID, Type: sessionevents.TypeRoomMessage, Seq: 1, RoomMessage: &sessionevents.RoomMessageEvent{Speaker: "bot", Text: "Hello"}}
	sender.err = context.DeadlineExceeded
	if err := service.process(context.Background(), event); err == nil {
		t.Fatal("failed delivery was reported successful")
	}
	sender.err = nil
	if err := service.process(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if len(sender.pushes) != 2 {
		t.Fatal("failed delivery was incorrectly deduplicated")
	}
}

func TestPendingQuestionSurvivesStatusUpdatesAndPushTimestampsIncrease(t *testing.T) {
	service, store, sender, session := pushFixture(t, "")
	if err := store.UpdateSessionStatus(session.ID, storage.StatusRunning, "", time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := service.RegisterActivity("phone", Activity{
		Registration: Registration{Token: strings.Repeat("cd", 32), Environment: "development"},
		ID:           "activity", SessionID: session.ID,
	}); err != nil {
		t.Fatal(err)
	}
	pending := true
	service.awaitsAnswer = func(string) bool {
		return pending
	}
	event := sessionevents.Event{SessionID: session.ID, Type: "acp", ACP: &sessionevents.ACPEvent{State: "running"}}
	if err := service.process(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	first := sender.pushes[0].payload["aps"].(map[string]any)
	if first["content-state"].(map[string]any)["status"] != "waiting" {
		t.Fatal("running state hid a pending question")
	}
	if err := service.process(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if len(sender.pushes) != 1 {
		t.Fatal("unchanged progress spent another push update")
	}
	pending = false
	event.Type = "permission_response"
	if err := service.process(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	second := sender.pushes[1].payload["aps"].(map[string]any)
	if second["content-state"].(map[string]any)["status"] != "working" || second["timestamp"].(float64) <= first["timestamp"].(float64) {
		t.Fatalf("resumed progress had an old Apple timestamp: %#v", second)
	}
}

type channelSender chan capturedPush

func (s channelSender) Enabled() bool {
	return true
}

func (s channelSender) Send(ctx context.Context, token, _, kind, _ string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	select {
	case s <- capturedPush{token: token, kind: kind, payload: parsed}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestWorkerClosesAnActivityRegisteredAfterTheTurnAlreadyFinished(t *testing.T) {
	service, _, _, session := pushFixture(t, "")
	sender := make(channelSender, 4)
	service.sender = sender
	ctx, cancel := context.WithCancel(context.Background())
	done := service.Start(ctx, sessionevents.New(), func(err error) {
		t.Errorf("worker delivery: %v", err)
	})
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("worker did not stop")
		}
	}()
	if err := service.RegisterActivity("phone", Activity{
		Registration: Registration{Token: strings.Repeat("cd", 32), Environment: "development"},
		ID:           "late", SessionID: session.ID,
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case push := <-sender:
		if push.kind != "liveactivity" || push.payload["aps"].(map[string]any)["event"] != "end" {
			t.Fatalf("late activity remained working: %#v", push)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("registration did not wake the progress worker")
	}
}
