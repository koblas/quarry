package cli

import (
	"testing"
	"time"
)

// UseZone makes zone the process-local zone for t, so black-box tests can pin local-time rendering.
func UseZone(t *testing.T, zone *time.Location) {
	t.Helper()
	useZone(t, zone)
}
