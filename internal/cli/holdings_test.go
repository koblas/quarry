package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"regexp"
	"strings"
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

const holdingsAsOfHelp = "value holdings on date (YYYY, YYYY-MM or YYYY-MM-DD; a year or month means its last day; default today)"

const holdingsCurrencyHelp = "add a column with each value in currency code: CAD or USD; native adds none (default reporting.currency in the config file, else CAD)"

// holdingsEnv is an Env whose config comes from load and whose report store is fake.
func holdingsEnv(load func(string) (config.Config, error), fake fakeReportStore, stdout, stderr io.Writer) cli.Env {
	return cli.Env{
		LoadConfig: load,
		Stdout:     stdout, Stderr: stderr,
		Now: func() time.Time { return spendNow },
		NewReport: func(context.Context, string) (*report.Server, error) {
			return report.NewServer(report.WithStore(fake)), nil
		},
	}
}

func executeHoldings(t *testing.T, fake fakeReportStore, stdout, stderr io.Writer, args ...string) error {
	t.Helper()
	return cli.Execute(t.Context(), append([]string{"holdings"}, args...), holdingsEnv(cadConfig, fake, stdout, stderr))
}

// holdingsTotalDoc and holdingsDoc read back the --json keys these tests pin.
type holdingsTotalDoc struct {
	Currency string `json:"currency"`
	Value    string `json:"value"`
}

type holdingsDoc struct {
	AsOf     string `json:"as_of"`
	Currency string `json:"currency"`
	Holdings []struct {
		Account        string  `json:"account"`
		Security       *string `json:"security"`
		Price          *string `json:"price"`
		PriceDate      *string `json:"price_date"`
		Value          *string `json:"value"`
		ConvertedValue *string `json:"converted_value"`
	} `json:"holdings"`
	Totals   []holdingsTotalDoc `json:"totals"`
	Warnings []string           `json:"warnings"`
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

func Test_holdings_help_shows_the_as_of_flag(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeHoldings(t, fakeReportStore{}, &stdout, &stderr, "--help")

	require.NoError(t, err)
	assert.Regexp(t, `(?m)--as-of date +`+regexp.QuoteMeta(holdingsAsOfHelp)+`$`, stdout.String())
}

func Test_holdings_as_of_reads_the_last_day_of_the_period_it_names(t *testing.T) {
	var got store.HoldingsParams
	var stdout, stderr bytes.Buffer

	err := executeHoldings(t, fakeReportStore{gotHoldings: &got}, &stdout, &stderr, "--as-of", "2025")

	require.NoError(t, err)
	assert.Equal(t, store.HoldingsParams{AsOf: time.Date(2025, time.December, 31, 0, 0, 0, 0, time.UTC)}, got)
}

func Test_holdings_as_of_names_its_day_in_the_json_document(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeHoldings(t, fakeReportStore{}, &stdout, &stderr, "--as-of", "2025", "--json")

	require.NoError(t, err)
	var doc holdingsDoc
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	assert.Equal(t, "2025-12-31", doc.AsOf)
}

func Test_holdings_as_of_names_its_day_in_the_native_caption(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeHoldings(t, fakeReportStore{}, &stdout, &stderr, "--as-of", "2025", "--currency", "native")

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Holdings on 2025-12-31 in all accounts; cash not included\n")
}

func Test_holdings_refuses_an_as_of_it_cannot_use_before_reading_anything(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"not a date", []string{"--as-of", "2024-13"}, `--as-of "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`},
		{"empty", []string{"--as-of", ""}, `--as-of "" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`},
		{
			"after today",
			[]string{"--as-of", "2099-01-01"},
			"--as-of 2099-01-01 is after today; holdings are valued up to today only, so pass an earlier --as-of",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := store.HoldingsParams{AsOf: time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC)}
			var stdout, stderr bytes.Buffer

			err := executeHoldings(t, fakeReportStore{gotHoldings: &got}, &stdout, &stderr, c.args...)

			var usage cli.UsageError
			require.ErrorAs(t, err, &usage)
			assert.Equal(t, c.want, usage.Error())
			assert.Empty(t, stdout.String())
			assert.Equal(t, time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC), got.AsOf, "the store was read")
		})
	}
}

func Test_holdings_refuses_a_bad_as_of_before_reading_the_config(t *testing.T) {
	var stdout, stderr bytes.Buffer
	unreadable := func(string) (config.Config, error) { return config.Config{}, errStoreRead }

	err := cli.Execute(t.Context(), []string{"holdings", "--as-of", "2024-13"}, holdingsEnv(unreadable, fakeReportStore{}, &stdout, &stderr))

	var usage cli.UsageError
	require.ErrorAs(t, err, &usage)
	assert.Equal(t, `--as-of "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`, usage.Error())
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
	usdConfig := func(string) (config.Config, error) { return config.Config{Currency: money.USD}, nil }
	fake := fakeReportStore{holdings: store.Holdings{Holdings: []store.Holding{brokerageHolding()}}}

	err := cli.Execute(t.Context(), []string{"holdings"}, holdingsEnv(usdConfig, fake, &stdout, &stderr))

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

	err := cli.Execute(t.Context(), []string{"holdings"}, holdingsEnv(warningConfig, fakeReportStore{}, &stdout, &stderr))

	require.NoError(t, err)
	assert.Equal(t, "quarry: warning: "+unknownKeyShown+"\n", stderr.String())
	assert.Contains(t, stdout.String(), "Holdings on 2026-09-29")
}

func Test_holdings_json_reads_back_the_listing_the_total_and_the_config_warning(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := fakeReportStore{holdings: store.Holdings{Holdings: []store.Holding{brokerageHolding()}}}

	err := cli.Execute(t.Context(), []string{"holdings", "--json"}, holdingsEnv(warningConfig, fake, &stdout, &stderr))

	require.NoError(t, err)
	var doc holdingsDoc
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	assert.Equal(t, "2026-09-29", doc.AsOf)
	assert.Equal(t, "CAD", doc.Currency)
	require.Len(t, doc.Holdings, 1)
	assert.Equal(t, "Brokerage", doc.Holdings[0].Account)
	assert.Equal(t, "37704.00", *doc.Holdings[0].Value)
	assert.Equal(t, "50.00", *doc.Holdings[0].ConvertedValue)
	assert.Equal(t, []holdingsTotalDoc{{Currency: "CAD", Value: "50.00"}}, doc.Totals)
	assert.Equal(t, []string{unknownKeyAbsolute}, doc.Warnings)
}

func Test_holdings_native_json_has_one_total_per_stored_currency_cad_then_usd_then_others(t *testing.T) {
	var stdout, stderr bytes.Buffer
	own := func(code *string, cents int64) store.Holding {
		return store.Holding{Currency: code, Value: big.NewInt(cents), ValueCAD: big.NewInt(1), ValueUSD: big.NewInt(1)}
	}
	fake := fakeReportStore{holdings: store.Holdings{Holdings: []store.Holding{
		own(new("EUR"), 200), own(new("USD"), 300), own(nil, 900), own(new("CAD"), 500), own(new("CAD"), 50),
	}}}

	err := executeHoldings(t, fake, &stdout, &stderr, "--currency", "native", "--json")

	require.NoError(t, err)
	var doc holdingsDoc
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	assert.Equal(t, "native", doc.Currency)
	assert.Equal(t, []holdingsTotalDoc{
		{Currency: "CAD", Value: "5.50"}, {Currency: "USD", Value: "3.00"}, {Currency: "EUR", Value: "2.00"},
	}, doc.Totals)
	assert.Nil(t, doc.Holdings[0].ConvertedValue)
}

// bareHolding is 40 shares of Bare Fund with no price on or before the as-of day, so no value.
func bareHolding() store.Holding {
	return store.Holding{
		AccountID: "acct-1", Account: "Brokerage", SecurityID: "sec-2", Security: new("Bare Fund"),
		Shares: 40_000_000, Currency: new("CAD"),
	}
}

const noPriceLine = "1 holding has no price on or before 2026-09-29, so it has no value and is left out of the total; " +
	"enter a price for it in Quicken, then run quarry sync"

func Test_holdings_prints_the_no_price_line_after_the_config_warning_on_stderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := fakeReportStore{holdings: store.Holdings{Holdings: []store.Holding{brokerageHolding(), bareHolding()}}}

	err := cli.Execute(t.Context(), []string{"holdings"}, holdingsEnv(warningConfig, fake, &stdout, &stderr))

	require.NoError(t, err)
	assert.Equal(t, "quarry: warning: "+unknownKeyShown+"\nquarry: warning: "+noPriceLine+"\n", stderr.String())
}

func Test_holdings_json_lists_the_no_price_line_after_the_config_warning_and_nulls_the_price(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := fakeReportStore{holdings: store.Holdings{Holdings: []store.Holding{bareHolding()}}}

	err := cli.Execute(t.Context(), []string{"holdings", "--json"}, holdingsEnv(warningConfig, fake, &stdout, &stderr))

	require.NoError(t, err)
	var doc holdingsDoc
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	assert.Equal(t, []string{unknownKeyAbsolute, noPriceLine}, doc.Warnings)
	require.Len(t, doc.Holdings, 1)
	assert.Nil(t, doc.Holdings[0].Price)
	assert.Nil(t, doc.Holdings[0].PriceDate)
	assert.Nil(t, doc.Holdings[0].Value)
	assert.Nil(t, doc.Holdings[0].ConvertedValue)
}

func Test_holdings_total_leaves_out_a_holding_with_no_price(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "converted to the reporting currency", args: nil, want: `(?m)^Total +50\.00$`},
		{name: "native", args: []string{"--currency", "native"}, want: `(?m)^Total +CAD +37,704\.00$`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			fake := fakeReportStore{holdings: store.Holdings{Holdings: []store.Holding{bareHolding(), brokerageHolding()}}}

			err := executeHoldings(t, fake, &stdout, &stderr, c.args...)

			require.NoError(t, err)
			assert.Regexp(t, c.want, stdout.String())
			assert.Contains(t, stdout.String(), "no price")
		})
	}
}

func Test_holdings_listing_of_only_unpriced_holdings_has_no_total_row(t *testing.T) {
	for _, args := range [][]string{nil, {"--currency", "native"}} {
		t.Run(strings.Join(append([]string{"holdings"}, args...), " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			fake := fakeReportStore{holdings: store.Holdings{Holdings: []store.Holding{bareHolding()}}}

			err := executeHoldings(t, fake, &stdout, &stderr, args...)

			require.NoError(t, err)
			assert.Contains(t, stdout.String(), "Bare Fund")
			assert.NotContains(t, stdout.String(), "Total")
		})
	}
}

// unconvertibleHolding is brokerageHolding priced in currency, which the store leaves unconverted.
func unconvertibleHolding(currency *string) store.Holding {
	h := brokerageHolding()
	h.Currency, h.ValueCAD, h.ValueUSD = currency, nil, nil
	return h
}

func Test_holdings_in_column_says_not_converted_for_a_priced_security_it_cannot_convert(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		currency *string
	}{
		{name: "no currency in CAD", currency: nil},
		{name: "no currency in USD", args: []string{"--currency", "USD"}, currency: nil},
		{name: "EUR in CAD", currency: new("EUR")},
		{name: "EUR in USD", args: []string{"--currency", "USD"}, currency: new("EUR")},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			fake := fakeReportStore{holdings: store.Holdings{Holdings: []store.Holding{unconvertibleHolding(c.currency)}}}

			err := executeHoldings(t, fake, &stdout, &stderr, c.args...)

			require.NoError(t, err)
			assert.Regexp(t, `(?m)^Brokerage +Acme Corp \(ACME\) .* +not converted$`, stdout.String())
			assert.NotContains(t, stdout.String(), "Total")
		})
	}
}

func Test_holdings_in_column_has_no_not_converted_where_the_row_is_unpriced_or_the_listing_native(t *testing.T) {
	unpricedNone, unpricedEUR := bareHolding(), bareHolding()
	unpricedNone.Currency, unpricedEUR.Currency = nil, new("EUR")
	cases := []struct {
		name string
		args []string
		row  store.Holding
	}{
		{name: "an unpriced security with no currency", row: unpricedNone},
		{name: "an unpriced EUR security", row: unpricedEUR},
		{name: "a priced security with no currency in native", args: []string{"--currency", "native"}, row: unconvertibleHolding(nil)},
		{name: "a priced EUR security in native", args: []string{"--currency", "native"}, row: unconvertibleHolding(new("EUR"))},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			fake := fakeReportStore{holdings: store.Holdings{Holdings: []store.Holding{c.row}}}

			err := executeHoldings(t, fake, &stdout, &stderr, c.args...)

			require.NoError(t, err)
			assert.Contains(t, stdout.String(), c.row.Account)
			assert.NotContains(t, stdout.String(), "not converted")
		})
	}
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
