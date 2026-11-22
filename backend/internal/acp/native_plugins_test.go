package acp_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/coordinator"
	"github.com/wins/jaz/backend/internal/plugins"
	"github.com/wins/jaz/backend/internal/runtimefiles"
	"github.com/wins/jaz/backend/internal/skills"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestNativePluginOwnershipAcrossNewAndRestore(t *testing.T) {
	for _, target := range []struct {
		agent   string
		managed bool
	}{
		{acp.AgentClaude, true},
		{acp.AgentClaude, false},
		{acp.AgentKimi, false},
	} {
		t.Run(target.agent+"/managed="+strconv.FormatBool(target.managed), func(t *testing.T) {
			root := t.TempDir()
			files := map[string]string{
				"plugins/alpha/plugin.json":              `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"alpha"}`,
				"plugins/alpha/skills/native/SKILL.md":   "---\nname: native\ndescription: Native-owned skill\n---\nRead the original skill.\n",
				"work/.agents/skills/query/SKILL.md":     "---\nname: alpha:query\ndescription: Workspace skill with a native name\n---\nKeep the workspace path in Jaz.\n",
				"plugins/alpha/skills/query/SKILL.md":    "---\nname: query\ndescription: Native default skill\n---\nRead the original skill.\n",
				"plugins/beta/.codex-plugin/plugin.json": `{"name":"beta","skills":"./extra"}`,
				"plugins/beta/extra/only/SKILL.md":       "---\nname: only\ndescription: Codex extra skill\n---\nKeep this in the Jaz catalogue.\n",
				"skills/plain/SKILL.md":                  "---\nname: plain\ndescription: Standalone skill\n---\nKeep this in the Jaz catalogue.\n",
			}
			for _, name := range []string{"broken-json", "broken-name", "broken-file"} {
				files["plugins/"+name+"/plugin.json"] = `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"` + name + `"}`
				files["plugins/"+name+"/skills/query/SKILL.md"] = "---\nname: query\ndescription: Skill retained after native loading fails\n---\nUse the original skill path.\n"
			}
			files["plugins/broken-json/.claude-plugin/plugin.json"] = "{invalid"
			files["plugins/broken-name/.claude-plugin/plugin.json"] = `{"name":"different"}`

			for path, body := range files {
				path = filepath.Join(root, filepath.FromSlash(path))
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.MkdirAll(filepath.Join(root, "plugins", "broken-file", ".claude-plugin", "plugin.json"), 0700); err != nil {
				t.Fatal(err)
			}

			store, err := sqlitestore.New(root)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			installed := plugins.New(runtimefiles.New(root))
			workspace := filepath.Join(root, "work")
			requests := filepath.Join(root, "requests.jsonl")
			newManager := func() *acp.Manager {
				return newFakeNamedAgentManagerWithOptions(t, store, root, target.agent, map[string]string{"JAZ_FAKE_ACP_RESUME": "1", "JAZ_FAKE_ACP_REQUEST_LOG": requests}, "", "", func(cfg *acp.Config) {
					cfg.ClaudePlugins = installed.ClaudePlugins
					cfg.Workspace = workspace
					cfg.SystemPrompt = coordinator.NewBuilder(root, workspace, "", nil)
					if target.managed {
						agent := cfg.Agents[target.agent]
						agent.ManagedAdapter = acp.AgentClaude
						agent.Command = ""
						agent.Args = nil
						cfg.Agents[target.agent] = agent
						cfg.Adapters = fakeAdapterResolver{launch: acp.AdapterLaunch{Command: os.Args[0], Args: []string{"-test.run=TestFakeACPAgentProcess"}}}
					}
				})
			}
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()
			first := newManager()
			job, err := first.Spawn(ctx, acp.SpawnRequest{ACPAgent: target.agent, Slug: "native-plugin", Directory: workspace})
			if err != nil {
				first.Close()
				t.Fatal(err)
			}
			sendAndWait(t, ctx, first, job.SessionID, "first")
			first.Close()
			second := newManager()
			t.Cleanup(second.Close)
			sendAndWait(t, ctx, second, job.SessionID, "restored")
			seen := map[string]int{}
			for _, request := range readFakeACPRequests(t, requests) {
				if request.Method != "session/new" && request.Method != "session/resume" {
					continue
				}
				var params struct {
					Meta struct {
						SystemPrompt any `json:"systemPrompt"`
						ClaudeCode   struct {
							Options struct {
								Plugins []map[string]any `json:"plugins"`
							} `json:"options"`
						} `json:"claudeCode"`
					} `json:"_meta"`
				}
				if err := json.Unmarshal(request.Params, &params); err != nil {
					t.Fatal(err)
				}
				prompt, _ := params.Meta.SystemPrompt.(string)
				if target.agent == acp.AgentClaude {
					prompt, _ = params.Meta.SystemPrompt.(map[string]any)["append"].(string)
				}
				if strings.Contains(prompt, "<name>alpha:native</name>") == target.managed || !strings.Contains(prompt, "<name>alpha:query</name>") || !strings.Contains(prompt, filepath.Join(workspace, ".agents", "skills", "query", "SKILL.md")) || !strings.Contains(prompt, "<name>beta:only</name>") || !strings.Contains(prompt, "<name>plain</name>") {
					t.Fatalf("incorrect skill ownership in %s: %s", request.Method, prompt)
				}
				for _, name := range []string{"broken-json", "broken-name", "broken-file"} {
					if !strings.Contains(prompt, "<name>"+name+":query</name>") || !strings.Contains(prompt, filepath.Join(root, "plugins", name, "skills", "query", "SKILL.md")) {
						t.Fatalf("%s lost the skipped plugin's original skill: %s", request.Method, name)
					}
				}

				native := params.Meta.ClaudeCode.Options.Plugins
				if target.managed {
					if len(native) != 2 {
						t.Fatalf("native startup folders in %s = %+v", request.Method, native)
					}
					for i, name := range []string{"alpha", "beta"} {
						expected := map[string]any{"type": "local", "path": filepath.Join(root, "plugins", name), "skipMcpDiscovery": true}
						if !reflect.DeepEqual(native[i], expected) {
							t.Fatalf("native startup option in %s = %+v", request.Method, native[i])
						}
					}
				} else if len(native) != 0 {
					t.Fatalf("unsupported adapter received native startup folders: %+v", native)
				}
				seen[request.Method]++
			}
			if seen["session/new"] != 1 || seen["session/resume"] != 1 {
				t.Fatalf("new/restore requests = %+v", seen)
			}
			catalog, err := skills.LoadForWorkspace(root, workspace)
			if err != nil || len(catalog.Skills) != 7 || !strings.Contains(catalog.Prompt(), filepath.Join(root, "plugins", "alpha", "skills", "native", "SKILL.md")) || !strings.Contains(catalog.Prompt(), filepath.Join(workspace, ".agents", "skills", "query", "SKILL.md")) {
				t.Fatalf("composer catalogue changed: %+v %v", catalog, err)
			}
			listed, err := installed.List()
			if err != nil || len(listed) != 5 {
				t.Fatalf("plugin inventory = %+v %v", listed, err)
			}
			for _, plugin := range listed {
				if strings.HasPrefix(plugin.Name, "broken-") {
					if len(plugin.Warnings) != 1 || !strings.Contains(plugin.Warnings[0], "Native Claude loading skipped:") {
						t.Fatalf("missing native fallback warning on %s: %+v", plugin.Name, plugin.Warnings)
					}
				} else if len(plugin.Warnings) != 0 {
					t.Fatalf("valid plugin %s received another plugin's warning: %+v", plugin.Name, plugin.Warnings)
				}
			}
		})
	}
}

func TestDisabledPluginExcludedFromNativeAndCatalogueFallback(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"plugins/enabled-broken/plugin.json":              `{"name":"enabled-broken"}`,
		"plugins/enabled-broken/skills/query/SKILL.md":    "---\nname: query\ndescription: Enabled fallback skill\n---\n",
		"plugins/disabled-broken/plugin.json":             `{"name":"disabled-broken"}`,
		"plugins/disabled-broken/skills/query/SKILL.md":   "---\nname: query\ndescription: Disabled fallback skill\n---\n",
		"plugins/disabled-native/plugin.json":             `{"name":"disabled-native"}`,
		"plugins/disabled-native/skills/native/SKILL.md":  "---\nname: native\ndescription: Disabled native skill\n---\n",
	}
	for path, body := range files {
		path = filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"enabled-broken", "disabled-broken"} {
		if err := os.MkdirAll(filepath.Join(root, "plugins", name, ".claude-plugin", "plugin.json"), 0700); err != nil {
			t.Fatal(err)
		}
	}

	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for _, id := range []string{"disabled-broken", "disabled-native"} {
		if err := store.SavePluginState(t.Context(), storage.PluginStateRecord{PluginID: id, Enabled: false}); err != nil {
			t.Fatal(err)
		}
	}

	installed := plugins.New(runtimefiles.New(root)).WithStateStore(store)
	workspace := filepath.Join(root, "work")
	if err := os.MkdirAll(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	requests := filepath.Join(root, "requests.jsonl")
	manager := newFakeNamedAgentManagerWithOptions(t, store, root, acp.AgentClaude, map[string]string{"JAZ_FAKE_ACP_REQUEST_LOG": requests}, "", "", func(cfg *acp.Config) {
		cfg.ClaudePlugins = installed.ClaudePlugins
		cfg.Workspace = workspace
		cfg.SystemPrompt = coordinator.NewBuilder(root, workspace, "", nil).WithPluginFilter(func(id string) bool {
			return installed.IsEnabled(context.Background(), id)
		})
		agent := cfg.Agents[acp.AgentClaude]
		agent.ManagedAdapter = acp.AgentClaude
		agent.Command = ""
		agent.Args = nil
		cfg.Agents[acp.AgentClaude] = agent
		cfg.Adapters = fakeAdapterResolver{launch: acp.AdapterLaunch{Command: os.Args[0], Args: []string{"-test.run=TestFakeACPAgentProcess"}}}
	})
	t.Cleanup(manager.Close)

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	job, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: acp.AgentClaude, Slug: "disabled-plugin", Directory: workspace})
	if err != nil {
		t.Fatal(err)
	}
	sendAndWait(t, ctx, manager, job.SessionID, "hello")

	seenNew := 0
	for _, request := range readFakeACPRequests(t, requests) {
		if request.Method != "session/new" {
			continue
		}
		seenNew++
		var params struct {
			Meta struct {
				SystemPrompt map[string]any `json:"systemPrompt"`
				ClaudeCode   struct {
					Options struct {
						Plugins []map[string]any `json:"plugins"`
					} `json:"options"`
				} `json:"claudeCode"`
			} `json:"_meta"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			t.Fatal(err)
		}
		prompt, _ := params.Meta.SystemPrompt["append"].(string)
		if !strings.Contains(prompt, "<name>enabled-broken:query</name>") || !strings.Contains(prompt, filepath.Join(root, "plugins", "enabled-broken", "skills", "query", "SKILL.md")) {
			t.Fatalf("enabled fallback plugin missing original skill path in catalogue: %s", prompt)
		}
		if strings.Contains(prompt, "disabled-broken:query") || strings.Contains(prompt, "disabled-native:native") {
			t.Fatalf("explicitly disabled plugin leaked into catalogue: %s", prompt)
		}
		if len(params.Meta.ClaudeCode.Options.Plugins) != 0 {
			t.Fatalf("disabled or broken plugins passed to native Claude options: %+v", params.Meta.ClaudeCode.Options.Plugins)
		}
	}
	if seenNew != 1 {
		t.Fatalf("session/new count = %d, want 1", seenNew)
	}
}
