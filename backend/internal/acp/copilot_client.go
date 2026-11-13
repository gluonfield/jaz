package acp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/textproto"
	"os/exec"
	"strconv"
	"time"

	"github.com/wins/jaz/backend/internal/processenv"
	"github.com/wins/jaz/backend/internal/subprocess"
)

// Copilot's SDK metadata RPC uses Content-Length framing, unlike its ACP transport.
func copilotCall(ctx context.Context, env map[string]string, method string, result any) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	command, err := executableInPath("copilot", env["PATH"])
	if err != nil {
		return fmt.Errorf("Copilot CLI executable (copilot) not found")
	}
	request, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": struct{}{}})
	if err != nil {
		return err
	}
	command, args := launchCommand(command, []string{"--headless", "--stdio", "--no-auto-update"})
	cmd := exec.CommandContext(ctx, command, args...)
	process := subprocess.New(cmd)
	cmd.Env = processenv.List(env)
	cmd.Stdin = bytes.NewBuffer(append(fmt.Appendf(nil, "Content-Length: %d\r\n\r\n", len(request)), request...))
	var output bytes.Buffer
	cmd.Stdout = &output
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("Copilot %s failed", method)
	}
	defer process.Terminate()
	if err := process.Started(); err != nil {
		_ = process.Terminate()
		_ = cmd.Wait()
		return fmt.Errorf("start Copilot %s process", method)
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("Copilot %s failed", method)
	}
	reader := bufio.NewReader(&output)
	for {
		header, err := textproto.NewReader(reader).ReadMIMEHeader()
		if err != nil {
			return fmt.Errorf("invalid Copilot %s response headers", method)
		}
		length, err := strconv.Atoi(header.Get("Content-Length"))
		if err != nil || length <= 0 || length > output.Len()+reader.Buffered() {
			return fmt.Errorf("invalid Copilot %s response length", method)
		}
		body := make([]byte, length)
		if _, err := io.ReadFull(reader, body); err != nil {
			return fmt.Errorf("incomplete Copilot %s response", method)
		}
		var response struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Code int `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(body, &response); err != nil {
			return fmt.Errorf("invalid Copilot %s response", method)
		}
		if response.ID != 1 {
			continue
		}
		if response.Error != nil {
			return fmt.Errorf("Copilot %s failed (RPC %d)", method, response.Error.Code)
		}
		if err := json.Unmarshal(response.Result, result); err != nil {
			return fmt.Errorf("invalid Copilot %s result", method)
		}
		return nil
	}
}
