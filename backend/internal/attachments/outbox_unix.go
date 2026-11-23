//go:build unix

package attachments

import "golang.org/x/sys/unix"

// Opening a replaced outbox entry must not follow a link or wait on a pipe.
const outboxReadFlags = unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_NONBLOCK
