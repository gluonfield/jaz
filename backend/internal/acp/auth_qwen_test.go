package acp

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	modelprovider "github.com/wins/jaz/backend/internal/provider"
)

func TestQwenBuiltinAgentUsesManagedACPAndCodingPlan(t *testing.T) {
	cfg := BuiltinAgents()[AgentQwen]
	if cfg.Command != "" || cfg.ManagedAdapter != "qwen" || !reflect.DeepEqual(cfg.ManagedAdapterArgs, []string{"--acp"}) {
		t.Fatalf("Qwen config = %#v, want managed qwen --acp", cfg)
	}
	if cfg.ModelProvider != modelprovider.ProviderQwenCodingPlan || cfg.Model != modelprovider.DefaultQwenCodingPlanModel {
		t.Fatalf("Qwen defaults = %#v", cfg)
	}
}

func TestProcessEnvPreparesQwenCodingPlanWithoutLeakingAnotherProviderKey(t *testing.T) {
	clearHostEnv(t)
	root := t.TempDir()
	writeACPTestSkill(t, root, "alpha")
	manager := NewManager(nil, Config{
		Root: root,
		Env: map[string]string{
			"JAZ_ACP_QWEN_API_KEY": "sk-sp-subscription",
			"OPENAI_API_KEY":       "must-not-leak",
			"UNUSED_PROVIDER_KEY":  "also-must-not-leak",
		},
		Providers: map[string]modelprovider.ModelProviderConfig{
			"unused": {Type: "openai-compatible", BaseURL: "https://unused.example/v1", APIKeyEnv: "UNUSED_PROVIDER_KEY"},
		},
	}, nil)
	env, err := manager.processEnvPrepared(AgentQwen, BuiltinAgents()[AgentQwen])
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "acp", "qwen")
	want := map[string]string{
		"QWEN_HOME":                   home,
		"QWEN_RUNTIME_DIR":            filepath.Join(home, "runtime"),
		"BAILIAN_CODING_PLAN_API_KEY": "sk-sp-subscription",
		"OPENAI_API_KEY":              "sk-sp-subscription",
		"OPENAI_BASE_URL":             "https://coding-intl.dashscope.aliyuncs.com/v1",
		"OPENAI_MODEL":                modelprovider.DefaultQwenCodingPlanModel,
	}
	for key, value := range want {
		if env[key] != value {
			t.Errorf("%s = %q, want %q", key, env[key], value)
		}
	}
	if _, ok := env["JAZ_ACP_QWEN_API_KEY"]; ok {
		t.Fatal("Jaz-owned source key leaked into Qwen")
	}
	if _, ok := env["UNUSED_PROVIDER_KEY"]; ok {
		t.Fatal("unselected provider key leaked into Qwen")
	}
	if _, err := os.Stat(filepath.Join(home, "skills", "alpha", "SKILL.md")); err != nil {
		t.Fatalf("Qwen skill not installed: %v", err)
	}
}

func TestQwenUsesConfiguredModelStudioProviderKey(t *testing.T) {
	clearHostEnv(t)
	root := t.TempDir()
	cfg := BuiltinAgents()[AgentQwen]
	cfg.ModelProvider = modelprovider.ProviderModelStudio
	cfg.Model = modelprovider.DefaultModelStudioModel
	manager := NewManager(nil, Config{
		Root: root,
		Providers: map[string]modelprovider.ModelProviderConfig{
			modelprovider.ProviderModelStudio: {APIKey: "dashscope-key"},
		},
	}, nil)
	env, err := manager.processEnvPrepared(AgentQwen, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if env["DASHSCOPE_API_KEY"] != "dashscope-key" || env["OPENAI_API_KEY"] != "dashscope-key" {
		t.Fatalf("Qwen provider key binding = %#v", env)
	}
	if env["OPENAI_BASE_URL"] != "https://dashscope-us.aliyuncs.com/compatible-mode/v1" {
		t.Fatalf("Qwen provider URL = %q", env["OPENAI_BASE_URL"])
	}
}

func TestQwenUsesTokenPlanForQwen38MaxPreview(t *testing.T) {
	clearHostEnv(t)
	root := t.TempDir()
	cfg := BuiltinAgents()[AgentQwen]
	cfg.ModelProvider = modelprovider.ProviderQwenTokenPlan
	cfg.Model = modelprovider.DefaultQwenTokenPlanModel
	manager := NewManager(nil, Config{
		Root: root,
		Providers: map[string]modelprovider.ModelProviderConfig{
			modelprovider.ProviderQwenTokenPlan: {APIKey: "token-plan-key"},
		},
	}, nil)
	env, err := manager.processEnvPrepared(AgentQwen, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if env["BAILIAN_TOKEN_PLAN_API_KEY"] != "token-plan-key" || env["OPENAI_API_KEY"] != "token-plan-key" {
		t.Fatalf("Qwen Token Plan key binding = %#v", env)
	}
	if env["OPENAI_BASE_URL"] != "https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1" ||
		env["OPENAI_MODEL"] != modelprovider.DefaultQwenTokenPlanModel {
		t.Fatalf("Qwen Token Plan launch = %#v", env)
	}
}

func TestQwenLaunchCarriesModelAndNonPersistedSystemPrompt(t *testing.T) {
	args := qwenLaunchArgs(AgentQwen, []string{"--acp"}, modelprovider.DefaultModelStudioModel, "Jaz system prompt")
	want := []string{"--acp", "--model", modelprovider.DefaultModelStudioModel, "--append-system-prompt", "Jaz system prompt"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("Qwen args = %#v, want %#v", args, want)
	}
	manager := &Manager{cfg: Config{SystemPrompt: testPrompt("Jaz system prompt")}}
	meta, err := manager.sessionPromptMeta(context.Background(), AgentQwen, "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if meta != nil {
		t.Fatalf("Qwen prompt must not be duplicated into session metadata: %#v", meta)
	}
}

func TestQwenStartErrorDoesNotExposeSystemPrompt(t *testing.T) {
	const prompt = "private Jaz memory and instructions"
	manager := NewManager(nil, Config{}, nil)
	_, _, err := manager.openConn(context.Background(), AgentQwen, AgentConfig{
		Command: filepath.Join(t.TempDir(), "missing-qwen"),
		Args:    []string{"--acp"},
	}, map[string]string{}, "", "", prompt)
	if err == nil {
		t.Fatal("missing Qwen command unexpectedly started")
	}
	if strings.Contains(err.Error(), prompt) || strings.Contains(err.Error(), "--append-system-prompt") {
		t.Fatalf("Qwen start error exposed launch prompt: %v", err)
	}
}
