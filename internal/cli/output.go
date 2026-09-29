package cli

import (
	"fmt"

	"github.com/koblas/quarry/internal/report"
	"github.com/spf13/cobra"
)

// openReport builds the report Server for read command cmd; a failure is a runtime error.
func openReport(cmd *cobra.Command, newReport ReportFactory) (*report.Server, error) {
	srv, err := newReport(cmd.Context(), cmd.Name())
	if err != nil {
		return nil, &runtimeError{err: err}
	}
	return srv, nil
}

// renderResult is a read command's result as JSON when asJSON, else as text.
func renderResult(asJSON bool, renderJSON func() ([]byte, error), renderText func() string) ([]byte, error) {
	if !asJSON {
		return []byte(renderText()), nil
	}
	out, err := renderJSON()
	if err != nil {
		// unreachable: every read command's JSON renderer returns marshalDocument, whose own error path is unreachable; see there.
		return nil, &runtimeError{err: err}
	}
	return out, nil
}

// emit writes out to cmd's stdout, then each warning to stderr behind
// warningPrefix, only once the stdout write succeeded.
func emit(cmd *cobra.Command, out []byte, warningPrefix string, warnings []string) error {
	if err := writeResult(cmd, out); err != nil {
		return err
	}
	for _, warning := range warnings {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), warningPrefix+warning)
	}
	return nil
}

// writeResult writes a read command's result to cmd's stdout, refusing a
// failed write as "cannot write the result to stdout: <write error>".
func writeResult(cmd *cobra.Command, out []byte) error {
	if _, err := cmd.OutOrStdout().Write(out); err != nil {
		return &runtimeError{err: fmt.Errorf("cannot write the result to stdout: %w", err)}
	}
	return nil
}
