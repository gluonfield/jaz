package acp

import (
	"testing"
	"time"
)

// SetProviderRetryDelays shortens the waits before a failed turn's
// continuations for one test.
func SetProviderRetryDelays(t testing.TB, delays ...time.Duration) {
	previous := providerRetryDelays
	providerRetryDelays = delays
	t.Cleanup(func() { providerRetryDelays = previous })
}
