package emailclean

import (
	"strings"
	"testing"
)

func TestBodyCleansHTMLTrackingAndKeepsMeaningfulLinks(t *testing.T) {
	tracker := "https://tracker.example.com/open/" + strings.Repeat("a", 260) + ".png?utm_source=newsletter"
	got := Body("", `<html><body>
		<style>.x{display:none}</style>
		<p>Quarterly update</p>
		<a href="https://click.mailchimp.com/?url=https%3A%2F%2Fexample.com%2Freport%3Futm_source%3Dnews">Read report</a>
		<a href="https://example.com/photo.jpg?utm=1">Photo</a>
		<a href="https://example.com/inline.png"><img src="cid:image001"></a>
		<img src="`+tracker+`">
		<script>alert("x")</script>
		<p><a href="https://example.com/unsubscribe">Unsubscribe</a></p>
	</body></html>`)

	for _, want := range []string{
		"Quarterly update",
		"Read report (https://example.com/report)",
		"Photo (https://example.com/photo.jpg)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("cleaned body missing %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"tracker.example.com", "mailchimp", "inline.png", "<img", "utm_source", "alert", "Unsubscribe"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("cleaned body contains %q:\n%s", unwanted, got)
		}
	}
}

func TestTextDropsLongTrackingNoise(t *testing.T) {
	longURL := "https://tracker.example.com/open/" + strings.Repeat("a", 1000)
	got := Text("Hello\n" + longURL + "\n" + strings.Repeat("A", 300) + "\nhttps://example.com/path?token=" + strings.Repeat("b", 1000))

	if !strings.Contains(got, "Hello") || !strings.Contains(got, "https://example.com/path") {
		t.Fatalf("cleaned text lost useful content:\n%s", got)
	}
	for _, unwanted := range []string{"tracker.example.com", strings.Repeat("a", 80), strings.Repeat("A", 240), strings.Repeat("b", 80), "token="} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("cleaned text contains %q:\n%s", unwanted, got)
		}
	}
}

func TestBodyCleansURLAnchorText(t *testing.T) {
	got := Body("", `<a href="https://example.com/report?utm_source=newsletter">https://example.com/report?utm_source=newsletter</a>`)

	if !strings.Contains(got, "https://example.com/report") {
		t.Fatalf("cleaned body lost useful URL:\n%s", got)
	}
	if strings.Contains(got, "utm_source") || strings.Contains(got, "?") {
		t.Fatalf("cleaned body leaked tracking URL text:\n%s", got)
	}
}

func TestTextKeepsNormalOpenLinks(t *testing.T) {
	got := Text("Read https://example.com/open-source/report?utm=1 and https://example.com/open/pixel.png")

	if !strings.Contains(got, "https://example.com/open-source/report") {
		t.Fatalf("cleaned text lost normal open link:\n%s", got)
	}
	if strings.Contains(got, "pixel.png") {
		t.Fatalf("cleaned text kept tracking link:\n%s", got)
	}
}
