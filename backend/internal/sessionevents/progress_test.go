package sessionevents

import "testing"

func TestNormalizeProgressEntryContentRejectsBareMarkdownHeading(t *testing.T) {
	if content, ok := NormalizeProgressEntryContent("#"); ok {
		t.Fatalf("accepted content %q", content)
	}
}

func TestNormalizePlanDocumentTextAcceptsMarkdownDocument(t *testing.T) {
	want := "# Plan\n\n- Build the page."
	got, ok := NormalizePlanDocumentText([]PlanEntry{{Content: want, Status: "completed"}})
	if !ok || got != want {
		t.Fatalf("plan document = %q, %v", got, ok)
	}
}

func TestNormalizePlanDocumentTextRejectsProgressEntry(t *testing.T) {
	if got, ok := NormalizePlanDocumentText([]PlanEntry{{Content: "Inspect project", Status: "in_progress"}}); ok {
		t.Fatalf("accepted progress entry %q", got)
	}
}
