package sessionevents

import "testing"

func TestNormalizeProgressEntryContentRejectsBareMarkdownHeading(t *testing.T) {
	if content, ok := NormalizeProgressEntryContent("#"); ok {
		t.Fatalf("accepted content %q", content)
	}
}
