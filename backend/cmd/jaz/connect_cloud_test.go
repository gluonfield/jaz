package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/machinebridge"
)

func TestParseConnectCloudArgs(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	env := map[string]string{
		"JAZ_CLOUD_URL":   "https://cloud.example.com",
		"JAZ_CLOUD_TOKEN": "env-tok",
	}
	getenv := func(k string) string { return env[k] }

	cfg, err := parseConnectCloudArgs([]string{
		"--device-id", "dev-1",
		"--name", "Work Mac",
		"--grant", "docs=ro:" + dirA,
		"--grant", "proj=rwd:" + dirB,
		"--allow-exec",
	}, getenv)
	if err != nil {
		t.Fatalf("parseConnectCloudArgs failed: %v", err)
	}
	if cfg.URL != "https://cloud.example.com" || cfg.Token != "env-tok" || cfg.DeviceID != "dev-1" || cfg.Name != "Work Mac" || !cfg.AllowExec {
		t.Fatalf("unexpected parsed config: %+v", cfg)
	}
	if len(cfg.Grants) != 2 || cfg.Grants[0].Mode != machinebridge.AccessReadOnly || cfg.Grants[1].Mode != machinebridge.AccessReadWriteDelete {
		t.Fatalf("unexpected grants: %+v", cfg.Grants)
	}

	helpCfg, err := parseConnectCloudArgs([]string{"--help"}, getenv)
	if err != nil || !helpCfg.Help {
		t.Fatalf("help parse = (%+v, %v), want Help=true", helpCfg, err)
	}

	var helpOut bytes.Buffer
	if err := runConnectCloud([]string{"--help"}, &helpOut); err != nil {
		t.Fatalf("runConnectCloud --help failed: %v", err)
	}
	if !strings.Contains(helpOut.String(), "jaz connect-cloud") {
		t.Fatalf("unexpected help output: %q", helpOut.String())
	}

	if _, err := parseConnectCloudArgs([]string{"--device-id", "dev-1"}, func(string) string { return "" }); err == nil {
		t.Fatal("expected error when --url and JAZ_CLOUD_URL are missing")
	}
}
