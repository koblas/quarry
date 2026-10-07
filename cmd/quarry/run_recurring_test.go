// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recurringHeader is the header cells of the recurring table.
var recurringHeader = []string{"Payee", "Currency", "Every", "Amount", "Per year", "First", "Last", "Status", "Price changes"}

// recurringRightAligned are the table's columns of money, which pad on the left.
var recurringRightAligned = map[int]bool{3: true, 4: true}

// recurringTable is the recurring table under caption: every cell but the last padded to its
// column's widest cell, two-space gaps, money columns right-aligned, trailing spaces trimmed.
func recurringTable(caption string, rows ...[]string) string {
	rows = append([][]string{recurringHeader}, rows...)
	widths := make([]int, len(recurringHeader)-1)
	for _, row := range rows {
		for i := range widths {
			widths[i] = max(widths[i], len(row[i]))
		}
	}
	var b strings.Builder
	b.WriteString(caption + "\n\n")
	for _, row := range rows {
		cells := make([]string, len(row))
		for i, cell := range row {
			switch {
			case i == len(widths):
				cells[i] = cell
			case recurringRightAligned[i]:
				cells[i] = fmt.Sprintf("%*s", widths[i], cell)
			default:
				cells[i] = fmt.Sprintf("%-*s", widths[i], cell)
			}
		}
		b.WriteString(strings.TrimRight(strings.Join(cells, "  "), " ") + "\n")
	}
	return b.String()
}

// groceryCharge is a one-split expense of cents (a positive number is money out) in cat-groceries.
func groceryCharge(payee string, day time.Time, cents int64) chargeTxn {
	return chargeTxn{
		id: payee + day.Format(time.DateOnly), account: "acct-cad", payee: payee, currency: "CAD", day: day,
		splits: []chargeSplit{{category: "cat-groceries", cents: -cents}},
	}
}

func Test_run_recurring_lists_a_monthly_subscription_with_its_yearly_cost(t *testing.T) {
	home := newHome(t)
	var charges []chargeTxn
	for month := time.October; len(charges) < 12; month++ {
		charges = append(charges, groceryCharge("Netflix.com", day(2025, month, 12), 2099))
	}
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)}, charges...))

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"recurring"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, recurringTable("Recurring charges 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD",
		[]string{"Netflix.com", "CAD", "month", "20.99", "251.88", "2025-10-12", "2026-09-12", "active", ""},
		[]string{"Total", "CAD", "", "", "251.88", "", "", "", ""}),
		stdout.String())
}

func Test_run_recurring_detects_weekly_quarterly_and_yearly_series(t *testing.T) {
	cases := []struct {
		name    string
		gapDays int
		count   int
		every   string
		cents   int64
		amount  string
		perYear string
		first   string
	}{
		{name: "weekly", gapDays: 7, count: 4, every: "week", cents: 1000, amount: "10.00", perYear: "520.00", first: "2026-08-30"},
		{name: "quarterly", gapDays: 91, count: 3, every: "quarter", cents: 3000, amount: "30.00", perYear: "120.00", first: "2026-03-22"},
		{name: "yearly", gapDays: 365, count: 2, every: "year", cents: 9900, amount: "99.00", perYear: "99.00", first: "2025-09-20"},
	}
	lastCharge := day(2026, time.September, 20)

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			var charges []chargeTxn
			for i := c.count - 1; i >= 0; i-- {
				charges = append(charges, groceryCharge("Gym", lastCharge.AddDate(0, 0, -c.gapDays*i), c.cents))
			}
			replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)}, charges...))

			exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"recurring", "--since", "2000"})

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Empty(t, stderr.String())
			assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in all accounts, amounts in CAD",
				[]string{"Gym", "CAD", c.every, c.amount, c.perYear, c.first, "2026-09-20", "active, new", ""},
				[]string{"Total", "CAD", "", "", c.perYear, "", "", "", ""}),
				stdout.String())
		})
	}
}

func Test_run_recurring_counts_a_split_charge_once_and_leaves_a_refund_out(t *testing.T) {
	home := newHome(t)
	charges := []chargeTxn{
		groceryCharge("Gym", day(2026, time.April, 10), 2099),
		groceryCharge("Gym", day(2026, time.May, 10), 2099),
		groceryCharge("Gym", day(2026, time.June, 10), 2099),
		groceryCharge("Gym", day(2026, time.July, 10), 2099),
		{
			id: "refund", account: "acct-cad", payee: "Gym", currency: "CAD", day: day(2026, time.July, 25),
			splits: []chargeSplit{{category: "cat-groceries", cents: 2099}},
		},
		groceryCharge("Gym", day(2026, time.August, 10), 2099),
		{
			id: "split", account: "acct-cad", payee: "Gym", currency: "CAD", day: day(2026, time.September, 10),
			splits: []chargeSplit{{category: "cat-groceries", cents: -1000}, {category: "cat-fuel", cents: -1099}},
		},
	}
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)}, charges...))

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"recurring", "--since", "2000"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in all accounts, amounts in CAD",
		[]string{"Gym", "CAD", "month", "20.99", "251.88", "2026-04-10", "2026-09-10", "active, new", ""},
		[]string{"Total", "CAD", "", "", "251.88", "", "", "", ""}),
		stdout.String())
}

// monthlyCharges is twelve monthly charges of cents by payee on account, the last on 2026-09-12.
func monthlyCharges(account, payee string, cents int64) []chargeTxn {
	var charges []chargeTxn
	for month := time.October; len(charges) < 12; month++ {
		charge := groceryCharge(payee, day(2025, month, 12), cents)
		charge.account = account
		charges = append(charges, charge)
	}
	return charges
}

func Test_run_recurring_lists_only_the_series_charged_in_the_named_account(t *testing.T) {
	home := newHome(t)
	visa := store.Account{ID: "acct-visa", SourceID: 2, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true}
	charges := append(monthlyCharges("acct-visa", "Netflix.com", 2099), monthlyCharges("acct-cad", "Gym", 4000)...)
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1), visa}, charges...))

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"recurring", "--since", "2000", "--account", "Visa"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in Visa, amounts in CAD",
		[]string{"Netflix.com", "CAD", "month", "20.99", "251.88", "2025-10-12", "2026-09-12", "active, new", ""},
		[]string{"Total", "CAD", "", "", "251.88", "", "", "", ""}),
		stdout.String())
}

func Test_run_recurring_lists_an_ended_series_charged_in_a_closed_account(t *testing.T) {
	home := newHome(t)
	oldCard := store.Account{ID: "acct-old", SourceID: 2, Name: "Old Card", Type: "credit_card", Currency: "CAD", Closed: true}
	var charges []chargeTxn
	for month := time.July; len(charges) < 12; month++ {
		charge := groceryCharge("Gym", day(2024, month, 12), 4000)
		charge.account = "acct-old"
		charges = append(charges, charge)
	}
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1), oldCard}, charges...))

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"recurring", "--since", "2000", "--account", "Old Card"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in Old Card, amounts in CAD",
		[]string{"Gym", "CAD", "month", "40.00", "", "2024-07-12", "2025-06-12", "ended, new", ""}),
		stdout.String())
}

func Test_run_recurring_says_when_no_series_runs_in_the_period(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)},
		groceryCharge("Bakery", day(2003, 1, 4), 1000),
		groceryCharge("Bakery", day(2025, 12, 31), 500)))

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"recurring"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, recurringTable("Recurring charges 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD"), stdout.String())
	assert.Equal(t, "quarry: warning: no recurring charges from 2026-01-01 to 2026-09-29; "+
		"the store's transactions run 2003-01-04 to 2025-12-31\n", stderr.String())
}

const (
	recurringEmptyWindow = "no recurring charges from 2026-01-01 to 2026-09-29"
	recurringHeaderOnly  = "Payee  Currency  Every  Amount  Per year  First  Last  Status  Price changes\n"
	recurringOldCardLine = "account \"Old Card\" is not used in reports in Quicken, so recurring leaves it out; " +
		"to include it, turn on reports for it in Quicken's account settings, then run quarry sync"
	recurringLinkedLine   = "account \"Linked\" uses linked account tracking in Quicken, so recurring leaves it out, as Quicken's reports do"
	recurringAccountsSpan = "their transactions run 2003-01-04 to 2025-12-31"
)

// bakeryHistory is two charges from one payee, 2003-01-04 and 2025-12-31: none runs in the default period.
func bakeryHistory() []chargeTxn {
	return []chargeTxn{
		groceryCharge("Bakery", day(2003, 1, 4), 1000),
		groceryCharge("Bakery", day(2025, 12, 31), 500),
	}
}

// warningLines is the stderr text of warnings: each on its own prefixed line.
func warningLines(warnings []string) string {
	var text strings.Builder
	for _, w := range warnings {
		text.WriteString("quarry: warning: " + w + "\n")
	}
	return text.String()
}

func Test_run_recurring_prints_each_empty_period_warning_on_stderr_and_in_the_json(t *testing.T) {
	cases := []struct {
		name        string
		args        []string
		charges     []chargeTxn
		wantCaption string
		wantWarns   []string
	}{
		{
			name: "a store with no transactions", args: []string{},
			wantCaption: "all accounts",
			wantWarns:   []string{recurringEmptyWindow + "; the store has no transactions"},
		},
		{
			name: "the named account has transactions, none recurring in the period", args: []string{"--account", "Chequing"}, charges: bakeryHistory(),
			wantCaption: "Chequing",
			wantWarns:   []string{recurringEmptyWindow + " in the named accounts; " + recurringAccountsSpan},
		},
		{
			name: "the named account has no transactions", args: []string{"--account", "Visa"}, charges: bakeryHistory(),
			wantCaption: "Visa",
			wantWarns:   []string{recurringEmptyWindow + " in the named accounts; they have no transactions"},
		},
		{
			name: "the only named account is not in reports", args: []string{"--account", "Old Card"}, charges: bakeryHistory(),
			wantCaption: "Old Card",
			wantWarns:   []string{recurringOldCardLine},
		},
		{
			name: "the only named account uses linked tracking", args: []string{"--account", "Linked"}, charges: bakeryHistory(),
			wantCaption: "Linked",
			wantWarns:   []string{recurringLinkedLine},
		},
		{
			name: "an account not in reports named beside one that is", args: []string{"--account", "Old Card", "--account", "Chequing"}, charges: bakeryHistory(),
			wantCaption: "Old Card, Chequing",
			wantWarns:   []string{recurringOldCardLine, recurringEmptyWindow + " in the named accounts; " + recurringAccountsSpan},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			accounts := []store.Account{
				chequingAccount("acct-cad", 1),
				{ID: "acct-visa", SourceID: 2, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true},
				{ID: "acct-old", SourceID: 3, Name: "Old Card", Type: "credit_card", Currency: "CAD", Active: true, NotInReports: true},
				{ID: "acct-401k", SourceID: 4, Name: "Linked", Type: "chequing", Currency: "CAD", Active: true, LinkedTracking: true},
			}
			replaceStore(t, home, chargeRows(accounts, c.charges...))
			var textOut, textErr, jsonOut, jsonErr bytes.Buffer

			textExit := runWith(context.Background(), append([]string{"recurring"}, c.args...), spendEnv(&textOut, &textErr))
			jsonExit := runWith(context.Background(), append([]string{"recurring", "--json"}, c.args...), spendEnv(&jsonOut, &jsonErr))

			require.Equal(t, 0, textExit, textErr.String())
			require.Equal(t, 0, jsonExit, jsonErr.String())
			wantStderr := warningLines(c.wantWarns)
			assert.Equal(t, "Recurring charges 2026-01-01 to 2026-09-29 in "+c.wantCaption+", amounts in CAD\n\n"+recurringHeaderOnly, textOut.String())
			assert.Equal(t, wantStderr, textErr.String())
			assert.Equal(t, wantStderr, jsonErr.String())
			var doc struct {
				Warnings []string `json:"warnings"`
			}
			require.NoError(t, json.Unmarshal(jsonOut.Bytes(), &doc), jsonOut.String())
			assert.Equal(t, c.wantWarns, doc.Warnings)
		})
	}
}

func Test_run_recurring_lists_nothing_for_a_future_period_that_until_allows(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)}, bakeryHistory()...))

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"recurring", "--since", "2030", "--until", "2031"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "Recurring charges 2030-01-01 to 2031-12-31 in all accounts, amounts in CAD\n\n"+recurringHeaderOnly, stdout.String())
	assert.Equal(t, "quarry: warning: no recurring charges from 2030-01-01 to 2031-12-31; "+
		"the store's transactions run 2003-01-04 to 2025-12-31\n", stderr.String())
}

// recurringFXStore holds, under a fresh HOME, a steady USD Netflix and a Gym in CAD and in USD (12.00 to 15.00),
// with rates 1.30, 1.40 from June and, after the last charge, 1.50.
func recurringFXStore(t *testing.T) {
	t.Helper()
	home := newHome(t)
	usdNetflix := inUSD(monthlySeries("Netflix.com", 2026, time.February, slices.Repeat([]int64{1000}, 8)...))
	cadGym := monthlySeries("Gym", 2026, time.February, slices.Repeat([]int64{2000}, 8)...)
	usdGym := inUSD(monthlySeries("Gym", 2026, time.February, slices.Concat(slices.Repeat([]int64{1200}, 4), slices.Repeat([]int64{1500}, 4))...))
	replaceStoreWithRates(t, home,
		chargeRows([]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
			slices.Concat(usdNetflix, cadGym, usdGym)...),
		store.Rate{Date: day(2026, time.January, 2), USDCAD: money.Rate(1_300_000), Series: "FXUSDCAD"},
		store.Rate{Date: day(2026, time.June, 1), USDCAD: money.Rate(1_400_000), Series: "FXUSDCAD"},
		store.Rate{Date: day(2026, time.September, 20), USDCAD: money.Rate(1_500_000), Series: "FXUSDCAD"},
	)
}

func Test_run_recurring_detects_in_native_currency_and_converts_at_the_latest_charges_rate(t *testing.T) {
	recurringFXStore(t)

	t.Run("text lists converted amounts and finds no price change in a steady USD price", func(t *testing.T) {
		exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"recurring", "--since", "2000"})

		require.Equal(t, 0, exitCode, stderr.String())
		assert.Empty(t, stderr.String())
		assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in all accounts, amounts in CAD",
			[]string{"Gym", "CAD (USD)", "month", "21.00", "252.00", "2026-02-12", "2026-09-12", "active, new", "1: USD 12.00 -> USD 15.00 (+25.0%)"},
			[]string{"Gym", "CAD", "month", "20.00", "240.00", "2026-02-12", "2026-09-12", "active, new", ""},
			[]string{"Netflix.com", "CAD (USD)", "month", "14.00", "168.00", "2026-02-12", "2026-09-12", "active, new", ""},
			[]string{"Total", "CAD", "", "", "660.00", "", "", "", ""}),
			stdout.String())
	})

	t.Run("json carries the native currency and amounts beside the converted ones", func(t *testing.T) {
		exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"recurring", "--since", "2000", "--json"})

		require.Equal(t, 0, exitCode, stderr.String())
		assert.Equal(t, recurringJSONDoc{
			Since:         "2000-01-01",
			Until:         "2026-09-29",
			Currency:      "CAD",
			AccountFilter: []recurringIDName{},
			Series: []recurringSeriesJSON{
				monthlyFXSeries(recurringSeriesJSON{
					Payee: "Gym", PayeeKey: new("gym"), Currency: "CAD", Amount: "21.00", FirstAmount: "15.60", PerYear: new("252.00"),
					NativeCurrency: "USD", NativeAmount: "15.00", NativeFirstAmount: "12.00",
					Accounts: []recurringIDName{{ID: "acct-usd", Name: "US Chequing"}},
					PriceChanges: []recurringPriceChangeJSON{
						{Date: "2026-06-12", Currency: "USD", From: "12.00", To: "15.00", ChangePct: 25.0},
					},
				}),
				monthlyFXSeries(recurringSeriesJSON{
					Payee: "Gym", PayeeKey: new("gym"), Currency: "CAD", Amount: "20.00", FirstAmount: "20.00", PerYear: new("240.00"),
					NativeCurrency: "CAD", NativeAmount: "20.00", NativeFirstAmount: "20.00",
					Accounts: []recurringIDName{{ID: "acct-cad", Name: "Chequing"}},
				}),
				monthlyFXSeries(recurringSeriesJSON{
					Payee: "Netflix.com", PayeeKey: new("netflix-com"), Currency: "CAD", Amount: "14.00", FirstAmount: "13.00", PerYear: new("168.00"),
					NativeCurrency: "USD", NativeAmount: "10.00", NativeFirstAmount: "10.00",
					Accounts: []recurringIDName{{ID: "acct-usd", Name: "US Chequing"}},
				}),
			},
			Totals:   []recurringTotalJSON{{Currency: "CAD", PerYear: "660.00"}},
			Warnings: []string{},
		}, decodeRecurringJSON(t, stdout.String()))
	})
}

// monthlyFXSeries is s with the fields every series of the store shares: monthly, eight charges from 2026-02-12
// to 2026-09-12, one payee named by s.Payee, and an empty price-change list unless s names some.
func monthlyFXSeries(s recurringSeriesJSON) recurringSeriesJSON {
	s.Payees = []recurringIDName{{ID: "payee-" + s.Payee, Name: s.Payee}}
	s.Cadence, s.State, s.New, s.ChargeCount = "monthly", "active", true, 8
	s.FirstCharge, s.LastCharge = "2026-02-12", "2026-09-12"
	if s.PriceChanges == nil {
		s.PriceChanges = []recurringPriceChangeJSON{}
	}
	return s
}

// runRecurring runs quarry recurring from 2000 with args and returns stdout and stderr; it must exit 0.
func runRecurring(t *testing.T, args ...string) (string, string) {
	t.Helper()

	exitCode, stdout, stderr := runSpendCapture(context.Background(), append([]string{"recurring", "--since", "2000"}, args...))

	require.Equal(t, 0, exitCode, stderr.String())
	return stdout.String(), stderr.String()
}

// monthlyRow is the cells of a monthly series charged 2026-02-12 to 2026-09-12 that is active and new.
func monthlyRow(payee, currency, amount, perYear, priceChanges string) []string {
	return []string{payee, currency, "month", amount, perYear, "2026-02-12", "2026-09-12", "active, new", priceChanges}
}

func Test_run_recurring_in_usd_lists_each_series_beside_its_native_currency(t *testing.T) {
	recurringFXStore(t)

	t.Run("text puts the series' own currency in the cell and keeps price changes native", func(t *testing.T) {
		stdout, stderr := runRecurring(t, "--currency", "USD")

		assert.Empty(t, stderr)
		assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in all accounts, amounts in USD",
			monthlyRow("Gym", "USD", "15.00", "180.00", "1: 12.00 -> 15.00 (+25.0%)"),
			monthlyRow("Gym", "USD (CAD)", "14.29", "171.48", ""),
			monthlyRow("Netflix.com", "USD", "10.00", "120.00", ""),
			[]string{"Total", "USD", "", "", "471.48", "", "", "", ""}),
			stdout)
	})

	t.Run("json lists the USD series with currency equal to native_currency", func(t *testing.T) {
		stdout, stderr := runRecurring(t, "--currency", "USD", "--json")

		assert.Empty(t, stderr)
		doc := decodeRecurringJSON(t, stdout)
		assert.Equal(t, "USD", doc.Currency)
		require.Len(t, doc.Series, 3)
		cadGym := doc.Series[1]
		assert.Equal(t, []string{"USD", "14.29", "CAD", "20.00", "20.00"},
			[]string{cadGym.Currency, cadGym.Amount, cadGym.NativeCurrency, cadGym.NativeAmount, cadGym.NativeFirstAmount})
		assert.Equal(t, []recurringTotalJSON{{Currency: "USD", PerYear: "471.48"}}, doc.Totals)
	})
}

func Test_run_recurring_prefixes_a_price_change_with_the_native_code_when_the_row_is_converted(t *testing.T) {
	home := newHome(t)
	cadHydro := monthlySeries("Hydro", 2026, time.February, slices.Concat(slices.Repeat([]int64{999}, 4), slices.Repeat([]int64{1299}, 4))...)
	replaceStoreWithRates(t, home,
		chargeRows([]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)}, cadHydro...),
		store.Rate{Date: day(2026, time.January, 2), USDCAD: money.Rate(1_300_000), Series: "FXUSDCAD"})

	stdout, stderr := runRecurring(t, "--currency", "USD")

	assert.Empty(t, stderr)
	assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in all accounts, amounts in USD",
		monthlyRow("Hydro", "USD (CAD)", "9.99", "119.88", "1: CAD 9.99 -> CAD 12.99 (+30.0%)"),
		[]string{"Total", "USD", "", "", "119.88", "", "", "", ""}),
		stdout)
}

func Test_run_recurring_native_on_a_store_with_rates_lists_every_series_as_it_was_charged(t *testing.T) {
	recurringFXStore(t)

	t.Run("text has no amounts clause and one Total per currency", func(t *testing.T) {
		stdout, stderr := runRecurring(t, "--currency", "native")

		assert.Empty(t, stderr)
		assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in all accounts",
			monthlyRow("Gym", "CAD", "20.00", "240.00", ""),
			monthlyRow("Gym", "USD", "15.00", "180.00", "1: 12.00 -> 15.00 (+25.0%)"),
			monthlyRow("Netflix.com", "USD", "10.00", "120.00", ""),
			[]string{"Total", "CAD", "", "", "240.00", "", "", "", ""},
			[]string{"Total", "USD", "", "", "300.00", "", "", "", ""}),
			stdout)
	})

	t.Run("json repeats each row's own values as its native ones", func(t *testing.T) {
		stdout, stderr := runRecurring(t, "--currency", "native", "--json")

		assert.Empty(t, stderr)
		doc := decodeRecurringJSON(t, stdout)
		assert.Equal(t, "native", doc.Currency)
		assert.Equal(t, [][]string{
			{"Gym", "CAD", "20.00", "20.00", "CAD", "20.00", "20.00"},
			{"Gym", "USD", "15.00", "12.00", "USD", "15.00", "12.00"},
			{"Netflix.com", "USD", "10.00", "10.00", "USD", "10.00", "10.00"},
		}, amountsOf(doc.Series))
	})
}

// amountsOf is each series' payee, currency, amount and first amount, then its native currency, amount and first amount.
func amountsOf(series []recurringSeriesJSON) [][]string {
	rows := make([][]string, len(series))
	for i, s := range series {
		rows[i] = []string{s.Payee, s.Currency, s.Amount, s.FirstAmount, s.NativeCurrency, s.NativeAmount, s.NativeFirstAmount}
	}
	return rows
}

func Test_run_recurring_narrows_to_one_usd_account_and_converts_it_to_cad(t *testing.T) {
	recurringFXStore(t)

	t.Run("text", func(t *testing.T) {
		stdout, stderr := runRecurring(t, "--account", "US Chequing")

		assert.Empty(t, stderr)
		assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in US Chequing, amounts in CAD",
			monthlyRow("Gym", "CAD (USD)", "21.00", "252.00", "1: USD 12.00 -> USD 15.00 (+25.0%)"),
			monthlyRow("Netflix.com", "CAD (USD)", "14.00", "168.00", ""),
			[]string{"Total", "CAD", "", "", "420.00", "", "", "", ""}),
			stdout)
	})

	t.Run("json", func(t *testing.T) {
		stdout, stderr := runRecurring(t, "--account", "US Chequing", "--json")

		assert.Empty(t, stderr)
		doc := decodeRecurringJSON(t, stdout)
		assert.Equal(t, []recurringIDName{{ID: "acct-usd", Name: "US Chequing"}}, doc.AccountFilter)
		assert.Equal(t, []recurringTotalJSON{{Currency: "CAD", PerYear: "420.00"}}, doc.Totals)
		assert.Equal(t, []string{"USD", "USD"}, []string{doc.Series[0].NativeCurrency, doc.Series[1].NativeCurrency})
	})
}

func Test_run_recurring_converts_an_ended_series_in_a_closed_account_at_the_rate_of_its_latest_charge(t *testing.T) {
	home := newHome(t)
	closed := usdChequingAccount("acct-old", 2)
	closed.Name, closed.Active = "Old USD", false
	old := inUSD(monthlySeries("Old Sub", 2026, time.February, slices.Repeat([]int64{800}, 5)...))
	for i := range old {
		old[i].account = "acct-old"
	}
	replaceStoreWithRates(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1), closed}, old...),
		store.Rate{Date: day(2026, time.January, 2), USDCAD: money.Rate(1_300_000), Series: "FXUSDCAD"},
		store.Rate{Date: day(2026, time.June, 1), USDCAD: money.Rate(1_400_000), Series: "FXUSDCAD"})

	t.Run("text has a blank Per year and no Total", func(t *testing.T) {
		stdout, stderr := runRecurring(t)

		assert.Empty(t, stderr)
		assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in all accounts, amounts in CAD",
			[]string{"Old Sub", "CAD (USD)", "month", "11.20", "", "2026-02-12", "2026-06-12", "ended, new", ""}),
			stdout)
	})

	t.Run("json has a null per_year, no totals and the first amount at the first charge's rate", func(t *testing.T) {
		stdout, stderr := runRecurring(t, "--json")

		assert.Empty(t, stderr)
		doc := decodeRecurringJSON(t, stdout)
		require.Len(t, doc.Series, 1)
		assert.Nil(t, doc.Series[0].PerYear)
		assert.Equal(t, []string{"11.20", "10.40", "8.00", "8.00"},
			[]string{doc.Series[0].Amount, doc.Series[0].FirstAmount, doc.Series[0].NativeAmount, doc.Series[0].NativeFirstAmount})
		assert.Equal(t, []recurringTotalJSON{}, doc.Totals)
	})
}

func Test_run_recurring_lists_a_series_charged_before_the_first_rate_in_its_own_currency_with_a_warning(t *testing.T) {
	home := newHome(t)
	usdGym := inUSD(monthlySeries("Gym", 2026, time.February, slices.Repeat([]int64{1200}, 8)...))
	cadRent := monthlySeries("Rent", 2026, time.February, slices.Repeat([]int64{5000}, 8)...)
	replaceStoreWithRates(t, home,
		chargeRows([]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)}, slices.Concat(usdGym, cadRent)...),
		store.Rate{Date: day(2026, time.April, 1), USDCAD: money.Rate(1_300_000), Series: "FXUSDCAD"},
		store.Rate{Date: day(2026, time.June, 1), USDCAD: money.Rate(1_400_000), Series: "FXUSDCAD"})
	const warning = "1 series with a charge dated before 2026-04-01, the first exchange rate in the store, " +
		"is listed in USD, not converted to CAD"

	t.Run("text shows the USD row plain, a USD Total and the series line", func(t *testing.T) {
		stdout, stderr := runRecurring(t)

		assert.Equal(t, warningLines([]string{warning}), stderr)
		assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in all accounts, amounts in CAD",
			monthlyRow("Rent", "CAD", "50.00", "600.00", ""),
			monthlyRow("Gym", "USD", "12.00", "144.00", ""),
			[]string{"Total", "CAD", "", "", "600.00", "", "", "", ""},
			[]string{"Total", "USD", "", "", "144.00", "", "", "", ""}),
			stdout)
	})

	t.Run("json lists the row's currency apart from the document's and echoes the warning", func(t *testing.T) {
		stdout, stderr := runRecurring(t, "--json")

		assert.Equal(t, warningLines([]string{warning}), stderr)
		doc := decodeRecurringJSON(t, stdout)
		assert.Equal(t, "CAD", doc.Currency)
		assert.Equal(t, []string{warning}, doc.Warnings)
		require.Len(t, doc.Series, 2)
		assert.Equal(t, []string{"USD", "12.00", "USD"}, []string{doc.Series[1].Currency, doc.Series[1].Amount, doc.Series[1].NativeCurrency})
	})
}

func Test_run_recurring_in_usd_lists_cad_series_charged_before_the_first_rate_in_cad_with_a_warning(t *testing.T) {
	home := newHome(t)
	usdGym := inUSD(monthlySeries("Gym", 2026, time.February, slices.Repeat([]int64{1200}, 8)...))
	cadRent := monthlySeries("Rent", 2026, time.February, slices.Repeat([]int64{5000}, 8)...)
	cadPhone := monthlySeries("Phone", 2026, time.February, slices.Repeat([]int64{3000}, 8)...)
	replaceStoreWithRates(t, home,
		chargeRows([]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)}, slices.Concat(usdGym, cadRent, cadPhone)...),
		store.Rate{Date: day(2026, time.April, 1), USDCAD: money.Rate(1_300_000), Series: "FXUSDCAD"},
		store.Rate{Date: day(2026, time.June, 1), USDCAD: money.Rate(1_400_000), Series: "FXUSDCAD"})
	const warning = "2 series with a charge dated before 2026-04-01, the first exchange rate in the store, " +
		"are listed in CAD, not converted to USD"

	t.Run("text lists the CAD rows plain ahead of the USD one", func(t *testing.T) {
		stdout, stderr := runRecurring(t, "--currency", "USD")

		assert.Equal(t, warningLines([]string{warning}), stderr)
		assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in all accounts, amounts in USD",
			monthlyRow("Rent", "CAD", "50.00", "600.00", ""),
			monthlyRow("Phone", "CAD", "30.00", "360.00", ""),
			monthlyRow("Gym", "USD", "12.00", "144.00", ""),
			[]string{"Total", "CAD", "", "", "960.00", "", "", "", ""},
			[]string{"Total", "USD", "", "", "144.00", "", "", "", ""}),
			stdout)
	})

	t.Run("json echoes the warning and names USD as the reporting currency", func(t *testing.T) {
		stdout, stderr := runRecurring(t, "--currency", "USD", "--json")

		assert.Equal(t, warningLines([]string{warning}), stderr)
		doc := decodeRecurringJSON(t, stdout)
		assert.Equal(t, "USD", doc.Currency)
		assert.Equal(t, []string{warning}, doc.Warnings)
		assert.Equal(t, [][]string{
			{"Rent", "CAD", "50.00", "50.00", "CAD", "50.00", "50.00"},
			{"Phone", "CAD", "30.00", "30.00", "CAD", "30.00", "30.00"},
			{"Gym", "USD", "12.00", "12.00", "USD", "12.00", "12.00"},
		}, amountsOf(doc.Series))
	})
}

func Test_run_recurring_on_a_store_without_rates_warns_only_when_a_usd_series_needs_converting(t *testing.T) {
	const noRates = "the store has no exchange rates, so amounts are listed in each account's own currency; run quarry sync to fetch them"
	home := newHome(t)
	usdGym := inUSD(monthlySeries("Gym", 2026, time.February, slices.Repeat([]int64{1200}, 8)...))
	cadRent := monthlySeries("Rent", 2026, time.February, slices.Repeat([]int64{5000}, 8)...)
	accounts := []store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)}

	t.Run("an unrated USD series in CAD draws the no-rates line and stays USD", func(t *testing.T) {
		replaceStore(t, home, chargeRows(accounts, slices.Concat(usdGym, cadRent)...))

		stdout, stderr := runRecurring(t)

		assert.Equal(t, warningLines([]string{noRates}), stderr)
		assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in all accounts, amounts in CAD",
			monthlyRow("Rent", "CAD", "50.00", "600.00", ""),
			monthlyRow("Gym", "USD", "12.00", "144.00", ""),
			[]string{"Total", "CAD", "", "", "600.00", "", "", "", ""},
			[]string{"Total", "USD", "", "", "144.00", "", "", "", ""}),
			stdout)
	})

	t.Run("json for an unrated USD series carries the line and the series' own values as native", func(t *testing.T) {
		replaceStore(t, home, chargeRows(accounts, slices.Concat(usdGym, cadRent)...))

		stdout, stderr := runRecurring(t, "--json")

		assert.Equal(t, warningLines([]string{noRates}), stderr)
		doc := decodeRecurringJSON(t, stdout)
		assert.Equal(t, "CAD", doc.Currency)
		assert.Equal(t, []string{noRates}, doc.Warnings)
		assert.Equal(t, [][]string{
			{"Rent", "CAD", "50.00", "50.00", "CAD", "50.00", "50.00"},
			{"Gym", "USD", "12.00", "12.00", "USD", "12.00", "12.00"},
		}, amountsOf(doc.Series))
	})

	t.Run("all-CAD series in CAD are silent", func(t *testing.T) {
		replaceStore(t, home, chargeRows(accounts, cadRent...))

		stdout, stderr := runRecurring(t)

		assert.Empty(t, stderr)
		assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in all accounts, amounts in CAD",
			monthlyRow("Rent", "CAD", "50.00", "600.00", ""),
			[]string{"Total", "CAD", "", "", "600.00", "", "", "", ""}),
			stdout)
	})

	t.Run("json for all-CAD series in CAD has no warnings", func(t *testing.T) {
		replaceStore(t, home, chargeRows(accounts, cadRent...))

		stdout, stderr := runRecurring(t, "--json")

		assert.Empty(t, stderr)
		doc := decodeRecurringJSON(t, stdout)
		assert.Equal(t, []string{}, doc.Warnings)
		assert.Equal(t, [][]string{{"Rent", "CAD", "50.00", "50.00", "CAD", "50.00", "50.00"}}, amountsOf(doc.Series))
	})

	t.Run("native is silent whatever the store holds", func(t *testing.T) {
		replaceStore(t, home, chargeRows(accounts, slices.Concat(usdGym, cadRent)...))

		_, stderr := runRecurring(t, "--currency", "native")

		assert.Empty(t, stderr)
	})
}

func Test_run_recurring_says_only_that_the_window_is_empty_on_an_unrated_usd_store_in_every_currency(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
		inUSD(monthlySeries("Gym", 2026, time.February, slices.Repeat([]int64{1200}, 8)...))...))
	cases := []struct{ name, flag, wantCaption string }{
		{name: "CAD", flag: "CAD", wantCaption: ", amounts in CAD"},
		{name: "USD", flag: "USD", wantCaption: ", amounts in USD"},
		{name: "native", flag: "native", wantCaption: ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			const emptyLine = "no recurring charges from 2030-01-01 to 2031-12-31; the store's transactions run 2026-02-12 to 2026-09-12"
			args := []string{"--since", "2030", "--until", "2031", "--currency", c.flag}
			var textOut, textErr, jsonOut, jsonErr bytes.Buffer

			textExit := runWith(context.Background(), append([]string{"recurring"}, args...), spendEnv(&textOut, &textErr))
			jsonExit := runWith(context.Background(), append([]string{"recurring", "--json"}, args...), spendEnv(&jsonOut, &jsonErr))

			require.Equal(t, 0, textExit, textErr.String())
			require.Equal(t, 0, jsonExit, jsonErr.String())
			assert.Equal(t, "Recurring charges 2030-01-01 to 2031-12-31 in all accounts"+c.wantCaption+"\n\n"+recurringHeaderOnly, textOut.String())
			assert.Equal(t, warningLines([]string{emptyLine}), textErr.String())
			doc := decodeRecurringJSON(t, jsonOut.String())
			assert.Equal(t, c.flag, doc.Currency)
			assert.Equal(t, []recurringTotalJSON{}, doc.Totals)
			assert.Equal(t, []recurringSeriesJSON{}, doc.Series)
			assert.Equal(t, []string{emptyLine}, doc.Warnings)
		})
	}
}

// recurringIDName is an {id, name} pair of the recurring document.
type recurringIDName struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// recurringPriceChangeJSON is one entry of a series' "price_changes".
type recurringPriceChangeJSON struct {
	Date      string  `json:"date"`
	Currency  string  `json:"currency"`
	From      string  `json:"from"`
	To        string  `json:"to"`
	ChangePct float64 `json:"change_pct"`
}

// recurringSeriesJSON is one entry of the document's "series".
type recurringSeriesJSON struct {
	Payee             string                     `json:"payee"`
	PayeeKey          *string                    `json:"payee_key"`
	Payees            []recurringIDName          `json:"payees"`
	Currency          string                     `json:"currency"`
	Cadence           string                     `json:"cadence"`
	Amount            string                     `json:"amount"`
	FirstAmount       string                     `json:"first_amount"`
	PerYear           *string                    `json:"per_year"`
	NativeCurrency    string                     `json:"native_currency"`
	NativeAmount      string                     `json:"native_amount"`
	NativeFirstAmount string                     `json:"native_first_amount"`
	FirstCharge       string                     `json:"first_charge"`
	LastCharge        string                     `json:"last_charge"`
	ChargeCount       int                        `json:"charge_count"`
	State             string                     `json:"state"`
	New               bool                       `json:"new"`
	Accounts          []recurringIDName          `json:"accounts"`
	PriceChanges      []recurringPriceChangeJSON `json:"price_changes"`
}

// recurringTotalJSON is one entry of the document's "totals".
type recurringTotalJSON struct {
	Currency string `json:"currency"`
	PerYear  string `json:"per_year"`
}

// recurringJSONDoc is quarry recurring --json's stdout.
type recurringJSONDoc struct {
	Since         string                `json:"since"`
	Until         string                `json:"until"`
	Currency      string                `json:"currency"`
	AccountFilter []recurringIDName     `json:"account_filter"`
	Series        []recurringSeriesJSON `json:"series"`
	Totals        []recurringTotalJSON  `json:"totals"`
	Warnings      []string              `json:"warnings"`
}

// decodeRecurringJSON reads stdout as the recurring document, refusing keys the document does not rule.
func decodeRecurringJSON(t *testing.T, stdout string) recurringJSONDoc {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(stdout))
	decoder.DisallowUnknownFields()
	var doc recurringJSONDoc

	require.NoError(t, decoder.Decode(&doc), stdout)
	return doc
}

// monthlySeries is one grocery charge of payee on the 12th of each month from (year, month) on, one per amount in cents.
func monthlySeries(payee string, year int, month time.Month, cents ...int64) []chargeTxn {
	charges := make([]chargeTxn, len(cents))
	for i, c := range cents {
		charges[i] = groceryCharge(payee, day(year, month+time.Month(i), 12), c)
	}
	return charges
}

// inUSD moves charges to the USD account acct-usd, under ids that do not collide with the CAD originals.
func inUSD(charges []chargeTxn) []chargeTxn {
	moved := slices.Clone(charges)
	for i := range moved {
		moved[i].id = "usd-" + moved[i].id
		moved[i].account = "acct-usd"
		moved[i].currency = "USD"
	}
	return moved
}

func Test_run_recurring_json_returns_the_series_document(t *testing.T) {
	home := newHome(t)
	charges := monthlySeries("Netflix.com", 2026, time.February, slices.Concat(slices.Repeat([]int64{999}, 4), slices.Repeat([]int64{1199}, 4))...)
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)}, charges...))

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"recurring", "--since", "2000", "--json"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, recurringJSONDoc{
		Since:         "2000-01-01",
		Until:         "2026-09-29",
		Currency:      "CAD",
		AccountFilter: []recurringIDName{},
		Series: []recurringSeriesJSON{{
			Payee:          "Netflix.com",
			PayeeKey:       new("netflix-com"),
			Payees:         []recurringIDName{{ID: "payee-Netflix.com", Name: "Netflix.com"}},
			Currency:       "CAD",
			Cadence:        "monthly",
			Amount:         "11.99",
			FirstAmount:    "9.99",
			PerYear:        new("143.88"),
			NativeCurrency: "CAD", NativeAmount: "11.99", NativeFirstAmount: "9.99",
			FirstCharge: "2026-02-12",
			LastCharge:  "2026-09-12",
			ChargeCount: 8,
			State:       "active",
			New:         true,
			Accounts:    []recurringIDName{{ID: "acct-cad", Name: "Chequing"}},
			PriceChanges: []recurringPriceChangeJSON{
				{Date: "2026-06-12", Currency: "CAD", From: "9.99", To: "11.99", ChangePct: 20.0},
			},
		}},
		Totals:   []recurringTotalJSON{{Currency: "CAD", PerYear: "143.88"}},
		Warnings: []string{},
	}, decodeRecurringJSON(t, stdout.String()))
}

func Test_run_recurring_merges_payees_differing_in_store_numbers_and_splits_currencies(t *testing.T) {
	home := newHome(t)
	charges := slices.Concat(
		monthlySeries("NETFLIX.COM 1234", 2026, time.April, 1500, 1500, 1500),
		monthlySeries("Netflix.com", 2026, time.July, 1500, 1500, 1500),
		inUSD(monthlySeries("Netflix.com", 2026, time.June, 1200, 1200, 1200, 1200)),
	)
	accounts := []store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)}
	replaceStore(t, home, chargeRows(accounts, charges...))

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"recurring", "--since", "2000", "--json"})

	require.Equal(t, 0, exitCode, stderr.String())
	doc := decodeRecurringJSON(t, stdout.String())
	require.Len(t, doc.Series, 2)
	cad, usd := doc.Series[0], doc.Series[1]
	assert.Equal(t, "CAD", cad.Currency)
	assert.Equal(t, "Netflix.com", cad.Payee)
	assert.Equal(t, 6, cad.ChargeCount)
	assert.Equal(t, []recurringIDName{
		{ID: "payee-NETFLIX.COM 1234", Name: "NETFLIX.COM 1234"},
		{ID: "payee-Netflix.com", Name: "Netflix.com"},
	}, cad.Payees)
	assert.Equal(t, "USD", usd.Currency)
	assert.Equal(t, 4, usd.ChargeCount)
	assert.Equal(t, []recurringIDName{{ID: "payee-Netflix.com", Name: "Netflix.com"}}, usd.Payees)
	assert.Equal(t, []recurringTotalJSON{{Currency: "CAD", PerYear: "180.00"}, {Currency: "USD", PerYear: "144.00"}}, doc.Totals)
}

func Test_run_recurring_json_marks_new_only_for_a_series_first_charged_inside_the_window(t *testing.T) {
	home := newHome(t)
	charges := slices.Concat(
		monthlySeries("Netflix.com", 2026, time.February, slices.Repeat([]int64{1199}, 8)...),
		monthlySeries("Spotify", 2026, time.July, slices.Repeat([]int64{1099}, 3)...),
	)
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)}, charges...))

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"recurring", "--since", "2026-05-01", "--json"})

	require.Equal(t, 0, exitCode, stderr.String())
	isNew := map[string]bool{}
	for _, s := range decodeRecurringJSON(t, stdout.String()).Series {
		isNew[s.Payee] = s.New
	}
	assert.Equal(t, map[string]bool{"Netflix.com": false, "Spotify": true}, isNew)
}

func Test_run_recurring_lists_price_changes_both_ways_from_first_to_latest(t *testing.T) {
	home := newHome(t)
	cents := slices.Concat(slices.Repeat([]int64{999}, 8), slices.Repeat([]int64{1199}, 8), slices.Repeat([]int64{1099}, 8))
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)},
		monthlySeries("Netflix.com", 2024, time.October, cents...)...))
	var textOut, textErr, jsonOut, jsonErr bytes.Buffer

	textCode := runWith(context.Background(), []string{"recurring", "--since", "2000"}, spendEnv(&textOut, &textErr))
	jsonCode := runWith(context.Background(), []string{"recurring", "--since", "2000", "--json"}, spendEnv(&jsonOut, &jsonErr))

	require.Equal(t, 0, textCode, textErr.String())
	require.Equal(t, 0, jsonCode, jsonErr.String())
	assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in all accounts, amounts in CAD",
		[]string{"Netflix.com", "CAD", "month", "10.99", "131.88", "2024-10-12", "2026-09-12", "active, new", "2: 9.99 -> 10.99 (+10.0%)"},
		[]string{"Total", "CAD", "", "", "131.88", "", "", "", ""}),
		textOut.String())
	doc := decodeRecurringJSON(t, jsonOut.String())
	require.Len(t, doc.Series, 1)
	assert.Equal(t, []recurringPriceChangeJSON{
		{Date: "2025-06-12", Currency: "CAD", From: "9.99", To: "11.99", ChangePct: 20.0},
		{Date: "2026-02-12", Currency: "CAD", From: "11.99", To: "10.99", ChangePct: -8.3},
	}, doc.Series[0].PriceChanges)
}

func Test_run_recurring_leaves_out_a_bill_whose_amount_changes_most_months(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)},
		monthlySeries("Hydro", 2026, time.February, 1000, 1200, 1000, 1200, 1000, 1200, 1000, 1200)...))

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"recurring", "--since", "2000"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "quarry: warning: no recurring charges from 2000-01-01 to 2026-09-29; "+
		"the store's transactions run 2026-02-12 to 2026-09-12\n", stderr.String())
	assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in all accounts, amounts in CAD"), stdout.String())
}

// noStoreLine is the refusal for a home that holds no store.
func noStoreLine(t *testing.T, home string) string {
	t.Helper()
	return "quarry: no store at " + abbreviated(t, storePathUnder(home), home) + " yet; run quarry sync to build it\n"
}

func Test_run_recurring_refuses_usage_and_account_problems(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		accounts []store.Account
		wantExit int
		wantLine string
	}{
		{
			name: "an argument", args: []string{"recurring", "extra"}, accounts: []store.Account{chequingAccount("acct-cad", 1)},
			wantExit: 2, wantLine: "quarry: recurring takes no arguments\n",
		},
		{
			name: "a since that is not a date", args: []string{"recurring", "--since", "2026-13"}, accounts: []store.Account{chequingAccount("acct-cad", 1)},
			wantExit: 2, wantLine: "quarry: --since \"2026-13\" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD\n",
		},
		{
			name: "an account no account is named", args: []string{"recurring", "--account", "Nope"}, accounts: []store.Account{chequingAccount("acct-cad", 1)},
			wantExit: 1, wantLine: "quarry: no account named \"Nope\"; run quarry accounts --all to list them\n",
		},
		{
			name: "an account name two accounts share", args: []string{"recurring", "--account", "Visa"},
			accounts: []store.Account{
				{ID: "acct-812", SourceID: 1, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true},
				{ID: "acct-977", SourceID: 2, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true},
			},
			wantExit: 1, wantLine: "quarry: 2 accounts are named \"Visa\"; pass one of their ids instead: acct-812, acct-977\n",
		},
		{
			name: "an account no account is named, with --json", args: []string{"recurring", "--json", "--account", "Nope"},
			accounts: []store.Account{chequingAccount("acct-cad", 1)},
			wantExit: 1, wantLine: "quarry: no account named \"Nope\"; run quarry accounts --all to list them\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			replaceStore(t, home, chargeRows(c.accounts))

			exitCode, stdout, stderr := runSpendCapture(context.Background(), c.args)

			assert.Equal(t, c.wantExit, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantLine, stderr.String())
		})
	}
}

func Test_run_recurring_refuses_when_there_is_no_store(t *testing.T) {
	home := newHome(t)

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"recurring"})

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, noStoreLine(t, home), stderr.String())
}

// HOME holds no store: exit 2 (not the missing-store 1) shows each check runs first.
func Test_run_recurring_rejects_a_period_it_cannot_use(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{
			name:       "a since after until",
			args:       []string{"recurring", "--since", "2025", "--until", "2024"},
			wantStderr: "quarry: --since 2025 is after --until 2024\n",
		},
		{
			name:       "an until before the default since",
			args:       []string{"recurring", "--until", "2024"},
			wantStderr: "quarry: --until 2024 is before the default --since 2026-01-01; pass --since too\n",
		},
		{
			name:       "a since after today",
			args:       []string{"recurring", "--since", "2099"},
			wantStderr: "quarry: --since 2099 is after today; recurring lists charges up to today only, so pass an earlier --since\n",
		},
		{
			name:       "a since after today, with --json",
			args:       []string{"recurring", "--json", "--since", "2099"},
			wantStderr: "quarry: --since 2099 is after today; recurring lists charges up to today only, so pass an earlier --since\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())

			exitCode, stdout, stderr := runSpendCapture(context.Background(), c.args)

			assert.Equal(t, 2, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}

// quietCase is a steady 10.00 series of count charges gapDays apart whose last charge came quietDays before today.
type quietCase struct {
	name      string
	every     string
	gapDays   int
	count     int
	quietDays int
	perYear   string
}

// quietSeriesOutput runs quarry recurring over the series c describes and returns stdout, then the series' first and last dates.
func quietSeriesOutput(t *testing.T, c quietCase) (string, string, string) {
	t.Helper()
	home := newHome(t)
	lastCharge := day(2026, time.September, 29).AddDate(0, 0, -c.quietDays)
	var charges []chargeTxn
	for i := c.count - 1; i >= 0; i-- {
		charges = append(charges, groceryCharge("Gym", lastCharge.AddDate(0, 0, -c.gapDays*i), 1000))
	}
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)}, charges...))
	var out, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"recurring", "--since", "2000"}, spendEnv(&out, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	return out.String(), lastCharge.AddDate(0, 0, -c.gapDays*(c.count-1)).Format(time.DateOnly), lastCharge.Format(time.DateOnly)
}

func Test_run_recurring_keeps_a_series_active_on_the_last_day_of_its_cadences_quiet_period(t *testing.T) {
	cases := []quietCase{
		{name: "weekly after 14 days", every: "week", gapDays: 7, count: 4, quietDays: 14, perYear: "520.00"},
		{name: "monthly after 45 days", every: "month", gapDays: 30, count: 3, quietDays: 45, perYear: "120.00"},
		{name: "quarterly after 120 days", every: "quarter", gapDays: 91, count: 3, quietDays: 120, perYear: "40.00"},
		{name: "annual after 400 days", every: "year", gapDays: 365, count: 2, quietDays: 400, perYear: "10.00"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stdout, first, last := quietSeriesOutput(t, c)

			assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in all accounts, amounts in CAD",
				[]string{"Gym", "CAD", c.every, "10.00", c.perYear, first, last, "active, new", ""},
				[]string{"Total", "CAD", "", "", c.perYear, "", "", "", ""}),
				stdout)
		})
	}
}

func Test_run_recurring_marks_a_series_ended_one_day_past_its_cadences_quiet_period(t *testing.T) {
	cases := []quietCase{
		{name: "weekly after 15 days", every: "week", gapDays: 7, count: 4, quietDays: 15},
		{name: "monthly after 46 days", every: "month", gapDays: 30, count: 3, quietDays: 46},
		{name: "quarterly after 121 days", every: "quarter", gapDays: 91, count: 3, quietDays: 121},
		{name: "annual after 401 days", every: "year", gapDays: 365, count: 2, quietDays: 401},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stdout, first, last := quietSeriesOutput(t, c)

			assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in all accounts, amounts in CAD",
				[]string{"Gym", "CAD", c.every, "10.00", "", first, last, "ended, new", ""}),
				stdout)
		})
	}
}

func Test_run_recurring_marks_a_series_first_charged_in_the_window_as_new(t *testing.T) {
	home := newHome(t)
	var charges []chargeTxn
	for month := time.March; month <= time.September; month++ {
		charges = append(charges, groceryCharge("Crave", day(2026, month, 2), 1500))
	}
	charges = append(charges, groceryCharge("Rogers", day(2025, time.December, 3), 2500))
	for month := time.January; month <= time.September; month++ {
		charges = append(charges, groceryCharge("Rogers", day(2026, month, 3), 2500))
	}
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)}, charges...))

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"recurring"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, recurringTable("Recurring charges 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD",
		[]string{"Rogers", "CAD", "month", "25.00", "300.00", "2025-12-03", "2026-09-03", "active", ""},
		[]string{"Crave", "CAD", "month", "15.00", "180.00", "2026-03-02", "2026-09-02", "active, new", ""},
		[]string{"Total", "CAD", "", "", "480.00", "", "", "", ""}),
		stdout.String())
}
