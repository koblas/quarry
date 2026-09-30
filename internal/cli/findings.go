package cli

import (
	"slices"
	"strings"

	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/platform/homepath"
	"github.com/koblas/quarry/internal/report"
	"github.com/spf13/cobra"
)

// findingsCommand is the word that names findings on the command line and in its usage lines.
const findingsCommand = "findings"

// newFindingsCommand builds findings: the findings sync found, filtered by --status and --type and grouped by type with the fix for each, as JSON when *jsonOut is set.
func newFindingsCommand(newReport ReportFactory, loadConfig ConfigLoader, jsonOut *bool) *cobra.Command {
	var status, typ string
	cmd := &cobra.Command{
		Use:   findingsCommand,
		Short: "List what to clean up in Quicken",
		Long: `List the problems sync found in the Quicken data, as a worklist to fix in
Quicken; quarry never changes the data itself. Each finding names what it
is about and what to change. After you fix them in Quicken, run quarry
sync: findings it no longer finds are marked fixed.

quarry looks for:
  duplicate           two transactions in one account with the same amount,
                      dated within 3 days of each other, unless both are
                      reconciled
  one-sided-transfer  a transfer with no matching transaction in the other
                      account
  unlinked-transfer   two transactions in different accounts of the same
                      currency that look like one transfer (opposite
                      amounts, within 3 days) but are not linked as one
  uncategorized       splits with no category, one finding per payee;
                      quarry cashflow counts them as income or spending
  mixed-categories    a payee whose transactions go back and forth between
                      categories
  payee-variants      payees whose names differ only in case, punctuation,
                      spacing, or store and reference numbers
  similar-categories  categories whose names differ only in case,
                      punctuation, spacing or a plural
  unused-category     a category no transaction uses; check that no
                      scheduled transaction or budget uses it before you
                      delete it

To keep a finding off the list after checking it, add its id to
findings.ignore in ~/Library/Application Support/quarry/config.toml:

  [findings]
  ignore = ["duplicate:txn-4410+txn-4412", "uncategorized:payee-88"]

It stays ignored across syncs; remove the id to list it again. quarry never
writes that file. Without --status, only open findings are listed; --csv
prints one row per item, for a spreadsheet.`,
		Example: `  quarry findings
  quarry findings --type duplicate
  quarry findings --status all --csv > findings.csv`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return UsageError{msg: findingsCommand + " takes no arguments; to ignore a finding add its id to findings.ignore in " +
					findingsConfigShown + "; Run '" + cmd.CommandPath() + " --help' for usage."}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateFindingsFlags(status, typ); err != nil {
				return err
			}

			cfg, err := loadConfig(findingsCommand)
			if err != nil {
				return &runtimeError{err: err}
			}
			printConfigWarnings(cmd, cfg.Warnings)

			srv, err := openReport(cmd, newReport)
			if err != nil {
				return err
			}

			req := report.FindingsRequest{Ignore: cfg.Ignore, Status: finding.Status(status), Type: finding.Type(typ)}
			view := findingsView{status: req.Status, typ: req.Type}
			listing, err := srv.Findings(cmd.Context(), req)
			if err != nil {
				return &runtimeError{err: err}
			}
			unmatched := unmatchedIgnoreWarnings(homepath.Abbreviate(srv.Home(), cfg.Path), listing.Unmatched)
			printConfigWarnings(cmd, unmatched)
			out, err := renderResult(*jsonOut,
				func() ([]byte, error) {
					return renderFindingsJSON(listing, view, append(slices.Clone(cfg.Warnings), unmatched...))
				},
				func() string {
					return renderFindings(listing, view, len(req.Ignore) == 0)
				})
			if err != nil {
				// unreachable: renderResult fails only via marshalDocument, and the findings document holds strings, ints and slices; see marshalDocument.
				return err
			}
			return writeResult(cmd, out)
		},
	}
	cmd.Flags().StringVar(&status, "status", string(finding.StatusOpen),
		"show only findings whose status is `status`: open, ignored, fixed or all")
	cmd.Flags().StringVar(&typ, "type", "",
		"show only findings of this `type`: duplicate, one-sided-transfer, unlinked-transfer, uncategorized, mixed-categories, payee-variants, similar-categories or unused-category")
	return cmd
}

// unmatchedIgnoreWarnings is one line per findings.ignore element that names no finding, each
// naming configShown, the config file as the user sees it.
func unmatchedIgnoreWarnings(configShown string, unmatched []string) []string {
	lines := make([]string, len(unmatched))
	for i, id := range unmatched {
		lines[i] = configShown + ": findings.ignore lists " + config.BasicString(id) +
			", which is not a finding in quarry's store; quarry skips it"
	}
	return lines
}

// validateFindingsFlags refuses a --status that is not open, ignored, fixed or all, and a --type
// that is not a finding type (or empty), as usage errors.
func validateFindingsFlags(status, typ string) error {
	if !slices.Contains([]string{string(finding.StatusOpen), string(finding.StatusIgnored), string(finding.StatusFixed), string(report.FindingsAll)}, status) {
		return UsageError{msg: "--status must be open, ignored, fixed or all"}
	}
	types := finding.Types()
	names := make([]string, len(types))
	for i, t := range types {
		names[i] = string(t)
	}
	if typ != "" && !slices.Contains(names, typ) {
		return UsageError{msg: "--type must be " + strings.Join(names[:len(names)-1], ", ") + " or " + names[len(names)-1]}
	}
	return nil
}
