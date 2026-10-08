package bots

import (
	"cmp"
	"testing"

	"github.com/wins/jaz/backend/internal/sessionevents"
)

func TestGroupMentionsLinkToTheirBot(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		want string
	}{
		{"named", "[@Business Opportunist] Please check this.", "[@Business Opportunist](bot:b) Please check this."},
		{"entity name", "[@R&amp;D] Please check this.", "[@R&amp;D](bot:f) Please check this."},
		{"numeric entity name", "[@R&#0038;D] Please check this.", "[@R&#0038;D](bot:f) Please check this."},
		{"escaped entity", "[@R\\&amp;D] Please check this.", "[@R\\&amp;D](bot:h) Please check this."},
		{"escaped name", "[@Research\\_Lab] Please check this.", "[@Research\\_Lab](bot:g) Please check this."},
		{"linked", "[@Business Opportunist](bot:b) Please check this.", ""},
		{"duplicate", "[@Business Opportunist] [@Business Opportunist](bot:b)", "[@Business Opportunist](bot:b) [@Business Opportunist](bot:b)"},
		{"explicit ID wins", "[@Business Opportunist](bot:c)", ""},
		{"reference link", "[@Business Opportunist][peer]\n\n[peer]: bot:b", ""},
		{"unknown", "[@Missing]", ""},
		{"ambiguous", "[@Shared name]", ""},
		{"outsider", "[@Outside](bot:outside)", ""},
		{"ordinary prose", "Business Opportunist has an idea.", ""},
		{"code", "`[@Business Opportunist]` and `[@Business Opportunist](bot:b)`", ""},
		{"fenced code", "```text\n[@Business Opportunist]\n```", ""},
		{"web link", "[@Business Opportunist](https://example.com)", ""},
		{"image", "![@Business Opportunist](bot:b)", ""},
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
			if err := service.post(record, sessionevents.RoomMessageEvent{Speaker: "bot", BotID: "a", Name: "Researcher", Text: tc.text}); err != nil {
				t.Fatal(err)
			}
			world.settle(t)
			want := cmp.Or(tc.want, tc.text)
			if got := world.roomMessages(group.ID); len(got) != 1 || got[0] != "Researcher: "+want {
				t.Fatalf("saved %q, want %q", got, want)
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
