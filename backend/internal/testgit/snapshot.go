package testgit

import (
	"os/exec"
	"strings"
	"testing"
)

// Fixture commits reuse the checkout's metadata without configuring an identity.
func Snapshot(t testing.TB, dir string) {
	t.Helper()
	run := func(directory, input string, args ...string) (string, error) {
		command := exec.Command("git", args...)
		command.Dir = directory
		command.Stdin = strings.NewReader(input)
		data, err := command.Output()
		return strings.TrimSpace(string(data)), err
	}
	metadata, err := run("", "", "cat-file", "commit", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	tree, err := run(dir, "", "write-tree")
	if err != nil {
		t.Fatal(err)
	}
	headers := []string{"tree " + tree}
	if parent, err := run(dir, "", "rev-parse", "--verify", "HEAD"); err == nil {
		headers = append(headers, "parent "+parent)
	}
	for _, line := range strings.Split(strings.SplitN(metadata, "\n\n", 2)[0], "\n") {
		if strings.HasPrefix(line, "author ") || strings.HasPrefix(line, "committer ") {
			headers = append(headers, line)
		}
	}
	commit, err := run(dir, strings.Join(headers, "\n")+"\n\nFixture snapshot\n", "hash-object", "-t", "commit", "-w", "--stdin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run(dir, "", "update-ref", "HEAD", commit); err != nil {
		t.Fatal(err)
	}
}
