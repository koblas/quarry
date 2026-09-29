package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// writeResult writes a read command's result to cmd's stdout, refusing a
// failed write as "cannot write the result to stdout: <write error>".
func writeResult(cmd *cobra.Command, out []byte) error {
	if _, err := cmd.OutOrStdout().Write(out); err != nil {
		return &runtimeError{err: fmt.Errorf("cannot write the result to stdout: %w", err)}
	}
	return nil
}
