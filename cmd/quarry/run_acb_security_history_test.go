package main

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// acbHistoryRowOf is one history table line with the first ten columns w wide, the last cell unheaded.
func acbHistoryRowOf(w [10]int, c [11]string) string {
	return strings.TrimRight(fmt.Sprintf("%-*s  %-*s  %-*s  %*s  %*s  %*s  %*s  %*s  %*s  %*s  %s",
		w[0], c[0], w[1], c[1], w[2], c[2], w[3], c[3], w[4], c[4], w[5], c[5], w[6], c[6], w[7], c[7], w[8], c[8], w[9], c[9], c[10]), " ") + "\n"
}

var acbHistoryHeaderCells = [11]string{"Date", "Account", "Action", "Shares", "Amount", "Rate", "CAD", "Shares held", "ACB", "Gain or loss"}

// acbPairFixture stores acbRows with USD rates, Acme and Vanguard pooled and Maple in the registered account.
func acbPairFixture(t *testing.T) {
	t.Helper()
	acbFixture(t, acbNonRegisteredPair, acbRows(),
		usdRate(day(2024, time.January, 2), 1_250_000), usdRate(day(2026, time.January, 2), 1_400_000))
}

func Test_run_acb_security_prints_a_block_for_each_named_security_in_walk_order_not_argument_order(t *testing.T) {
	acbPairFixture(t)
	acme := [10]int{10, 13, 6, 6, 13, 4, 9, 11, 8, 12}
	vanguard := [10]int{10, 13, 6, 6, 13, 6, 9, 11, 8, 12}

	stdout, stderr := runACB(t, "--security", "vti", "--security", "ACME")

	assert.Empty(t, stderr)
	assert.Equal(t, "ACB history of \"Acme Corp\" (ACME), in CAD\n\n"+
		acbHistoryRowOf(acme, acbHistoryHeaderCells)+
		acbHistoryRowOf(acme, [11]string{"2024-02-01", "CAD Brokerage", "buy", "100", "-1,000.00 CAD", "", "-1,000.00", "100", "1,000.00", ""})+
		acbHistoryRowOf(acme, [11]string{"2024-03-01", "USD Brokerage", "buy", "50", "-600.00 CAD", "", "-600.00", "150", "1,600.00", ""})+
		acbHistoryRowOf(acme, [11]string{"2025-06-02", "CAD Brokerage", "sell", "60", "900.00 CAD", "", "900.00", "90", "960.00", "260.00"})+
		"\nACB history of \"Vanguard Total Stock\" (VTI), in CAD\n\n"+
		acbHistoryRowOf(vanguard, acbHistoryHeaderCells)+
		acbHistoryRowOf(vanguard, [11]string{"2024-05-01", "USD Brokerage", "buy", "10", "-1,000.00 USD", "1.2500", "-1,250.00", "10", "1,250.00", ""})+
		acbHistoryRowOf(vanguard, [11]string{"2026-02-02", "USD Brokerage", "sell", "4", "500.00 USD", "1.4000", "700.00", "6", "750.00", "200.00"}),
		stdout)
}

func Test_run_acb_security_names_a_security_by_its_id(t *testing.T) {
	acbPairFixture(t)

	stdout, _ := runACB(t, "--security", "sec-vti")

	assert.True(t, strings.HasPrefix(stdout, "ACB history of \"Vanguard Total Stock\" (VTI), in CAD\n\n"), stdout)
	assert.NotContains(t, stdout, "Acme Corp")
}

func Test_run_acb_security_prints_a_block_for_each_security_sharing_the_ticker(t *testing.T) {
	rows := acbSuperficialRows()
	rows.Securities = []store.Security{
		{ID: "sec-a", SourceID: 1, Name: "Alpha Fund", Ticker: new("DUP"), Currency: new("CAD")},
		{ID: "sec-b", SourceID: 2, Name: "Beta Fund", Ticker: new("DUP"), Currency: new("CAD")},
	}
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		acbTrade("inv-a", 1, "acct-cad", "sec-a", store.ActionBuy, "CAD", day(2024, time.February, 1), 10_000_000, -100_000),
		acbTrade("inv-b", 2, "acct-cad", "sec-b", store.ActionBuy, "CAD", day(2024, time.February, 1), 10_000_000, -100_000),
	}
	acbFixture(t, acbNonRegistered, rows)
	w := [10]int{10, 13, 6, 6, 13, 4, 9, 11, 8, 12}
	buy := [11]string{"2024-02-01", "CAD Brokerage", "buy", "10", "-1,000.00 CAD", "", "-1,000.00", "10", "1,000.00", ""}

	stdout, stderr := runACB(t, "--security", "dup")

	assert.Equal(t, "ACB history of \"Alpha Fund\" (DUP), in CAD\n\n"+acbHistoryRowOf(w, acbHistoryHeaderCells)+acbHistoryRowOf(w, buy)+
		"\nACB history of \"Beta Fund\" (DUP), in CAD\n\n"+acbHistoryRowOf(w, acbHistoryHeaderCells)+acbHistoryRowOf(w, buy), stdout)
	assert.Equal(t, stderrWarnings(`"DUP" is 2 securities in Quicken (Alpha Fund, Beta Fund); `+
		"quarry keeps a separate ACB for each; if they are the same, merge them in Quicken"), stderr)
}

func Test_run_acb_security_prints_only_the_caption_and_header_for_a_security_held_only_in_registered_accounts(t *testing.T) {
	acbPairFixture(t)

	stdout, _ := runACB(t, "--security", "MPL")

	assert.Equal(t, "ACB history of \"Maple Fund\" (MPL), in CAD\n\n"+
		"Date  Account  Action  Shares  Amount  Rate  CAD  Shares held  ACB  Gain or loss\n", stdout)
}

func Test_run_acb_security_warns_a_security_held_only_in_registered_accounts(t *testing.T) {
	acbPairFixture(t)
	want := `"Maple Fund" is held only in registered accounts, so it has no ACB`

	_, stderr := runACB(t, "--security", "Maple Fund")
	doc, docStderr := runACB(t, "--security", "Maple Fund", "--json")

	assert.Equal(t, stderrWarnings(want), stderr)
	assert.Equal(t, stderrWarnings(want), docStderr)
	got := decodeACBYear(t, doc)
	assert.Empty(t, got.Securities)
	assert.Equal(t, []string{want}, got.Warnings)
}

func Test_run_acb_security_gives_a_registered_only_block_beside_a_pooled_one_and_warns_for_the_first_only(t *testing.T) {
	acbPairFixture(t)

	stdout, stderr := runACB(t, "--security", "MPL", "--security", "ACME")

	assert.Equal(t, stderrWarnings(`"Maple Fund" is held only in registered accounts, so it has no ACB`), stderr)
	assert.Equal(t, 1, strings.Count(stdout, "ACB history of \"Acme Corp\" (ACME), in CAD\n"))
	assert.Equal(t, 1, strings.Count(stdout, "ACB history of \"Maple Fund\" (MPL), in CAD\n"))
	assert.Less(t, strings.Index(stdout, "Acme Corp"), strings.Index(stdout, "Maple Fund"))
}

func Test_run_acb_security_writes_only_the_named_securities_and_their_years_in_json(t *testing.T) {
	acbPairFixture(t)

	stdout, _ := runACB(t, "--security", "ACME", "--json")

	doc := decodeACBYear(t, stdout)
	assert.Nil(t, doc.Year)
	require.Len(t, doc.Securities, 1)
	assert.Equal(t, "sec-acme", doc.Securities[0].ID)
	require.Len(t, doc.Years, 1)
	assert.Equal(t, 2025, doc.Years[0].Year)
	assert.Equal(t, "260.00", doc.Years[0].Gain)
	assert.Equal(t, "910.00", doc.Years[0].Proceeds)
}

func Test_run_acb_security_with_year_prints_the_years_sales_of_the_named_security_only(t *testing.T) {
	acbPairFixture(t)
	w := [7]int{10, 8, 6, 8, 7, 6, 12}

	stdout, stderr := runACB(t, "--year", "2026", "--security", "VTI")

	assert.Empty(t, stderr)
	assert.Equal(t, "Sales in 2026, in CAD\n\n"+
		acbSaleRowOf(w, [8]string{"Date", "Security", "Shares", "Proceeds", "Outlays", "ACB", "Gain or loss"})+
		acbSaleRowOf(w, [8]string{"2026-02-02", "VTI", "4", "707.00", "7.00", "500.00", "200.00"})+
		acbSaleRowOf(w, [8]string{"Total", "", "", "707.00", "7.00", "500.00", "200.00"}), stdout)
}

func Test_run_acb_security_with_year_prints_a_zero_total_when_the_named_security_sold_nothing_that_year(t *testing.T) {
	acbPairFixture(t)
	w := [7]int{5, 8, 6, 8, 7, 4, 12}

	stdout, stderr := runACB(t, "--year", "2026", "--security", "ACME")

	assert.Empty(t, stderr)
	assert.Equal(t, "Sales in 2026, in CAD\n\n"+
		acbSaleRowOf(w, [8]string{"Date", "Security", "Shares", "Proceeds", "Outlays", "ACB", "Gain or loss"})+
		acbSaleRowOf(w, [8]string{"Total", "", "", "0.00", "0.00", "0.00", "0.00"}), stdout)
}

func Test_run_acb_security_with_year_writes_that_year_re_summed_over_the_named_security_in_json(t *testing.T) {
	acbPairFixture(t)

	stdout, _ := runACB(t, "--year", "2026", "--security", "VTI", "--json")

	doc := decodeACBYear(t, stdout)
	require.NotNil(t, doc.Year)
	assert.Equal(t, 2026, *doc.Year)
	require.Len(t, doc.Years, 1)
	assert.Equal(t, "707.00", doc.Years[0].Proceeds)
	require.Len(t, doc.Securities, 1)
	assert.Equal(t, "sec-vti", doc.Securities[0].ID)
}

func Test_run_acb_security_refuses_a_name_that_matches_no_security(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "an unknown name", args: []string{"--security", "XYZ"}, want: "XYZ"},
		{name: "an empty name", args: []string{"--security", ""}, want: ""},
		{name: "one unknown name beside a known one", args: []string{"--security", "ACME", "--security", "XYZ"}, want: "XYZ"},
		{name: "the first unknown name in argument order", args: []string{"--security", "NOPE-2", "--security", "ACME", "--security", "NOPE-1"}, want: "NOPE-2"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			acbPairFixture(t)
			var stdout, stderr bytes.Buffer

			exitCode := runWith(context.Background(), append([]string{"acb"}, c.args...), spendEnvAt(&stdout, &stderr, holdingsClock()))

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, fmt.Sprintf("quarry: acb covers no security named %q; quarry acb --json lists every security it covers\n", c.want), stderr.String())
		})
	}
}

// acbReinvestRows is one non-registered CAD brokerage whose Acme holding buys 10 for 1,000.00, reinvests a dividend
// into 1 share with no cost, then sells 2 for 300.00 and removes 1.
func acbReinvestRows() store.Rows {
	rows := acbSuperficialRows()
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		acbTrade("inv-buy", 1, "acct-cad", "sec-acme", store.ActionBuy, "CAD", day(2024, time.February, 1), 10_000_000, -100_000),
		acbTrade("inv-reinvest", 2, "acct-cad", "sec-acme", store.ActionReinvestDividend, "CAD", day(2024, time.March, 1), 1_000_000, 0),
		acbTrade("inv-sell", 3, "acct-cad", "sec-acme", store.ActionSell, "CAD", day(2025, time.March, 3), -2_000_000, 30_000),
		acbTrade("inv-remove", 4, "acct-cad", "sec-acme", store.ActionRemoveShares, "CAD", day(2025, time.June, 2), -1_000_000, 0),
	}
	return rows
}

func Test_run_acb_security_lists_the_reinvested_dividends_the_no_cost_warning_names_and_marks_each_unknown_cost(t *testing.T) {
	acbFixture(t, acbNonRegistered, acbReinvestRows())
	_, warned := runACB(t)
	advice := regexp.MustCompile(`quarry (acb --security \S+) lists them`).FindStringSubmatch(warned)
	require.Len(t, advice, 2, warned)
	args := strings.Fields(advice[1])
	w := [10]int{10, 13, 17, 6, 13, 4, 9, 11, 8, 12}

	stdout, _ := runACB(t, args[1:]...)

	assert.Equal(t, "ACB history of \"Acme Corp\" (ACME), in CAD\n\n"+
		acbHistoryRowOf(w, acbHistoryHeaderCells)+
		acbHistoryRowOf(w, [11]string{"2024-02-01", "CAD Brokerage", "buy", "10", "-1,000.00 CAD", "", "-1,000.00", "10", "1,000.00", ""})+
		acbHistoryRowOf(w, [11]string{"2024-03-01", "CAD Brokerage", "reinvest_dividend", "1", "0.00 CAD", "", "0.00", "11", "1,000.00", "", "unknown cost"})+
		acbHistoryRowOf(w, [11]string{"2025-03-03", "CAD Brokerage", "sell", "2", "300.00 CAD", "", "300.00", "9", "818.18", "118.18", "unknown cost"})+
		acbHistoryRowOf(w, [11]string{"2025-06-02", "CAD Brokerage", "remove_shares", "1", "0.00 CAD", "", "0.00", "8", "727.27", "", "unknown cost"}),
		stdout)
}
