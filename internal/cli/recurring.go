package cli

import (
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/spf13/cobra"
)

// recurringFlagHelp is the usage text of recurring's --since, --until and --account flags.
var recurringFlagHelp = reportFlagHelp{
	since:   "list series running on or after `date` (YYYY, YYYY-MM or YYYY-MM-DD; default January 1 this year)",
	until:   "list series that started on or before `date` (YYYY, YYYY-MM or YYYY-MM-DD; default today)",
	account: "list only series with a charge in the account with this `name` or id; repeat for more",
}

// newRecurringCommand builds recurring: the charges that repeat on a schedule and were running in the --since/--until period.
func newRecurringCommand(newReport ReportFactory, now func() time.Time, jsonOut *bool) *cobra.Command {
	var flags reportFlags
	cmd := &cobra.Command{
		Use:   "recurring",
		Short: "List charges that repeat every week, month, quarter or year",
		Long: `List charges that repeat on a schedule: the same payee and currency every
week, month, quarter or year, at a steady amount. quarry finds them in all
your history, with the rules of quarry spend: expense splits only, without
transfers, refunds or accounts left out of reports. A transaction counts
once, with all its splits. Payees whose names differ only in store or
reference numbers count as one payee.

A charge that comes off schedule starts the series again. A series has
ended when no charge has come for 14 days (weekly), 45 days (monthly), 120
days (quarterly) or 400 days (yearly). Bills whose amount changes most
times, such as hydro, are not listed; see quarry spend --by payee.

--since and --until choose which series to list: those running at any
time in the period. A series whose first charge falls in the period is
marked new. A price change is a step of more than 5% from one charge to
the next. Per year is the latest amount times the charges in a year, for
active series only.`,
		Example: `  quarry recurring
  quarry recurring --since 2026-09 --until 2026-09 --json
  quarry recurring --since 2000`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolved, err := flags.window(cmd, now())
			if err != nil {
				return err
			}

			srv, err := openReport(cmd, newReport)
			if err != nil {
				return err
			}

			rec, err := srv.Recurring(cmd.Context(), report.RecurringRequest{Window: resolved, Now: now()})
			if err != nil {
				return &runtimeError{err: err}
			}

			return emitReport(cmd, *jsonOut, []string{},
				func() ([]byte, error) { return renderRecurringJSON(rec, []string{}) },
				func() string { return renderRecurring(rec) })
		},
	}
	flags.bind(cmd, recurringFlagHelp)
	return cmd
}
