// White-box: formatMB, renderSuccess and the phrase builders are unexported
// formatting rules best driven directly, rather than through a full command
// run for every rounding and boundary case.
package cli

import (
	"strings"
	"testing"
	"time"

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
			want:   "0 transactions, 0 splits, 0 transfers, 0 payees, 0 categories, 0 tags",
		},
		{
			name:   "singular at exactly one",
			counts: store.Counts{Transactions: 1, Splits: 1, Transfers: 1, Payees: 1, Categories: 1, Tags: 1},
			want:   "1 transaction, 1 split, 1 transfer, 1 payee, 1 category, 1 tag",
		},
		{
			name: "thousands-grouped at many",
			counts: store.Counts{
				Transactions: 18204, Splits: 21977, Transfers: 3112, Payees: 1873, Categories: 312, Tags: 14,
			},
			want: "18,204 transactions, 21,977 splits, 3,112 transfers, 1,873 payees, 312 categories, 14 tags",
		},
		{
			name:        "one investment transaction not imported, singular",
			notImported: store.NotImported{InvestmentTransactions: 1},
			want:        "0 transactions, 0 splits, 0 transfers, 0 payees, 0 categories, 0 tags; 1 investment transaction not imported",
		},
		{
			name:        "many investment transactions not imported, thousands-grouped",
			notImported: store.NotImported{InvestmentTransactions: 1605},
			want:        "0 transactions, 0 splits, 0 transfers, 0 payees, 0 categories, 0 tags; 1,605 investment transactions not imported",
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

func Test_renderStore_lists_one_sided_transfers_after_the_transfers_line(t *testing.T) {
	result := store.Result{
		Path: "/Users/dave/Library/Application Support/quarry/quarry.duckdb",
		Validation: store.Validation{Transfers: store.TransferCheck{Paired: 2, OneSided: []store.OneSidedTransfer{
			{Date: time.Date(2019, 6, 14, 0, 0, 0, 0, time.UTC), Account: "Chequing", Currency: "CAD", Active: true, Amount: -50000},
		}}},
	}

	got := renderStore(result, "/Users/dave")

	assert.True(t, strings.HasSuffix(got, "Transfers 2 paired, 1 one-sided\n"+
		"  ? 2019-06-14  Chequing (CAD)  (no payee)  -500.00  other account: unknown\n"), got)
}

func Test_renderStore_renders_the_store_rows_balances_splits_and_transfers_lines(t *testing.T) {
	result := store.Result{
		Path:   "/Users/dave/Library/Application Support/quarry/quarry.duckdb",
		Counts: store.Counts{Transactions: 1},
		Validation: store.Validation{
			Balances:  store.BalanceCheck{Checked: 1},
			Splits:    store.SplitCheck{Checked: 1},
			Transfers: store.TransferCheck{Paired: 2},
		},
		NotImported: store.NotImported{InvestmentTransactions: 3},
	}

	got := renderStore(result, "/Users/dave")

	assert.Equal(t, "Store     ~/Library/Application Support/quarry/quarry.duckdb\n"+
		"Rows      1 transaction, 0 splits, 0 transfers, 0 payees, 0 categories, 0 tags; 3 investment transactions not imported\n"+
		"Balances  1 account matches Quicken's last reconciled balance\n"+
		"Splits    the 1 transaction equals the sum of its splits\n"+
		"Transfers 2 paired\n", got)
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
			"Rows      0 transactions, 0 splits, 0 transfers, 0 payees, 0 categories, 0 tags; 1 investment transaction not imported\n"+
			"Balances  DIFFER for 1 of 2 accounts\n"+
			"  ! Chequing (CAD)  2026-08-31  quarry 1.00  Quicken 2.00  difference -1.00\n"+
			"Splits    all 5 transactions equal the sum of their splits\n"+
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
