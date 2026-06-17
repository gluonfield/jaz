package main

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/pressly/goose/v3"
	configloader "github.com/wins/jaz/backend/internal/config"
	"github.com/wins/jaz/backend/internal/deviceauth"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

type devicesArgs struct {
	Root   string
	Action string
	Ref    string
	Help   bool
}

func runDevices(args []string, out io.Writer) error {
	parsed, err := parseDevicesArgs(args)
	if err != nil {
		return err
	}
	if parsed.Help {
		devicesUsage(out)
		return nil
	}
	service, closeStore, err := openDeviceService(parsed.Root)
	if err != nil {
		return err
	}
	defer closeStore()
	switch parsed.Action {
	case "", "list":
		return printDevices(out, service)
	case "approve":
		return approveDevice(out, service, parsed.Ref)
	default:
		return fmt.Errorf("unknown devices command: %s", parsed.Action)
	}
}

func parseDevicesArgs(args []string) (devicesArgs, error) {
	var parsed devicesArgs
	var rest []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case isHelp(arg):
			parsed.Help = true
		case arg == "--root" || arg == "-root":
			i++
			if i >= len(args) || strings.TrimSpace(args[i]) == "" {
				return devicesArgs{}, fmt.Errorf("--root requires a path")
			}
			parsed.Root = args[i]
		case strings.HasPrefix(arg, "--root="):
			parsed.Root = strings.TrimPrefix(arg, "--root=")
		case strings.HasPrefix(arg, "-root="):
			parsed.Root = strings.TrimPrefix(arg, "-root=")
		default:
			rest = append(rest, arg)
		}
	}
	if len(rest) > 0 {
		parsed.Action = rest[0]
	}
	switch parsed.Action {
	case "", "list":
		if len(rest) > 1 {
			return devicesArgs{}, fmt.Errorf("list takes no arguments")
		}
	case "approve":
		if len(rest) != 2 {
			return devicesArgs{}, fmt.Errorf("approve requires a pairing or device id")
		}
		parsed.Ref = rest[1]
	default:
		return devicesArgs{}, fmt.Errorf("unknown devices command: %s", parsed.Action)
	}
	return parsed, nil
}

func openDeviceService(root string) (*deviceauth.Service, func(), error) {
	root = strings.TrimSpace(root)
	if root == "" {
		loaded, err := configloader.Load()
		if err != nil {
			return nil, nil, err
		}
		root = loaded.Jaz.Root
	}
	goose.SetLogger(goose.NopLogger())
	store, err := sqlitestore.New(root)
	if err != nil {
		return nil, nil, err
	}
	return deviceauth.New(store), func() { _ = store.Close() }, nil
}

func printDevices(out io.Writer, service *deviceauth.Service) error {
	devices, pairings, err := service.List()
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "DEVICES")
	fmt.Fprintln(w, "ID\tSTATUS\tKIND\tNAME\tDETAILS\tAPP\tLAST SEEN\tAPPROVED")
	if len(devices) == 0 {
		fmt.Fprintln(w, "-\t-\t-\t-\t-\t-\t-\t-")
	} else {
		for _, device := range devices {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				device.ID,
				device.Status,
				device.Kind,
				device.Name,
				deviceDetails(device),
				emptyDash(device.AppVersion),
				formatDeviceTime(device.LastSeenAt),
				formatDeviceTime(device.ApprovedAt),
			)
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "PAIRING REQUESTS")
	fmt.Fprintln(w, "ID\tSTATUS\tDEVICE\tDEVICE STATUS\tKIND\tNAME\tDETAILS\tCREATED\tEXPIRES")
	if len(pairings) == 0 {
		fmt.Fprintln(w, "-\t-\t-\t-\t-\t-\t-\t-\t-")
	} else {
		for _, pairing := range pairings {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				pairing.ID,
				pairingStatus(pairing),
				pairing.DeviceID,
				pairing.Device.Status,
				pairing.Device.Kind,
				pairing.Device.Name,
				deviceDetails(pairing.Device),
				formatDeviceTime(pairing.CreatedAt),
				formatDeviceTime(pairing.ExpiresAt),
			)
		}
	}
	return w.Flush()
}

func approveDevice(out io.Writer, service *deviceauth.Service, ref string) error {
	pairingID, err := pendingPairingID(service, ref)
	if err != nil {
		return err
	}
	pairing, err := service.ApprovePairing(pairingID)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "approved %s %s %s\n", pairing.ID, pairing.Device.ID, pairing.Device.Name)
	return nil
}

func pendingPairingID(service *deviceauth.Service, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("pairing or device id is required")
	}
	_, pairings, err := service.List()
	if err != nil {
		return "", err
	}
	for _, pairing := range pairings {
		if pairing.ID == ref && pairingStatus(pairing) == storage.PairingStatusPending {
			return pairing.ID, nil
		}
	}
	for _, pairing := range pairings {
		if pairing.DeviceID == ref && pairingStatus(pairing) == storage.PairingStatusPending {
			return pairing.ID, nil
		}
	}
	return "", fmt.Errorf("pending pairing not found: %s", ref)
}

func pairingStatus(pairing storage.DevicePairing) string {
	if pairing.Status == storage.PairingStatusPending && time.Now().UTC().After(pairing.ExpiresAt) {
		return storage.PairingStatusExpired
	}
	return pairing.Status
}

func formatDeviceTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.UTC().Format(time.RFC3339)
}

func emptyDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

func deviceDetails(device storage.Device) string {
	return emptyDash(strings.Join(uniqueDeviceParts(device.Platform, device.Family, device.Model), " / "))
}

func uniqueDeviceParts(parts ...string) []string {
	seen := map[string]bool{}
	var out []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key := strings.ToLower(part)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, part)
	}
	return out
}

func devicesUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: jaz devices [--root path]\n       jaz devices [--root path] approve <pairing-or-device-id>\n\nList connected devices and approve pending device requests.")
}
