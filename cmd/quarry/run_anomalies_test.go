package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// anomaliesHeader is the header cells of the anomalies table.
var anomaliesHeader = []string{"Date", "Account", "Payee", "Category", "Amount", "Usual", "Times", "Compared with"}

// anomaliesRightAligned are the table's columns of numbers, which pad on the left.
var anomaliesRightAligned = map[int]bool{4: true, 5: true, 6: true}

// anomaliesTable is the anomalies table under caption followed by a blank line and footer: every cell
// but the last padded to its column's widest cell, two-space gaps, numbers right-aligned, trailing spaces trimmed.
func anomaliesTable(caption, footer string, rows ...[]string) string {
	return caption + "\n\n" + paddedTable(anomaliesHeader, anomaliesRightAligned, rows) + "\n" + footer + "\n"
}

// anomaliesCADTable is anomaliesTable for the default period over all accounts, amounts in CAD.
func anomaliesCADTable(footer string, rows ...[]string) string {
	return anomaliesTable("Unusually large charges 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD", footer, rows...)
}

// weeklyCharges is one grocery charge of payee a week from 2025-03-03 on, one per amount in cents.
func weeklyCharges(payee string, cents ...int64) []chargeTxn {
	charges := make([]chargeTxn, len(cents))
	for i, c := range cents {
		charges[i] = groceryCharge(payee, day(2025, time.March, 3+7*i), c)
	}
	return charges
}

// emptyWindowAccounts are the accounts of the empty-window tests: one in reports, one without transactions,
// one left out of reports and one on linked tracking.
func emptyWindowAccounts() []store.Account {
	return []store.Account{
		chequingAccount("acct-cad", 1),
		{ID: "acct-visa", SourceID: 2, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true},
		{ID: "acct-old", SourceID: 3, Name: "Old Card", Type: "credit_card", Currency: "CAD", Active: true, NotInReports: true},
		{ID: "acct-401k", SourceID: 4, Name: "Linked", Type: "chequing", Currency: "CAD", Active: true, LinkedTracking: true},
	}
}

// spendRun is what one command line exited with and printed.
type spendRun struct {
	exitCode       int
	stdout, stderr string
}

// runSpendTextAndJSON runs command with args over the spend clock, once as text and once with --json after the command;
// it returns the text run, then the JSON one.
func runSpendTextAndJSON(command string, args ...string) (spendRun, spendRun) {
	var textOut, textErr, jsonOut, jsonErr bytes.Buffer
	textExit := runWith(context.Background(), append([]string{command}, args...), spendEnv(&textOut, &textErr))
	jsonExit := runWith(context.Background(), append([]string{command, "--json"}, args...), spendEnv(&jsonOut, &jsonErr))
	return spendRun{textExit, textOut.String(), textErr.String()}, spendRun{jsonExit, jsonOut.String(), jsonErr.String()}
}

func Test_run_anomalies_lists_a_charge_over_twice_the_payees_usual(t *testing.T) {
	home := newHome(t)
	charges := append(weeklyCharges("Bell Canada", 9000, 9300, 9605, 9900, 10200), groceryCharge("Bell Canada", day(2026, time.March, 2), 41200))
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)}, charges...))

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"anomalies"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, anomaliesCADTable("1 charge checked",
		[]string{"2026-03-02", "Chequing (CAD)", "Bell Canada", "Food:Groceries", "412.00", "96.05", "4.3x", "payee, 5 earlier"}),
		stdout.String())
}

func Test_run_anomalies_account_judges_the_named_accounts_charge_against_history_from_every_account(t *testing.T) {
	home := newHome(t)
	visa := store.Account{ID: "acct-visa", SourceID: 2, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true}
	onVisa := groceryCharge("Bell Canada", day(2026, time.March, 2), 41200)
	onVisa.account = "acct-visa"
	charges := append(weeklyCharges("Bell Canada", 9000, 9300, 9605), onVisa, groceryCharge("Bell Canada", day(2026, time.April, 6), 50000))
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1), visa}, charges...))

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"anomalies", "--account", "Visa"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, anomaliesTable("Unusually large charges 2026-01-01 to 2026-09-29 in Visa, amounts in CAD", "1 charge checked",
		[]string{"2026-03-02", "Visa (CAD)", "Bell Canada", "Food:Groceries", "412.00", "93.00", "4.4x", "payee, 3 earlier"}),
		stdout.String())
}

// historyWithBigCharge is three 2025 charges of Bell Canada on Chequing, then a 412.00 charge in 2026 on account.
func historyWithBigCharge(account string) []chargeTxn {
	charges := weeklyCharges("Bell Canada", 9000, 9300, 9605)
	big := groceryCharge("Bell Canada", day(2026, time.March, 2), 41200)
	big.account = account
	return append(charges, big)
}

func Test_run_anomalies_lists_a_charge_in_a_closed_account(t *testing.T) {
	home := newHome(t)
	oldCard := store.Account{ID: "acct-old", SourceID: 2, Name: "Old Card", Type: "credit_card", Currency: "CAD", Closed: true}
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1), oldCard}, historyWithBigCharge("acct-old")...))

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"anomalies", "--account", "Old Card"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, anomaliesTable("Unusually large charges 2026-01-01 to 2026-09-29 in Old Card, amounts in CAD", "1 charge checked",
		[]string{"2026-03-02", "Old Card (CAD, closed)", "Bell Canada", "Food:Groceries", "412.00", "93.00", "4.4x", "payee, 3 earlier"}),
		stdout.String())
}

func Test_run_anomalies_json_names_the_account_and_counts_only_its_charges(t *testing.T) {
	home := newHome(t)
	visa := store.Account{ID: "acct-visa", SourceID: 2, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true}
	charges := append(historyWithBigCharge("acct-visa"), groceryCharge("Bell Canada", day(2026, time.April, 6), 500))
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1), visa}, charges...))

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"anomalies", "--json", "--account", "Visa"})

	require.Equal(t, 0, exitCode, stderr.String())
	doc := decodeAnomaliesJSON(t, stdout.String())
	assert.Equal(t, []recurringIDName{{ID: "acct-visa", Name: "Visa"}}, doc.AccountFilter)
	assert.Len(t, doc.Anomalies, 1)
	assert.Equal(t, 1, doc.Checked)
	assert.Equal(t, 0, doc.NotJudged)
}

func Test_run_anomalies_judges_a_first_time_payee_against_its_category(t *testing.T) {
	home := newHome(t)
	charges := make([]chargeTxn, 0, 11)
	for i, cents := range []int64{19000, 19500, 20000, 20500, 21040, 21040, 21500, 22000, 22500, 23000} {
		charges = append(charges, groceryCharge(fmt.Sprintf("Vendor %d", i), day(2025, time.March, 3+7*i), cents))
	}
	charges = append(charges, groceryCharge("Home Depot", day(2026, time.August, 14), 184210))
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)}, charges...))

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"anomalies"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, anomaliesCADTable("1 charge checked",
		[]string{"2026-08-14", "Chequing (CAD)", "Home Depot", "Food:Groceries", "1,842.10", "210.40", "8.8x", "category, 10 earlier"}),
		stdout.String())
}

func Test_run_anomalies_counts_an_uncategorized_first_time_charge_as_not_judged(t *testing.T) {
	home := newHome(t)
	uncategorized := chargeTxn{
		id: "tool-shed", account: "acct-cad", payee: "Tool Shed", currency: "CAD", day: day(2026, time.June, 9),
		splits: []chargeSplit{{cents: -15000}},
	}
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)},
		uncategorized, groceryCharge("Corner Store", day(2026, time.June, 10), 4500)))

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"anomalies"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, anomaliesCADTable("2 charges checked; 1 had too little history to judge"), stdout.String())
}

func Test_run_anomalies_says_when_no_charge_falls_in_the_window(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)},
		groceryCharge("Bakery", day(2003, 1, 4), 1000),
		groceryCharge("Bakery", day(2025, 12, 31), 500)))

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"anomalies"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, anomaliesCADTable("0 charges checked"), stdout.String())
	assert.Equal(t, "quarry: warning: no unusually large charges from 2026-01-01 to 2026-09-29; "+
		"the store's transactions run 2003-01-04 to 2025-12-31\n", stderr.String())
}

const (
	anomaliesEmptyWindow = "no unusually large charges from 2026-01-01 to 2026-09-29"
	anomaliesOldCardLine = "account \"Old Card\" is not used in reports in Quicken, so anomalies leaves it out; " +
		"to include it, turn on reports for it in Quicken's account settings, then run quarry sync"
	anomaliesLinkedLine   = "account \"Linked\" uses linked account tracking in Quicken, so anomalies leaves it out, as Quicken's reports do"
	anomaliesAccountsSpan = "their transactions run 2003-01-04 to 2025-12-31"
)

func Test_run_anomalies_prints_each_empty_window_warning_on_stderr_and_in_the_json(t *testing.T) {
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
			wantWarns:   []string{anomaliesEmptyWindow + "; the store has no transactions"},
		},
		{
			name: "the named account has transactions, none in the period", args: []string{"--account", "Chequing"}, charges: bakeryHistory(),
			wantCaption: "Chequing",
			wantWarns:   []string{anomaliesEmptyWindow + " in the named accounts; " + anomaliesAccountsSpan},
		},
		{
			name: "the named account has no transactions", args: []string{"--account", "Visa"}, charges: bakeryHistory(),
			wantCaption: "Visa",
			wantWarns:   []string{anomaliesEmptyWindow + " in the named accounts; they have no transactions"},
		},
		{
			name: "the only named account is not in reports", args: []string{"--account", "Old Card"}, charges: bakeryHistory(),
			wantCaption: "Old Card",
			wantWarns:   []string{anomaliesOldCardLine},
		},
		{
			name: "the only named account uses linked tracking", args: []string{"--account", "Linked"}, charges: bakeryHistory(),
			wantCaption: "Linked",
			wantWarns:   []string{anomaliesLinkedLine},
		},
		{
			name: "an account not in reports named beside one that is", args: []string{"--account", "Old Card", "--account", "Chequing"}, charges: bakeryHistory(),
			wantCaption: "Old Card, Chequing",
			wantWarns:   []string{anomaliesOldCardLine, anomaliesEmptyWindow + " in the named accounts; " + anomaliesAccountsSpan},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			replaceStore(t, home, chargeRows(emptyWindowAccounts(), c.charges...))

			text, jsonRun := runSpendTextAndJSON("anomalies", c.args...)

			require.Equal(t, 0, text.exitCode, text.stderr)
			require.Equal(t, 0, jsonRun.exitCode, jsonRun.stderr)
			wantStderr := warningLines(c.wantWarns)
			assert.Equal(t, anomaliesTable("Unusually large charges 2026-01-01 to 2026-09-29 in "+c.wantCaption+", amounts in CAD", "0 charges checked"), text.stdout)
			assert.Equal(t, wantStderr, text.stderr)
			assert.Equal(t, wantStderr, jsonRun.stderr)
			var doc struct {
				Warnings []string `json:"warnings"`
			}
			require.NoError(t, json.Unmarshal([]byte(jsonRun.stdout), &doc), jsonRun.stdout)
			assert.Equal(t, c.wantWarns, doc.Warnings)
		})
	}
}

func Test_run_anomalies_prints_no_warning_when_charges_were_checked_but_none_is_unusual(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)},
		groceryCharge("Bakery", day(2026, 3, 1), 500)))

	text, jsonRun := runSpendTextAndJSON("anomalies")

	require.Equal(t, 0, text.exitCode, text.stderr)
	require.Equal(t, 0, jsonRun.exitCode, jsonRun.stderr)
	assert.Equal(t, anomaliesCADTable("1 charge checked"), text.stdout)
	assert.Empty(t, text.stderr)
	assert.Empty(t, jsonRun.stderr)
	var doc struct {
		Warnings []string `json:"warnings"`
		Checked  int      `json:"checked"`
	}
	require.NoError(t, json.Unmarshal([]byte(jsonRun.stdout), &doc), jsonRun.stdout)
	assert.Equal(t, []string{}, doc.Warnings)
	assert.Equal(t, 1, doc.Checked)
}

// anomaliesFXStore holds, under a fresh HOME, a USD Hardware payee with five earlier charges (median 40.00),
// a 250.00 charge on 2026-03-02 and a 90.00 charge on 2026-05-04. The rates are 1.30 for the earlier charges,
// 1.40 from the 250.00 charge's date and, after both, 1.50.
func anomaliesFXStore(t *testing.T) {
	t.Helper()
	home := newHome(t)
	charges := append(hardwareHistory(), bigHardware(), groceryCharge("Hardware", day(2026, time.May, 4), 9000))
	replaceStoreWithRates(t, home,
		chargeRows([]store.Account{usdChequingAccount("acct-usd", 1)}, inUSD(charges)...),
		store.Rate{Date: day(2025, time.January, 2), USDCAD: money.Rate(1_300_000), Series: "FXUSDCAD"},
		store.Rate{Date: day(2026, time.March, 2), USDCAD: money.Rate(1_400_000), Series: "FXUSDCAD"},
		store.Rate{Date: day(2026, time.September, 20), USDCAD: money.Rate(1_500_000), Series: "FXUSDCAD"},
	)
}

func Test_run_anomalies_judges_in_native_currency_and_shows_converted_amounts(t *testing.T) {
	anomaliesFXStore(t)

	t.Run("text converts Amount and Usual at the charge's rate and keeps Times and the footer native", func(t *testing.T) {
		exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"anomalies"})

		require.Equal(t, 0, exitCode, stderr.String())
		assert.Empty(t, stderr.String())
		assert.Equal(t, anomaliesCADTable("2 charges checked",
			[]string{"2026-03-02", "US Chequing (USD)", "Hardware", "Food:Groceries", "350.00", "56.00", "6.3x", "payee, 5 earlier"}),
			stdout.String())
	})

	t.Run("json carries the native currency, amount and usual beside the converted ones", func(t *testing.T) {
		exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"anomalies", "--json"})

		require.Equal(t, 0, exitCode, stderr.String())
		assert.Equal(t, anomaliesJSONDoc{
			Since:         "2026-01-01",
			Until:         "2026-09-29",
			Currency:      "CAD",
			AccountFilter: []recurringIDName{},
			Anomalies: []anomalyJSON{{
				TransactionID:  "txn-usd-Hardware2026-03-02",
				Date:           "2026-03-02",
				AccountID:      "acct-usd",
				Account:        "US Chequing",
				Currency:       "CAD",
				Payee:          new("Hardware"),
				Category:       new("Food:Groceries"),
				Amount:         "350.00",
				Baseline:       "payee",
				Usual:          "56.00",
				NativeCurrency: "USD",
				NativeAmount:   "250.00",
				NativeUsual:    "40.00",
				Earlier:        5,
				Times:          6.3,
			}},
			Checked:   2,
			NotJudged: 0,
			Warnings:  []string{},
		}, decodeAnomaliesJSON(t, stdout.String()))
	})
}

const (
	beforeAprilUSDInCAD = "1 charge dated before 2026-04-01, the first exchange rate in the store, is listed in USD, not converted to CAD"
	beforeAprilCADInUSD = "1 charge dated before 2026-04-01, the first exchange rate in the store, is listed in CAD, not converted to USD"
)

// runAnomaliesOK runs quarry anomalies with args and returns stdout and stderr; it must exit 0.
func runAnomaliesOK(t *testing.T, args ...string) (string, string) {
	t.Helper()

	exitCode, stdout, stderr := runSpendCapture(context.Background(), append([]string{"anomalies"}, args...))

	require.Equal(t, 0, exitCode, stderr.String())
	return stdout.String(), stderr.String()
}

// jsonCells is the currency, amount and usual of an entry, then its native twins.
func jsonCells(a anomalyJSON) []string {
	return []string{a.Currency, a.Amount, a.Usual, a.NativeCurrency, a.NativeAmount, a.NativeUsual}
}

// hardwareHistory is five Hardware charges of 38.00 to 42.00 (median 40.00), weekly from 2025-03-03.
func hardwareHistory() []chargeTxn {
	return weeklyCharges("Hardware", 3800, 3900, 4000, 4100, 4200)
}

// usdRate is a FXUSDCAD rate of rate millionths from date.
func usdRate(date time.Time, rate int64) store.Rate {
	return store.Rate{Date: date, USDCAD: money.Rate(rate), Series: "FXUSDCAD"}
}

// anomaliesStore stores charges on accounts under a fresh HOME, with rates when any are given.
func anomaliesStore(t *testing.T, accounts []store.Account, charges []chargeTxn, rates ...store.Rate) {
	t.Helper()
	home := newHome(t)
	if len(rates) == 0 {
		replaceStore(t, home, chargeRows(accounts, charges...))
		return
	}
	replaceStoreWithRates(t, home, chargeRows(accounts, charges...), rates...)
}

// bigHardware is the 250.00 Hardware charge of 2026-03-02 that history makes unusual.
func bigHardware() chargeTxn { return groceryCharge("Hardware", day(2026, time.March, 2), 25000) }

// hardwareRow is the row bigHardware makes in account, with Amount and Usual cells as given.
func hardwareRow(account, amount, usual string) []string {
	return []string{"2026-03-02", account, "Hardware", "Food:Groceries", amount, usual, "6.3x", "payee, 5 earlier"}
}

func Test_run_anomalies_in_usd_lists_a_usd_charge_as_it_was_charged_and_says_nothing(t *testing.T) {
	anomaliesStore(t, []store.Account{usdChequingAccount("acct-usd", 1)}, inUSD(append(hardwareHistory(), bigHardware())),
		usdRate(day(2025, time.January, 2), 1_300_000))

	t.Run("text", func(t *testing.T) {
		stdout, stderr := runAnomaliesOK(t, "--currency", "USD")

		assert.Empty(t, stderr)
		assert.Equal(t, anomaliesTable("Unusually large charges 2026-01-01 to 2026-09-29 in all accounts, amounts in USD", "1 charge checked",
			hardwareRow("US Chequing (USD)", "250.00", "40.00")), stdout)
	})

	t.Run("json", func(t *testing.T) {
		stdout, stderr := runAnomaliesOK(t, "--currency", "USD", "--json")

		assert.Empty(t, stderr)
		doc := decodeAnomaliesJSON(t, stdout)
		assert.Equal(t, []string{}, doc.Warnings)
		assert.Equal(t, "USD", doc.Currency)
		require.Len(t, doc.Anomalies, 1)
		assert.Equal(t, []string{"USD", "250.00", "40.00", "USD", "250.00", "40.00"}, jsonCells(doc.Anomalies[0]))
	})
}

func Test_run_anomalies_lists_a_usd_charge_before_the_first_rate_in_usd_with_a_warning(t *testing.T) {
	anomaliesStore(t, []store.Account{usdChequingAccount("acct-usd", 1)}, inUSD(append(hardwareHistory(), bigHardware())),
		usdRate(day(2026, time.April, 1), 1_300_000))

	t.Run("text prefixes both cells with USD", func(t *testing.T) {
		stdout, stderr := runAnomaliesOK(t)

		assert.Equal(t, warningLines([]string{beforeAprilUSDInCAD}), stderr)
		assert.Equal(t, anomaliesCADTable("1 charge checked",
			hardwareRow("US Chequing (USD)", "USD 250.00", "USD 40.00")), stdout)
	})

	t.Run("json echoes the warning", func(t *testing.T) {
		stdout, stderr := runAnomaliesOK(t, "--json")

		assert.Equal(t, warningLines([]string{beforeAprilUSDInCAD}), stderr)
		doc := decodeAnomaliesJSON(t, stdout)
		assert.Equal(t, []string{beforeAprilUSDInCAD}, doc.Warnings)
		assert.Equal(t, "CAD", doc.Currency)
		require.Len(t, doc.Anomalies, 1)
		assert.Equal(t, []string{"USD", "250.00", "40.00", "USD", "250.00", "40.00"}, jsonCells(doc.Anomalies[0]))
	})
}

func Test_run_anomalies_in_usd_lists_a_cad_charge_before_the_first_rate_in_cad_with_a_warning(t *testing.T) {
	anomaliesStore(t, []store.Account{chequingAccount("acct-cad", 1)}, append(hardwareHistory(), bigHardware()),
		usdRate(day(2026, time.April, 1), 1_300_000))

	t.Run("text prefixes both cells with CAD", func(t *testing.T) {
		stdout, stderr := runAnomaliesOK(t, "--currency", "USD")

		assert.Equal(t, warningLines([]string{beforeAprilCADInUSD}), stderr)
		assert.Equal(t, anomaliesTable("Unusually large charges 2026-01-01 to 2026-09-29 in all accounts, amounts in USD", "1 charge checked",
			hardwareRow("Chequing (CAD)", "CAD 250.00", "CAD 40.00")), stdout)
	})

	t.Run("json echoes the warning", func(t *testing.T) {
		stdout, stderr := runAnomaliesOK(t, "--currency", "USD", "--json")

		assert.Equal(t, warningLines([]string{beforeAprilCADInUSD}), stderr)
		doc := decodeAnomaliesJSON(t, stdout)
		assert.Equal(t, []string{beforeAprilCADInUSD}, doc.Warnings)
		assert.Equal(t, "USD", doc.Currency)
		require.Len(t, doc.Anomalies, 1)
		assert.Equal(t, []string{"CAD", "250.00", "40.00", "CAD", "250.00", "40.00"}, jsonCells(doc.Anomalies[0]))
	})
}

func Test_run_anomalies_counts_each_unconverted_listed_charge_and_says_are(t *testing.T) {
	const want = "2 charges dated before 2026-04-01, the first exchange rate in the store, are listed in USD, not converted to CAD"
	other := append(weeklyCharges("Lumber", 3800, 3900, 4000, 4100, 4200), groceryCharge("Lumber", day(2026, time.March, 9), 25000))
	anomaliesStore(t, []store.Account{usdChequingAccount("acct-usd", 1)}, inUSD(append(append(hardwareHistory(), bigHardware()), other...)),
		usdRate(day(2026, time.April, 1), 1_300_000))

	t.Run("text", func(t *testing.T) {
		stdout, stderr := runAnomaliesOK(t)

		assert.Equal(t, warningLines([]string{want}), stderr)
		assert.Equal(t, anomaliesCADTable("2 charges checked",
			[]string{"2026-03-09", "US Chequing (USD)", "Lumber", "Food:Groceries", "USD 250.00", "USD 40.00", "6.3x", "payee, 5 earlier"},
			hardwareRow("US Chequing (USD)", "USD 250.00", "USD 40.00")), stdout)
	})

	t.Run("json", func(t *testing.T) {
		stdout, stderr := runAnomaliesOK(t, "--json")

		assert.Equal(t, warningLines([]string{want}), stderr)
		doc := decodeAnomaliesJSON(t, stdout)
		assert.Equal(t, []string{want}, doc.Warnings)
		require.Len(t, doc.Anomalies, 2)
		for _, entry := range doc.Anomalies {
			assert.Equal(t, []string{"USD", "250.00", "40.00", "USD", "250.00", "40.00"}, jsonCells(entry))
		}
	})
}

func Test_run_anomalies_on_a_store_without_rates_warns_only_when_a_usd_charge_needs_converting(t *testing.T) {
	t.Run("an unrated USD charge in CAD draws the no-rates line and stays USD", func(t *testing.T) {
		anomaliesStore(t, []store.Account{usdChequingAccount("acct-usd", 1)}, inUSD(append(hardwareHistory(), bigHardware())))

		stdout, stderr := runAnomaliesOK(t)

		assert.Equal(t, warningLines([]string{noRatesLine}), stderr)
		assert.Equal(t, anomaliesCADTable("1 charge checked",
			hardwareRow("US Chequing (USD)", "USD 250.00", "USD 40.00")), stdout)
	})

	t.Run("json carries the line", func(t *testing.T) {
		anomaliesStore(t, []store.Account{usdChequingAccount("acct-usd", 1)}, inUSD(append(hardwareHistory(), bigHardware())))

		stdout, stderr := runAnomaliesOK(t, "--json")

		assert.Equal(t, warningLines([]string{noRatesLine}), stderr)
		doc := decodeAnomaliesJSON(t, stdout)
		assert.Equal(t, []string{noRatesLine}, doc.Warnings)
		assert.Equal(t, "CAD", doc.Currency)
		require.Len(t, doc.Anomalies, 1)
		assert.Equal(t, []string{"USD", "250.00", "40.00", "USD", "250.00", "40.00"}, jsonCells(doc.Anomalies[0]))
	})

	t.Run("an all-CAD store in CAD is silent and plain", func(t *testing.T) {
		anomaliesStore(t, []store.Account{chequingAccount("acct-cad", 1)}, append(hardwareHistory(), bigHardware()))

		stdout, stderr := runAnomaliesOK(t)

		assert.Empty(t, stderr)
		assert.Equal(t, anomaliesCADTable("1 charge checked",
			hardwareRow("Chequing (CAD)", "250.00", "40.00")), stdout)
	})

	t.Run("an all-CAD store in CAD is silent in json too", func(t *testing.T) {
		anomaliesStore(t, []store.Account{chequingAccount("acct-cad", 1)}, append(hardwareHistory(), bigHardware()))

		stdout, stderr := runAnomaliesOK(t, "--json")

		assert.Empty(t, stderr)
		doc := decodeAnomaliesJSON(t, stdout)
		assert.Equal(t, []string{}, doc.Warnings)
		assert.Equal(t, "CAD", doc.Currency)
		require.Len(t, doc.Anomalies, 1)
		assert.Equal(t, []string{"CAD", "250.00", "40.00", "CAD", "250.00", "40.00"}, jsonCells(doc.Anomalies[0]))
	})
}

func Test_run_anomalies_native_on_a_rated_usd_store_prints_what_it_printed_before_conversion_existed(t *testing.T) {
	anomaliesFXStore(t)

	t.Run("text has no amounts clause and the USD cells", func(t *testing.T) {
		stdout, stderr := runAnomaliesOK(t, "--currency", "native")

		assert.Empty(t, stderr)
		assert.Equal(t, anomaliesTable("Unusually large charges 2026-01-01 to 2026-09-29 in all accounts", "2 charges checked",
			hardwareRow("US Chequing (USD)", "250.00", "40.00")), stdout)
	})

	t.Run("json", func(t *testing.T) {
		stdout, stderr := runAnomaliesOK(t, "--currency", "native", "--json")

		assert.Empty(t, stderr)
		doc := decodeAnomaliesJSON(t, stdout)
		assert.Equal(t, []string{}, doc.Warnings)
		assert.Equal(t, "native", doc.Currency)
		require.Len(t, doc.Anomalies, 1)
		assert.Equal(t, []string{"USD", "250.00", "40.00", "USD", "250.00", "40.00"}, jsonCells(doc.Anomalies[0]))
	})
}

func Test_run_anomalies_converts_a_category_baseline_anomaly_with_no_payee(t *testing.T) {
	history := make([]chargeTxn, 0, 11)
	for i := range 10 {
		history = append(history, groceryCharge("", day(2025, time.March, 3+7*i), 4000))
	}
	history = append(history, groceryCharge("", day(2026, time.March, 2), 60000))
	anomaliesStore(t, []store.Account{usdChequingAccount("acct-usd", 1)}, inUSD(history),
		usdRate(day(2025, time.January, 2), 1_300_000), usdRate(day(2026, time.March, 2), 1_400_000))
	want := []string{"2026-03-02", "US Chequing (USD)", "(no payee)", "Food:Groceries", "840.00", "56.00", "15.0x", "category, 10 earlier"}

	t.Run("text", func(t *testing.T) {
		stdout, stderr := runAnomaliesOK(t)

		assert.Empty(t, stderr)
		assert.Equal(t, anomaliesCADTable("1 charge checked", want), stdout)
	})

	t.Run("json lists one entry without a payee", func(t *testing.T) {
		stdout, stderr := runAnomaliesOK(t, "--json")

		assert.Empty(t, stderr)
		doc := decodeAnomaliesJSON(t, stdout)
		require.Len(t, doc.Anomalies, 1)
		assert.Nil(t, doc.Anomalies[0].Payee)
		assert.Equal(t, "category", doc.Anomalies[0].Baseline)
		assert.Equal(t, "CAD", doc.Currency)
		assert.Equal(t, []string{"CAD", "840.00", "56.00", "USD", "600.00", "40.00"}, jsonCells(doc.Anomalies[0]))
	})
}

func Test_run_anomalies_keeps_the_not_judged_count_in_every_reporting_currency(t *testing.T) {
	const footer = "2 charges checked; 1 had too little history to judge"
	lone := groceryCharge("Lone", day(2026, time.April, 6), 20000)
	anomaliesStore(t, []store.Account{usdChequingAccount("acct-usd", 1)}, inUSD(append(append(hardwareHistory(), bigHardware()), lone)),
		usdRate(day(2025, time.January, 2), 1_300_000))

	for _, currency := range []string{"CAD", "USD", "native"} {
		t.Run(currency, func(t *testing.T) {
			text, textStderr := runAnomaliesOK(t, "--currency", currency)
			js, docStderr := runAnomaliesOK(t, "--currency", currency, "--json")

			assert.Empty(t, textStderr)
			assert.Empty(t, docStderr)
			assert.True(t, strings.HasSuffix(text, "\n"+footer+"\n"), text)
			doc := decodeAnomaliesJSON(t, js)
			assert.Equal(t, []int{2, 1}, []int{doc.Checked, doc.NotJudged})
			assert.Equal(t, currency, doc.Currency)
		})
	}
}

func Test_run_anomalies_narrows_to_one_usd_account_and_converts_it_to_cad(t *testing.T) {
	accounts := []store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)}
	anomaliesStore(t, accounts, inUSD(append(hardwareHistory(), bigHardware())),
		usdRate(day(2025, time.January, 2), 1_300_000), usdRate(day(2026, time.March, 2), 1_400_000))

	t.Run("text names the account and the currency", func(t *testing.T) {
		stdout, stderr := runAnomaliesOK(t, "--account", "US Chequing")

		assert.Empty(t, stderr)
		assert.Equal(t, anomaliesTable("Unusually large charges 2026-01-01 to 2026-09-29 in US Chequing, amounts in CAD", "1 charge checked",
			hardwareRow("US Chequing (USD)", "350.00", "56.00")), stdout)
	})

	t.Run("json", func(t *testing.T) {
		stdout, stderr := runAnomaliesOK(t, "--account", "US Chequing", "--json")

		assert.Empty(t, stderr)
		doc := decodeAnomaliesJSON(t, stdout)
		assert.Equal(t, []recurringIDName{{ID: "acct-usd", Name: "US Chequing"}}, doc.AccountFilter)
		require.Len(t, doc.Anomalies, 1)
		assert.Equal(t, "CAD", doc.Currency)
		assert.Equal(t, []string{"CAD", "350.00", "56.00", "USD", "250.00", "40.00"}, jsonCells(doc.Anomalies[0]))
	})
}

func Test_run_anomalies_converts_a_charge_in_a_closed_usd_account(t *testing.T) {
	closed := usdChequingAccount("acct-old", 2)
	closed.Name, closed.Active, closed.Closed = "Old USD", false, true
	charges := inUSD(append(hardwareHistory(), bigHardware()))
	for i := range charges {
		charges[i].account = "acct-old"
	}
	anomaliesStore(t, []store.Account{chequingAccount("acct-cad", 1), closed}, charges,
		usdRate(day(2025, time.January, 2), 1_300_000), usdRate(day(2026, time.March, 2), 1_400_000))

	t.Run("text", func(t *testing.T) {
		stdout, stderr := runAnomaliesOK(t)

		assert.Empty(t, stderr)
		assert.Equal(t, anomaliesCADTable("1 charge checked",
			hardwareRow("Old USD (USD, closed)", "350.00", "56.00")), stdout)
	})

	t.Run("json", func(t *testing.T) {
		stdout, stderr := runAnomaliesOK(t, "--json")

		assert.Empty(t, stderr)
		doc := decodeAnomaliesJSON(t, stdout)
		assert.Equal(t, []string{}, doc.Warnings)
		require.Len(t, doc.Anomalies, 1)
		assert.Equal(t, "Old USD", doc.Anomalies[0].Account)
		assert.Equal(t, "CAD", doc.Currency)
		assert.Equal(t, []string{"CAD", "350.00", "56.00", "USD", "250.00", "40.00"}, jsonCells(doc.Anomalies[0]))
	})
}

func Test_run_anomalies_on_an_unrated_usd_store_says_only_that_the_period_is_empty(t *testing.T) {
	anomaliesStore(t, []store.Account{usdChequingAccount("acct-usd", 1)}, inUSD(append(hardwareHistory(), bigHardware())))
	const empty = "no unusually large charges from 2026-09-01 to 2026-09-29; the store's transactions run 2025-03-03 to 2026-03-02"

	for _, c := range []struct{ currency, caption string }{
		{currency: "CAD", caption: ", amounts in CAD"},
		{currency: "USD", caption: ", amounts in USD"},
		{currency: "native", caption: ""},
	} {
		t.Run(c.currency, func(t *testing.T) {
			text, textStderr := runAnomaliesOK(t, "--since", "2026-09", "--currency", c.currency)
			js, docStderr := runAnomaliesOK(t, "--since", "2026-09", "--currency", c.currency, "--json")

			assert.Equal(t, warningLines([]string{empty}), textStderr)
			assert.Equal(t, warningLines([]string{empty}), docStderr)
			assert.Equal(t, anomaliesTable("Unusually large charges 2026-09-01 to 2026-09-29 in all accounts"+c.caption, "0 charges checked"), text)
			doc := decodeAnomaliesJSON(t, js)
			assert.Equal(t, []string{empty}, doc.Warnings)
			assert.Equal(t, c.currency, doc.Currency)
			assert.Equal(t, []anomalyJSON{}, doc.Anomalies)
		})
	}
}

// anomalyJSON is one entry of the anomalies document's "anomalies".
type anomalyJSON struct {
	TransactionID string  `json:"transaction_id"`
	Date          string  `json:"date"`
	AccountID     string  `json:"account_id"`
	Account       string  `json:"account"`
	Currency      string  `json:"currency"`
	Payee         *string `json:"payee"`
	Category      *string `json:"category"`
	Amount        string  `json:"amount"`
	Baseline      string  `json:"baseline"`
	Usual         string  `json:"usual"`
	// NativeCurrency, NativeAmount and NativeUsual are the charge's own currency, Amount and Usual.
	NativeCurrency string  `json:"native_currency"`
	NativeAmount   string  `json:"native_amount"`
	NativeUsual    string  `json:"native_usual"`
	Earlier        int     `json:"earlier"`
	Times          float64 `json:"times"`
}

// anomaliesJSONDoc is quarry anomalies --json's stdout.
type anomaliesJSONDoc struct {
	Since         string            `json:"since"`
	Until         string            `json:"until"`
	Currency      string            `json:"currency"`
	AccountFilter []recurringIDName `json:"account_filter"`
	Anomalies     []anomalyJSON     `json:"anomalies"`
	Checked       int               `json:"checked"`
	NotJudged     int               `json:"not_judged"`
	Warnings      []string          `json:"warnings"`
}

// decodeAnomaliesJSON reads stdout as the anomalies document, refusing keys the document does not rule.
func decodeAnomaliesJSON(t *testing.T, stdout string) anomaliesJSONDoc {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(stdout))
	decoder.DisallowUnknownFields()
	var doc anomaliesJSONDoc

	require.NoError(t, decoder.Decode(&doc), stdout)
	return doc
}

func Test_run_anomalies_json_returns_the_anomalies_document(t *testing.T) {
	home := newHome(t)
	charges := append(weeklyCharges("Bell Canada", 9000, 9300, 9605, 9900, 10200), groceryCharge("Bell Canada", day(2026, time.March, 2), 41200))
	charges = append(charges, chargeTxn{
		id: "tool-shed", account: "acct-cad", payee: "Tool Shed", currency: "CAD", day: day(2026, time.June, 9),
		splits: []chargeSplit{{cents: -15000}},
	})
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)}, charges...))

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"anomalies", "--json"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, anomaliesJSONDoc{
		Since:         "2026-01-01",
		Until:         "2026-09-29",
		Currency:      "CAD",
		AccountFilter: []recurringIDName{},
		Anomalies: []anomalyJSON{{
			TransactionID:  "txn-Bell Canada2026-03-02",
			Date:           "2026-03-02",
			AccountID:      "acct-cad",
			Account:        "Chequing",
			Currency:       "CAD",
			Payee:          new("Bell Canada"),
			Category:       new("Food:Groceries"),
			Amount:         "412.00",
			Baseline:       "payee",
			Usual:          "96.05",
			NativeCurrency: "CAD",
			NativeAmount:   "412.00",
			NativeUsual:    "96.05",
			Earlier:        5,
			Times:          4.3,
		}},
		Checked:   2,
		NotJudged: 1,
		Warnings:  []string{},
	}, decodeAnomaliesJSON(t, stdout.String()))
}

func Test_run_anomalies_refuses_usage_and_store_problems(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		accounts []store.Account
		wantExit int
		wantLine string
	}{
		{
			name: "an argument", args: []string{"anomalies", "extra"}, accounts: []store.Account{chequingAccount("acct-cad", 1)},
			wantExit: 2, wantLine: "quarry: anomalies takes no arguments\n",
		},
		{
			name: "an until that is not a date", args: []string{"anomalies", "--until", "2026-02-30"}, accounts: []store.Account{chequingAccount("acct-cad", 1)},
			wantExit: 2, wantLine: "quarry: --until \"2026-02-30\" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD\n",
		},
		{
			name: "an account no account is named", args: []string{"anomalies", "--account", "Nope"}, accounts: []store.Account{chequingAccount("acct-cad", 1)},
			wantExit: 1, wantLine: "quarry: no account named \"Nope\"; run quarry accounts --all to list them\n",
		},
		{
			name: "an account name two accounts share", args: []string{"anomalies", "--account", "Visa"},
			accounts: []store.Account{
				{ID: "acct-812", SourceID: 1, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true},
				{ID: "acct-977", SourceID: 2, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true},
			},
			wantExit: 1, wantLine: "quarry: 2 accounts are named \"Visa\"; pass one of their ids instead: acct-812, acct-977\n",
		},
		{
			name: "an account no account is named, with --json", args: []string{"anomalies", "--json", "--account", "Nope"},
			accounts: []store.Account{chequingAccount("acct-cad", 1)},
			wantExit: 1, wantLine: "quarry: no account named \"Nope\"; run quarry accounts --all to list them\n",
		},
		{
			name: "a since that is not a date, with --json", args: []string{"anomalies", "--json", "--since", "2026-13"},
			accounts: []store.Account{chequingAccount("acct-cad", 1)},
			wantExit: 2, wantLine: "quarry: --since \"2026-13\" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD\n",
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

func Test_run_anomalies_refuses_when_there_is_no_store(t *testing.T) {
	home := newHome(t)

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"anomalies"})

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, noStoreLine(t, home), stderr.String())
}

// HOME holds no store: exit 2 (not the missing-store 1) shows each check runs first.
func Test_run_anomalies_rejects_a_period_it_cannot_use(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{
			name:       "a since after until",
			args:       []string{"anomalies", "--since", "2025", "--until", "2024"},
			wantStderr: "quarry: --since 2025 is after --until 2024\n",
		},
		{
			name:       "an until before the default since",
			args:       []string{"anomalies", "--until", "2024"},
			wantStderr: "quarry: --until 2024 is before the default --since 2026-01-01; pass --since too\n",
		},
		{
			name:       "a since after today",
			args:       []string{"anomalies", "--since", "2099"},
			wantStderr: "quarry: --since 2099 is after today; anomalies lists charges up to today only, so pass an earlier --since\n",
		},
		{
			name:       "a since after today, with --json",
			args:       []string{"anomalies", "--json", "--since", "2099"},
			wantStderr: "quarry: --since 2099 is after today; anomalies lists charges up to today only, so pass an earlier --since\n",
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
