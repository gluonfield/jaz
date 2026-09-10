//go:build browserintegration

package browsercontrol_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/browsercontrol"
	"github.com/wins/jaz/backend/internal/mcpsession"
)

func checkCodexBrowser(ctx context.Context, binary, endpoint, session, url string, client *mcp.ClientSession) error {
	directory := os.Getenv("JAZ_BROWSER_SMOKE_DIR")
	version, err := exec.CommandContext(ctx, binary, "--version").Output()
	if err != nil {
		return err
	}
	fmt.Printf("Native browser compatibility: %s", version)
	config := fmt.Sprintf(`mcp_servers.jaztools={url=%q, http_headers={%q=%q}, required=true, tools.browser_js.approval_mode="approve"}`, endpoint, mcpsession.HeaderName, session)
	command := exec.CommandContext(ctx, binary, "exec", "--ignore-user-config", "--ephemeral", "--json", "--skip-git-repo-check", "--sandbox", "read-only", "-C", directory, "-c", config, "-")
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "OPENAI_API_KEY=") {
			command.Env = append(command.Env, entry)
		}
	}
	command.Stdin = strings.NewReader(fmt.Sprintf(`Verify Jaz's browser MCP integration using only the provided browser tools and the disposable local page %s.
Use browser_js to navigate there, read tab.getAXState(), and define const codexBrowserProbe = 731.
In a separate browser_js call, use the observed numeric AX index to hover and click the button named "Run this step" exactly once. Read the updated AX state and capture a screenshot using tab.getScreenshot().
In another browser_js call, verify codexBrowserProbe still equals 731 and the page visibly reports "Step completed with a trusted browser click.".
Do not use shell, filesystem or other tools. This prompt authorizes the disposable page interaction. Finish with JAZ_BROWSER_CODEX_OK only after the checks succeed.`, url))
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if writeErr := os.WriteFile(filepath.Join(directory, "codex.jsonl"), output, 0o600); writeErr != nil {
		return writeErr
	}
	if err != nil {
		return fmt.Errorf("native Codex check failed: %w: %s", err, stderr.String())
	}
	completed, verified, screenshot, calls := false, false, false, 0
	for line := range bytes.SplitSeq(output, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var event struct {
			Type string `json:"type"`
			Item struct {
				Type   string `json:"type"`
				Server string `json:"server"`
				Tool   string `json:"tool"`
				Status string `json:"status"`
				Text   string `json:"text"`
				Result struct {
					Content []struct {
						Type string `json:"type"`
					} `json:"content"`
				} `json:"result"`
			} `json:"item"`
		}
		if err := json.Unmarshal(line, &event); err != nil {
			return err
		}
		completed = completed || event.Type == "turn.completed"
		if event.Type != "item.completed" {
			continue
		}
		if event.Item.Type == "agent_message" {
			verified = strings.TrimSpace(event.Item.Text) == "JAZ_BROWSER_CODEX_OK"
		}
		if event.Item.Type == "mcp_tool_call" && event.Item.Status == "completed" && event.Item.Server == "jaztools" && event.Item.Tool == browsercontrol.ToolScript {
			calls++
			for _, content := range event.Item.Result.Content {
				screenshot = screenshot || content.Type == "image"
			}
		}
	}
	if !completed || !verified || !screenshot || calls < 3 {
		return fmt.Errorf("native Codex did not complete the browser workflow (completed=%v, verified=%v, screenshot=%v, calls=%d); see %s", completed, verified, screenshot, calls, filepath.Join(directory, "codex.jsonl"))
	}
	result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: browsercontrol.ToolScript, Arguments: map[string]any{"code": `if (codexBrowserProbe !== 731) {
  throw new Error('Native Codex did not preserve its browser binding')
}
const codexPageCheck = await tab.cdp.send('Runtime.evaluate', {expression:'window.clicks === 1 && window.trusted === true && location.search === "?codex=1"',returnByValue:true})
if (codexPageCheck.result.value !== true) {
  throw new Error('Native Codex did not perform the expected trusted page action')
}
nodeRepl.write('Native Codex browser state verified')`}})
	if err != nil {
		return err
	}
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok && !result.IsError && strings.Contains(text.Text, "Native Codex browser state verified") {
			fmt.Printf("Native Codex browser check passed (%d browser_js calls)\n", calls)
			return nil
		}
	}
	return fmt.Errorf("native Codex's final browser state did not pass independent verification")
}
