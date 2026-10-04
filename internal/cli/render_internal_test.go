// White-box: formatMB, renderSuccess and the phrase builders are unexported
// formatting rules best driven directly, rather than through a full command
// run for every rounding and boundary case.
package cli

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

func Test_formatMB(t *testing.T) {
	cases := []struct {
		name  string
		bytes int64
		want  string
	}{
		{name: "rounds to one decimal place", bytes: 212_400_000, want: "212.4 MB"},
		{name: "rounding crosses the thousands-grouping boundary", bytes: 999_960_000, want: "1,000.0 MB"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, formatMB(c.bytes))
		})
	}
}

func Test_renderSuccess_reports_one_account_in_the_singular(t *testing.T) {
	m := snapshot.Manifest{Snapshot: snapshot.Info{Accounts: 1}}

	got := renderSuccess(m, "/Users/dave")

	assert.Contains(t, got, "1 account\n")
}

func Test_renderSuccess_reports_many_accounts_in_the_plural_with_thousands_grouped(t *testing.T) {
	m := snapshot.Manifest{Snapshot: snapshot.Info{Accounts: 1000}}

	got := renderSuccess(m, "/Users/dave")

	assert.Contains(t, got, "1,000 accounts\n")
}

func Test_schemaLine(t *testing.T) {
	cases := []struct {
		name string
		info snapshot.SchemaInfo
		want string
	}{
		{
			name: "exact match",
			info: snapshot.SchemaInfo{Reference: "ref", Verified: true, ReferenceTables: 71, ReferenceColumns: 1042},
			want: "matches reference ref (71 tables, 1,042 columns)",
		},
		{
			name: "extras only",
			info: snapshot.SchemaInfo{
				Reference: "ref", Verified: true, ReferenceTables: 71, ReferenceColumns: 1042,
				UnexpectedTables: []string{"ZNEWENTITY"},
				UnexpectedColumns: []snapshot.ColumnRef{
					{Table: "ZACCOUNT", Column: "ZNEWFLAG"}, {Table: "ZTAG", Column: "ZCOLOR"},
				},
			},
			want: "matches reference ref (71 tables, 1,042 columns), plus 1 table and 2 columns not in it",
		},
		{
			name: "missing tables only",
			info: snapshot.SchemaInfo{Reference: "ref", Verified: false, MissingTables: []string{"ZLOT"}},
			want: "DIFFERS from reference ref: 1 table missing",
		},
		{
			name: "missing columns only",
			info: snapshot.SchemaInfo{
				Reference: "ref", Verified: false,
				MissingColumns: []snapshot.ColumnRef{{Table: "ZACCOUNT", Column: "ZFAKE"}},
			},
			want: "DIFFERS from reference ref: 1 column missing",
		},
		{
			name: "missing tables and columns",
			info: snapshot.SchemaInfo{
				Reference: "ref", Verified: false,
				MissingTables: []string{"ZLOT"},
				MissingColumns: []snapshot.ColumnRef{
					{Table: "ZCASHFLOWTRANSACTIONENTRY", Column: "ZMEMO"}, {Table: "ZSECURITY", Column: "ZCUSIP"},
				},
			},
			want: "DIFFERS from reference ref: 1 table and 2 columns missing",
		},
		{
			name: "missing plus extras",
			info: snapshot.SchemaInfo{
				Reference: "ref", Verified: false,
				MissingTables: []string{"ZLOT"},
				MissingColumns: []snapshot.ColumnRef{
					{Table: "ZCASHFLOWTRANSACTIONENTRY", Column: "ZMEMO"}, {Table: "ZSECURITY", Column: "ZCUSIP"},
				},
				UnexpectedColumns: []snapshot.ColumnRef{{Table: "ZACCOUNT", Column: "ZNEWFLAG"}},
			},
			want: "DIFFERS from reference ref: 1 table and 2 columns missing, 1 column not in reference",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, schemaLine(c.info))
		})
	}
}

func Test_renderSuccess_writes_diff_rows_missing_before_extras_and_tables_before_columns(t *testing.T) {
	m := snapshot.Manifest{
		Schema: snapshot.SchemaInfo{
			Reference:      "ref",
			Verified:       false,
			MissingTables:  []string{"ZLOT"},
			MissingColumns: []snapshot.ColumnRef{{Table: "ZCASHFLOWTRANSACTIONENTRY", Column: "ZMEMO"}, {Table: "ZSECURITY", Column: "ZCUSIP"}},
			UnexpectedColumns: []snapshot.ColumnRef{
				{Table: "ZACCOUNT", Column: "ZNEWFLAG"},
			},
		},
	}

	got := renderSuccess(m, "/Users/dave")

	assert.True(t, strings.HasSuffix(got,
		"  - table   ZLOT\n"+
			"  - column  ZCASHFLOWTRANSACTIONENTRY.ZMEMO\n"+
			"  - column  ZSECURITY.ZCUSIP\n"+
			"  + column  ZACCOUNT.ZNEWFLAG\n"),
		"got: %q", got)
}

func Test_renderSuccess_writes_no_diff_rows_on_an_exact_match(t *testing.T) {
	m := snapshot.Manifest{Schema: snapshot.SchemaInfo{Reference: "ref", Verified: true}}

	got := renderSuccess(m, "/Users/dave")

	assert.NotContains(t, got, "  -")
	assert.NotContains(t, got, "  +")
}

func Test_renderSuccess_reports_the_exact_match_schema_line(t *testing.T) {
	m := snapshot.Manifest{
		Snapshot: snapshot.Info{
			Path:     "/Users/dave/Library/Application Support/quarry/snapshots/20260927T143005Z.sqlite",
			Manifest: "/Users/dave/Library/Application Support/quarry/snapshots/20260927T143005Z.json",
			Source:   "/Users/dave/Documents/Home.quicken",
			Bytes:    212_400_000,
			SHA256:   "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
			Accounts: 42,
		},
		Schema: snapshot.SchemaInfo{
			Reference:        "hardkoded/quicken-skills@752107b",
			Verified:         true,
			ReferenceTables:  71,
			ReferenceColumns: 1042,
		},
	}

	got := renderSuccess(m, "/Users/dave")

	assert.Equal(t, "Snapshot  ~/Library/Application Support/quarry/snapshots/20260927T143005Z.sqlite\n"+
		"Manifest  ~/Library/Application Support/quarry/snapshots/20260927T143005Z.json\n"+
		"Source    ~/Documents/Home.quicken\n"+
		"Size      212.4 MB, 42 accounts\n"+
		"SHA-256   9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08\n"+
		"Schema    matches reference hardkoded/quicken-skills@752107b (71 tables, 1,042 columns)\n",
		got)
}

func Test_rowsPhrase(t *testing.T) {
	cases := []struct {
		name        string
		counts      store.Counts
		notImported store.NotImported
		want        string
	}{
		{
			name:   "zero counts",
			counts: store.Counts{},
			want:   "0 transactions, 0 splits, 0 transfers, 0 payees, 0 categories, 0 tags; 0 investment transactions, 0 securities, 0 prices",
		},
		{
			name:   "singular at exactly one",
			counts: store.Counts{Transactions: 1, Splits: 1, Transfers: 1, Payees: 1, Categories: 1, Tags: 1, InvestmentTransactions: 1, Securities: 1, Prices: 1},
			want:   "1 transaction, 1 split, 1 transfer, 1 payee, 1 category, 1 tag; 1 investment transaction, 1 security, 1 price",
		},
		{
			name: "thousands-grouped at many",
			counts: store.Counts{
				Transactions: 18204, Splits: 21977, Transfers: 3112, Payees: 1873, Categories: 312, Tags: 14,
				InvestmentTransactions: 1605, Securities: 84, Prices: 99352,
			},
			want: "18,204 transactions, 21,977 splits, 3,112 transfers, 1,873 payees, 312 categories, 14 tags; 1,605 investment transactions, 84 securities, 99,352 prices",
		},
		{
			name:        "one investment transaction not imported, singular",
			notImported: store.NotImported{InvestmentTransactions: 1},
			want:        "0 transactions, 0 splits, 0 transfers, 0 payees, 0 categories, 0 tags; 0 investment transactions, 0 securities, 0 prices; 1 investment transaction not imported",
		},
		{
			name:        "many investment transactions not imported, thousands-grouped",
			notImported: store.NotImported{InvestmentTransactions: 1605},
			want:        "0 transactions, 0 splits, 0 transfers, 0 payees, 0 categories, 0 tags; 0 investment transactions, 0 securities, 0 prices; 1,605 investment transactions not imported",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, rowsPhrase(c.counts, c.notImported))
		})
	}
}

func Test_transfersPhrase(t *testing.T) {
	cases := []struct {
		name             string
		paired, oneSided int
		want             string
	}{
		{name: "no transfers", want: "none"},
		{name: "one pair", paired: 1, want: "1 paired"},
		{name: "many pairs, thousands-grouped", paired: 3112, want: "3,112 paired"},
		{name: "pairs and one-sided legs", paired: 3112, oneSided: 3, want: "3,112 paired, 3 one-sided"},
		{name: "only one-sided legs", oneSided: 2, want: "0 paired, 2 one-sided"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, transfersPhrase(c.paired, c.oneSided))
		})
	}
}

func Test_renderStore_prints_the_transfers_count_line_without_one_sided_rows(t *testing.T) {
	result := store.Result{
		Path: "/Users/dave/Library/Application Support/quarry/quarry.duckdb",
		Validation: store.Validation{Transfers: store.TransferCheck{Paired: 2, OneSided: []store.OneSidedTransfer{
			{Date: time.Date(2019, 6, 14, 0, 0, 0, 0, time.UTC), Account: "Chequing", Currency: "CAD", Active: true, Amount: -50000},
		}}},
		Findings: finding.Counts{Open: 2},
	}

	got := renderStore(result, "/Users/dave")

	assert.Contains(t, got, "Transfers 2 paired, 1 one-sided\n"+
		"Findings  2 open; run quarry findings to list them\n")
	assert.NotContains(t, got, "?")
}

func Test_renderStore_prints_new_findings_only_when_the_history_was_carried(t *testing.T) {
	counts := finding.Counts{Open: 2, New: 1}
	cases := []struct {
		name    string
		carried bool
		want    string
	}{
		{name: "carried", carried: true, want: "Findings  2 open (1 new); run quarry findings to list them\n"},
		{name: "not carried", carried: false, want: "Findings  2 open; run quarry findings to list them\n"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderStore(store.Result{Findings: counts, FindingsCarried: c.carried}, "/Users/dave")

			assert.Contains(t, got, c.want)
		})
	}
}

func Test_renderStore_renders_the_store_rows_balances_splits_shares_and_transfers_lines(t *testing.T) {
	result := store.Result{
		Path:   "/Users/dave/Library/Application Support/quarry/quarry.duckdb",
		Counts: store.Counts{Transactions: 1},
		Validation: store.Validation{
			Balances:  store.BalanceCheck{Checked: 1},
			Splits:    store.SplitCheck{Checked: 1},
			Shares:    store.ShareCheck{Checked: 2},
			Transfers: store.TransferCheck{Paired: 2},
		},
		NotImported: store.NotImported{InvestmentTransactions: 3},
	}

	got := renderStore(result, "/Users/dave")

	assert.Equal(t, "Store     ~/Library/Application Support/quarry/quarry.duckdb\n"+
		"Rows      1 transaction, 0 splits, 0 transfers, 0 payees, 0 categories, 0 tags; 0 investment transactions, 0 securities, 0 prices; 3 investment transactions not imported\n"+
		"Balances  1 account matches Quicken's last reconciled balance\n"+
		"Splits    the 1 transaction equals the sum of its splits\n"+
		"Shares    2 holdings match Quicken's share counts\n"+
		"Transfers 2 paired\n"+
		"Findings  none open\n"+
		"Rates     none (the Bank of Canada has no rates for your transaction dates)\n", got)
}

func Test_renderStore_ends_with_the_rates_line(t *testing.T) {
	result := store.Result{
		Counts: store.Counts{Transactions: 1},
		Rates:  store.RatesSummary{First: time.Date(2005, 3, 1, 0, 0, 0, 0, time.UTC), Last: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), Added: 12},
	}

	got := renderStore(result, "/Users/dave")

	assert.True(t, strings.HasSuffix(got, "Findings  none open\nRates     USD/CAD 2005-03-01 to 2026-03-01 (12 new)\n"), got)
}

func Test_ratesPhrase(t *testing.T) {
	first, last := time.Date(2005, 3, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name         string
		rates        store.RatesSummary
		transactions int
		want         string
	}{
		{name: "rates fetched", rates: store.RatesSummary{First: first, Last: last, Added: 12}, transactions: 5, want: "USD/CAD 2005-03-01 to 2026-03-01 (12 new)"},
		{name: "one rate fetched", rates: store.RatesSummary{First: first, Last: last, Added: 1}, transactions: 5, want: "USD/CAD 2005-03-01 to 2026-03-01 (1 new)"},
		{name: "new rates thousands-grouped", rates: store.RatesSummary{First: first, Last: last, Added: 6012}, transactions: 5, want: "USD/CAD 2005-03-01 to 2026-03-01 (6,012 new)"},
		{name: "rates stored, none fetched", rates: store.RatesSummary{First: first, Last: last}, transactions: 5, want: "USD/CAD 2005-03-01 to 2026-03-01 (up to date)"},
		{
			name: "rates stored, the fetch failed", rates: store.RatesSummary{First: first, Last: last, FetchError: "unreachable"}, transactions: 5,
			want: "USD/CAD 2005-03-01 to 2026-03-01 (not refreshed; see warning)",
		},
		{
			name: "some rates fetched, the rest not", rates: store.RatesSummary{First: first, Last: last, Added: 12, FetchError: "unreachable", Partial: true}, transactions: 5,
			want: "USD/CAD 2005-03-01 to 2026-03-01 (12 new, not all fetched; see warning)",
		},
		{
			name: "one rate fetched, the rest not", rates: store.RatesSummary{First: first, Last: last, Added: 1, FetchError: "unreachable", Partial: true}, transactions: 5,
			want: "USD/CAD 2005-03-01 to 2026-03-01 (1 new, not all fetched; see warning)",
		},
		{
			name: "thousands of rates fetched, the rest not", rates: store.RatesSummary{First: first, Last: last, Added: 6012, FetchError: "unreachable", Partial: true}, transactions: 5,
			want: "USD/CAD 2005-03-01 to 2026-03-01 (6,012 new, not all fetched; see warning)",
		},
		{name: "no rates, no transactions", transactions: 0, want: "none (no transactions to convert)"},
		{name: "no rates, transactions exist", transactions: 1, want: "none (the Bank of Canada has no rates for your transaction dates)"},
		{name: "no rates, the fetch failed", rates: store.RatesSummary{FetchError: "unreachable"}, transactions: 1, want: "none (not fetched; see warning)"},
		{name: "no rates, the fetch failed, no transactions", rates: store.RatesSummary{FetchError: "unreachable"}, want: "none (not fetched; see warning)"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, ratesPhrase(c.rates, c.transactions))
		})
	}
}

func Test_findingsPhrase(t *testing.T) {
	cases := []struct {
		name    string
		counts  finding.Counts
		carried bool
		want    string
	}{
		{name: "none open", want: "none open"},
		{name: "one open", counts: finding.Counts{Open: 1, New: 1}, want: "1 open; run quarry findings to list them"},
		{name: "many open, thousands-grouped", counts: finding.Counts{Open: 1204}, want: "1,204 open; run quarry findings to list them"},
		{
			name: "new but not carried", counts: finding.Counts{Open: 12, New: 12}, carried: false,
			want: "12 open; run quarry findings to list them",
		},
		{
			name: "carried, none new", counts: finding.Counts{Open: 12}, carried: true,
			want: "12 open; run quarry findings to list them",
		},
		{
			name: "carried, some new and some fixed", counts: finding.Counts{Open: 12, New: 3, Fixed: 2, NewlyFixed: 2}, carried: true,
			want: "12 open (3 new), 2 fixed since the last sync; run quarry findings to list them",
		},
		{
			name: "carried, new count thousands-grouped", counts: finding.Counts{Open: 2000, New: 1500}, carried: true,
			want: "2,000 open (1,500 new); run quarry findings to list them",
		},
		{name: "none open, some fixed", counts: finding.Counts{Fixed: 2, NewlyFixed: 2}, carried: true, want: "none open, 2 fixed since the last sync"},
		{name: "none open, fixed earlier not counted", counts: finding.Counts{Fixed: 2}, carried: true, want: "none open"},
		{
			name: "fixed clause without new", counts: finding.Counts{Open: 1, Fixed: 1, NewlyFixed: 1}, carried: true,
			want: "1 open, 1 fixed since the last sync; run quarry findings to list them",
		},
		{name: "fixed count thousands-grouped", counts: finding.Counts{NewlyFixed: 1200}, carried: true, want: "none open, 1,200 fixed since the last sync"},
		{
			name: "new, fixed and ignored", counts: finding.Counts{Open: 12, New: 3, Ignored: 4, Fixed: 2, NewlyFixed: 2}, carried: true,
			want: "12 open (3 new), 2 fixed since the last sync, 4 ignored; run quarry findings to list them",
		},
		{name: "open and ignored", counts: finding.Counts{Open: 1, Ignored: 1}, want: "1 open, 1 ignored; run quarry findings to list them"},
		{name: "none open, some ignored", counts: finding.Counts{Ignored: 4}, want: "none open, 4 ignored"},
		{
			name: "none open, fixed and ignored", counts: finding.Counts{Ignored: 4, Fixed: 2, NewlyFixed: 2}, carried: true,
			want: "none open, 2 fixed since the last sync, 4 ignored",
		},
		{name: "ignored count thousands-grouped", counts: finding.Counts{Ignored: 1200}, want: "none open, 1,200 ignored"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, findingsPhrase(c.counts, c.carried))
		})
	}
}

func Test_balancesPhrase(t *testing.T) {
	cases := []struct {
		name   string
		counts balanceCounts
		want   string
	}{
		{name: "nothing to check", want: "no accounts to check"},
		{name: "one account matches", counts: balanceCounts{Checked: 1}, want: "1 account matches Quicken's last reconciled balance"},
		{name: "many accounts match, thousands-grouped", counts: balanceCounts{Checked: 1000}, want: "1,000 accounts match Quicken's last reconciled balance"},
		{name: "one never reconciled", counts: balanceCounts{NeverReconciled: 1}, want: "no accounts to check; 1 never reconciled"},
		{name: "one investment account", counts: balanceCounts{InvestmentAccounts: 1}, want: "no accounts to check; 1 investment account not checked"},
		{
			name: "never reconciled and investment accounts joined", counts: balanceCounts{NeverReconciled: 3, InvestmentAccounts: 4},
			want: "no accounts to check; 3 never reconciled and 4 investment accounts not checked",
		},
		{
			name: "checked, never reconciled and investment accounts", counts: balanceCounts{Checked: 35, NeverReconciled: 3, InvestmentAccounts: 4},
			want: "35 accounts match Quicken's last reconciled balance; 3 never reconciled and 4 investment accounts not checked",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, balancesPhrase(c.counts))
		})
	}
}

func Test_splitsPhrase(t *testing.T) {
	cases := []struct {
		name    string
		checked int
		want    string
	}{
		{name: "nothing to check", checked: 0, want: "no transactions to check"},
		{name: "one transaction", checked: 1, want: "the 1 transaction equals the sum of its splits"},
		{name: "many transactions, thousands-grouped", checked: 18204, want: "all 18,204 transactions equal the sum of their splits"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, splitsPhrase(c.checked))
		})
	}
}

func Test_sharesPhrase(t *testing.T) {
	cases := []struct {
		name    string
		checked int
		want    string
	}{
		{name: "nothing to check", checked: 0, want: "no holdings to check"},
		{name: "one holding", checked: 1, want: "1 holding matches Quicken's share count"},
		{name: "two holdings", checked: 2, want: "2 holdings match Quicken's share counts"},
		{name: "many holdings, thousands-grouped", checked: 1605, want: "1,605 holdings match Quicken's share counts"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, sharesPhrase(c.checked))
		})
	}
}

func Test_formatMoney(t *testing.T) {
	cases := []struct {
		name  string
		cents int64
		want  string
	}{
		{name: "zero", cents: 0, want: "0.00"},
		{name: "positive", cents: 12345, want: "123.45"},
		{name: "negative", cents: -2000, want: "-20.00"},
		{name: "thousands-grouping boundary", cents: 100000, want: "1,000.00"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, formatMoney(c.cents))
		})
	}
}

func Test_formatShares(t *testing.T) {
	cases := []struct {
		name       string
		millionths int64
		want       string
	}{
		{name: "one millionth", millionths: 1, want: "0.000001"},
		{name: "negative one millionth keeps its sign", millionths: -1, want: "-0.000001"},
		{name: "zero", millionths: 0, want: "0"},
		{name: "whole count has no fraction", millionths: 10_000_000, want: "10"},
		{name: "trailing fractional zeros are trimmed", millionths: 120_500_000, want: "120.5"},
		{name: "whole count thousands-grouped", millionths: 1_200_000_000, want: "1,200"},
		{name: "full six-decimal fraction", millionths: 1_000_001, want: "1.000001"},
		{name: "smallest count does not overflow on negation", millionths: math.MinInt64, want: "-9,223,372,036,854.775808"},
		{name: "largest count", millionths: math.MaxInt64, want: "9,223,372,036,854.775807"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, formatShares(c.millionths))
		})
	}
}

func Test_securityLabel(t *testing.T) {
	cases := []struct {
		name         string
		securityName string
		ticker       *string
		want         string
	}{
		{name: "no ticker shows the name alone", securityName: "Bare Fund", ticker: nil, want: "Bare Fund"},
		{name: "ticker equal to the name is not repeated", securityName: "XEQT", ticker: new("XEQT"), want: "XEQT"},
		{name: "ticker differing from the name follows it", securityName: "iShares Core Equity ETF", ticker: new("XEQT"), want: "iShares Core Equity ETF (XEQT)"},
		{name: "control characters in the name are escaped", securityName: "Bad\nFund", ticker: nil, want: `Bad\nFund`},
		{name: "control characters in the ticker are escaped", securityName: "Fund", ticker: new("A\tB"), want: `Fund (A\tB)`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, securityLabel(c.securityName, c.ticker))
		})
	}
}

func Test_shareMismatchRows(t *testing.T) {
	t.Run("single row needs no padding", func(t *testing.T) {
		rows := shareMismatchRows([]store.ShareMismatch{
			{Account: "RRSP", Currency: "CAD", Closed: true, Security: "iShares Core Equity ETF", Ticker: new("XEQT"), Quarry: 120_500_000, Quicken: 110_500_000, Difference: 10_000_000},
		})

		assert.Equal(t, []string{"  ! RRSP (CAD, closed)  iShares Core Equity ETF (XEQT)  quarry 120.5  Quicken 110.5  difference 10"}, rows)
	})

	t.Run("labels pad to the widest and figures right-align per column", func(t *testing.T) {
		rows := shareMismatchRows([]store.ShareMismatch{
			{Account: "Brokerage", Currency: "CAD", Active: true, Security: "Bare Fund", Quarry: 5_000_000, Quicken: 0, Difference: 5_000_000},
			{Account: "RRSP", Currency: "USD", Closed: true, Security: "iShares Core Equity ETF", Ticker: new("XEQT"), Quarry: 1_200_000_000, Quicken: 1_199_999_999, Difference: -123},
		})

		assert.Equal(t, []string{
			"  ! Brokerage (CAD)     Bare Fund                       quarry     5  Quicken            0  difference         5",
			"  ! RRSP (USD, closed)  iShares Core Equity ETF (XEQT)  quarry 1,200  Quicken 1,199.999999  difference -0.000123",
		}, rows)
	})
}

func Test_accountLabel(t *testing.T) {
	cases := []struct {
		name           string
		closed, active bool
		want           string
	}{
		{name: "neither", closed: false, active: true, want: "Chequing (CAD)"},
		{
			name:   "closed suppresses inactive, not merely absent",
			closed: true, active: false,
			want: "Chequing (CAD, closed)",
		},
		{name: "open and inactive", closed: false, active: false, want: "Chequing (CAD, inactive)"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, accountLabel("Chequing", "CAD", c.closed, c.active))
		})
	}
}

func Test_balanceMismatchRows(t *testing.T) {
	day := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)

	t.Run("single row needs no padding", func(t *testing.T) {
		rows := balanceMismatchRows([]store.BalanceMismatch{
			{Name: "Chequing", Currency: "CAD", Active: true, StatementDate: day, Quarry: 831000, Quicken: 830000, Difference: 1000},
		})
		assert.Equal(t, []string{"  ! Chequing (CAD)  2026-08-31  quarry 8,310.00  Quicken 8,300.00  difference 10.00"}, rows)
	})

	t.Run("widest label, negative-vs-positive amount alignment", func(t *testing.T) {
		rows := balanceMismatchRows([]store.BalanceMismatch{
			{Name: "US Chequing", Currency: "USD", Active: true, StatementDate: day, Quarry: 831000, Quicken: 830000, Difference: 1000},
			{
				Name: "Visa Infinite", Currency: "CAD", Closed: true,
				StatementDate: time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC), Quarry: -120417, Quicken: -118417, Difference: -2000,
			},
		})
		assert.Equal(t, []string{
			"  ! US Chequing (USD)            2026-08-31  quarry  8,310.00  Quicken  8,300.00  difference  10.00",
			"  ! Visa Infinite (CAD, closed)  2026-07-15  quarry -1,204.17  Quicken -1,184.17  difference -20.00",
		}, rows)
	})
}

func Test_splitMismatchRows(t *testing.T) {
	date := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)

	t.Run("single row needs no padding", func(t *testing.T) {
		rows := splitMismatchRows([]store.SplitMismatch{
			{Account: "Visa Infinite", Currency: "CAD", Active: true, Payee: "Costco", Date: date, Amount: -21240, SplitsTotal: -20240},
		})
		assert.Equal(t, []string{"  ! 2024-03-02  Visa Infinite (CAD)  Costco  amount -212.40  splits -202.40"}, rows)
	})

	t.Run("empty payee falls back to (no payee)", func(t *testing.T) {
		rows := splitMismatchRows([]store.SplitMismatch{
			{Account: "Visa Infinite", Currency: "CAD", Active: true, Date: date, Amount: -21240, SplitsTotal: -20240},
		})
		assert.Equal(t, []string{"  ! 2024-03-02  Visa Infinite (CAD)  (no payee)  amount -212.40  splits -202.40"}, rows)
	})

	t.Run("widest label and payee, 2+ rows", func(t *testing.T) {
		rows := splitMismatchRows([]store.SplitMismatch{
			{Account: "Chequing", Currency: "CAD", Active: true, Payee: "Costco Wholesale", Date: date, Amount: -21240, SplitsTotal: -20240},
			{Account: "Visa Infinite", Currency: "CAD", Active: true, Payee: "A", Date: date, Amount: 100, SplitsTotal: -100},
		})
		assert.Equal(t, []string{
			"  ! 2024-03-02  Chequing (CAD)       Costco Wholesale  amount -212.40  splits -202.40",
			"  ! 2024-03-02  Visa Infinite (CAD)  A                 amount    1.00  splits   -1.00",
		}, rows)
	})
	t.Run("closed and inactive accounts carry the full label", func(t *testing.T) {
		rows := splitMismatchRows([]store.SplitMismatch{
			{Account: "Old Visa", Currency: "CAD", Closed: true, Payee: "A", Date: date, Amount: 100, SplitsTotal: 0},
			{Account: "Savings", Currency: "USD", Payee: "A", Date: date, Amount: 100, SplitsTotal: 0},
		})
		assert.Equal(t, []string{
			"  ! 2024-03-02  Old Visa (CAD, closed)   A  amount 1.00  splits 0.00",
			"  ! 2024-03-02  Savings (USD, inactive)  A  amount 1.00  splits 0.00",
		}, rows)
	})
}

func Test_oneSidedRows(t *testing.T) {
	date := time.Date(2019, 6, 14, 0, 0, 0, 0, time.UTC)

	t.Run("numeric link renders other account unknown", func(t *testing.T) {
		rows := oneSidedRows([]store.OneSidedTransfer{
			{Date: date, Account: "Chequing", Currency: "CAD", Active: true, Payee: "Rent", Amount: -50000},
		})
		assert.Equal(t, []string{"  ? 2019-06-14  Chequing (CAD)  Rent  -500.00  other account: unknown"}, rows)
	})

	t.Run("name matching an account renders the name", func(t *testing.T) {
		rows := oneSidedRows([]store.OneSidedTransfer{
			{
				Date: date, Account: "Chequing", Currency: "CAD", Active: true, Payee: "Rent", Amount: -50000,
				OtherAccount: new("Savings"), OtherAccountID: new("acct-2"),
			},
		})
		assert.Equal(t, []string{"  ? 2019-06-14  Chequing (CAD)  Rent  -500.00  other account: Savings"}, rows)
	})

	t.Run("name matching no account is marked not in this file", func(t *testing.T) {
		rows := oneSidedRows([]store.OneSidedTransfer{
			{Date: date, Account: "Chequing", Currency: "CAD", Active: true, Payee: "Rent", Amount: -50000, OtherAccount: new("Old Visa")},
		})
		assert.Equal(t, []string{"  ? 2019-06-14  Chequing (CAD)  Rent  -500.00  other account: Old Visa (not in this file)"}, rows)
	})

	t.Run("empty payee falls back to (no payee)", func(t *testing.T) {
		rows := oneSidedRows([]store.OneSidedTransfer{
			{Date: date, Account: "Chequing", Currency: "CAD", Active: true, Amount: -50000},
		})
		assert.Equal(t, []string{"  ? 2019-06-14  Chequing (CAD)  (no payee)  -500.00  other account: unknown"}, rows)
	})

	t.Run("full label, widest label and payee, right-aligned amounts", func(t *testing.T) {
		rows := oneSidedRows([]store.OneSidedTransfer{
			{Date: date, Account: "Visa", Currency: "CAD", Closed: true, Payee: "Costco Wholesale", Amount: 100},
			{Date: date, Account: "Savings", Currency: "USD", Payee: "A", Amount: -120417},
		})
		assert.Equal(t, []string{
			"  ? 2019-06-14  Visa (CAD, closed)       Costco Wholesale       1.00  other account: unknown",
			"  ? 2019-06-14  Savings (USD, inactive)  A                 -1,204.17  other account: unknown",
		}, rows)
	})
}

func Test_renderStoreFailure(t *testing.T) {
	t.Run("NOT REBUILT when a previous store existed", func(t *testing.T) {
		result := store.Result{
			Path: "/Users/dave/Library/Application Support/quarry/quarry.duckdb",
			Validation: store.Validation{
				Balances: store.BalanceCheck{Checked: 2, Mismatched: []store.BalanceMismatch{
					{Name: "Chequing", Currency: "CAD", Active: true, StatementDate: time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC), Quarry: 100, Quicken: 200, Difference: -100},
				}},
				Splits: store.SplitCheck{Checked: 5},
				Shares: store.ShareCheck{Checked: 7},
				Transfers: store.TransferCheck{Paired: 1, OneSided: []store.OneSidedTransfer{
					{
						Date: time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC), Account: "Chequing", Currency: "CAD", Active: true,
						Payee: "Rent", Amount: -50000, OtherAccount: new("Old Visa"),
					},
				}},
			},
			NotImported: store.NotImported{InvestmentTransactions: 1},
		}

		got := renderStoreFailure(result, true, "/Users/dave")

		assert.Equal(t, "Store     NOT REBUILT (~/Library/Application Support/quarry/quarry.duckdb unchanged)\n"+
			"Rows      0 transactions, 0 splits, 0 transfers, 0 payees, 0 categories, 0 tags; 0 investment transactions, 0 securities, 0 prices; 1 investment transaction not imported\n"+
			"Balances  DIFFER for 1 of 2 accounts\n"+
			"  ! Chequing (CAD)  2026-08-31  quarry 1.00  Quicken 2.00  difference -1.00\n"+
			"Splits    all 5 transactions equal the sum of their splits\n"+
			"Shares    7 holdings match Quicken's share counts\n"+
			"Transfers 1 paired, 1 one-sided\n"+
			"  ? 2026-08-02  Chequing (CAD)  Rent  -500.00  other account: Old Visa (not in this file)\n", got)
	})

	t.Run("NOT BUILT for a first run", func(t *testing.T) {
		result := store.Result{Path: "/Users/dave/Library/Application Support/quarry/quarry.duckdb"}

		got := renderStoreFailure(result, false, "/Users/dave")

		assert.Contains(t, got, "Store     NOT BUILT (no store at ~/Library/Application Support/quarry/quarry.duckdb yet)\n")
	})

	t.Run("Splits line stays non-DIFFER when only Balances failed", func(t *testing.T) {
		result := store.Result{
			Validation: store.Validation{
				Balances: store.BalanceCheck{Checked: 1, Mismatched: []store.BalanceMismatch{{Name: "Chequing", Active: true}}},
				Splits:   store.SplitCheck{Checked: 3},
			},
		}

		got := renderStoreFailure(result, true, "/Users/dave")

		assert.Contains(t, got, "Splits    all 3 transactions equal the sum of their splits\n")
		assert.NotContains(t, got, "Splits    DIFFER")
	})

	t.Run("Shares pass line shows when only Balances failed", func(t *testing.T) {
		result := store.Result{
			Validation: store.Validation{
				Balances: store.BalanceCheck{Checked: 1, Mismatched: []store.BalanceMismatch{{Name: "Chequing", Active: true}}},
				Shares:   store.ShareCheck{Checked: 1},
			},
		}

		got := renderStoreFailure(result, true, "/Users/dave")

		assert.Contains(t, got, "Shares    1 holding matches Quicken's share count\n")
	})

	t.Run("Shares DIFFER block sits between Splits and Transfers, and the pass line is replaced", func(t *testing.T) {
		result := store.Result{
			Validation: store.Validation{
				Splits: store.SplitCheck{Checked: 5},
				Shares: store.ShareCheck{Checked: 3, Mismatched: []store.ShareMismatch{
					{Account: "RRSP", Currency: "CAD", Closed: true, Security: "iShares Core Equity ETF", Ticker: new("XEQT"), Quarry: 120500000, Quicken: 110500000, Difference: 10000000},
				}},
				Transfers: store.TransferCheck{Paired: 1},
			},
		}

		got := renderStoreFailure(result, true, "/Users/dave")

		assert.Contains(t, got, "Splits    all 5 transactions equal the sum of their splits\n"+
			"Shares    DIFFER for 1 of 3 holdings\n"+
			"  ! RRSP (CAD, closed)  iShares Core Equity ETF (XEQT)  quarry 120.5  Quicken 110.5  difference 10\n"+
			"Transfers 1 paired\n")
		assert.NotContains(t, got, "holdings match")
	})

	t.Run("Balances block precedes the Shares block when both DIFFER", func(t *testing.T) {
		result := store.Result{
			Validation: store.Validation{
				Balances: store.BalanceCheck{Checked: 2, Mismatched: []store.BalanceMismatch{
					{Name: "Chequing", Currency: "CAD", Active: true, StatementDate: time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC), Quarry: 100, Quicken: 200, Difference: -100},
				}},
				Splits: store.SplitCheck{Checked: 5},
				Shares: store.ShareCheck{Checked: 3, Mismatched: []store.ShareMismatch{
					{Account: "RRSP", Currency: "CAD", Active: true, Security: "Acme Corp", Quarry: 2000000, Quicken: 1000000, Difference: 1000000},
				}},
			},
		}

		got := renderStoreFailure(result, true, "/Users/dave")

		assert.Contains(t, got, "Balances  DIFFER for 1 of 2 accounts\n"+
			"  ! Chequing (CAD)  2026-08-31  quarry 1.00  Quicken 2.00  difference -1.00\n"+
			"Splits    all 5 transactions equal the sum of their splits\n"+
			"Shares    DIFFER for 1 of 3 holdings\n"+
			"  ! RRSP (CAD)  Acme Corp  quarry 2  Quicken 1  difference 1\n")
	})

	t.Run("Shares DIFFER noun is singular at 1 of 1", func(t *testing.T) {
		result := store.Result{
			Validation: store.Validation{Shares: store.ShareCheck{Checked: 1, Mismatched: []store.ShareMismatch{{Account: "RRSP"}}}},
		}

		got := renderStoreFailure(result, true, "/Users/dave")

		assert.Contains(t, got, "Shares    DIFFER for 1 of 1 holding\n")
	})

	t.Run("Shares DIFFER counts are plural and grouped by thousands", func(t *testing.T) {
		result := store.Result{
			Validation: store.Validation{Shares: store.ShareCheck{Checked: 1204, Mismatched: make([]store.ShareMismatch, 1000)}},
		}

		got := renderStoreFailure(result, true, "/Users/dave")

		assert.Contains(t, got, "Shares    DIFFER for 1,000 of 1,204 holdings\n")
	})

	t.Run("singular DIFFER noun at 1 of 1", func(t *testing.T) {
		result := store.Result{
			Validation: store.Validation{
				Balances: store.BalanceCheck{Checked: 1, Mismatched: []store.BalanceMismatch{{Name: "Chequing", Active: true}}},
				Splits:   store.SplitCheck{Checked: 1, Mismatched: []store.SplitMismatch{{Account: "Chequing"}}},
			},
		}

		got := renderStoreFailure(result, true, "/Users/dave")

		assert.Contains(t, got, "Balances  DIFFER for 1 of 1 account\n")
		assert.Contains(t, got, "Splits    DIFFER for 1 of 1 transaction\n")
	})

	t.Run("plural DIFFER noun control at 1 of 2", func(t *testing.T) {
		result := store.Result{
			Validation: store.Validation{
				Balances: store.BalanceCheck{Checked: 2, Mismatched: []store.BalanceMismatch{{Name: "Chequing", Active: true}}},
				Splits:   store.SplitCheck{Checked: 2, Mismatched: []store.SplitMismatch{{Account: "Chequing"}}},
			},
		}

		got := renderStoreFailure(result, true, "/Users/dave")

		assert.Contains(t, got, "Balances  DIFFER for 1 of 2 accounts\n")
		assert.Contains(t, got, "Splits    DIFFER for 1 of 2 transactions\n")
	})

	t.Run("DIFFER counts grouped by thousands", func(t *testing.T) {
		result := store.Result{
			Validation: store.Validation{
				Balances: store.BalanceCheck{Checked: 1035, Mismatched: make([]store.BalanceMismatch, 1000)},
				Splits:   store.SplitCheck{Checked: 1204, Mismatched: make([]store.SplitMismatch, 1000)},
			},
		}

		got := renderStoreFailure(result, true, "/Users/dave")

		assert.Contains(t, got, "Balances  DIFFER for 1,000 of 1,035 accounts\n")
		assert.Contains(t, got, "Splits    DIFFER for 1,000 of 1,204 transactions\n")
	})
}
