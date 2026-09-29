package cli

import (
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/spf13/cobra"
)

// newSpendCommand builds spend: this year's spending by category, read at now().
func newSpendCommand(newReport ReportFactory, now func() time.Time, jsonOut *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "spend",
		Short: "Show spending by category, payee, tag or month",
		Long: `Show how much you spent, grouped by category, payee, tag or month, in each
account's own currency: CAD and USD are listed separately, never added
together.

Spending is every split in an expense category, plus uncategorized splits
that take money out. Refunds in an expense category are netted against it,
so a category can come out negative. Transfers between your own accounts,
splits in Quicken's system categories and transactions marked "exclude from
reports" in Quicken are left out. Accounts Quicken leaves out of reports are
left out here too; quarry accounts marks them "not in reports". Closed
accounts are included.

The period runs from --since to --until, both included; a bare year or month
covers all of it (--since 2024 --until 2024 is the whole of 2024). Without
them it is this year up to today, so future-dated transactions are left out
unless --until is later than today.

A split with more than one tag counts under each of them, so with --by tag
the rows can add up to more than the total.`,
		Example: `  quarry spend
  quarry spend --by payee --since 2025-01 --until 2025-03
  quarry spend --since 2024 --until 2024 --json
  quarry spend --account "Visa Infinite" --account Chequing`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			srv, err := openReport(cmd, newReport)
			if err != nil {
				return err
			}

			spending, err := srv.Spend(cmd.Context(), report.SpendRequest{Now: now()})
			if err != nil {
				return &runtimeError{err: err}
			}

			out, err := renderResult(*jsonOut,
				func() ([]byte, error) { return renderSpendingJSON(spending) },
				func() string { return renderSpending(spending) })
			if err != nil {
				return err
			}
			return emit(cmd, out, "quarry: warning: ", []string{})
		},
	}
}
