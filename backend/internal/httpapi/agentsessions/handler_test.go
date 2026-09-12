package agentsessions

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/storage"
)

type runtimeStub struct {
	session, id, value string
	err                error
	contexts           []storage.MessageContext
}

func (r *runtimeStub) SetSessionConfig(_ context.Context, session, id, value string) error {
	r.session, r.id, r.value = session, id, value
	return r.err
}

func (r *runtimeStub) StopBackgroundTask(_ context.Context, session, id string) error {
	r.session, r.id = session, id
	return r.err
}

func (r *runtimeStub) Input(_ context.Context, request acp.SteerRequest) (acp.Job, error) {
	r.session, r.value = request.Session, request.Message
	r.contexts = request.Contexts
	return acp.Job{}, r.err
}

func TestInputRoutesToAgentAndReportsUnsupportedSteering(t *testing.T) {
	for _, failure := range []struct {
		err    error
		status int
	}{
		{nil, http.StatusOK},
		{acp.ErrSteeringUnsupported, http.StatusNotImplemented},
		{errors.New("provider disconnected"), http.StatusConflict},
	} {
		runtime := &runtimeStub{err: failure.err}
		handler := NewHandler(runtime)
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"message":"Use Thursday instead.","contexts":[{"type":"voice","id":"call","text":"Voice: This Thursday?"}]}`))
		request.SetPathValue("session", "claude-thread")
		response := httptest.NewRecorder()
		handler.Input(response, request)
		if response.Code != failure.status || runtime.session != "claude-thread" || runtime.value != "Use Thursday instead." {
			t.Fatalf("steering = %d, %#v", response.Code, runtime)
		}
		if len(runtime.contexts) != 1 || runtime.contexts[0].Type != storage.ContextTypeVoice || runtime.contexts[0].Text != "Voice: This Thursday?" {
			t.Fatalf("voice context lost: %#v", runtime.contexts)
		}
	}
}

func TestConfigBoundaryPreservesProviderValue(t *testing.T) {
	for _, test := range []struct {
		body   string
		status int
	}{
		{`{"id":" effort ","value":" low "}`, http.StatusOK},
		{`{"id":"effort","value":""}`, http.StatusOK},
		{`{"id":"effort"}`, http.StatusBadRequest},
		{`{"id":" ","value":"low"}`, http.StatusBadRequest},
		{`{"id":"effort","value":true}`, http.StatusBadRequest},
		{strings.Repeat("x", 65<<10), http.StatusBadRequest},
	} {
		runtime := &runtimeStub{}
		handler := NewHandler(runtime)
		request := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(test.body))
		request.SetPathValue("session", "session-1")
		response := httptest.NewRecorder()
		handler.SetConfig(response, request)
		if response.Code != test.status {
			t.Fatalf("config %q = %d", test.body[:min(len(test.body), 80)], response.Code)
		}
		if test.status == http.StatusOK {
			if runtime.session != "session-1" || runtime.id != "effort" {
				t.Fatalf("config routing = %#v", runtime)
			}
			if strings.Contains(test.body, " low ") && runtime.value != " low " {
				t.Fatalf("provider value changed: %q", runtime.value)
			}
		} else if runtime.session != "" {
			t.Fatal("invalid request reached runtime")
		}
	}
}

func TestRuntimeConflictsAndTaskRouting(t *testing.T) {
	runtime := &runtimeStub{err: errors.New("native task is no longer stoppable")}
	handler := NewHandler(runtime)
	request := httptest.NewRequest(http.MethodPost, "/", nil)
	request.SetPathValue("session", "session-1")
	request.SetPathValue("task", "native/task-2")
	response := httptest.NewRecorder()
	handler.StopTask(response, request)
	if response.Code != http.StatusConflict || runtime.session != "session-1" || runtime.id != "native/task-2" {
		t.Fatalf("stop route = %d, %#v", response.Code, runtime)
	}
	request = httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"id":"effort","value":"low"}`))
	response = httptest.NewRecorder()
	handler.SetConfig(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("config conflict = %d", response.Code)
	}
}
