//go:build !unix

package attachments

import "os"

const outboxReadFlags = os.O_RDONLY
