package bots

import (
	"slices"
	"testing"

	"github.com/wins/jaz/backend/internal/sessionevents"
)

func TestGroupMentionRouting(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		want []string
	}{
		{"named", "[@Business Opportunist] Please check this.", []string{"b"}},
		{"entity name", "[@R&amp;D] Please check this.", []string{"f"}},
		{"numeric entity name", "[@R&#0038;D] Please check this.", []string{"f"}},
		{"escaped entity", "[@R\\&amp;D] Please check this.", []string{"h"}},
		{"escaped name", "[@Research\\_Lab] Please check this.", []string{"g"}},
		{"linked", "[@Business Opportunist](bot:b) Please check this.", []string{"b"}},
		{"duplicate", "[@Business Opportunist] [@Business Opportunist](bot:b)", []string{"b"}},
		{"explicit ID wins", "[@Business Opportunist](bot:c)", []string{"c"}},
		{"reference link", "[@Business Opportunist][peer]\n\n[peer]: bot:b", []string{"b"}},
		{"unknown", "[@Missing]", nil},
		{"ambiguous", "[@Shared name]", nil},
		{"ambiguous with ID", "[@Shared name](bot:d)", []string{"d"}},
		{"outsider", "[@Outside](bot:outside)", nil},
		{"self", "[@Researcher]", nil},
		{"ordinary prose", "Business Opportunist has an idea.", nil},
		{"code", "`[@Business Opportunist]` and `[@Business Opportunist](bot:b)`", nil},
		{"fenced code", "```text\n[@Business Opportunist](bot:b)\n```", nil},
		{"web link", "[@Business Opportunist](https://example.com)", nil},
		{"image", "![@Business Opportunist](bot:b)", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			world := newFakeWorld()
			world.addBot("a", "Researcher")
			world.addBot("b", "Business Opportunist")
			world.addBot("c", "Planner")
			world.addBot("d", "Shared name")
			world.addBot("e", "Shared name")
			world.addBot("f", "R&D")
			world.addBot("g", "Research_Lab")
			world.addBot("h", "R&amp;D")
			service := newTestService(world)
			group, err := service.CreateGroup("Discovery", []string{"a", "b", "c", "d", "e", "f", "g", "h"})
			if err != nil {
				t.Fatal(err)
			}
			record, err := world.LoadBot(group.ID)
			if err != nil {
				t.Fatal(err)
			}
			threads := map[string]string{}
			for _, id := range record.Members {
				threads[id] = memberThread(t, service, group.ID, id)
			}
			err = service.post(record, sessionevents.RoomMessageEvent{
				Speaker: "bot", BotID: "a", Name: "Researcher", Text: tc.text,
			})
			if err != nil {
				t.Fatal(err)
			}
			world.settle(t)
			for _, id := range record.Members {
				want := 0
				if slices.Contains(tc.want, id) {
					want = 1
				}
				if got := world.promptCount(threads[id]); got != want {
					t.Errorf("bot %s got %d turns, want %d", id, got, want)
				}
			}
		})
	}
}

func TestNamedGroupMentionKeepsItsRecipientAfterRename(t *testing.T) {
	world := newFakeWorld()
	world.addBot("a", "Researcher")
	world.addBot("b", "Business Opportunist")
	service := newTestService(world)
	group, err := service.CreateGroup("Discovery", []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	opportunist := memberThread(t, service, group.ID, "b")
	if err := service.Post(group.ID, "[@Business Opportunist] Please check this.", nil); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool { return world.promptCount(opportunist) == 1 })
	if err := world.UpdateSessionTitle("b", "Strategy"); err != nil {
		t.Fatal(err)
	}
	if err := world.UpdateSessionTitle("a", "Business Opportunist"); err != nil {
		t.Fatal(err)
	}
	events, err := world.LoadSessionEvents(group.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := events[0].RoomMessage.Text; got != "[@Business Opportunist](bot:b) Please check this." {
		t.Fatalf("saved mention lost its recipient: %q", got)
	}
}

func TestUnresolvedUserMentionDoesNotBroadcast(t *testing.T) {
	world := newFakeWorld()
	world.addBot("a", "Research")
	world.addBot("b", "Research")
	service := newTestService(world)
	group, err := service.CreateGroup("Discovery", []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	first, second := memberThread(t, service, group.ID, "a"), memberThread(t, service, group.ID, "b")
	for _, message := range []string{"[@Research] Check this.", "[@Missing] Check this."} {
		if err := service.Post(group.ID, message, nil); err != nil {
			t.Fatal(err)
		}
		world.settle(t)
		if world.promptCount(first) != 0 || world.promptCount(second) != 0 {
			t.Fatal("unresolved mention woke the group")
		}
	}
	if err := service.Post(group.ID, "Hello everyone.", nil); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool { return world.promptCount(first) == 1 && world.promptCount(second) == 1 })
}
