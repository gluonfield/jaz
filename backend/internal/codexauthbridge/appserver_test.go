package codexauthbridge

import (
	"context"

	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/acpcredentials"
)

func TestRelayAuthenticatesBeforeAdapterUsesCodexAndRefreshesRejectedVersion(t *testing.T) {
	adapterInput, adapterWrite := io.Pipe()
	defer adapterWrite.Close()
	adapterRead, adapterOut := io.Pipe()
	defer adapterRead.Close()
	nativeRead, nativeInput := io.Pipe()
	defer nativeRead.Close()
	nativeOutput, nativeWrite := io.Pipe()
	defer nativeWrite.Close()

	resolved := make(chan int64, 3)
	done := make(chan error, 1)
	go func() {
		defer adapterOut.Close()
		done <- relay(t.Context(), adapterInput, adapterOut, nativeInput, nativeOutput, func(_ context.Context, version int64) (acpcredentials.Profile, error) {
			resolved <- version
			account := "account-a"
			if version == 12 {
				account = "account-b"
			}
			return acpcredentials.Profile{Version: max(11, version+1), ChatGPT: &acpcredentials.ChatGPTTokens{
				AccessToken: "provider-token",
				AccountID:   account,
			}}, nil
		})
	}()

	send := func(dst io.Writer, wire string) {
		t.Helper()
		if _, err := io.WriteString(dst, wire+"\n"); err != nil {
			t.Fatal(err)
		}
	}
	read := func(src *json.Decoder) string {
		t.Helper()
		var raw json.RawMessage
		if err := src.Decode(&raw); err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	native, adapter := json.NewDecoder(nativeRead), json.NewDecoder(adapterRead)
	t.Cleanup(func() { _ = adapterInput.Close() })
	send(adapterWrite, `{"id":1,"method":"initialize","params":{"capabilities":{"experimentalApi":true}}}`)
	if got := read(native); !strings.Contains(got, `"method":"initialize"`) {
		t.Fatalf("expected native initialize, got %s", got)
	}
	send(nativeWrite, `{"id":1,"result":{"codexHome":"isolated"}}`)
	if got := read(native); !strings.Contains(got, `"type":"chatgptAuthTokens"`) || !strings.Contains(got, `"chatgptAccountId":"account-a"`) {
		t.Fatalf("expected an external token login, got %s", got)
	}
	send(nativeWrite, `{"id":"jaz-cloud-login","result":{"type":"chatgptAuthTokens"}}`)
	if version := <-resolved; version != 0 {
		t.Fatalf("resolved initial version %d", version)
	}
	if got := read(adapter); got != `{"id":1,"result":{"codexHome":"isolated"}}` {
		t.Fatalf("adapter saw authentication traffic: %s", got)
	}

	send(adapterWrite, `{"id":2,"method":"model/list","params":{}}`)
	if got := read(native); !strings.Contains(got, `"method":"model/list"`) {
		t.Fatalf("lost adapter traffic: %s", got)
	}
	send(nativeWrite, `{"id":5,"method":"account/chatgptAuthTokens/refresh","params":{"reason":"unauthorized"}}`)
	if got := read(native); !strings.Contains(got, `"id":5`) || !strings.Contains(got, `"accessToken":"provider-token"`) {
		t.Fatalf("invalid native refresh reply: %s", got)
	}
	if version := <-resolved; version != 11 {
		t.Fatalf("reported rejected version %d", version)
	}
	send(nativeWrite, `{"id":2,"result":{"data":[]}}`)
	if got := read(adapter); got != `{"id":2,"result":{"data":[]}}` {
		t.Fatalf("adapter saw credential traffic during model discovery: %s", got)
	}
	send(nativeWrite, `{"id":6,"method":"account/chatgptAuthTokens/refresh","params":{"reason":"unauthorized"}}`)
	if err := <-done; err == nil || !strings.Contains(err.Error(), "account changed") {
		t.Fatalf("account changes must stop the app-server: %v", err)
	}
	if version := <-resolved; version != 12 {
		t.Fatalf("second refresh reported rejected version %d", version)
	}
}
