// White-box: reportFlags.bind is unexported, and the help it registers is read back from the flag set.
package cli

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func Test_bind_registers_the_help_each_command_gives_for_its_flags(t *testing.T) {
	var flags reportFlags
	cmd := &cobra.Command{Use: "demo"}

	flags.bind(cmd, reportFlagHelp{since: "from help", until: "to help", account: "who help"})

	assert.Equal(t, "from help", cmd.Flags().Lookup("since").Usage)
	assert.Equal(t, "to help", cmd.Flags().Lookup("until").Usage)
	assert.Equal(t, "who help", cmd.Flags().Lookup("account").Usage)
}
