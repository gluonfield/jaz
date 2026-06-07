package mistral

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSpeakDecodesSSEStream(t *testing.T) {
	chunk1 := []byte("ID3-first-chunk-")
	chunk2 := []byte("second-chunk")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/audio/speech" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("auth = %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "event: speech.audio.delta\ndata: {\"type\":\"speech.audio.delta\",\"audio_data\":%q}\n\n", base64.StdEncoding.EncodeToString(chunk1))
		fmt.Fprintf(w, "event: speech.audio.delta\ndata: {\"type\":\"speech.audio.delta\",\"audio_data\":%q}\n\n", base64.StdEncoding.EncodeToString(chunk2))
		fmt.Fprint(w, "event: speech.audio.done\ndata: {\"type\":\"speech.audio.done\",\"usage\":{}}\n\n")
	}))
	defer server.Close()

	client := New(Config{APIKey: "test-key", BaseURL: server.URL})
	stream, err := client.Speak(context.Background(), "hello")
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	got, err := io.ReadAll(stream)
	if err != nil {
		t.Fatal(err)
	}
	want := append(append([]byte(nil), chunk1...), chunk2...)
	if !bytes.Equal(got, want) {
		t.Fatalf("audio = %q, want %q", got, want)
	}
}

func TestSpeakDecodesJSONFallback(t *testing.T) {
	audio := []byte("whole-mp3-body")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "{\"audio_data\":%q}", base64.StdEncoding.EncodeToString(audio))
	}))
	defer server.Close()

	client := New(Config{APIKey: "k", BaseURL: server.URL})
	stream, err := client.Speak(context.Background(), "hello")
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	got, err := io.ReadAll(stream)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, audio) {
		t.Fatalf("audio = %q", got)
	}
}

func TestSpeakSurfacesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprint(w, `{"object":"error","message":"No model provided for speech."}`)
	}))
	defer server.Close()

	client := New(Config{APIKey: "k", BaseURL: server.URL})
	if _, err := client.Speak(context.Background(), "hello"); err == nil || !strings.Contains(err.Error(), "No model provided") {
		t.Fatalf("err = %v", err)
	}
}

func TestTranscribe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/audio/transcriptions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		if got := r.FormValue("model"); got != "voxtral-mini-latest" {
			t.Errorf("model = %q", got)
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if header.Filename != "clip.webm" {
			t.Errorf("filename = %q", header.Filename)
		}
		fmt.Fprint(w, `{"text":" Hello Jazz. "}`)
	}))
	defer server.Close()

	client := New(Config{APIKey: "k", BaseURL: server.URL})
	text, err := client.Transcribe(context.Background(), strings.NewReader("fake-audio"), "clip.webm")
	if err != nil {
		t.Fatal(err)
	}
	if text != "Hello Jazz." {
		t.Fatalf("text = %q", text)
	}
}
