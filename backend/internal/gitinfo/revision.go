package gitinfo

import "context"

type Revision struct {
	Branch string `json:"branch,omitempty"`
	Head   string `json:"head,omitempty"`
}

func CurrentRevision(ctx context.Context, dir string) Revision {
	branch, _ := git(ctx, dir, "symbolic-ref", "--short", "-q", "HEAD")
	head, _ := git(ctx, dir, "rev-parse", "--verify", "--quiet", "HEAD")
	return Revision{Branch: branch, Head: head}
}
