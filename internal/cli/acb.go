package cli

import (
	"time"

	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/platform/homepath"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/spf13/cobra"
)

// acbCurrencyHelp is --currency's help: the backticked word is the placeholder in usage text.
const acbCurrencyHelp = "ACB is in CAD only; any other `currency` is refused"

// acbYearHelp is --year's help: the backticked word is the placeholder in usage text.
const acbYearHelp = "list the sales in tax `year` (YYYY) one by one"

// acbSecurityHelp is --security's help: the backticked word is the placeholder in usage text.
const acbSecurityHelp = "show the full history of the security with this `name`, ticker or id; repeat for more"

// newAcbCommand builds acb: the adjusted cost base of each security and the capital gains realized each tax year.
// It always reads the config, for the account classification, even when --currency is given.
func newAcbCommand(newReport ReportFactory, loadConfig ConfigLoader, now func() time.Time, jsonOut *bool) *cobra.Command {
	var currency currencyFlag
	var yearFlag string
	var securities []string
	cmd := &cobra.Command{
		Use:   "acb",
		Short: "Show adjusted cost base and realized capital gains per tax year, in CAD",
		Long: `Show the adjusted cost base (ACB) of each security and the capital gains
and losses realized each tax year, the way the CRA defines them: average
cost per security, pooled across every account listed in
accounts.non-registered in ~/Library/Application Support/quarry/config.toml.
Registered accounts are left out. A buy adds what it cost, commission
included; a sale removes its share of the ACB, and its gain is the
proceeds less commission less that ACB. On a day with both, purchases
count before sales. Reinvested dividends add their cost; splits change
shares, not ACB. USD trades convert to CAD at the Bank of Canada rate for
their date. Return of capital and reinvested distributions from T3 slips
come from acb.adjustment in the config file.

Amounts are always in CAD, as the CRA requires; reporting.currency does not
apply. A sale's tax year is the year of the date Quicken records, usually
the trade date; the CRA uses the settlement date, so check late-December
sales against your T5008.

Possible superficial losses are marked, not adjusted. Shares added with no
cost count at no cost until you enter it in Quicken (quarry findings
--type shares-without-cost). This is a worksheet to review with your
accountant, not a tax filing.`,
		Example: `  quarry acb
  quarry acb --year 2024
  quarry acb --security XEQT --json`,
		Args: currency.args,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var year int
			if cmd.Flags().Changed("year") {
				parsed, err := report.ParseACBYear(yearFlag, now())
				if err != nil {
					return UsageError{msg: err.Error()}
				}
				year = parsed
			}

			cfg, err := readConfig(cmd, loadConfig)
			if err != nil {
				return err
			}

			srv, err := openReport(cmd, newReport)
			if err != nil {
				return err
			}

			acb, err := srv.ACB(cmd.Context(), report.ACBRequest{
				Classification: classificationOf(cfg), Today: report.Today(now()), Year: year, Securities: securities, Adjustments: acbAdjustmentsOf(cfg),
			})
			if err != nil {
				return &runtimeError{err: err}
			}

			// Warnings read the whole report: a cut would drop those of a security it left out.
			cut := acb.Cut()

			return emitReport(cmd, *jsonOut, document.ACBWarnings(acb, homepath.Abbreviate(srv.Home(), cfg.Path)),
				func() ([]byte, error) {
					return renderACBJSON(cut, withConfigWarnings(cfg.WarningsAbsolute, document.ACBWarnings(acb, cfg.Path)))
				},
				func() string { return renderACB(cut) })
		},
	}
	currency.bind(cmd, acbCurrencyHelp)
	cmd.Flags().StringVar(&yearFlag, "year", "", acbYearHelp)
	// StringArray, not StringSlice: a security's name may contain a comma.
	cmd.Flags().StringArrayVar(&securities, "security", nil, acbSecurityHelp)
	return cmd
}

// acbAdjustmentsOf is the adjustments cfg lists, in file order.
func acbAdjustmentsOf(cfg config.Config) []report.ACBAdjustment {
	adjustments := make([]report.ACBAdjustment, 0, len(cfg.Adjustments))
	for _, a := range cfg.Adjustments {
		adjustments = append(adjustments, report.ACBAdjustment{
			SecurityID: a.Security, Date: a.Date, ReturnOfCapital: a.ReturnOfCapital, ReinvestedDistribution: a.ReinvestedDistribution,
		})
	}

	return adjustments
}
