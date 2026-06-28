package gmail

import "testing"

func TestAttachmentSourceRefRoundTrip(t *testing.T) {
	ref := FormatAttachmentSourceRef("Augustinas Gmail", "m/1", 2)
	if ref != "att:gmail/augustinas-gmail/m%2F1/2" {
		t.Fatalf("ref = %q", ref)
	}
	parsed, ok, err := ParseAttachmentSourceRef(ref)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || parsed.Account != "augustinas-gmail" || parsed.MessageID != "m/1" || parsed.Index != 2 {
		t.Fatalf("parsed = %#v ok=%v", parsed, ok)
	}
}

func TestFormatAttachmentSourceRefRejectsMissingParts(t *testing.T) {
	for _, ref := range []string{
		FormatAttachmentSourceRef("", "m1", 1),
		FormatAttachmentSourceRef("account", "", 1),
		FormatAttachmentSourceRef("account", "m1", 0),
	} {
		if ref != "" {
			t.Fatalf("ref = %q, want empty", ref)
		}
	}
}

func TestParseAttachmentSourceRefIgnoresProviderIDs(t *testing.T) {
	if _, ok, err := ParseAttachmentSourceRef("gmail-provider-id"); err != nil || ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}
