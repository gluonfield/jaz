package acp

import (
	"context"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

// A thread awaits an answer while a question shown in it is held open: its
// own, or a child's surfaced to it, until answered or dropped.
func TestAwaitsAnswerFollowsHeldQuestions(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, Config{}, nil)
	manager.Events = sessionevents.New()
	ids := map[string]string{}
	for _, slug := range []string{"bot", "helper", "quiet-helper", "other"} {
		session, err := store.CreateSession(storage.CreateSession{Slug: slug, Runtime: storage.RuntimeACP})
		if err != nil {
			t.Fatal(err)
		}
		ids[slug] = session.ID
	}
	jobs := map[string]*jobState{
		"bot":          {Job: Job{ID: ids["bot"], State: StateRunning}},
		"helper":       {Job: Job{ID: ids["helper"], State: StateRunning, ParentID: ids["bot"], ParentVisible: true}},
		"quiet-helper": {Job: Job{ID: ids["quiet-helper"], State: StateRunning, ParentID: ids["bot"]}},
		"other":        {Job: Job{ID: ids["other"], State: StateRunning}},
	}
	for slug, job := range jobs {
		manager.jobsByID[ids[slug]] = job
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ask := func(slug, id string) <-chan permissionAnswer {
		answered := make(chan permissionAnswer, 1)
		question := sessionevents.ACPPermission{ID: id, SessionID: ids[slug], Status: "pending", Questions: []sessionevents.ACPQuestion{{ID: "q", Question: "Which one?"}}}
		go func() {
			answered <- manager.awaitPermissionAnswer(ctx, jobs[slug], question, nil)
		}()
		return answered
	}
	awaiting := func() map[string]bool {
		seen := map[string]bool{}
		for slug, id := range ids {
			seen[slug] = manager.AwaitsAnswer(id)
		}
		return seen
	}
	until := func(what string, want map[string]bool) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for {
			got := awaiting()
			same := true
			for slug := range ids {
				same = same && got[slug] == want[slug]
			}
			if same {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: awaiting %v, want %v", what, got, want)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}

	until("nothing asked yet", map[string]bool{})

	answered := ask("helper", "perm-helper")
	until("a helper's surfaced question", map[string]bool{"helper": true, "bot": true})
	if err := manager.AnswerInteractive(ctx, InteractiveAnswer{Session: ids["helper"], RequestID: "perm-helper", Answers: map[string]InteractiveAnswerValue{"q": {Answers: []string{"This one"}}}}); err != nil {
		t.Fatal(err)
	}
	<-answered
	until("an answered question", map[string]bool{})

	ask("quiet-helper", "perm-quiet")
	until("a helper's unsurfaced question", map[string]bool{"quiet-helper": true})
	manager.cancelPendingPermissions(ids["quiet-helper"])
	until("a dropped question", map[string]bool{})
}
