// Package voice defines the speech provider interfaces; implementations live
// in subpackages, mirroring internal/provider's layout.
package voice

import (
	"context"
	"io"
)

type STT interface {
	Transcribe(ctx context.Context, audio io.Reader, filename string) (string, error)
}

// TTS streams encoded audio as the provider generates it.
type TTS interface {
	Speak(ctx context.Context, text string) (io.ReadCloser, error)
	ContentType() string
}
