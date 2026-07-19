package server

func knownSessionAction(action string) bool {
	switch action {
	case "messages:stream",
		"attachments",
		"archive",
		"unarchive",
		"pin",
		"unpin",
		"seen",
		"rename",
		"interactive-response",
		"permission",
		"cancel",
		"compact",
		"queue",
		"repo/push",
		"repo/commit",
		"repo/merge",
		"repo/merge-from-main",
		"repo/restore-worktree":
		return true
	default:
		return false
	}
}
