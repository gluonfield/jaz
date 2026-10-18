package loops

import (
	"testing"
	"time"
)

// SetSettleDelays shortens a test's waits between tries at settling open runs.
func SetSettleDelays(t *testing.T, delays ...time.Duration) {
	saved := settleDelays
	settleDelays = delays
	t.Cleanup(func() { settleDelays = saved })
}
