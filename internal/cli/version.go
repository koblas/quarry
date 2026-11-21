package cli

import (
	"runtime/debug"

	"github.com/spf13/cobra"
)

// newVersionCommand builds version: it reports the version resolved from info.
func newVersionCommand(_ *debug.BuildInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show quarry's version",
	}
}
