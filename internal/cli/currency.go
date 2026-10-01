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
