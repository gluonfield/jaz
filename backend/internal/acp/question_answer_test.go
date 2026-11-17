package acp_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestQuestionAnswerSurvivesStartupAndAdmissionRetry(t *testing.T) {
	store, root := newTestStore(t)
	ctx := testContext(t)
	jaz := startJaz(t, ctx, store, root, map[string]string{})
	thread, err := jaz.manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: acp.AgentCodex, Slug: "durable-question", Directory: root})
	if err != nil {
		t.Fatal(err)
	}
	jaz.release()
	if _, err := jaz.manager.Send(ctx, acp.SendRequest{Session: thread.SessionID, Message: "Choose an approach", FromUser: true}); err != nil {
		t.Fatal(err)
	}
	jaz.idle(thread.SessionID)
	events := jaz.events.Subscribe(ctx, thread.SessionID)
	call, cancel := context.WithCancel(ctx)
	done := make(chan acp.AskUserOutput, 1)
	go func() {
		out, _ := jaz.manager.AskUser(call, thread.SessionID, acp.AskUserInput{Questions: []acp.UserQuestion{{ID: "choice", Question: "Which approach?", Options: []string{"Phased", "All at once"}}}})
		done <- out
	}()
	var question sessionevents.ACPPermission
	for question.ID == "" {
		select {
		case event := <-events:
			if event.Type == "permission_request" {
				question = *event.Permission
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	cancel()
	if out := <-done; !out.Cancelled {
		t.Fatalf("detached tool = %+v", out)
	}
	for range 300 {
		if err := store.AppendSessionEvents(thread.SessionID, sessionevents.Event{Type: "text", Content: "Later activity"}); err != nil {
			t.Fatal(err)
		}
	}
	jaz = jaz.restart(nil)
	if !jaz.manager.AwaitsAnswer(thread.SessionID) {
		t.Fatal("startup retired the unanswered question")
	}
	answer := acp.InteractiveAnswer{Session: thread.SessionID, RequestID: question.ID, Answers: map[string]acp.InteractiveAnswerValue{"choice": {Answers: []string{"Phased"}}}}
	other, err := store.CreateSession(storage.CreateSession{RuntimeRef: &storage.RuntimeRef{Agent: acp.AgentCodex, Cwd: root}})
	if err != nil {
		t.Fatal(err)
	}
	wrong := answer
	wrong.Session = other.ID
	if err := jaz.manager.AnswerInteractive(ctx, wrong); err == nil {
		t.Fatal("accepted another thread's question")
	}
	db, err := sql.Open("sqlite", filepath.Join(root, "jaz.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TRIGGER reject_question_response BEFORE INSERT ON session_events WHEN NEW.type = 'permission_response' BEGIN SELECT RAISE(ABORT, 'response write unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if err := jaz.manager.AnswerInteractive(ctx, answer); err == nil {
		t.Fatal("ignored a failed resolution write")
	}
	if _, err := db.Exec(`DROP TRIGGER reject_question_response`); err != nil {
		t.Fatal(err)
	}
	jaz.idle(thread.SessionID)
	jaz = jaz.restart(nil)
	var calls sync.WaitGroup
	errors := make(chan error, 2)
	for range 2 {
		calls.Go(func() { errors <- jaz.manager.AnswerInteractive(ctx, answer) })
	}
	calls.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	records, err := store.LoadMessageRecords(thread.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	receipts := 0
	for _, record := range records {
		for _, block := range record.Blocks {
			if record.FromUser && block.Type == storage.BlockTypeQuestionAnswer && block.RequestID == question.ID {
				receipts++
			}
		}
	}
	if receipts != 1 || jaz.promptsContaining("Answers to your questions:") != 1 {
		t.Fatalf("answer was dispatched more than once: records=%+v", records)
	}
	saved, err := store.LoadSessionEvents(thread.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	questions := sessionevents.Questions(saved)
	if len(questions) != 1 || sessionevents.QuestionOpen(questions[0]) || questions[0].Answers["choice"][0] != "Phased" {
		t.Fatalf("saved answer = %+v", questions)
	}
}

func TestExplicitUserActionsCancelLiveAndRestoredQuestions(t *testing.T) {
	for _, action := range []string{"user_input", "user_stop"} {
		for _, restored := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/restored=%v", action, restored), func(t *testing.T) {
				store, root := newTestStore(t)
				ctx := testContext(t)
				jaz := startJaz(t, ctx, store, root, map[string]string{})
				thread, err := jaz.manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: acp.AgentCodex, Slug: "question-cancel", Directory: root})
				if err != nil {
					t.Fatal(err)
				}
				jaz.release()
				if _, err := jaz.manager.Send(ctx, acp.SendRequest{Session: thread.SessionID, Message: "Initial topic", FromUser: true}); err != nil {
					t.Fatal(err)
				}
				jaz.idle(thread.SessionID)
				events := jaz.events.Subscribe(ctx, thread.SessionID)
				call, cancel := context.WithCancel(ctx)
				defer cancel()
				done := make(chan acp.AskUserOutput, 1)
				go func() {
					out, _ := jaz.manager.AskUser(call, thread.SessionID, acp.AskUserInput{Questions: []acp.UserQuestion{{ID: "q", Question: "Choose one"}}})
					done <- out
				}()
				select {
				case <-events:
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
				if restored {
					cancel()
					<-done
					jaz = jaz.restart(nil)
				}
				if action == "user_stop" {
					_, err = jaz.manager.Stop(ctx, thread.SessionID)
				} else {
					_, err = jaz.manager.Input(ctx, acp.SendRequest{Session: thread.SessionID, Message: "Use another approach", FromUser: true})
				}
				if err != nil {
					t.Fatal(err)
				}
				if !restored {
					select {
					case out := <-done:
						if !out.Cancelled {
							t.Fatalf("live tool=%+v", out)
						}
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					}
				}
				if jaz.manager.AwaitsAnswer(thread.SessionID) {
					t.Fatal("superseded question remained open")
				}
				saved, err := store.LoadSessionQuestionEvents(thread.SessionID)
				questions := sessionevents.Questions(saved)
				if err != nil || len(questions) != 1 || questions[0].Status != "cancelled" || questions[0].Origin != action {
					t.Fatalf("resolution=%+v, %v", questions, err)
				}
				if err := jaz.manager.AnswerInteractive(ctx, acp.InteractiveAnswer{Session: thread.SessionID, RequestID: questions[0].ID, Answers: map[string]acp.InteractiveAnswerValue{"q": {Answers: []string{"Old choice"}}}}); err == nil {
					t.Fatal("accepted the superseded card answer")
				}
			})
		}
	}
}
