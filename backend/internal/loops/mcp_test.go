package loops

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/sessionevents"
)

type fakeEventSink struct {
	appended  []sessionevents.Event
	published []sessionevents.Event
}

func (f *fakeEventSink) AppendSessionEvents(_ string, events ...sessionevents.Event) error {
	for i := range events {
		if events[i].Seq == 0 {
			events[i].Seq = int64(len(f.appended) + 1)
		}
		f.appended = append(f.appended, events[i])
	}
	return nil
}

func (f *fakeEventSink) Publish(event sessionevents.Event) {
	f.published = append(f.published, event)
}

type fakeMCPService struct {
	createCalled bool
	created      CreateLoop
	updated      UpdateLoop
	loop         Loop
	runs         []Run
}

func (f *fakeMCPService) Create(in CreateLoop) (Loop, error) {
	f.createCalled = true
	f.created = in
	f.loop = Loop{
		ID:       "loop-1",
		Name:     in.Name,
		Runtime:  in.Runtime,
		ACPAgent: in.ACPAgent,
		Schedule: in.Schedule,
		Status:   in.Status,
	}
	return f.loop, nil
}

func (f *fakeMCPService) Update(_ string, in UpdateLoop) (Loop, error) {
	f.updated = in
	return f.loop, nil
}

func (f *fakeMCPService) Delete(string) error             { return nil }
func (f *fakeMCPService) Load(string) (Loop, error)       { return f.loop, nil }
func (f *fakeMCPService) List() ([]Loop, error)           { return []Loop{f.loop}, nil }
func (f *fakeMCPService) Runs(string, int) ([]Run, error) { return f.runs, nil }
func (f *fakeMCPService) RunNow(context.Context, string) (Run, error) {
	return Run{}, nil
}

type fakeBoardService struct {
	boards      []BoardSummary
	validated   [][]string
	assigned    [][]string
	assignedTo  []string
	forLoop     []string
	validateErr error
}

func (f *fakeBoardService) ListBoards() ([]BoardSummary, error) { return f.boards, nil }

func (f *fakeBoardService) ValidateBoardIDs(ids []string) error {
	f.validated = append(f.validated, ids)
	return f.validateErr
}

func (f *fakeBoardService) AssignLoopBoards(loop Loop, ids []string) error {
	f.assigned = append(f.assigned, ids)
	f.assignedTo = append(f.assignedTo, loop.ID)
	return nil
}

func (f *fakeBoardService) BoardsForLoop(string) ([]string, error) { return f.forLoop, nil }

func TestMCPCreateAssignsBoardsAndLeavesAgentEmpty(t *testing.T) {
	svc := &fakeMCPService{}
	boards := &fakeBoardService{}
	tools := NewMCPTools(svc, WithBoards(boards))

	loop, err := callCreate(t, tools, MCPCreateInput{
		Name:     "News",
		Prompt:   "politics",
		Runtime:  RuntimeACP,
		Schedule: Schedule{Kind: ScheduleCron, Expr: "13 * * * *", Timezone: "UTC"},
		BoardIDs: []string{"board-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if loop.ACPAgent != "" {
		t.Fatalf("expected empty agent (manager resolves default), got %q", loop.ACPAgent)
	}
	if want := [][]string{{"board-1"}}; !reflect.DeepEqual(boards.validated, want) {
		t.Fatalf("validated = %#v, want %#v", boards.validated, want)
	}
	if want := [][]string{{"board-1"}}; !reflect.DeepEqual(boards.assigned, want) {
		t.Fatalf("assigned = %#v, want %#v", boards.assigned, want)
	}
	if want := []string{"loop-1"}; !reflect.DeepEqual(boards.assignedTo, want) {
		t.Fatalf("assignedTo = %#v, want %#v", boards.assignedTo, want)
	}
}

func TestMCPCreateRejectsBadBoardsBeforePersisting(t *testing.T) {
	svc := &fakeMCPService{}
	boards := &fakeBoardService{validateErr: errors.New("unknown board")}
	tools := NewMCPTools(svc, WithBoards(boards))

	if _, err := callCreate(t, tools, MCPCreateInput{
		Prompt:   "x",
		Runtime:  RuntimeACP,
		Schedule: Schedule{Kind: ScheduleCron, Expr: "13 * * * *", Timezone: "UTC"},
		BoardIDs: []string{"nope"},
	}); err == nil {
		t.Fatal("expected error for bad board id")
	}
	if svc.createCalled {
		t.Fatal("loop must not be created when board validation fails")
	}
	if len(boards.assigned) != 0 {
		t.Fatalf("assignment must not run on validation failure: %#v", boards.assigned)
	}
}

func TestMCPGetReturnsBoardIDs(t *testing.T) {
	svc := &fakeMCPService{loop: Loop{ID: "loop-1"}}
	boards := &fakeBoardService{forLoop: []string{"board-1", "board-2"}}
	tools := NewMCPTools(svc, WithBoards(boards))

	_, out, err := tools.Get(context.Background(), nil, MCPIDInput{ID: "loop-1"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"board-1", "board-2"}; !reflect.DeepEqual(out.BoardIDs, want) {
		t.Fatalf("board_ids = %#v, want %#v", out.BoardIDs, want)
	}
}

func TestMCPUpdateEmptyBoardsClearsAssignments(t *testing.T) {
	svc := &fakeMCPService{loop: Loop{ID: "loop-1"}}
	boards := &fakeBoardService{}
	tools := NewMCPTools(svc, WithBoards(boards))

	empty := []string{}
	if _, _, err := tools.Update(context.Background(), nil, MCPUpdateInput{
		ID:       "loop-1",
		BoardIDs: &empty,
	}); err != nil {
		t.Fatal(err)
	}
	if want := [][]string{{}}; !reflect.DeepEqual(boards.assigned, want) {
		t.Fatalf("assigned = %#v, want a single empty reassignment", boards.assigned)
	}
}

func TestMCPBoardsListsAvailableBoards(t *testing.T) {
	want := []BoardSummary{{ID: "board-1", Name: "Home", IsDefault: true}}
	tools := NewMCPTools(&fakeMCPService{}, WithBoards(&fakeBoardService{boards: want}))

	_, out, err := tools.Boards(context.Background(), nil, MCPListInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out.Boards, want) {
		t.Fatalf("boards = %#v, want %#v", out.Boards, want)
	}
}

func TestMCPCreateWithoutBoardServiceIgnoresBoardIDs(t *testing.T) {
	svc := &fakeMCPService{}
	tools := NewMCPTools(svc)

	if _, err := callCreate(t, tools, MCPCreateInput{
		Prompt:   "x",
		Runtime:  RuntimeACP,
		Schedule: Schedule{Kind: ScheduleCron, Expr: "13 * * * *", Timezone: "UTC"},
		BoardIDs: []string{"board-1"},
	}); err != nil {
		t.Fatal(err)
	}
	if !svc.createCalled {
		t.Fatal("loop should still be created when no board service is wired")
	}
}

func TestMCPAvailableAgentsUsesListerOrder(t *testing.T) {
	tools := NewMCPTools(&fakeMCPService{}, WithAgentNames(func() []string {
		return []string{"codex", "claude", "opencode"}
	}))
	if want := []string{"codex", "claude", "opencode"}; !reflect.DeepEqual(tools.availableAgents(), want) {
		t.Fatalf("availableAgents = %#v, want %#v", tools.availableAgents(), want)
	}
	if NewMCPTools(&fakeMCPService{}).availableAgents() != nil {
		t.Fatal("availableAgents must be nil without a lister")
	}
}

func TestMCPCreateEmitsLoopCreatedCardToThread(t *testing.T) {
	svc := &fakeMCPService{}
	boards := &fakeBoardService{boards: []BoardSummary{{ID: "board-1", Name: "News"}}}
	sink := &fakeEventSink{}
	tools := NewMCPTools(svc, WithBoards(boards), WithEvents(sink, sink))

	req := &mcp.CallToolRequest{Extra: &mcp.RequestExtra{Header: mcpsession.Header("thread-1")}}
	_, loop, err := tools.Create(context.Background(), req, MCPCreateInput{
		Name:     "News",
		Prompt:   "politics",
		Runtime:  RuntimeACP,
		Status:   StatusActive,
		Schedule: Schedule{Kind: ScheduleCron, Expr: "13 * * * *", Timezone: "Europe/London"},
		BoardIDs: []string{"board-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sink.appended) != 1 || len(sink.published) != 1 {
		t.Fatalf("want 1 appended + 1 published, got %d/%d", len(sink.appended), len(sink.published))
	}
	ev := sink.published[0]
	if ev.SessionID != "thread-1" || ev.Type != sessionevents.TypeLoopCreated {
		t.Fatalf("unexpected envelope: %#v", ev)
	}
	lc := ev.LoopCreated
	if lc == nil || lc.LoopID != loop.ID || lc.Schedule != "13 * * * *" || lc.Timezone != "Europe/London" {
		t.Fatalf("unexpected loop payload: %#v", lc)
	}
	if len(lc.Boards) != 1 || lc.Boards[0].ID != "board-1" || lc.Boards[0].Name != "News" {
		t.Fatalf("unexpected boards: %#v", lc.Boards)
	}
}

func TestMCPCreateWithoutSessionHeaderSkipsCard(t *testing.T) {
	sink := &fakeEventSink{}
	tools := NewMCPTools(&fakeMCPService{}, WithEvents(sink, sink))

	if _, _, err := tools.Create(context.Background(), &mcp.CallToolRequest{}, MCPCreateInput{
		Prompt:   "x",
		Runtime:  RuntimeACP,
		Schedule: Schedule{Kind: ScheduleCron, Expr: "13 * * * *", Timezone: "UTC"},
	}); err != nil {
		t.Fatal(err)
	}
	if len(sink.appended) != 0 || len(sink.published) != 0 {
		t.Fatalf("card must be skipped without a session header: %#v", sink)
	}
}

func callCreate(t *testing.T, tools *MCPTools, in MCPCreateInput) (Loop, error) {
	t.Helper()
	_, loop, err := tools.Create(context.Background(), nil, in)
	return loop, err
}
