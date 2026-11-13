package acp

import (
	"os"
	"path/filepath"
)

func configureCopilotPrompt(root string, env map[string]string, prompt string) (func(), error) {
	if prompt == "" {
		return func() {}, nil
	}
	directory, err := filepath.Abs(filepath.Join(root, "acp"))
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	directory, err = os.MkdirTemp(directory, "copilot-instructions-*")
	if err != nil {
		return nil, err
	}
	cleanup := func() {
		_ = os.RemoveAll(directory)
	}
	if err := os.WriteFile(filepath.Join(directory, "jaz.instructions.md"), []byte(prompt), 0o600); err != nil {
		cleanup()
		return nil, err
	}
	instructions := directory
	if existing := env["COPILOT_CUSTOM_INSTRUCTIONS_DIRS"]; existing != "" {
		instructions = existing + "," + directory
	}
	env["COPILOT_CUSTOM_INSTRUCTIONS_DIRS"] = instructions
	return cleanup, nil
}
