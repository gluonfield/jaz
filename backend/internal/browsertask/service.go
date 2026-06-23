package browsertask

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/mcpsession"
	jazsettings "github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/storage"
)

const (
	ToolDo         = "browser_do"
	ToolGet        = "browser_get"
	ToolCheck      = "browser_check"
	defaultKey     = "default"
	Timeout        = 5 * time.Minute
	workerDir      = ".jaz-runtime/browser"
	sourceIDPrefix = "parent:"
)

type Manager interface {
	Spawn(context.Context, acp.SpawnRequest) (acp.SpawnResult, error)
	Send(context.Context, acp.SendRequest) (acp.Job, error)
	Wait(context.Context, acp.WaitRequest) (acp.Job, error)
	Cancel(context.Context, string) (acp.Job, error)
	Status(string) (acp.Job, error)
}

type Service struct {
	Store   storage.SettingsStorage
	Manager Manager
	Catalog acp.AgentCatalog
	Timeout time.Duration
}

type TaskKind string

const (
	KindDo    TaskKind = "do"
	KindGet   TaskKind = "get"
	KindCheck TaskKind = "check"
)

type Request struct {
	Kind       TaskKind
	Task       string
	URL        string
	SessionKey string
	ParentID   string
}

type Result struct {
	Answer     string `json:"answer"`
	SessionID  string `json:"session_id"`
	SessionKey string `json:"session_key"`
	Slug       string `json:"slug"`
	State      string `json:"state"`
}

type Input struct {
	Task       string `json:"task" jsonschema:"browser task to complete"`
	URL        string `json:"url,omitempty" jsonschema:"optional starting URL"`
	SessionKey string `json:"session_key,omitempty" jsonschema:"friendly browser session key such as linkedin, red-rooster, or default"`
}

func New(store storage.SettingsStorage, manager Manager, catalog acp.AgentCatalog) *Service {
	return &Service{Store: store, Manager: manager, Catalog: catalog}
}

func (s *Service) AddMCPTools(server *mcp.Server) {
	tools := mcpTools{service: s}
	mcp.AddTool(server, &mcp.Tool{
		Name:        ToolDo,
		Title:       "Do browser task",
		Description: "Delegate a browser action task to a persistent keyed browser worker. Raw browser context stays in the child session.",
	}, tools.Do)
	mcp.AddTool(server, &mcp.Tool{
		Name:        ToolGet,
		Title:       "Get from browser",
		Description: "Delegate browser information retrieval to a persistent keyed browser worker. Raw browser context stays in the child session.",
	}, tools.Get)
	mcp.AddTool(server, &mcp.Tool{
		Name:        ToolCheck,
		Title:       "Check browser state",
		Description: "Delegate browser verification to a persistent keyed browser worker. Raw browser context stays in the child session.",
	}, tools.Check)
}

func (s *Service) RemoveMCPTools(server *mcp.Server) {
	if server != nil {
		server.RemoveTools(ToolDo, ToolGet, ToolCheck)
	}
}

func (s *Service) MCPToolsEnabled() bool {
	return jazsettings.BrowserEnabled(s.Store)
}

func (s *Service) Run(ctx context.Context, req Request) (Result, error) {
	task := strings.TrimSpace(req.Task)
	if task == "" {
		return Result{}, errors.New("task is required")
	}
	parentID := strings.TrimSpace(req.ParentID)
	if parentID == "" {
		return Result{}, errors.New("browser task requires a parent session")
	}
	defaults, err := loadAgentDefaults(s.Store, s.Catalog)
	if err != nil {
		return Result{}, err
	}
	settings, err := jazsettings.LoadBrowserSettings(s.Store)
	if err != nil {
		return Result{}, err
	}
	if !settings.Enabled {
		return Result{}, errors.New("browser tools are disabled in settings")
	}
	agent := jazsettings.BrowserAgent(settings, defaults)
	if agent == "" {
		return Result{}, errors.New("browser agent is not configured")
	}
	if agent == acp.AgentJaz {
		return Result{}, errors.New("built-in Jaz cannot be used as the browser agent yet")
	}
	sessionKey := normalizeSessionKey(req.SessionKey)
	slug := workerSlug(parentID, sessionKey)
	sessionID, err := s.ensureWorker(ctx, parentID, sessionKey, slug, agent, defaults)
	if err != nil {
		return Result{}, err
	}
	prompt := renderPrompt(req, sessionKey)
	if _, err := s.Manager.Send(ctx, acp.SendRequest{
		Session:    sessionID,
		Message:    prompt,
		Completion: acp.CompletionInline,
	}); err != nil {
		return Result{}, err
	}
	job, err := s.Manager.Wait(ctx, acp.WaitRequest{Session: sessionID, Timeout: s.timeout()})
	if err != nil {
		s.cancelWorker(sessionID)
		return Result{}, err
	}
	if job.State == acp.StateRunning || job.State == acp.StateStarting {
		s.cancelWorker(sessionID)
		return Result{}, fmt.Errorf("browser task timed out after %s", s.timeout())
	}
	if job.State != acp.StateIdle {
		if strings.TrimSpace(job.Error) != "" {
			return Result{}, fmt.Errorf("browser task failed: %s", job.Error)
		}
		return Result{}, fmt.Errorf("browser task finished with state %q", job.State)
	}
	answer := strings.TrimSpace(job.Assistant)
	if answer == "" {
		return Result{}, errors.New("browser task returned an empty answer")
	}
	return Result{Answer: answer, SessionID: job.ID, SessionKey: sessionKey, Slug: job.Slug, State: job.State}, nil
}

func (s *Service) ensureWorker(ctx context.Context, parentID, sessionKey, slug, agent string, defaults jazsettings.AgentDefaults) (string, error) {
	if job, err := s.Manager.Status(slug); err == nil {
		if job.State == acp.StateRunning || job.State == acp.StateStarting {
			return "", fmt.Errorf("browser session %q is busy", sessionKey)
		}
		if strings.TrimSpace(job.ID) != "" {
			return job.ID, nil
		}
	}
	spawned, err := s.Manager.Spawn(ctx, acp.SpawnRequest{
		ParentID:        parentID,
		ACPAgent:        agent,
		Slug:            slug,
		Title:           "Browser: " + sessionKey,
		Directory:       workerDir,
		Model:           jazsettings.WorkerAgentModel(agent, defaults),
		ReasoningEffort: jazsettings.WorkerAgentReasoningEffort(agent),
		SourceType:      storage.SourceBrowserTask,
		SourceID:        sourceID(parentID, sessionKey),
		MCPServerPolicy: acp.MCPServerPolicyBrowserWorker,
	})
	if err != nil {
		return "", err
	}
	return spawned.SessionID, nil
}

func loadAgentDefaults(store storage.SettingsStorage, catalog acp.AgentCatalog) (jazsettings.AgentDefaults, error) {
	if len(catalog) == 0 {
		catalog = acp.BuiltinAgents()
	}
	return jazsettings.LoadEffectiveAgentDefaults(store, catalog)
}

func (s *Service) timeout() time.Duration {
	if s.Timeout > 0 {
		return s.Timeout
	}
	return Timeout
}

func (s *Service) cancelWorker(sessionID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _ = s.Manager.Cancel(ctx, sessionID)
}

type mcpTools struct {
	service *Service
}

func (t mcpTools) Do(ctx context.Context, req *mcp.CallToolRequest, input Input) (*mcp.CallToolResult, Result, error) {
	return t.run(ctx, req, KindDo, input)
}

func (t mcpTools) Get(ctx context.Context, req *mcp.CallToolRequest, input Input) (*mcp.CallToolResult, Result, error) {
	return t.run(ctx, req, KindGet, input)
}

func (t mcpTools) Check(ctx context.Context, req *mcp.CallToolRequest, input Input) (*mcp.CallToolResult, Result, error) {
	return t.run(ctx, req, KindCheck, input)
}

func (t mcpTools) run(ctx context.Context, call *mcp.CallToolRequest, kind TaskKind, input Input) (*mcp.CallToolResult, Result, error) {
	out, err := t.service.Run(ctx, Request{
		Kind:       kind,
		Task:       input.Task,
		URL:        input.URL,
		SessionKey: input.SessionKey,
		ParentID:   mcpsession.SessionID(call),
	})
	if err != nil {
		return nil, Result{}, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: out.Answer}}}, out, nil
}

func renderPrompt(req Request, sessionKey string) string {
	var b strings.Builder
	kind := strings.TrimSpace(string(req.Kind))
	if kind == "" {
		kind = string(KindDo)
	}
	b.WriteString("You are Jaz's delegated browser worker. Complete this browser task and return only the compact final answer for the parent agent.\n\n")
	b.WriteString("Rules:\n")
	b.WriteString("- Keep raw browser snapshots, DOM, accessibility trees, screenshots, and step logs inside this child session.\n")
	b.WriteString("- Use browser_get, browser_do, and browser_check first; they return compact page state with stable ref= targets and deterministic waits.\n")
	b.WriteString("- Reuse returned ref= targets for follow-up actions instead of repeatedly inspecting full snapshots.\n")
	b.WriteString("- Use the low-level browser tool only when the high-level tools cannot express the action, or for PDF/raw screenshot escape hatches.\n")
	b.WriteString("- Set visual=true or request screenshots only when image understanding matters.\n")
	b.WriteString("- Reuse the current browser/page state for this persistent session key.\n")
	b.WriteString("- If no browser backend/tool is available, say that directly and name the missing bridge.\n")
	b.WriteString("- Do not store secrets, cookies, tokens, or private page content in notes.\n\n")
	b.WriteString("Session key: ")
	b.WriteString(sessionKey)
	b.WriteString("\nTask kind: ")
	b.WriteString(kind)
	if rawURL := strings.TrimSpace(req.URL); rawURL != "" {
		b.WriteString("\nStarting URL: ")
		b.WriteString(rawURL)
		if host := urlHost(rawURL); host != "" {
			b.WriteString("\nWebsite notes path: ")
			b.WriteString(workerDir)
			b.WriteString("/website-notes/")
			b.WriteString(host)
			b.WriteString(".md")
		}
	}
	b.WriteString("\n\nTask:\n")
	b.WriteString(strings.TrimSpace(req.Task))
	return b.String()
}

func normalizeSessionKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return defaultKey
	}
	if len(out) > 40 {
		out = strings.Trim(out[:40], "-")
	}
	if out == "" {
		return defaultKey
	}
	return out
}

func workerSlug(parentID, sessionKey string) string {
	hash := sha1.Sum([]byte(parentID))
	return "browser-" + sessionKey + "-" + hex.EncodeToString(hash[:])[:10]
}

func sourceID(parentID, sessionKey string) string {
	return sourceIDPrefix + parentID + "/key:" + sessionKey
}

func urlHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host == "" {
		return ""
	}
	return normalizeSessionKey(host)
}
