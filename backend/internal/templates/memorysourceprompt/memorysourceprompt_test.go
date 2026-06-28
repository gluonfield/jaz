package memorysourceprompt

import (
	"strings"
	"testing"
)

func TestRenderListsEverySource(t *testing.T) {
	got, err := Render(Data{
		Root: "/tmp/memory",
		Sources: []Source{
			{Path: "sources/gmail/a/messages/2026/06/28/x.md", Content: "hello"},
			{Path: "sources/telegram/b/2026/06/28.md", Truncated: true, Content: "world"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got = strings.ReplaceAll(got, "\r\n", "\n")
	for _, want := range []string{
		"Memory root: `/tmp/memory`",
		"- `sources/gmail/a/messages/2026/06/28/x.md`",
		"- `sources/telegram/b/2026/06/28.md` (content truncated in prompt; inspect file directly if needed)",
		"### sources/gmail/a/messages/2026/06/28/x.md",
		"### sources/telegram/b/2026/06/28.md",
		"world",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered source prompt missing %q:\n%s", want, got)
		}
	}
}
