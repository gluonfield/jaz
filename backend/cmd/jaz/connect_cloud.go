package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/machinebridge"
)

type connectCloudConfig struct {
	URL       string
	Token     string
	DeviceID  string
	Name      string
	Grants    []machinebridge.FolderGrant
	AllowExec bool
	Help      bool
}

type grantListFlag struct {
	grants []machinebridge.FolderGrant
}

func (f *grantListFlag) String() string {
	return ""
}

func (f *grantListFlag) Set(value string) error {
	grant, err := machinebridge.ParseGrantSpec(value)
	if err != nil {
		return err
	}
	f.grants = append(f.grants, grant)
	return nil
}

func runConnectCloud(args []string, out io.Writer) error {
	cfg, err := parseConnectCloudArgs(args, os.Getenv)
	if err != nil {
		return err
	}
	if cfg.Help {
		connectCloudUsage(out)
		return nil
	}
	var execLauncher machinebridge.ExecLauncher
	if cfg.AllowExec {
		guard, err := machinebridge.NewPolicyGuard(cfg.Grants)
		if err != nil {
			return err
		}
		execLauncher, err = machinebridge.NewCodexExecLauncher(machinebridge.CodexExecConfig{Guard: guard})
		if err != nil {
			return err
		}
	}
	client, err := machinebridge.NewClient(machinebridge.ClientConfig{
		CloudURL:    cfg.URL,
		Token:       cfg.Token,
		DeviceID:    cfg.DeviceID,
		DisplayName: cfg.Name,
		AppVersion:  version,
		Capabilities: machinebridge.Capabilities{
			FileStaging: true,
			LocalExec:   cfg.AllowExec,
		},
		FolderGrants: cfg.Grants,
		ExecLauncher: execLauncher,
		OnConnect: func(caps machinebridge.Capabilities) {
			log.Info("connected to jaz cloud", "device_id", cfg.DeviceID, "file_staging", caps.FileStaging, "local_exec", caps.LocalExec, "max_stage_bytes", caps.MaxStageBytes)
		},
		OnDisconnect: func(err error) {
			log.Warn("disconnected from jaz cloud; reconnecting", "device_id", cfg.DeviceID, "error", err)
		},
	})
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err = client.Run(ctx)
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func parseConnectCloudArgs(args []string, getenv func(string) string) (connectCloudConfig, error) {
	fs := flag.NewFlagSet("connect-cloud", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var cfg connectCloudConfig
	var grants grantListFlag
	fs.StringVar(&cfg.URL, "url", strings.TrimSpace(getenv("JAZ_CLOUD_URL")), "Jaz Cloud base URL")
	fs.StringVar(&cfg.Token, "token", strings.TrimSpace(getenv("JAZ_CLOUD_TOKEN")), "Machine authentication bearer token")
	fs.StringVar(&cfg.DeviceID, "device-id", "", "Paired device ID")
	fs.StringVar(&cfg.Name, "name", "", "Display name for this machine")
	fs.Var(&grants, "grant", "Folder grant in id=mode:path form (repeatable)")
	fs.BoolVar(&cfg.AllowExec, "allow-exec", false, "Advertise local execution capability")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return connectCloudConfig{Help: true}, nil
		}
		return connectCloudConfig{}, err
	}
	if fs.NArg() > 0 {
		return connectCloudConfig{}, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	cfg.URL = strings.TrimSpace(cfg.URL)
	cfg.Token = strings.TrimSpace(cfg.Token)
	cfg.DeviceID = strings.TrimSpace(cfg.DeviceID)
	cfg.Name = strings.TrimSpace(cfg.Name)
	cfg.Grants = grants.grants

	if cfg.URL == "" {
		return connectCloudConfig{}, errors.New("--url (or JAZ_CLOUD_URL) is required")
	}
	if cfg.Token == "" {
		return connectCloudConfig{}, errors.New("--token (or JAZ_CLOUD_TOKEN) is required")
	}
	if cfg.DeviceID == "" {
		return connectCloudConfig{}, errors.New("--device-id is required")
	}
	if cfg.Name == "" {
		if host, err := os.Hostname(); err == nil && strings.TrimSpace(host) != "" {
			cfg.Name = strings.TrimSpace(host)
		} else {
			cfg.Name = cfg.DeviceID
		}
	}
	return cfg, nil
}

func connectCloudUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: jaz connect-cloud --url <https://...> --token <token> --device-id <id> [--name <display>] [--grant <id=mode:path>...] [--allow-exec]\n\nConnect this machine to Jaz Cloud over outbound WSS.\nReads JAZ_CLOUD_URL and JAZ_CLOUD_TOKEN when flags are omitted.")
}
