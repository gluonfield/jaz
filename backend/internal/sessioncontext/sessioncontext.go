package sessioncontext

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/wins/jaz/backend/internal/pathsafe"
)

type sessionIDKey struct{}
type cwdKey struct{}
type collaborationModeKey struct{}

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

func WorkspaceBase(ctx context.Context, workspace string) (string, error) {
	root := strings.TrimSpace(workspace)
	if root == "" {
		root = "."
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	cwd := strings.TrimSpace(CWD(ctx))
	if cwd == "" {
		return rootAbs, nil
	}
	cwdAbs, err := filepath.Abs(cwd)
	if err != nil {
		return "", err
	}
	if !pathsafe.Within(rootAbs, cwdAbs) {
		return "", fmt.Errorf("session cwd escapes workspace: %s", cwd)
	}
	return cwdAbs, nil
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
