package gitinfo

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

type MergeBranch struct {
	Revision
	Ahead int `json:"ahead"`
}

type MergeBranchesView struct {
	Target   string        `json:"target"`
	Branches []MergeBranch `json:"branches"`
}

func MergeBranches(ctx context.Context, dir string) (MergeBranchesView, error) {
	_, target, _ := mainCheckout(ctx, dir)
	if target == "" {
		return MergeBranchesView{}, fmt.Errorf("the session has no main checkout branch")
	}
	branches, err := worktreeBranches(ctx, dir)
	if err != nil {
		return MergeBranchesView{}, err
	}
	current := CurrentRevision(ctx, dir).Branch
	sources := make([]MergeBranch, 0)
	for _, branch := range branches {
		if branch.Branch == target || branch.Branch == current {
			continue
		}
		count, err := git(ctx, dir, "rev-list", "--count", "refs/heads/"+target+".."+branch.Head)
		if err != nil {
			return MergeBranchesView{}, err
		}
		ahead, err := strconv.Atoi(count)
		if err != nil {
			return MergeBranchesView{}, err
		}
		if ahead > 0 {
			sources = append(sources, MergeBranch{Revision: branch, Ahead: ahead})
		}
	}
	slices.SortFunc(sources, func(a, b MergeBranch) int { return strings.Compare(a.Branch, b.Branch) })
	return MergeBranchesView{Target: target, Branches: sources}, nil
}

func MergeBranchIntoMain(ctx context.Context, dir string, source Revision, target string) error {
	root, current, _ := mainCheckout(ctx, dir)
	if target == "" || current != target {
		return fmt.Errorf("merge destination changed or is unavailable; refresh worktree branches")
	}
	branches, err := worktreeBranches(ctx, dir)
	if err != nil {
		return err
	}
	if source.Branch == target || source.Head == "" || !slices.Contains(branches, source) {
		return fmt.Errorf("source changed or is not a linked worktree branch; refresh worktree branches")
	}
	_, err = mergeBranch(ctx, root, target, source.Head)
	return err
}

func worktreeBranches(ctx context.Context, dir string) ([]Revision, error) {
	out, err := git(ctx, dir, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	var branches []Revision
	var revision Revision
	for field := range strings.SplitSeq(out, "\x00") {
		if head, ok := strings.CutPrefix(field, "HEAD "); ok {
			revision.Head = head
		}
		if branch, ok := strings.CutPrefix(field, "branch refs/heads/"); ok {
			revision.Branch = branch
			branches = append(branches, revision)
		}
	}
	return branches, nil
}
