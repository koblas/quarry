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

// ReportedError is a failure the command has already reported on stderr: the process exits 1 and prints nothing more.
type ReportedError struct{}

// unreachable: Execute returns ReportedError as is and the process entry point tests for it without reading its text.
func (ReportedError) Error() string { return "already reported" }
