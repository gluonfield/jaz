package memorydreamprompt

import (
	"strings"
	"testing"
)

func TestRenderKeepsPolicyAndSlugBoundaries(t *testing.T) {
	got, err := Render(Data{
		Root:            "/tmp/memory",
		RunSlug:         "dreams/runs/2026-06-17",
		ReviewSlug:      "dreams/review/2026-06-17",
		LongTermPolicy:  "profile-level memory only",
		ShortTermPolicy: "active working set only",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Memory root:\n/tmp/memory",
		"Long-term policy:\nprofile-level memory only",
		"Short-term policy:\nactive working set only",
		"Write a run report to dreams/runs/2026-06-17",
		"Leave uncertain candidates in dreams/review/2026-06-17",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered memory dream prompt missing %q:\n%s", want, got)
		}
	}
}
