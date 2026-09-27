// White-box: UsageError's Error() is a one-line accessor best pinned
// directly rather than through a full command run.
package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_UsageError_reports_its_message_verbatim(t *testing.T) {
	err := UsageError{msg: "sync takes no arguments; pass the file with --quicken <path>"}

	assert.Equal(t, "sync takes no arguments; pass the file with --quicken <path>", err.Error())
}
