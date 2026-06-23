package acp

import (
	"reflect"
	"testing"
)

func TestMergeAgentsCommandOverrideReplacesManagedAdapterLaunch(t *testing.T) {
	merged := MergeAgents(BuiltinAgents(), map[string]AgentConfig{
		AgentCodex: {
			Command: "/opt/jaz/codex-acp",
			Args:    []string{"stdio"},
			Token:   "stale-remote-token",
		},
	})
	got, ok := merged.Agent(AgentCodex)
	if !ok {
		t.Fatal("codex missing")
	}
	if got.Command != "/opt/jaz/codex-acp" || !reflect.DeepEqual(got.Args, []string{"stdio"}) {
		t.Fatalf("command launch = %q %#v", got.Command, got.Args)
	}
	if got.ManagedAdapter != "" || got.ManagedAdapterArgs != nil || got.Local || got.URL != "" || got.Token != "" {
		t.Fatalf("mixed launch mode survived: %#v", got)
	}
	if got.Model != "gpt-5.5" || got.ReasoningEffort != "xhigh" {
		t.Fatalf("non-launch defaults lost: %#v", got)
	}
}

func TestMergeAgentsURLOverrideReplacesManagedAdapterLaunch(t *testing.T) {
	merged := MergeAgents(BuiltinAgents(), map[string]AgentConfig{
		AgentCodex: {
			URL:   "http://127.0.0.1:7777/acp",
			Token: "remote-token",
		},
	})
	got, ok := merged.Agent(AgentCodex)
	if !ok {
		t.Fatal("codex missing")
	}
	if got.URL != "http://127.0.0.1:7777/acp" || got.Token != "remote-token" {
		t.Fatalf("remote launch = %#v", got)
	}
	if got.Command != "" || got.Args != nil || got.ManagedAdapter != "" || got.ManagedAdapterArgs != nil || got.Local {
		t.Fatalf("mixed launch mode survived: %#v", got)
	}
}

func TestMergeAgentsManagedAdapterOverrideReplacesCommandLaunch(t *testing.T) {
	merged := MergeAgents(BuiltinAgents(), map[string]AgentConfig{
		AgentGrok: {
			ManagedAdapter:     "grok",
			ManagedAdapterArgs: []string{"--stdio"},
		},
	})
	got, ok := merged.Agent(AgentGrok)
	if !ok {
		t.Fatal("grok missing")
	}
	if got.ManagedAdapter != "grok" || !reflect.DeepEqual(got.ManagedAdapterArgs, []string{"--stdio"}) {
		t.Fatalf("managed launch = %#v", got)
	}
	if got.Command != "" || got.Args != nil || got.URL != "" || got.Token != "" || got.Local {
		t.Fatalf("mixed launch mode survived: %#v", got)
	}
}

func TestMergeAgentsCanonicalizesNewAgentLaunch(t *testing.T) {
	merged := MergeAgents(nil, map[string]AgentConfig{
		"remote-helper": {
			Command:        "ignored-command",
			Args:           []string{"ignored"},
			ManagedAdapter: "ignored-adapter",
			URL:            "http://127.0.0.1:7777/acp",
			Token:          "remote-token",
		},
	})
	got, ok := merged.Agent("remote-helper")
	if !ok {
		t.Fatal("remote helper missing")
	}
	if got.URL != "http://127.0.0.1:7777/acp" || got.Token != "remote-token" {
		t.Fatalf("remote launch = %#v", got)
	}
	if got.Command != "" || got.Args != nil || got.ManagedAdapter != "" || got.ManagedAdapterArgs != nil || got.Local {
		t.Fatalf("mixed launch mode survived: %#v", got)
	}
}
