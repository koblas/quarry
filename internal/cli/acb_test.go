package cli_test

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const acbCurrencyHelp = "ACB is in CAD only; any other currency is refused"

// acbTrade is a CAD trade of millionths shares of Acme in acct-1, with amount in cents.
func acbTrade(sourceID int64, date time.Time, action string, millionths, amount int64) store.InvestmentTransaction {
	acme := "sec-acme"
	return store.InvestmentTransaction{
		ID: "itxn-" + strconv.FormatInt(sourceID, 10), SourceID: sourceID, AccountID: "acct-1", SecurityID: &acme,
		Date: date, Action: action, Shares: &millionths, Amount: amount, Currency: "CAD",
	}
}

// acbHistory is a CAD brokerage that bought 10 shares of Acme for 1,000.00 on 2026-09-01 and sold 4 for 640.00
// on 2026-09-28.
func acbHistory() store.InvestmentHistory {
	return store.InvestmentHistory{
		Accounts:   []store.Account{{ID: "acct-1", Name: "Brokerage", Type: store.AccountTypeBrokerage, Currency: "CAD", Active: true}},
		Securities: []store.Security{{ID: "sec-acme", Name: "Acme Corp", Ticker: new("ACME"), Currency: new("CAD")}},
		Transactions: []store.InvestmentTransaction{
			acbTrade(1, time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC), store.ActionBuy, 10_000_000, -100_000),
			acbTrade(2, time.Date(2026, time.September, 28, 0, 0, 0, 0, time.UTC), store.ActionSell, -4_000_000, 64_000),
		},
	}
}

// nonRegisteredConfig is a ConfigLoader whose config lists acct-1 as non-registered.
func nonRegisteredConfig(string) (config.Config, error) {
	return config.Config{Currency: money.CAD, NonRegistered: []string{"acct-1"}}, nil
}

func Test_acb_help_says_how_the_cost_base_is_pooled_and_what_the_amounts_are(t *testing.T) {
	const long = `Show the adjusted cost base (ACB) of each security and the capital gains
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
accountant, not a tax filing.
`
	var stdout, stderr bytes.Buffer

	err := cli.Execute(t.Context(), []string{"acb", "--help"}, currencyEnv(&stdout, &stderr))

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), long)
}

func Test_acb_help_shows_examples(t *testing.T) {
	const examples = `Examples:
  quarry acb
  quarry acb --year 2024
  quarry acb --security XEQT --json
`
	var stdout, stderr bytes.Buffer

	err := cli.Execute(t.Context(), []string{"acb", "--help"}, currencyEnv(&stdout, &stderr))

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), examples)
}

func Test_acb_help_shows_the_currency_flag(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := cli.Execute(t.Context(), []string{"acb", "--help"}, currencyEnv(&stdout, &stderr))

	require.NoError(t, err)
	assert.Regexp(t, `(?m)--currency currency +`+regexp.QuoteMeta(acbCurrencyHelp)+`$`, stdout.String())
}

func Test_acb_pools_the_accounts_the_config_lists_as_non_registered(t *testing.T) {
	var stdout, stderr bytes.Buffer
	env := holdingsEnv(nonRegisteredConfig, fakeReportStore{history: acbHistory()}, &stdout, &stderr)

	err := cli.Execute(t.Context(), []string{"acb", "--json"}, env)

	require.NoError(t, err)
	var doc struct {
		AsOf  string `json:"as_of"`
		Years []struct {
			Year      int    `json:"year"`
			SaleCount int    `json:"sale_count"`
			Gain      string `json:"gain"`
		} `json:"years"`
		Securities []struct {
			Shares string `json:"shares"`
			ACB    string `json:"acb"`
		} `json:"securities"`
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), stdout.String())
	assert.Equal(t, "2026-09-29", doc.AsOf)
	require.Len(t, doc.Years, 1)
	assert.Equal(t, 2026, doc.Years[0].Year)
	assert.Equal(t, 1, doc.Years[0].SaleCount)
	assert.Equal(t, "240.00", doc.Years[0].Gain)
	require.Len(t, doc.Securities, 1)
	assert.Equal(t, "6.000000", doc.Securities[0].Shares)
	assert.Equal(t, "600.00", doc.Securities[0].ACB)
	assert.Equal(t, []string{}, doc.Warnings)
}

func Test_acb_applies_the_configs_adjustments(t *testing.T) {
	var stdout, stderr bytes.Buffer
	withReturnOfCapital := func(string) (config.Config, error) {
		cfg, _ := nonRegisteredConfig("")
		cfg.Adjustments = []config.Adjustment{
			{Security: "sec-acme", Date: time.Date(2026, time.September, 15, 0, 0, 0, 0, time.UTC), ReturnOfCapital: 10_000},
		}
		return cfg, nil
	}
	env := holdingsEnv(withReturnOfCapital, fakeReportStore{history: acbHistory()}, &stdout, &stderr)

	err := cli.Execute(t.Context(), []string{"acb", "--json"}, env)

	require.NoError(t, err)
	var doc struct {
		Years []struct {
			Gain string `json:"gain"`
		} `json:"years"`
		Securities []struct {
			ACB string `json:"acb"`
		} `json:"securities"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), stdout.String())
	require.Len(t, doc.Securities, 1)
	assert.Equal(t, "540.00", doc.Securities[0].ACB)
	require.Len(t, doc.Years, 1)
	assert.Equal(t, "280.00", doc.Years[0].Gain)
}

func Test_acb_leaves_an_unlisted_account_out_of_the_pool(t *testing.T) {
	var stdout, stderr bytes.Buffer
	env := holdingsEnv(cadConfig, fakeReportStore{history: acbHistory()}, &stdout, &stderr)

	err := cli.Execute(t.Context(), []string{"acb", "--json"}, env)

	require.NoError(t, err)
	var doc struct {
		Years      []json.RawMessage `json:"years"`
		Securities []json.RawMessage `json:"securities"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), stdout.String())
	assert.Empty(t, doc.Years)
	assert.Empty(t, doc.Securities)
}

func Test_acb_returns_a_failed_store_read(t *testing.T) {
	var stdout, stderr bytes.Buffer
	env := holdingsEnv(cadConfig, fakeReportStore{err: errStoreRead}, &stdout, &stderr)

	err := cli.Execute(t.Context(), []string{"acb"}, env)

	require.ErrorIs(t, err, errStoreRead)
	assert.NotErrorAs(t, err, new(cli.UsageError))
	assert.Empty(t, stdout.String())
}

func Test_acb_returns_a_failed_report_open(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := cli.Execute(t.Context(), []string{"acb"}, refusedEnv(&stdout, &stderr))

	require.ErrorIs(t, err, errStoreRead)
	assert.Empty(t, stdout.String())
}

func Test_acb_refuses_a_currency_that_is_not_cad_usd_or_native(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := cli.Execute(t.Context(), []string{"acb", "--currency", "EUR"}, currencyEnv(&stdout, &stderr))

	var usage cli.UsageError
	require.ErrorAs(t, err, &usage)
	assert.Equal(t, badCurrencyFlag, usage.Error())
	assert.Empty(t, stdout.String())
}
