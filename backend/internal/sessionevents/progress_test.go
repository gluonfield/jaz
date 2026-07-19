package sessionevents

import "testing"

func TestNormalizeProgressEntriesRejectsBareMarkdownHeading(t *testing.T) {
	if entries, ok := NormalizeProgressEntries([]PlanEntry{{Content: "#"}}); ok {
		t.Fatalf("accepted entries %#v", entries)
	}
}
