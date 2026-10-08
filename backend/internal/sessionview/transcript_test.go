package sessionview

import (
	"testing"

	"github.com/wins/jaz/backend/internal/messagepayload"
	"github.com/wins/jaz/backend/internal/sessionevents"
)

func TestEventKeepsGroupFilesServerPathsFromClients(t *testing.T) {
	event := sessionevents.Event{Type: sessionevents.TypeRoomMessage, RoomMessage: &sessionevents.RoomMessageEvent{
		Speaker:     "user",
		Name:        "You",
		Attachments: []messagepayload.Attachment{{ID: "f1", Name: "report.pdf", ServerPath: "/srv/attachments/f1-report.pdf"}},
	}}

	view := Event(event)
	if files := view.RoomMessage.Attachments; len(files) != 1 || files[0].Name != "report.pdf" || files[0].ServerPath != "" {
		t.Fatalf("client sees %+v", files)
	}
	if event.RoomMessage.Attachments[0].ServerPath == "" {
		t.Fatal("the view cleared the server path members are shown")
	}
}
