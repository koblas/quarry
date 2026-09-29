package cli

import "github.com/spf13/cobra"

// UsageError is a final, exit-2 error message: Execute returns it verbatim,
// never appending cobra's own hint to it.
type UsageError struct {
	msg string
}

// Error returns the message verbatim.
func (e UsageError) Error() string { return e.msg }

// noArgs refuses any positional argument as "<cmd> takes no arguments".
func noArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return UsageError{msg: cmd.Name() + " takes no arguments"}
	}
	return nil
}
