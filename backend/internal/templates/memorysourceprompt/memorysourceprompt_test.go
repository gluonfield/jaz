package memorysourceprompt

import (
	"strings"
	"testing"
)

func TestRenderListsEverySource(t *testing.T) {
	got, err := Render(Data{
		Sources: []Source{
			{Path: "sources/email/gmail/a/messages/2026/06/28/x.md", Content: "hello"},
			{Path: "sources/chat/telegram/b/2026/06/28.md", Truncated: true, Content: "world"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got = strings.ReplaceAll(got, "\r\n", "\n")
	for _, want := range []string{
		"- `sources/email/gmail/a/messages/2026/06/28/x.md`",
		"- `sources/chat/telegram/b/2026/06/28.md` (truncated below; read this path for the full text)",
		"### sources/email/gmail/a/messages/2026/06/28/x.md",
		"### sources/chat/telegram/b/2026/06/28.md",
		"world",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered source prompt missing %q:\n%s", want, got)
		}
	}
}

func TestSystemPromptStatesTheBar(t *testing.T) {
	sys := System()
	for _, want := range []string{"promotion bar", "memory_search", "Do NOT promote", "Do NOT search the filesystem"} {
		if !strings.Contains(sys, want) {
			t.Fatalf("system prompt missing %q", want)
		}
	}
}
