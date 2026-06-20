package loops

import (
	"errors"
	"reflect"
	"testing"
)

func TestCoordinatorCreateEmptyThreadAssignsBoardsButSkipsCard(t *testing.T) {
	svc := &fakeMCPService{}
	boards := &fakeBoardService{boards: []BoardSummary{{ID: "board-1", Name: "News"}}}
	sink := &fakeEventSink{}
	c := Coordinator{Loops: svc, Boards: boards, Card: CardSink{Store: sink, Bus: sink}}

	loop, err := c.Create(CreateLoop{Name: "News", Runtime: RuntimeACP}, []string{"board-1"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if loop.ID == "" {
		t.Fatalf("loop not created: %#v", loop)
	}
	if want := [][]string{{"board-1"}}; !reflect.DeepEqual(boards.assigned, want) {
		t.Fatalf("assigned = %#v, want %#v", boards.assigned, want)
	}
	if len(sink.appended) != 0 || len(sink.published) != 0 {
		t.Fatalf("a threadless (REST) create must not emit a card: %#v", sink)
	}
}

func TestCoordinatorCreateAnnouncesToThread(t *testing.T) {
	svc := &fakeMCPService{}
	boards := &fakeBoardService{boards: []BoardSummary{{ID: "board-1", Name: "News"}}}
	sink := &fakeEventSink{}
	c := Coordinator{Loops: svc, Boards: boards, Card: CardSink{Store: sink, Bus: sink}}

	if _, err := c.Create(
		CreateLoop{Name: "News", Runtime: RuntimeACP, Schedule: Schedule{Expr: "13 * * * *"}},
		[]string{"board-1"}, "thread-1",
	); err != nil {
		t.Fatal(err)
	}
	if len(sink.published) != 1 || sink.published[0].SessionID != "thread-1" {
		t.Fatalf("expected one card on thread-1: %#v", sink.published)
	}
	lc := sink.published[0].LoopCreated
	if lc == nil || lc.Schedule != "13 * * * *" || len(lc.Boards) != 1 || lc.Boards[0].Name != "News" {
		t.Fatalf("unexpected card payload: %#v", lc)
	}
}

func TestCoordinatorRejectsBadBoardsBeforeCreate(t *testing.T) {
	svc := &fakeMCPService{}
	boards := &fakeBoardService{validateErr: errors.New("unknown board")}
	c := Coordinator{Loops: svc, Boards: boards}

	if _, err := c.Create(CreateLoop{Runtime: RuntimeACP}, []string{"nope"}, ""); err == nil {
		t.Fatal("expected validation error")
	}
	if svc.createCalled {
		t.Fatal("loop must not be created when board validation fails")
	}
}
