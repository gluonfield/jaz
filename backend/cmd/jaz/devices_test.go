package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/deviceauth"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestParseDevicesArgs(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want devicesArgs
	}{
		{name: "list default", want: devicesArgs{Action: ""}},
		{name: "list explicit", in: []string{"list"}, want: devicesArgs{Action: "list"}},
		{name: "root before action", in: []string{"--root", "/var/lib/jaz", "approve", "pair_1"}, want: devicesArgs{Root: "/var/lib/jaz", Action: "approve", Ref: "pair_1"}},
		{name: "root after action", in: []string{"approve", "--root=/var/lib/jaz", "pair_1"}, want: devicesArgs{Root: "/var/lib/jaz", Action: "approve", Ref: "pair_1"}},
		{name: "help", in: []string{"--help"}, want: devicesArgs{Help: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDevicesArgs(tt.in)
			if err != nil {
				t.Fatalf("parseDevicesArgs: %v", err)
			}
			if got != tt.want {
				t.Fatalf("args = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestRunDevicesListAndApprove(t *testing.T) {
	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	service := deviceauth.New(store)
	if _, err := service.Register(deviceauth.ClientInfo{Name: "Owner", Kind: "desktop"}); err != nil {
		t.Fatalf("register owner: %v", err)
	}
	pairing, _, err := service.CreatePairing(deviceauth.ClientInfo{Name: "New desktop", Kind: "desktop"})
	if err != nil {
		t.Fatalf("create pairing: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	var listed bytes.Buffer
	if err := runDevices([]string{"--root", root}, &listed); err != nil {
		t.Fatalf("list devices: %v", err)
	}
	out := listed.String()
	for _, want := range []string{"DEVICES", "PAIRING REQUESTS", pairing.ID, "New desktop", "pending"} {
		if !strings.Contains(out, want) {
			t.Fatalf("list output missing %q:\n%s", want, out)
		}
	}

	var approved bytes.Buffer
	if err := runDevices([]string{"approve", "--root", root, pairing.DeviceID}, &approved); err != nil {
		t.Fatalf("approve device: %v", err)
	}
	if !strings.Contains(approved.String(), "approved "+pairing.ID) {
		t.Fatalf("approve output = %q", approved.String())
	}

	store, err = sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	devices, pairings, err := deviceauth.New(store).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(pairings) != 1 || pairings[0].Status != "approved" {
		t.Fatalf("pairings = %#v", pairings)
	}
	var found bool
	for _, device := range devices {
		if device.ID == pairing.DeviceID && device.Status == "approved" {
			found = true
		}
	}
	if !found {
		t.Fatalf("approved device not found: %#v", devices)
	}
}
