package cli

import (
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/spf13/cobra"
)

// currencyFlagName is the name of the --currency flag.
const currencyFlagName = "currency"

// Help for --currency; the backticked word is the placeholder in usage text.
const (
	reportCurrencyHelp   = "show amounts in currency `code`: CAD, USD, or native for each account's own (default reporting.currency in the config file, else CAD)"
	accountsCurrencyHelp = "add a column with each balance in currency `code`: CAD or USD; native adds none (default reporting.currency in the config file, else CAD)"
)

// reportCurrencyLong is the paragraph of a report's Long help that says which currency its amounts are in.
const reportCurrencyLong = `Amounts are in CAD unless --currency or reporting.currency in
~/Library/Application Support/quarry/config.toml names another currency.
Each split converts at the Bank of Canada rate for its date, or the latest
earlier rate on weekends, holidays and dates after the last stored rate,
and is rounded to the cent before it is added. With --currency native, CAD
and USD are listed separately, never added together. Amounts dated before
the first stored rate stay in their own currency, on rows of their own,
with a warning.`

// currencyFlag is the --currency flag shared by the commands that report in a chosen currency.
type currencyFlag struct {
	code string
}

// bind registers --currency on cmd with help for usage text. The default is empty, because
// the reporting.currency setting decides when the flag is absent.
func (f *currencyFlag) bind(cmd *cobra.Command, help string) {
	cmd.Flags().StringVar(&f.code, currencyFlagName, "", help)
}

// args refuses a positional argument, then a --currency that was given and is not CAD, USD
// or native in any letter case, as a UsageError. It never echoes the value.
func (f *currencyFlag) args(cmd *cobra.Command, args []string) error {
	if err := noArgs(cmd, args); err != nil {
		return err
	}
	if !cmd.Flags().Changed(currencyFlagName) {
		return nil
	}
	if _, ok := money.ParseCurrency(f.code); !ok {
		return UsageError{msg: "--currency must be CAD, USD or native"}
	}
	return nil
}

// resolve is the --currency flag when given, else reporting.currency from the config (loader unread
// with the flag); the second result is the config's absolute warnings, its ~ form printed to stderr.
func (f *currencyFlag) resolve(cmd *cobra.Command, loadConfig ConfigLoader) (money.Currency, []string, error) {
	if cmd.Flags().Changed(currencyFlagName) {
		// The flag was validated in args.
		currency, _ := money.ParseCurrency(f.code)
		return currency, nil, nil
	}
	cfg, err := loadConfig(cmd.Name())
	if err != nil {
		return money.Native, nil, &runtimeError{err: err}
	}
	printConfigWarnings(cmd, cfg.Warnings)
	return cfg.Currency, cfg.WarningsAbsolute, nil
}

// withConfigWarnings is the --json warnings of a read command: the config's, absolute, then the
// command's own. It is never nil.
func withConfigWarnings(config, own []string) []string {
	return append(append([]string{}, config...), own...)
}
