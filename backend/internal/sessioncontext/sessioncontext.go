package sessioncontext

import (
	"context"
	"path/filepath"
	"strings"
)

type sessionIDKey struct{}
type cwdKey struct{}
type collaborationModeKey struct{}
type clientPlatformKey struct{}

const (
	CollaborationModeDefault = "default"
	CollaborationModePlan    = "plan"
)

func WithSessionID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, sessionIDKey{}, id)
}

func SessionID(ctx context.Context) string {
	id, _ := ctx.Value(sessionIDKey{}).(string)
	return id
}

func WithCWD(ctx context.Context, cwd string) context.Context {
	return context.WithValue(ctx, cwdKey{}, cwd)
}

func CWD(ctx context.Context) string {
	cwd, _ := ctx.Value(cwdKey{}).(string)
	return cwd
}

func SessionBase(ctx context.Context, fallback string) (string, error) {
	base := strings.TrimSpace(fallback)
	if base == "" {
		base = "."
	}
	if cwd := strings.TrimSpace(CWD(ctx)); cwd != "" {
		base = cwd
	}
	return filepath.Abs(base)
}

func WithCollaborationMode(ctx context.Context, mode string) context.Context {
	return context.WithValue(ctx, collaborationModeKey{}, mode)
}

func CollaborationMode(ctx context.Context) string {
	mode, _ := ctx.Value(collaborationModeKey{}).(string)
	if mode == "" {
		return CollaborationModeDefault
	}
	return mode
}

func WithClientPlatform(ctx context.Context, platform string) context.Context {
	return context.WithValue(ctx, clientPlatformKey{}, platform)
}

func ClientPlatform(ctx context.Context) string {
	platform, _ := ctx.Value(clientPlatformKey{}).(string)
	if platform == "" {
		return "desktop"
	}
	return platform
}
