package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const githubReleasesURL = "https://github.com/gluonfield/jaz/releases"

type updateArgs struct {
	Version string
	Help    bool
}

type updater struct {
	BaseURL    string
	Executable string
	GOOS       string
	GOARCH     string
	Client     *http.Client
}

func runUpdate(args []string, out io.Writer) error {
	parsed, err := parseUpdateArgs(args)
	if err != nil {
		return err
	}
	if parsed.Help {
		updateUsage(out)
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("current executable: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	u := updater{
		BaseURL:    githubReleasesURL,
		Executable: exe,
		GOOS:       runtime.GOOS,
		GOARCH:     runtime.GOARCH,
		Client:     http.DefaultClient,
	}
	if err := u.update(ctx, parsed); err != nil {
		return err
	}
	fmt.Fprintf(out, "updated %s to %s\n", exe, parsed.releaseLabel())
	return nil
}

func parseUpdateArgs(args []string) (updateArgs, error) {
	var parsed updateArgs
	var latest bool
	fs := flag.NewFlagSet("jaz update", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&latest, "latest", false, "install latest release")
	fs.StringVar(&parsed.Version, "version", "", "install release version")
	fs.BoolVar(&parsed.Help, "help", false, "show help")
	fs.BoolVar(&parsed.Help, "h", false, "show help")
	if err := fs.Parse(args); err != nil {
		return updateArgs{}, err
	}
	if parsed.Help {
		return parsed, nil
	}
	if fs.NArg() != 0 {
		return updateArgs{}, fmt.Errorf("unexpected argument: %s", fs.Arg(0))
	}
	if parsed.Version != "" && latest {
		return updateArgs{}, errors.New("use --version or --latest, not both")
	}
	if parsed.Version != "" && !strings.HasPrefix(parsed.Version, "v") {
		parsed.Version = "v" + parsed.Version
	}
	return parsed, nil
}

func (a updateArgs) releaseLabel() string {
	if a.Version != "" {
		return a.Version
	}
	return "latest"
}

func (u updater) update(ctx context.Context, args updateArgs) error {
	asset, err := backendAssetName(u.GOOS, u.GOARCH)
	if err != nil {
		return err
	}
	archiveURL := u.assetURL(args, asset)
	sumURL := archiveURL + ".sha256"
	want, err := u.downloadChecksum(ctx, sumURL)
	if err != nil {
		return err
	}
	archive, err := u.download(ctx, archiveURL)
	if err != nil {
		return err
	}
	if got := sha256.Sum256(archive); hex.EncodeToString(got[:]) != want {
		return fmt.Errorf("checksum mismatch for %s", asset)
	}
	binary, mode, err := extractBackendBinary(archive)
	if err != nil {
		return err
	}
	return replaceExecutable(u.Executable, binary, mode)
}

func backendAssetName(goos, goarch string) (string, error) {
	if goos != "linux" && goos != "darwin" {
		return "", fmt.Errorf("backend self-update is unsupported on %s/%s", goos, goarch)
	}
	switch goarch {
	case "amd64", "arm64":
		return fmt.Sprintf("jaz-backend-%s-%s.tar.gz", goos, goarch), nil
	default:
		return "", fmt.Errorf("backend self-update is unsupported on %s/%s", goos, goarch)
	}
}

func (u updater) assetURL(args updateArgs, asset string) string {
	base := strings.TrimRight(u.BaseURL, "/")
	if args.Version != "" {
		return base + "/download/" + args.Version + "/" + asset
	}
	return base + "/latest/download/" + asset
}

func (u updater) downloadChecksum(ctx context.Context, url string) (string, error) {
	body, err := u.download(ctx, url)
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(body))
	if len(fields) == 0 {
		return "", fmt.Errorf("empty checksum: %s", url)
	}
	sum := strings.ToLower(fields[0])
	if len(sum) != sha256.Size*2 {
		return "", fmt.Errorf("invalid checksum in %s", url)
	}
	if _, err := hex.DecodeString(sum); err != nil {
		return "", fmt.Errorf("invalid checksum in %s: %w", url, err)
	}
	return sum, nil
}

func (u updater) download(ctx context.Context, url string) ([]byte, error) {
	client := u.Client
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: HTTP %d", url, res.StatusCode)
	}
	return io.ReadAll(res.Body)
}

func extractBackendBinary(archive []byte) ([]byte, os.FileMode, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, 0, fmt.Errorf("read release archive: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, 0, fmt.Errorf("read release archive: %w", err)
		}
		if filepath.Base(header.Name) != "jaz" {
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return nil, 0, errors.New("release archive jaz entry is not a file")
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, 0, fmt.Errorf("read release binary: %w", err)
		}
		mode := os.FileMode(header.Mode) & 0777
		if mode == 0 {
			mode = 0755
		}
		return data, mode, nil
	}
	return nil, 0, errors.New("release archive does not contain jaz")
}

func replaceExecutable(path string, binary []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".jaz-update-*")
	if err != nil {
		return fmt.Errorf("create replacement beside %s: %w", path, err)
	}
	tmpName := tmp.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(binary); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write replacement: %w", err)
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod replacement: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close replacement: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	renamed = true
	return nil
}

func updateUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: jaz update [--latest|--version vX.Y.Z]\n\nDownload a Jaz backend release from GitHub, verify its sha256, and replace the current server binary.")
}
