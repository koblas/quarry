// Black-box seam: UseZone lets the cli_test package pin the process-local zone for local-time rendering.
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
