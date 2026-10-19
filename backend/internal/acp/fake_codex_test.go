package acp_test

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

// fakeCodexLaunch is what Jaz's Codex launch configuration names.
type fakeCodexLaunch struct {
	Model    string          `json:"model"`
	Effort   string          `json:"model_reasoning_effort"`
	Features map[string]bool `json:"features"`
}

func readFakeCodexLaunch(t *testing.T) (fakeCodexLaunch, bool) {
	raw, ok := os.LookupEnv("CODEX_CONFIG")
	if !ok {
		return fakeCodexLaunch{}, false
	}
	if path, found := strings.CutPrefix(raw, "@"); found {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		raw = string(data)
	}
	var launch fakeCodexLaunch
	if err := json.Unmarshal([]byte(raw), &launch); err != nil {
		t.Fatalf("parse Codex config: %v", err)
	}
	return launch, true
}

// fakeDefaultModel is the first model the agent offers.
func fakeDefaultModel() string {
	model, _, _ := strings.Cut(os.Getenv("JAZ_FAKE_ACP_MODELS"), ",")
	if i := strings.LastIndex(model, "/"); i > 0 {
		model = model[:i]
	}
	return strings.TrimSpace(model)
}

// fakeTurn is one turn the fake agent ran.
type fakeTurn struct {
	Model    string
	Effort   string
	Switched bool
}

func readFakeTurns(t *testing.T, path string) []fakeTurn {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var turns []fakeTurn
	for line := range strings.Lines(string(raw)) {
		fields := strings.Split(strings.TrimSuffix(line, "\n"), "\t")
		turns = append(turns, fakeTurn{Model: fields[0], Effort: fields[1], Switched: fields[2] == "true"})
	}
	return turns
}

// logFakeTurn records the model and effort a turn ran on and whether it ran
// on another model than the thread had.
func logFakeTurn(t *testing.T, model, effort string, switched bool) {
	path := os.Getenv("JAZ_FAKE_ACP_TURN_LOG")
	if path == "" {
		return
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := fmt.Fprintf(file, "%s\t%s\t%t\n", model, effort, switched); err != nil {
		t.Fatal(err)
	}
}

// fakeModelEffortOption is the reasoning effort option of the active model,
// when the test describes one.
func fakeModelEffortOption() map[string]any {
	id := strings.TrimSpace(os.Getenv("JAZ_FAKE_ACP_MODEL_CONFIG_EFFORT_ID"))
	if id == "" {
		return nil
	}
	values := cmp.Or(strings.TrimSpace(os.Getenv("JAZ_FAKE_ACP_MODEL_CONFIG_EFFORT_OPTIONS")), os.Getenv("JAZ_FAKE_ACP_EXPECT_EFFORT"))
	options := []map[string]any{}
	for _, value := range strings.Split(values, ",") {
		if value = strings.TrimSpace(value); value != "" {
			options = append(options, map[string]any{"value": value})
		}
	}
	return map[string]any{"id": id, "category": "thought_level", "type": "select", "options": options}
}
