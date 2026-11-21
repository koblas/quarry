package cli

import (
	"runtime/debug"

	"github.com/koblas/quarry/internal/version"
	"github.com/spf13/cobra"
)

// newVersionCommand builds version: "quarry <version>" for the version resolved
// from info (which may be nil), or the JSON document when *jsonOut is set.
func newVersionCommand(info *debug.BuildInfo, jsonOut *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Args:  noArgs,
		Short: "Show quarry's version",
		Long: `Print the version of this quarry binary: the release tag, such as v1.0.0,
for a binary installed with go install
github.com/koblas/quarry/cmd/quarry@<version>, or Go's pseudo-version for
one built from a checkout, which names the commit and ends in +dirty when
the checkout had uncommitted changes.

version reads only the binary itself: it needs no store, reads no config
and never looks at Quicken.

--json adds the full commit, whether the checkout had uncommitted changes,
and the Go version quarry was built with.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolved := version.Resolve(info)
			return emitReport(cmd, *jsonOut, nil,
				func() ([]byte, error) { return renderVersionJSON(resolved) },
				func() string { return "quarry " + resolved.Version + "\n" })
		},
	}
}
