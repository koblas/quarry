package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"regexp"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func executeHoldings(t *testing.T, fake fakeReportStore, stdout, stderr io.Writer, args ...string) error {
	t.Helper()
	env := cli.Env{
		LoadConfig: cadConfig,
		Stdout:     stdout, Stderr: stderr,
		Now: func() time.Time { return spendNow },
		NewReport: func(context.Context, string) (*report.Server, error) {
			return report.NewServer(report.WithStore(fake)), nil
		},
	}
	return cli.Execute(t.Context(), append([]string{"holdings"}, args...), env)
}

// brokerageHolding is 1,200 shares of Acme Corp (ACME) priced 31.42 on 2026-09-23: 37,704.00 CAD,
// which is 50.00 in the CAD report and 27.00 in the USD one.
func brokerageHolding() store.Holding {
	return store.Holding{
		AccountID: "acct-1", Account: "Brokerage", SecurityID: "sec-1", Security: new("Acme Corp"), Ticker: new("ACME"),
		Shares: 1_200_000_000, Price: new(int64(31_420_000)), PriceDate: new(time.Date(2026, time.September, 23, 0, 0, 0, 0, time.UTC)),
		Currency: new("CAD"), Value: big.NewInt(3_770_400), ValueCAD: big.NewInt(5_000), ValueUSD: big.NewInt(2_700),
	}
}

// holdingsRow is one table line, each cell as wide as brokerageHolding's widest.
func holdingsRow(account, security, shares, price, pricedOn, currency, value, in string) string {
	return fmt.Sprintf("%-9s  %-16s  %6s  %5s  %-10s  %-8s  %9s  %6s\n", account, security, shares, price, pricedOn, currency, value, in)
}

func Test_holdings_help_says_what_holdings_lists(t *testing.T) {
	const long = `List each security held in a brokerage or retirement account on one day
(--as-of, default today): its share count, the latest price Quicken
recorded on or before that day with that price's date, and its value,
shares times price rounded to the cent. Share counts are the ones quarry
sync checks against Quicken. A holding with no price on or before that day
is listed with no value and left out of the total.

Values are in each security's own currency. A column shows each value in
the reporting currency (--currency, else reporting.currency in the config
file, else CAD) at the Bank of Canada rate for the --as-of day, or the
latest earlier one; --currency native leaves it out and totals each
currency separately.

The total is the value of the securities only. Cash held in investment
accounts is not included, so it is not those accounts' balance.
`
	var stdout, stderr bytes.Buffer

	err := executeHoldings(t, fakeReportStore{}, &stdout, &stderr, "--help")

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), long)
}

func Test_holdings_help_shows_examples(t *testing.T) {
	const examples = `Examples:
  quarry holdings
  quarry holdings --as-of 2025-12-31
  quarry holdings --account RRSP --currency native --json
`
	var stdout, stderr bytes.Buffer

	err := executeHoldings(t, fakeReportStore{}, &stdout, &stderr, "--help")

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), examples)
}

func Test_holdings_help_shows_the_currency_flag(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeHoldings(t, fakeReportStore{}, &stdout, &stderr, "--help")

	require.NoError(t, err)
	assert.Regexp(t, `(?m)--currency code +`+regexp.QuoteMeta(holdingsCurrencyHelp)+`$`, stdout.String())
}

const holdingsCurrencyHelp = "add a column with each value in currency code: CAD or USD; native adds none (default reporting.currency in the config file, else CAD)"

func Test_holdings_is_listed_in_the_root_help_with_its_short_description(t *testing.T) {
	var stdout, stderr bytes.Buffer
	env := cli.Env{Stdout: &stdout, Stderr: &stderr}

	err := cli.Execute(t.Context(), []string{"--help"}, env)

	require.NoError(t, err)
	assert.Regexp(t, `(?m)^  holdings +List the securities held in each account and their value$`, stdout.String())
}

func Test_holdings_reads_the_day_of_the_clock_as_its_as_of_day(t *testing.T) {
	var got store.HoldingsParams
	var stdout, stderr bytes.Buffer

	err := executeHoldings(t, fakeReportStore{gotHoldings: &got}, &stdout, &stderr)

	require.NoError(t, err)
	assert.Equal(t, store.HoldingsParams{AsOf: time.Date(2026, time.September, 29, 0, 0, 0, 0, time.UTC)}, got)
}

func Test_holdings_lists_each_value_in_the_reporting_currency_and_totals_it(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := fakeReportStore{holdings: store.Holdings{Holdings: []store.Holding{brokerageHolding()}}}

	err := executeHoldings(t, fake, &stdout, &stderr)

	require.NoError(t, err)
	assert.Equal(t, "Holdings on 2026-09-29 in all accounts, amounts in CAD; cash not included\n\n"+
		holdingsRow("Account", "Security", "Shares", "Price", "Priced on", "Currency", "Value", "In CAD")+
		holdingsRow("Brokerage", "Acme Corp (ACME)", "1,200", "31.42", "2026-09-23", "CAD", "37,704.00", "50.00")+
		holdingsRow("Total", "", "", "", "", "", "", "50.00"), stdout.String())
	assert.Empty(t, stderr.String())
}

func Test_holdings_currency_flag_picks_the_value_the_column_and_total_show(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := fakeReportStore{holdings: store.Holdings{Holdings: []store.Holding{brokerageHolding()}}}

	err := executeHoldings(t, fake, &stdout, &stderr, "--currency", "USD")

	require.NoError(t, err)
	assert.Equal(t, "Holdings on 2026-09-29 in all accounts, amounts in USD; cash not included\n\n"+
		holdingsRow("Account", "Security", "Shares", "Price", "Priced on", "Currency", "Value", "In USD")+
		holdingsRow("Brokerage", "Acme Corp (ACME)", "1,200", "31.42", "2026-09-23", "CAD", "37,704.00", "27.00")+
		holdingsRow("Total", "", "", "", "", "", "", "27.00"), stdout.String())
}

func Test_holdings_uses_the_config_currency_when_the_flag_is_absent(t *testing.T) {
	var stdout, stderr bytes.Buffer
	env := cli.Env{
		LoadConfig: func(string) (config.Config, error) { return config.Config{Currency: money.USD}, nil },
		Stdout:     &stdout, Stderr: &stderr,
		Now: func() time.Time { return spendNow },
		NewReport: func(context.Context, string) (*report.Server, error) {
			return report.NewServer(report.WithStore(fakeReportStore{holdings: store.Holdings{Holdings: []store.Holding{brokerageHolding()}}})), nil
		},
	}

	err := cli.Execute(t.Context(), []string{"holdings"}, env)

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), ", amounts in USD; cash not included\n")
}

func Test_holdings_prints_the_table_header_and_no_total_when_nothing_is_held(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeHoldings(t, fakeReportStore{}, &stdout, &stderr)

	require.NoError(t, err)
	assert.Equal(t, "Holdings on 2026-09-29 in all accounts, amounts in CAD; cash not included\n\n"+
		"Account  Security  Shares  Price  Priced on  Currency  Value  In CAD\n", stdout.String())
}

func Test_holdings_prints_a_config_warning_on_stderr_and_still_lists_the_holdings(t *testing.T) {
	var stdout, stderr bytes.Buffer
	env := cli.Env{
		LoadConfig: warningConfig,
		Stdout:     &stdout, Stderr: &stderr,
		Now: func() time.Time { return spendNow },
		NewReport: func(context.Context, string) (*report.Server, error) {
			return report.NewServer(report.WithStore(fakeReportStore{})), nil
		},
	}

	err := cli.Execute(t.Context(), []string{"holdings"}, env)

	require.NoError(t, err)
	assert.Equal(t, "quarry: warning: "+unknownKeyShown+"\n", stderr.String())
	assert.Contains(t, stdout.String(), "Holdings on 2026-09-29")
}

func Test_holdings_json_reads_back_the_listing_the_total_and_the_config_warning(t *testing.T) {
	var stdout, stderr bytes.Buffer
	env := cli.Env{
		LoadConfig: warningConfig,
		Stdout:     &stdout, Stderr: &stderr,
		Now: func() time.Time { return spendNow },
		NewReport: func(context.Context, string) (*report.Server, error) {
			return report.NewServer(report.WithStore(fakeReportStore{holdings: store.Holdings{Holdings: []store.Holding{brokerageHolding()}}})), nil
		},
	}

	err := cli.Execute(t.Context(), []string{"holdings", "--json"}, env)

	require.NoError(t, err)
	type total struct {
		Currency string `json:"currency"`
		Value    string `json:"value"`
	}
	var doc struct {
		AsOf     string `json:"as_of"`
		Currency string `json:"currency"`
		Holdings []struct {
			Account        string  `json:"account"`
			Security       *string `json:"security"`
			Price          *string `json:"price"`
			Value          *string `json:"value"`
			ConvertedValue *string `json:"converted_value"`
		} `json:"holdings"`
		Totals   []total  `json:"totals"`
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	assert.Equal(t, "2026-09-29", doc.AsOf)
	assert.Equal(t, "CAD", doc.Currency)
	require.Len(t, doc.Holdings, 1)
	assert.Equal(t, "Brokerage", doc.Holdings[0].Account)
	assert.Equal(t, "37704.00", *doc.Holdings[0].Value)
	assert.Equal(t, "50.00", *doc.Holdings[0].ConvertedValue)
	assert.Equal(t, []total{{Currency: "CAD", Value: "50.00"}}, doc.Totals)
	assert.Equal(t, []string{unknownKeyAbsolute}, doc.Warnings)
}

func Test_holdings_json_lists_no_holdings_as_an_empty_array(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeHoldings(t, fakeReportStore{}, &stdout, &stderr, "--json")

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), `"holdings": []`)
	assert.Contains(t, stdout.String(), `"totals": []`)
}

func Test_holdings_returns_a_failed_store_read(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeHoldings(t, fakeReportStore{err: errStoreRead}, &stdout, &stderr)

	require.ErrorIs(t, err, errStoreRead)
	assert.Empty(t, stdout.String())
}

func Test_holdings_returns_a_failed_report_open(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := cli.Execute(t.Context(), []string{"holdings"}, refusedEnv(&stdout, &stderr))

	require.ErrorIs(t, err, errStoreRead)
	assert.Empty(t, stdout.String())
}

func Test_holdings_returns_a_failed_stdout_write(t *testing.T) {
	var stderr bytes.Buffer

	err := executeHoldings(t, fakeReportStore{}, failingWriter{err: errNoSpace}, &stderr)

	require.ErrorIs(t, err, errNoSpace)
	assert.EqualError(t, err, "cannot write the result to stdout: "+errNoSpace.Error())
}
