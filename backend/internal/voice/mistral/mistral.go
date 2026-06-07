// Package mistral implements voice.STT and voice.TTS on Mistral's Voxtral
// audio APIs. Transcription is the OpenAI-compatible multipart shape; speech
// is Mistral-specific: SSE frames carrying base64 audio chunks.
package mistral

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
)

type Config struct {
	APIKey   string
	BaseURL  string
	STTModel string
	TTSModel string
	Voice    string
}

type Client struct {
	cfg  Config
	http *http.Client
}

func New(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.mistral.ai/v1"
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.STTModel == "" {
		cfg.STTModel = "voxtral-mini-latest"
	}
	if cfg.TTSModel == "" {
		cfg.TTSModel = "voxtral-mini-tts-2603"
	}
	if cfg.Voice == "" {
		cfg.Voice = "en_paul_neutral"
	}
	// No client timeout: Speak streams for as long as synthesis runs.
	return &Client{cfg: cfg, http: &http.Client{}}
}

func (c *Client) Transcribe(ctx context.Context, audio io.Reader, filename string) (string, error) {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", filename)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, audio); err != nil {
		return "", err
	}
	if err := form.WriteField("model", c.cfg.STTModel); err != nil {
		return "", err
	}
	if err := form.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/audio/transcriptions", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	req.Header.Set("Content-Type", form.FormDataContentType())

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", apiError("transcribe", resp)
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.Text), nil
}

func (c *Client) ContentType() string { return "audio/mpeg" }

func (c *Client) Speak(ctx context.Context, text string) (io.ReadCloser, error) {
	payload, err := json.Marshal(map[string]any{
		"model":           c.cfg.TTSModel,
		"input":           text,
		"voice_id":        c.cfg.Voice,
		"response_format": "mp3",
		"stream":          true,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/audio/speech", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, apiError("speak", resp)
	}

	pr, pw := io.Pipe()
	go func() {
		defer resp.Body.Close()
		pw.CloseWithError(decodeSpeechStream(resp.Body, pw))
	}()
	return pr, nil
}

// decodeSpeechStream writes the audio carried by an SSE speech stream
// ("speech.audio.delta" frames with base64 audio_data, closed by
// "speech.audio.done") to w. A body starting with '{' is the non-streaming
// JSON fallback.
func decodeSpeechStream(body io.Reader, w io.Writer) error {
	reader := bufio.NewReaderSize(body, 64<<10)

	if first, err := reader.Peek(1); err == nil && first[0] == '{' {
		var out struct {
			AudioData string `json:"audio_data"`
		}
		if err := json.NewDecoder(reader).Decode(&out); err != nil {
			return err
		}
		return writeBase64(w, out.AudioData)
	}

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64<<10), 16<<20)
	for scanner.Scan() {
		data, ok := strings.CutPrefix(scanner.Text(), "data:")
		if !ok {
			continue
		}
		var frame struct {
			Type      string `json:"type"`
			AudioData string `json:"audio_data"`
			Message   string `json:"message"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(data)), &frame); err != nil {
			continue
		}
		switch {
		case frame.AudioData != "":
			if err := writeBase64(w, frame.AudioData); err != nil {
				return err
			}
		case strings.Contains(frame.Type, "error") || frame.Message != "":
			return fmt.Errorf("tts stream error: %s", firstNonEmpty(frame.Message, frame.Type))
		case frame.Type == "speech.audio.done":
			return nil
		}
	}
	return scanner.Err()
}

func writeBase64(w io.Writer, data string) error {
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return fmt.Errorf("decode audio chunk: %w", err)
	}
	_, err = w.Write(raw)
	return err
}

func apiError(op string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	var apiErr struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &apiErr); err == nil && apiErr.Message != "" {
		return fmt.Errorf("mistral %s: %s (HTTP %d)", op, apiErr.Message, resp.StatusCode)
	}
	return fmt.Errorf("mistral %s: HTTP %d", op, resp.StatusCode)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
