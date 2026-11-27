package codexauthbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"

	"github.com/wins/jaz/backend/internal/acpcredentials"
)

func Enabled(args []string) bool {
	return os.Getenv(brokerURLEnv) != "" && slices.Contains(args, "app-server")
}

func Run(ctx context.Context, args []string) error {
	cmd := exec.CommandContext(ctx, os.Getenv(nativeExecutableEnv), args...)
	cmd.Env = slices.DeleteFunc(os.Environ(), func(entry string) bool {
		name, _, _ := strings.Cut(entry, "=")
		switch name {
		case brokerURLEnv, brokerTokenEnv, nativeExecutableEnv, "CODEX_PATH":
			return true
		default:
			return false
		}
	})
	childIn, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	childOut, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	return relay(ctx, os.Stdin, os.Stdout, childIn, childOut, fetchProfile)
}

type frame struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Error  json.RawMessage `json:"error,omitempty"`
}

func relay(ctx context.Context, adapterIn io.Reader, adapterOut io.Writer, childIn io.WriteCloser, childOut io.Reader, resolve func(context.Context, int64) (acpcredentials.Profile, error)) error {
	adapter := json.NewDecoder(adapterIn)
	native := json.NewDecoder(childOut)
	toAdapter := json.NewEncoder(adapterOut)
	toNative := json.NewEncoder(childIn)

	var initRaw json.RawMessage
	var init frame
	if err := adapter.Decode(&initRaw); err != nil {
		return err
	}
	if json.Unmarshal(initRaw, &init) != nil || init.Method != "initialize" || len(init.ID) == 0 {
		return errors.New("expected Codex initialize request")
	}
	if err := toNative.Encode(initRaw); err != nil {
		return err
	}
	initReply, err := awaitReply(native, toAdapter, init.ID)
	if err != nil {
		return err
	}
	profile, err := resolve(ctx, 0)
	if err != nil {
		return err
	}
	if err := toNative.Encode(map[string]any{
		"id":     "jaz-cloud-login",
		"method": "account/login/start",
		"params": struct {
			Type string `json:"type"`
			acpcredentials.ChatGPTTokens
		}{"chatgptAuthTokens", *profile.ChatGPT},
	}); err != nil {
		return err
	}
	if _, err := awaitReply(native, toAdapter, json.RawMessage(`"jaz-cloud-login"`)); err != nil {
		return err
	}
	if err := toAdapter.Encode(initReply); err != nil {
		return err
	}

	// Native replies and forwarded adapter messages share the child's stdin.
	writer := &lockedWriter{Writer: childIn}
	go func() {
		encoder := json.NewEncoder(writer)
		for {
			var raw json.RawMessage
			if adapter.Decode(&raw) != nil || encoder.Encode(raw) != nil {
				break
			}
		}
		_ = childIn.Close()
	}()
	toNative = json.NewEncoder(writer)
	for {
		var raw json.RawMessage
		if err := native.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		var msg frame
		if err := json.Unmarshal(raw, &msg); err != nil {
			return err
		}
		if msg.Method != "account/chatgptAuthTokens/refresh" {
			if err := toAdapter.Encode(raw); err != nil {
				return err
			}
			continue
		}
		next, err := resolve(ctx, profile.Version)
		if err != nil {
			if err := toNative.Encode(map[string]any{"id": msg.ID, "error": map[string]any{"code": -32000, "message": "Cloud Codex credentials unavailable"}}); err != nil {
				return err
			}
			continue
		}
		if next.ChatGPT.AccountID != profile.ChatGPT.AccountID {
			return errors.New("workspace Codex account changed, restarting app-server")
		}
		if err := toNative.Encode(map[string]any{"id": msg.ID, "result": next.ChatGPT}); err != nil {
			return err
		}
		profile = next
	}
}

func awaitReply(native *json.Decoder, toAdapter *json.Encoder, id json.RawMessage) (json.RawMessage, error) {
	for {
		var raw json.RawMessage
		if err := native.Decode(&raw); err != nil {
			return nil, err
		}
		var msg frame
		if err := json.Unmarshal(raw, &msg); err != nil {
			return nil, err
		}
		if bytes.Equal(msg.ID, id) && msg.Method == "" {
			if len(msg.Error) != 0 && string(msg.Error) != "null" {
				return nil, errors.New("Codex rejected workspace authentication initialization")
			}
			return raw, nil
		}
		if err := toAdapter.Encode(raw); err != nil {
			return nil, err
		}
	}
}

type lockedWriter struct {
	io.Writer
	mu sync.Mutex
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.Writer.Write(p)
}
