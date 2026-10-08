package bots

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

type fakeWorld struct {
	mu        sync.Mutex
	records   map[string]storage.BotRecord
	sessions  map[string]storage.Session
	events    map[string][]sessionevents.Event
	prompts   map[string][]string
	replies   map[string][]string
	created   []acp.SpawnRequest
	models    map[string]string
	loops     []loops.Loop
	published []sessionevents.Event
	held      map[string]chan struct{}
	// memberships holds each bot's place in each group.
	memberships []storage.BotMembership
	steered     map[string][]string
	// steerless makes every thread's agent unable to take messages mid-turn.
	steerless bool
	// unqueueable makes queueing a thread's next turn fail.
	unqueueable map[string]bool
	service     *Service
}

func newFakeWorld() *fakeWorld {
	return &fakeWorld{
		records:  map[string]storage.BotRecord{},
		sessions: map[string]storage.Session{},
		events:   map[string][]sessionevents.Event{},
		prompts:  map[string][]string{},
		replies:  map[string][]string{},
		models:   map[string]string{},
		held:     map[string]chan struct{}{},
		steered:  map[string][]string{},

		unqueueable: map[string]bool{},
	}
}

func (w *fakeWorld) SaveBot(record storage.BotRecord) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.records[record.ThreadID] = record
	return nil
}

func (w *fakeWorld) LoadBot(id string) (storage.BotRecord, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	record, ok := w.records[id]
	if !ok {
		return storage.BotRecord{}, storage.ErrBotNotFound
	}
	return record, nil
}

func (w *fakeWorld) ListBots() ([]storage.BotRecord, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]storage.BotRecord, 0, len(w.records))
	for _, record := range w.records {
		out = append(out, record)
	}
	return out, nil
}

func (w *fakeWorld) PinBots(ids []string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	for id, record := range w.records {
		record.Pinned = slices.Index(ids, id) + 1
		w.records[id] = record
	}
	return nil
}

func (w *fakeWorld) CreateSession(input storage.CreateSession) (storage.Session, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	// Like the store, every new thread gets an id of its own.
	id := "thread-" + input.Slug
	for n := 2; w.sessions[id].ID != ""; n++ {
		id = fmt.Sprintf("thread-%s-%d", input.Slug, n)
	}
	session := storage.Session{ID: id, Title: input.Title, SourceType: input.SourceType, SourceID: input.SourceID}
	w.sessions[session.ID] = session
	return session, nil
}

func (w *fakeWorld) SaveMembership(membership storage.BotMembership) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	for i, saved := range w.memberships {
		if saved.GroupID == membership.GroupID && saved.BotID == membership.BotID {
			w.memberships[i] = membership
			return nil
		}
	}
	w.memberships = append(w.memberships, membership)
	return nil
}

func (w *fakeWorld) LoadMembership(groupID, botID string) (storage.BotMembership, error) {
	return w.findMembership(func(m storage.BotMembership) bool { return m.GroupID == groupID && m.BotID == botID })
}

func (w *fakeWorld) LoadMembershipByThread(threadID string) (storage.BotMembership, error) {
	return w.findMembership(func(m storage.BotMembership) bool { return m.ThreadID == threadID })
}

func (w *fakeWorld) findMembership(match func(storage.BotMembership) bool) (storage.BotMembership, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if i := slices.IndexFunc(w.memberships, match); i >= 0 {
		return w.memberships[i], nil
	}
	return storage.BotMembership{}, storage.ErrMembershipNotFound
}

func (w *fakeWorld) ListMemberships() ([]storage.BotMembership, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.memberships), nil
}

func (w *fakeWorld) LoadSession(id string) (storage.Session, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	session, ok := w.sessions[id]
	if !ok {
		return storage.Session{}, storage.ErrBotNotFound
	}
	return session, nil
}

func (w *fakeWorld) ListSessions(storage.SessionFilter) ([]storage.Session, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]storage.Session, 0, len(w.sessions))
	for _, session := range w.sessions {
		out = append(out, session)
	}
	return out, nil
}

func (w *fakeWorld) UpdateSessionTitle(id, title string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	session := w.sessions[id]
	session.Title = title
	w.sessions[id] = session
	return nil
}

func (w *fakeWorld) SetArchived(id string, archived bool) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	session := w.sessions[id]
	session.Archived = archived
	w.sessions[id] = session
	return nil
}

func (w *fakeWorld) LoadSessionEvents(id string) ([]sessionevents.Event, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]sessionevents.Event(nil), w.events[id]...), nil
}

func (w *fakeWorld) AppendSessionEvents(id string, events ...sessionevents.Event) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	for i := range events {
		events[i].Seq = int64(len(w.events[id]) + 1)
		w.events[id] = append(w.events[id], events[i])
	}
	return nil
}

func (w *fakeWorld) LoadLatestSessionEvent(id, eventType string) (sessionevents.Event, bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for i := len(w.events[id]) - 1; i >= 0; i-- {
		if w.events[id][i].Type == eventType {
			return w.events[id][i], true, nil
		}
	}
	return sessionevents.Event{}, false, nil
}

func (w *fakeWorld) Publish(event sessionevents.Event) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.published = append(w.published, event)
}

func (w *fakeWorld) List() ([]loops.Loop, error) {
	return w.loops, nil
}

func (w *fakeWorld) Update(id string, input loops.UpdateLoop) (loops.Loop, error) {
	for i := range w.loops {
		if w.loops[i].ID == id && input.BotID != nil {
			w.loops[i].BotID = *input.BotID
			return w.loops[i], nil
		}
	}
	return loops.Loop{}, nil
}

func (w *fakeWorld) Delete(string) error {
	return nil
}

func (w *fakeWorld) OnBoard(loop loops.Loop) bool {
	return strings.HasPrefix(loop.ID, "widget-")
}

type fakeThreads struct {
	world *fakeWorld
}

// QueueInternalTurn queues a hidden turn on a thread and, like the server,
// runs the thread's queued turns one at a time while it is free.
func (t fakeThreads) QueueInternalTurn(_ context.Context, id string, message storage.QueuedMessage) error {
	t.world.mu.Lock()
	if t.world.unqueueable[id] {
		delete(t.world.unqueueable, id)
		t.world.mu.Unlock()
		return errors.New("queue unavailable")
	}
	session := t.world.sessions[id]
	session.QueuedMessages = append(session.QueuedMessages, message.AsInternal())
	t.world.sessions[id] = session
	t.world.mu.Unlock()
	go t.drain(id)
	return nil
}

func (t fakeThreads) drain(id string) {
	for {
		t.world.mu.Lock()
		session := t.world.sessions[id]
		if session.Turn != nil || len(session.QueuedMessages) == 0 {
			t.world.mu.Unlock()
			return
		}
		message := session.QueuedMessages[0]
		session.QueuedMessages = session.QueuedMessages[1:]
		session.Turn = &storage.Turn{Output: message.Output}
		t.world.sessions[id] = session
		t.world.prompts[id] = append(t.world.prompts[id], message.Text)
		t.world.mu.Unlock()
		if _, err := t.Wait(context.Background(), acp.WaitRequest{Session: id}); err != nil {
			return
		}
	}
}

func (w *fakeWorld) AppendTurnReply(id, message string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	session := w.sessions[id]
	session.Turn.Output.Replies = append(session.Turn.Output.Replies, message)
	w.sessions[id] = session
	return nil
}

func (t fakeThreads) CreateSession(_ context.Context, req acp.SpawnRequest) (storage.Session, error) {
	t.world.mu.Lock()
	t.world.created = append(t.world.created, req)
	t.world.mu.Unlock()
	session, err := t.world.CreateSession(storage.CreateSession{Slug: req.Slug, Title: req.Title, SourceType: req.SourceType, SourceID: req.SourceID})
	// Like the manager, an omitted agent is the default one.
	session.RuntimeRef = &storage.RuntimeRef{Agent: cmp.Or(req.ACPAgent, acp.AgentCodex)}
	t.world.mu.Lock()
	t.world.sessions[session.ID] = session
	t.world.mu.Unlock()
	return session, err
}

func (t fakeThreads) StartInternalTurnWhenIdle(_ context.Context, req acp.InternalTurnRequest) (acp.Job, error) {
	t.world.mu.Lock()
	defer t.world.mu.Unlock()
	t.world.prompts[req.Session] = append(t.world.prompts[req.Session], req.Message)
	session := t.world.sessions[req.Session]
	session.Turn = &storage.Turn{Output: req.Output}
	t.world.sessions[req.Session] = session
	return acp.Job{ID: req.Session}, nil
}

// SteerInternal hands a message to the thread's running turn, as the manager
// does, failing when no turn runs or its agent cannot take it.
func (t fakeThreads) SteerInternal(_ context.Context, session, message string) (acp.Job, error) {
	t.world.mu.Lock()
	defer t.world.mu.Unlock()
	if t.world.sessions[session].Turn == nil {
		return acp.Job{}, errors.New("turn ended")
	}
	if t.world.steerless {
		return acp.Job{}, acp.ErrSteeringUnsupported
	}
	t.world.steered[session] = append(t.world.steered[session], message)
	return acp.Job{ID: session}, nil
}

// Wait runs the turn: the bot sends its scripted message, if any, the way it
// would call send_message.
func (t fakeThreads) Wait(_ context.Context, req acp.WaitRequest) (acp.Job, error) {
	t.world.mu.Lock()
	reply := ""
	if queue := t.world.replies[req.Session]; len(queue) > 0 {
		reply = queue[0]
		t.world.replies[req.Session] = queue[1:]
	}
	held := t.world.held[req.Session]
	t.world.mu.Unlock()
	if held != nil {
		<-held
	}
	if reply != "" {
		if err := t.world.service.Say(req.Session, reply); err != nil {
			return acp.Job{}, err
		}
	}
	t.world.mu.Lock()
	session := t.world.sessions[req.Session]
	session.Turn = nil
	t.world.sessions[req.Session] = session
	t.world.mu.Unlock()
	return acp.Job{ID: req.Session, State: acp.StateIdle, Assistant: "private notes"}, nil
}

func (t fakeThreads) SetModel(_ context.Context, sessionID, model, effort string) error {
	t.world.mu.Lock()
	defer t.world.mu.Unlock()
	t.world.models[sessionID] = model + "/" + effort
	return nil
}

func (fakeThreads) SwitchAgent(context.Context, string, string) error {
	return nil
}

func newTestService(world *fakeWorld) *Service {
	world.service = NewService(world, "/bots", fakeThreads{world: world}, fakeThreads{world: world}, world, world, world, log.New(nil))
	return world.service
}

// ResolveAttachments finds each id as a file uploaded to thread, as the
// server's attachment store does.
func (w *fakeWorld) ResolveAttachments(thread string, ids []string) ([]storage.Attachment, error) {
	attachments := make([]storage.Attachment, 0, len(ids))
	for _, id := range ids {
		attachments = append(attachments, storage.Attachment{ID: id, Name: id + ".pdf", ServerPath: "/attachments/" + thread + "/" + id + ".pdf"})
	}
	return attachments, nil
}

func (w *fakeWorld) addBot(id, name string) {
	w.sessions[id] = storage.Session{ID: id, Title: name}
	w.records[id] = storage.BotRecord{ThreadID: id, Kind: KindBot}
}

func (w *fakeWorld) roomMessages(id string) []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []string
	for _, event := range w.events[id] {
		if event.RoomMessage != nil {
			out = append(out, event.RoomMessage.Name+": "+event.RoomMessage.Text)
		}
	}
	return out
}

func (w *fakeWorld) promptCount(id string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.prompts[id])
}

// newGroup creates the group Launch of Research (a) and Marketing (b) and
// returns it with the threads Research and Marketing take their turns in
// there.
func newGroup(t *testing.T, service *Service) (Bot, string, string) {
	t.Helper()
	group, err := service.CreateGroup("Launch", []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	return group, memberThread(t, service, group.ID, "a"), memberThread(t, service, group.ID, "b")
}

// memberThread is the thread bot takes its turns in for group, created as it
// would be before the bot's first delivery there.
func memberThread(t *testing.T, service *Service, groupID, bot string) string {
	t.Helper()
	membership, err := service.join(groupID, bot)
	if err != nil {
		t.Fatal(err)
	}
	return membership.ThreadID
}

func (w *fakeWorld) running(id string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.sessions[id].Turn != nil
}

// settle waits until no thread is taking or has queued a turn and stays so
// briefly, long enough for deliveries on their way to land.
func (w *fakeWorld) settle(t *testing.T) {
	t.Helper()
	waitUntil(t, func() bool {
		for range 3 {
			if w.busy() {
				return false
			}
			time.Sleep(10 * time.Millisecond)
		}
		return true
	})
}

func (w *fakeWorld) busy() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, session := range w.sessions {
		if session.Turn != nil || len(session.QueuedMessages) > 0 {
			return true
		}
	}
	return false
}

func waitUntil(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !check() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestRoutineOwnerChecksNamedBotsKeepsBotThreadAndGivesOtherThreadsANewBot(t *testing.T) {
	world := newFakeWorld()
	world.addBot("gimli", "Gimli")
	service := newTestService(world)

	owner, err := service.RoutineOwner("gimli", loops.CreateLoop{Name: "Digest"})
	if err != nil || owner != "gimli" {
		t.Fatalf("owner from a bot thread = %q, %v", owner, err)
	}
	owner, err = service.RoutineOwner("chat-thread", loops.CreateLoop{Name: "Morning triage", ACPAgent: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	record, err := world.LoadBot(owner)
	if err != nil || record.Kind != KindBot {
		t.Fatalf("new owner record = %+v, %v", record, err)
	}
	if len(world.created) != 1 || world.created[0].SourceType != storage.SourceBot || world.created[0].ACPAgent != "claude" || world.created[0].Title != "Morning triage" {
		t.Fatalf("created threads = %+v", world.created)
	}
	if owner, err := service.RoutineOwner("chat-thread", loops.CreateLoop{BotID: "gimli"}); err != nil || owner != "gimli" {
		t.Fatalf("named owner = %q, %v", owner, err)
	}
	world.sessions["research"] = storage.Session{ID: "research", SourceType: storage.SourceBotWorker, SourceID: "gimli"}
	if owner, err := service.RoutineOwner("research", loops.CreateLoop{Name: "Watch"}); err != nil || owner != "gimli" || len(world.created) != 1 {
		t.Fatalf("a subtask's routine went to %q (%v), %d bots created", owner, err, len(world.created))
	}
	world.sessions["crew"] = storage.Session{ID: "crew", Title: "Crew"}
	world.records["crew"] = storage.BotRecord{ThreadID: "crew", Kind: KindGroup}
	for _, named := range []string{"crew", "missing"} {
		if _, err := service.RoutineOwner("gimli", loops.CreateLoop{BotID: named}); err == nil {
			t.Fatalf("routine was given to %q, which is not a bot", named)
		}
	}
}

func TestAMembersPostReachesTheOthersAfterTheirTurnsEnd(t *testing.T) {
	world := newFakeWorld()
	world.addBot("a", "Research")
	world.addBot("b", "Marketing")
	service := newTestService(world)
	group, research, marketing := newGroup(t, service)
	world.replies[marketing] = []string{"@Research can you check the numbers?"}
	release := make(chan struct{})
	world.held[marketing] = release

	if err := service.Post(group.ID, "where is the launch draft?", nil); err != nil {
		t.Fatal(err)
	}
	// Marketing answers once Research's turn has ended, with a plain-text tag
	// that names no member.
	waitUntil(t, func() bool {
		return world.promptCount(research) == 1 && !world.running(research) && world.running(marketing)
	})
	close(release)
	waitUntil(t, func() bool { return world.promptCount(research) == 2 })
	world.settle(t)
	world.mu.Lock()
	defer world.mu.Unlock()
	if prompt := world.prompts[research][1]; !strings.Contains(prompt, "Marketing: @Research can you check the numbers?") {
		t.Fatalf("Research's second turn:\n%s", prompt)
	}
	if len(world.prompts[marketing]) != 1 {
		t.Fatalf("Marketing took %d turns: %q", len(world.prompts[marketing]), world.prompts[marketing])
	}
}

// progressDuringTurn has Research post three findings while it and Marketing
// both work on their turns for the user's post.
func progressDuringTurn(t *testing.T, world *fakeWorld, service *Service, group Bot, research, marketing string, releaseMarketing chan struct{}) {
	t.Helper()
	world.held[research] = make(chan struct{})
	world.held[marketing] = releaseMarketing
	if err := service.Post(group.ID, "dig into pricing", nil); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool { return world.running(research) && world.running(marketing) })
	for _, finding := range findings {
		if err := service.Say(research, finding); err != nil {
			t.Fatal(err)
		}
	}
}

var findings = []string{"first finding", "second finding", "third finding"}

func TestPostsDuringAMembersTurnReachItBeforeTheTurnEnds(t *testing.T) {
	world := newFakeWorld()
	world.addBot("a", "Research")
	world.addBot("b", "Marketing")
	service := newTestService(world)
	group, research, marketing := newGroup(t, service)
	release := make(chan struct{})
	progressDuringTurn(t, world, service, group, research, marketing, release)

	steered := func() string {
		world.mu.Lock()
		defer world.mu.Unlock()
		return strings.Join(world.steered[marketing], "\n")
	}
	waitUntil(t, func() bool { return strings.Contains(steered(), "Research: third finding") })
	for _, finding := range findings {
		if strings.Count(steered(), "Research: "+finding) != 1 {
			t.Fatalf("Marketing was handed:\n%s", steered())
		}
	}
	if strings.Contains(steered(), "dig into pricing") {
		t.Fatalf("Marketing was handed its turn's own post again:\n%s", steered())
	}
	close(release)
	close(world.held[research])
	world.settle(t)
	if world.promptCount(marketing) != 1 {
		world.mu.Lock()
		defer world.mu.Unlock()
		t.Fatalf("Marketing took %d turns: %q", len(world.prompts[marketing]), world.prompts[marketing])
	}
}

func TestPostsAnAgentCannotTakeMidTurnReachItInTurnsAfterIt(t *testing.T) {
	world := newFakeWorld()
	world.addBot("a", "Research")
	world.addBot("b", "Marketing")
	world.steerless = true
	service := newTestService(world)
	group, research, marketing := newGroup(t, service)
	world.replies[marketing] = []string{"On it."}
	release := make(chan struct{})
	progressDuringTurn(t, world, service, group, research, marketing, release)

	close(release)
	later := func() string {
		world.mu.Lock()
		defer world.mu.Unlock()
		if len(world.prompts[marketing]) < 2 {
			return ""
		}
		return strings.Join(world.prompts[marketing][1:], "\n")
	}
	waitUntil(t, func() bool { return strings.Contains(later(), "third finding") })
	close(world.held[research])
	for _, finding := range findings {
		if strings.Count(later(), finding) != 1 {
			t.Fatalf("Marketing's later turns:\n%s", later())
		}
	}
	if strings.Contains(later(), "dig into pricing") {
		t.Fatalf("Marketing's later turns repeat what it had seen:\n%s", later())
	}
}

func TestMessagesAQueueRefusedReachTheNextTurn(t *testing.T) {
	world := newFakeWorld()
	world.addBot("a", "Research")
	world.addBot("b", "Marketing")
	service := newTestService(world)
	group, _, marketing := newGroup(t, service)
	world.unqueueable[marketing] = true

	if err := service.Post(group.ID, "draft the launch post", nil); err != nil {
		t.Fatal(err)
	}
	unreachable := func() bool {
		world.mu.Lock()
		defer world.mu.Unlock()
		return slices.ContainsFunc(world.events[group.ID], func(event sessionevents.Event) bool {
			return event.BotActivity != nil && event.BotActivity.Kind == "unreachable" && strings.HasPrefix(event.BotActivity.Label, "Marketing · queue unavailable")
		})
	}
	waitUntil(t, unreachable)
	if err := service.Post(group.ID, "and keep it short", nil); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool { return world.promptCount(marketing) == 1 })
	world.mu.Lock()
	defer world.mu.Unlock()
	if prompt := world.prompts[marketing][0]; !strings.Contains(prompt, "draft the launch post") || !strings.Contains(prompt, "keep it short") {
		t.Fatalf("Marketing's first turn:\n%s", prompt)
	}
}

func TestBotsAnsweringEachOtherStopAtTheFollowUpCap(t *testing.T) {
	world := newFakeWorld()
	world.addBot("a", "Research")
	world.addBot("b", "Marketing")
	service := newTestService(world)
	group, research, marketing := newGroup(t, service)
	for range 20 {
		world.replies[research] = append(world.replies[research], "you?")
		world.replies[marketing] = append(world.replies[marketing], "no, you?")
	}
	turns := func() int { return world.promptCount(research) + world.promptCount(marketing) }

	if err := service.Post(group.ID, "who goes first?", nil); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool {
		before := turns()
		time.Sleep(30 * time.Millisecond)
		return before > 2 && turns() == before
	})
	if turns() > 2+maxFollowUps {
		t.Fatalf("turns = %d, cap is %d", turns(), 2+maxFollowUps)
	}
}

func TestASlowMemberDoesNotHoldUpTheOthers(t *testing.T) {
	world := newFakeWorld()
	world.addBot("a", "Research")
	world.addBot("b", "Marketing")
	service := newTestService(world)
	group, research, marketing := newGroup(t, service)
	release := make(chan struct{})
	world.held[research] = release
	world.replies[research] = []string{"Here is a meme."}
	world.replies[marketing] = []string{"Quick one."}

	if err := service.Post(group.ID, "a good meme please", nil); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool { return slices.Contains(world.roomMessages(group.ID), "Marketing: Quick one.") })
	close(release)
	waitUntil(t, func() bool { return slices.Contains(world.roomMessages(group.ID), "Research: Here is a meme.") })
}

func TestUpdateChangesNothingWhenAnyPartIsInvalid(t *testing.T) {
	world := newFakeWorld()
	world.addBot("gimli", "Gimli")
	service := newTestService(world)

	name := "Thorin"
	if _, err := service.Update(t.Context(), "gimli", UpdateBot{Name: &name, Avatar: &Avatar{Shape: "blob", Color: "plaid"}}); err == nil {
		t.Fatal("an unsupported avatar was accepted")
	}
	if title := world.sessions["gimli"].Title; title != "Gimli" {
		t.Fatalf("a rejected update renamed the bot to %q", title)
	}
}

func TestRoutineRunsAsAnnouncedTurnInItsBotsThread(t *testing.T) {
	world := newFakeWorld()
	world.addBot("gimli", "Gimli")
	service := newTestService(world)

	job, err := service.RunRoutine(t.Context(), "gimli", "Digest", "summarise the inbox")
	if err != nil || job.State != acp.StateIdle {
		t.Fatalf("routine turn = %+v, %v", job, err)
	}
	world.mu.Lock()
	defer world.mu.Unlock()
	if prompts := world.prompts["gimli"]; len(prompts) != 1 || !strings.HasPrefix(prompts[0], "summarise the inbox") {
		t.Fatalf("routine prompts = %q", prompts)
	}
	if events := world.events["gimli"]; len(events) != 1 || events[0].BotActivity == nil || events[0].BotActivity.Kind != "routine" || events[0].BotActivity.Label != "Digest" {
		t.Fatalf("bot chat events = %+v", events)
	}
}

func TestSayOutsideAServiceTurnReachesTheBotsOwnChat(t *testing.T) {
	world := newFakeWorld()
	world.addBot("gimli", "Gimli")
	service := newTestService(world)

	if err := service.Say("gimli", "  Moved it to In Progress.  "); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(world.roomMessages("gimli"), "\n"); got != "Gimli: Moved it to In Progress." {
		t.Fatalf("own chat = %q", got)
	}
	if bot, err := service.Load("gimli"); err != nil || bot.Preview != "Moved it to In Progress." {
		t.Fatalf("preview = %q, %v", bot.Preview, err)
	}
	if err := service.Say("plain-chat", "hi"); err == nil {
		t.Fatal("a thread that is not a bot sent a bot message")
	}
}

func TestAdoptLoopsGivesEachOwnerlessLoopItsOwnBotExceptBoardWidgets(t *testing.T) {
	world := newFakeWorld()
	world.addBot("gimli", "Gimli")
	world.loops = []loops.Loop{
		{ID: "loop-1", Name: "Morning triage", ACPAgent: "codex", Directory: "triage"},
		{ID: "loop-2", Name: "Digest", BotID: "gimli"},
		{ID: "widget-loop", Name: "world-clocks"},
	}
	if err := newTestService(world).AdoptLoops(context.Background()); err != nil {
		t.Fatal(err)
	}
	adopted := world.loops[0].BotID
	record, err := world.LoadBot(adopted)
	if err != nil || record.Kind != KindBot || world.sessions[adopted].Title != "Morning triage" {
		t.Fatalf("adopted by %q: %+v, %v", adopted, record, err)
	}
	if len(world.created) != 1 || world.created[0].ACPAgent != "codex" || world.created[0].Home != "/bots" || world.created[0].Directory != "" {
		t.Fatalf("created bots = %+v", world.created)
	}
	if world.loops[1].BotID != "gimli" {
		t.Fatal("an owned loop changed owner")
	}
	if world.loops[2].BotID != "" {
		t.Fatal("a board widget was given a bot")
	}
}

func TestGroupTurnsRunInTheBotsGroupThreadAndPostAsTheBot(t *testing.T) {
	world := newFakeWorld()
	world.addBot("a", "Research")
	world.addBot("b", "Marketing")
	service := newTestService(world)
	group, err := service.CreateGroup("Launch", []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	world.held["a"] = make(chan struct{})
	world.replies["thread-Research in Launch"] = []string{"On it."}

	if err := service.Post(group.ID, "dig into pricing", nil); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool { return slices.Contains(world.roomMessages(group.ID), "Research: On it.") })
	world.settle(t)
	membership, err := world.LoadMembership(group.ID, "a")
	if err != nil || membership.ThreadID != "thread-Research in Launch" || membership.Seen == 0 {
		t.Fatalf("membership = %+v, %v", membership, err)
	}
	if service.AppVisible(membership.ThreadID) {
		t.Error("apps opened in a group thread were reported as shown to the user")
	}
	thread, err := world.LoadSession(membership.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	if modules, err := Prompt(world, thread); err != nil || len(modules) == 0 {
		t.Fatalf("group thread identity = %v, %v", modules, err)
	}
	world.mu.Lock()
	defer world.mu.Unlock()
	if len(world.prompts["a"]) != 0 || len(world.prompts[membership.ThreadID]) != 1 {
		t.Fatalf("turns in Research's chat %q, in its group thread %q", world.prompts["a"], world.prompts[membership.ThreadID])
	}
	if !slices.ContainsFunc(world.created, func(req acp.SpawnRequest) bool {
		return req.Title == "Research in Launch" && req.SourceType == storage.SourceBotMember && req.SourceID == "a"
	}) {
		t.Fatalf("group threads created as %+v", world.created)
	}
}

func TestFilesPostedToAGroupReachEveryMember(t *testing.T) {
	world := newFakeWorld()
	world.addBot("a", "Research")
	world.addBot("b", "Marketing")
	service := newTestService(world)
	group, research, marketing := newGroup(t, service)

	if err := service.Post(group.ID, "", []string{"report"}); err != nil {
		t.Fatal(err)
	}
	world.settle(t)
	world.mu.Lock()
	defer world.mu.Unlock()
	for _, thread := range []string{research, marketing} {
		if prompts := world.prompts[thread]; len(prompts) != 1 || !strings.Contains(prompts[0], "report.pdf: /attachments/"+group.ID+"/report.pdf") {
			t.Fatalf("%s was shown %q", thread, prompts)
		}
	}
}

func TestBotModelChangesReachItsGroupThreads(t *testing.T) {
	world := newFakeWorld()
	world.addBot("a", "Research")
	world.addBot("b", "Marketing")
	service := newTestService(world)
	_, research, marketing := newGroup(t, service)
	model := "sonnet"
	if _, err := service.Update(t.Context(), "a", UpdateBot{Model: &model, ReasoningEffort: "low"}); err != nil {
		t.Fatal(err)
	}
	if world.models["a"] != "sonnet/low" || world.models[research] != "sonnet/low" || world.models[marketing] != "" {
		t.Fatalf("models = %v", world.models)
	}
}

func TestRoutinesFromAGroupThreadBelongToItsBot(t *testing.T) {
	world := newFakeWorld()
	world.addBot("a", "Research")
	world.addBot("b", "Marketing")
	service := newTestService(world)
	_, research, _ := newGroup(t, service)
	if owner, err := service.RoutineOwner(research, loops.CreateLoop{Name: "Watch prices"}); err != nil || owner != "a" {
		t.Fatalf("owner = %q, %v", owner, err)
	}
}
