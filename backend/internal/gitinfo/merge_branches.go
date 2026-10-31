package gitinfo

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

type MergeBranch struct {
	Branch string `json:"branch"`
	Ahead  int    `json:"ahead"`
}

func MergeBranches(ctx context.Context, dir string) ([]MergeBranch, error) {
	_, target, ok := mainCheckout(ctx, dir)
	if !ok || target == "" {
		return nil, fmt.Errorf("the session has no main checkout branch")
	}
	branches, err := worktreeBranches(ctx, dir)
	if err != nil {
		return nil, err
	}
	current := CurrentRevision(ctx, dir).Branch
	sources := make([]MergeBranch, 0)
	for _, branch := range branches {
		if branch == target || branch == current {
			continue
		}
		count, err := git(ctx, dir, "rev-list", "--count", "refs/heads/"+target+"..refs/heads/"+branch)
		if err != nil {
			return nil, err
		}
		ahead, err := strconv.Atoi(count)
		if err != nil {
			return nil, err
		}
		if ahead > 0 {
			sources = append(sources, MergeBranch{Branch: branch, Ahead: ahead})
		}
	}
	slices.SortFunc(sources, func(a, b MergeBranch) int { return strings.Compare(a.Branch, b.Branch) })
	return sources, nil
}

func MergeBranchIntoMain(ctx context.Context, dir, source string) (string, error) {
	root, target, ok := mainCheckout(ctx, dir)
	if !ok || target == "" {
		return "", fmt.Errorf("the session has no main checkout branch")
	}
	branches, err := worktreeBranches(ctx, dir)
	if err != nil {
		return "", err
	}
	if source == target || !slices.Contains(branches, source) {
		return "", fmt.Errorf("source must be a linked worktree branch in this repository")
	}
	return mergeBranch(ctx, root, target, source)
}

func worktreeBranches(ctx context.Context, dir string) ([]string, error) {
	out, err := git(ctx, dir, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	var branches []string
	for field := range strings.SplitSeq(out, "\x00") {
		if branch, ok := strings.CutPrefix(field, "branch refs/heads/"); ok {
			branches = append(branches, branch)
		}
	}
	return branches, nil
}
