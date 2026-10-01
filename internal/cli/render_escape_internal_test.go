// White-box: every text renderer escapes a name's control characters through one
// escaper, and measures column widths after escaping.
package cli

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

func Test_escapeCell_writes_newline_tab_and_carriage_return_as_backslash_forms(t *testing.T) {
	assert.Equal(t, `a\nb\tc\rd`, escapeCell("a\nb\tc\rd"))
	assert.Equal(t, "plain 'name' \\ é", escapeCell("plain 'name' \\ é"))
}

func Test_accountLabel_escapes_control_characters_in_the_name(t *testing.T) {
	assert.Equal(t, `\tAccount (CAD)`, accountLabel("\tAccount", "CAD", false, true))
	assert.Equal(t, "Account (CAD)", accountLabel("Account", "CAD", false, true))
}

func Test_payeeLabel_escapes_control_characters_in_the_payee(t *testing.T) {
	assert.Equal(t, `Tim\nHortons`, payeeLabel("Tim\nHortons"))
	assert.Equal(t, "Tim Hortons", payeeLabel("Tim Hortons"))
}

func Test_otherAccountLabel_escapes_control_characters_in_the_name(t *testing.T) {
	cases := []struct {
		name string
		leg  store.OneSidedTransfer
		want string
	}{
		{"not in this file", store.OneSidedTransfer{OtherAccount: new("\tAccount Not Synced")}, `\tAccount Not Synced (not in this file)`},
		{"in this file", store.OneSidedTransfer{OtherAccount: new("Sav\ringS"), OtherAccountID: new("acct-2")}, `Sav\ringS`},
		{"a clean name is unchanged", store.OneSidedTransfer{OtherAccount: new("Old Visa")}, "Old Visa (not in this file)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, otherAccountLabel(c.leg))
		})
	}
}

func Test_renderStoreFailure_escapes_the_names_of_a_one_sided_leg_and_a_mismatch(t *testing.T) {
	result := store.Result{
		Validation: store.Validation{
			Balances: store.BalanceCheck{Checked: 1, Mismatched: []store.BalanceMismatch{
				{Name: "Cheq\tuing", Currency: "CAD", Active: true, StatementDate: time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)},
			}},
			Transfers: store.TransferCheck{OneSided: []store.OneSidedTransfer{
				{
					Date: time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC), Account: "Chequing", Currency: "CAD", Active: true,
					Payee: "Rent", Amount: -50000, OtherAccount: new("\tAccount Not Synced"),
				},
			}},
		},
	}

	got := renderStoreFailure(result, true, "/Users/dave")

	assert.Contains(t, got, "  ! Cheq\\tuing (CAD)  2026-08-31  quarry 0.00  Quicken 0.00  difference 0.00\n")
	assert.Contains(t, got, "  ? 2026-08-02  Chequing (CAD)  Rent  -500.00  other account: \\tAccount Not Synced (not in this file)\n")
}

func Test_oneSidedRows_measure_widths_after_escaping(t *testing.T) {
	date := time.Date(2019, 6, 14, 0, 0, 0, 0, time.UTC)

	rows := oneSidedRows([]store.OneSidedTransfer{
		{Date: date, Account: "A\tB", Currency: "CAD", Active: true, Payee: "x", Amount: 100},
		{Date: date, Account: "Visa", Currency: "CAD", Active: true, Payee: "y", Amount: 100},
	})

	assert.Equal(t, []string{
		`  ? 2019-06-14  A\tB (CAD)  x  1.00  other account: unknown`,
		`  ? 2019-06-14  Visa (CAD)  y  1.00  other account: unknown`,
	}, rows)
}

func Test_renderAccounts_escapes_an_account_name_and_pads_after_it(t *testing.T) {
	got := renderAccounts(store.AccountList{Accounts: []store.AccountBalance{
		balanceRow("\tAccount Not Synced", "chequing", "CAD", new(int64(100)), false, true),
		balanceRow("Chequing", "chequing", "CAD", new(int64(100)), false, true),
	}})

	assert.Equal(t, ""+
		"Account               Type      Currency  Balance  Status\n"+
		"\\tAccount Not Synced  chequing  CAD          1.00\n"+
		"Chequing              chequing  CAD          1.00\n", got)
}

func Test_renderSpending_escapes_a_category_key_and_an_account_name_in_the_caption(t *testing.T) {
	got := renderSpending(report.Spending{
		Window:   spendingWindow(),
		Accounts: []store.Account{{Name: "Chequ\ning"}},
		Rows:     []report.SpendingRow{{Key: new("Auto\t:Fuel"), Currency: "CAD", Spent: 100}, {Key: new("Food"), Currency: "CAD", Spent: 100}},
		Totals:   []store.SpendingTotal{{Currency: "CAD", Spent: 200}},
	})

	assert.Equal(t, ""+
		"Spending 2026-01-01 to 2026-03-09 in Chequ\\ning\n"+
		"\n"+
		"Category     Currency  Spent\n"+
		"Auto\\t:Fuel  CAD        1.00\n"+
		"Food         CAD        1.00\n"+
		"Total        CAD        2.00\n", got)
}

func Test_renderCashFlow_escapes_an_account_name_in_the_caption(t *testing.T) {
	got := renderCashFlow(report.CashFlow{
		Window:   spendingWindow(),
		Accounts: []store.Account{{Name: "A\tB"}, {Name: "C"}},
		Totals:   []store.CashFlowTotal{{Currency: "CAD"}},
	})

	assert.Contains(t, got, "Cash flow 2026-01-01 to 2026-03-09 in A\\tB, C\n")
}

func Test_findingsRows_escape_the_names_they_show(t *testing.T) {
	date := findingDay(2026, time.May, 1)
	t.Run("duplicate item account and payee", func(t *testing.T) {
		item := store.FindingItem{Account: "Chq\tA", Currency: "CAD", Active: true, Payee: "Tim\nHortons", Date: date, Amount: 100}
		rows := itemRows([]store.FindingItem{item})
		assert.Equal(t, []string{`2026-05-01  Chq\tA (CAD)  Tim\nHortons  1.00`}, rows)
	})
	t.Run("unlinked category path", func(t *testing.T) {
		item := store.FindingItem{Account: "A", Currency: "CAD", Active: true, Date: date, Amount: 100, Category: new("Auto\t:Fuel")}
		assert.Equal(t, []string{`2026-05-01  A (CAD)  (no payee)  1.00  Auto\t:Fuel`}, unlinkedRows([]store.FindingItem{item}))
	})
	t.Run("one-sided other account", func(t *testing.T) {
		item := store.FindingItem{
			Account: "Chequing", Currency: "CAD", Active: true, Payee: "Rent", Date: date, Amount: -50000,
			OtherAccount: new("\tAccount Not Synced"),
		}
		rows := oneSidedFindingRows([]report.ListedFinding{oneSidedFinding("one-sided-transfer:x-1", item)}, openView)
		assert.Equal(t, []string{`  one-sided-transfer:x-1  2026-05-01  Chequing (CAD)  Rent  -500.00  other account: \tAccount Not Synced (not in this file)`}, rows)
	})
	t.Run("uncategorized payee", func(t *testing.T) {
		rows := uncategorizedRows([]report.ListedFinding{uncategorizedFinding("uncategorized:payee-1", "Cafe\rBar", 1, date, date)}, openView)
		assert.Equal(t, []string{`  uncategorized:payee-1  Cafe\rBar  1 split  2026-05-01`}, rows)
	})
	t.Run("mixed payee and category paths", func(t *testing.T) {
		rows := mixedRows([]report.ListedFinding{mixedFinding("mixed-categories:payee-1", "Cost\tco", mixedRow{"Auto\t:Fuel", 3}, mixedRow{"Food", 1})}, openView)
		assert.Equal(t, []string{
			`  mixed-categories:payee-1  Cost\tco  2 categories, 4 transactions`,
			`    Auto\t:Fuel  3 transactions`,
			`    Food          1 transaction`,
		}, rows)
	})
	t.Run("payee variants names", func(t *testing.T) {
		rows := payeeVariantRows([]report.ListedFinding{variantFinding("payee-variants:tim", variantRow{"Tim\tHortons", 2}, variantRow{"Tim", 1})}, openView)
		assert.Equal(t, []string{
			`  payee-variants:tim  2 payees, 3 transactions`,
			`    Tim\tHortons  2 transactions`,
			`    Tim            1 transaction`,
		}, rows)
	})
	t.Run("similar categories paths", func(t *testing.T) {
		rows := similarCategoryRows([]report.ListedFinding{similarFinding("similar-categories:grocery", similarRow{"Gro\tceries", 2}, similarRow{"Grocery", 1})}, openView)
		assert.Equal(t, []string{
			`  similar-categories:grocery  2 categories`,
			`    Gro\tceries  2 splits`,
			`    Grocery       1 split`,
		}, rows)
	})
	t.Run("unused category path", func(t *testing.T) {
		rows := unusedCategoryRows([]report.ListedFinding{unusedFinding("unused-category:cat-1", "Va\tcation")}, openView)
		assert.Equal(t, []string{`  unused-category:cat-1  Va\tcation`}, rows)
	})
	t.Run("a clean name is unchanged", func(t *testing.T) {
		rows := unusedCategoryRows([]report.ListedFinding{unusedFinding("unused-category:cat-1", "Vacation")}, openView)
		assert.Equal(t, []string{`  unused-category:cat-1  Vacation`}, rows)
	})
}

func Test_renderFindings_a_hidden_finding_status_leaves_names_escaped(t *testing.T) {
	listing := report.FindingsListing{
		Groups: []report.FindingsGroup{variantsGroup(openFinding(store.Finding{
			ID: "payee-variants:a", Type: finding.PayeeVariants,
			Items: []store.FindingItem{{Payee: "A\nB", Transactions: 1}, {Payee: "a b", Transactions: 1}},
		}))},
		Counts: finding.Counts{Open: 1},
	}

	got := renderFindings(listing, openView, false)

	assert.Contains(t, got, "    A\\nB  1 transaction\n    a b   1 transaction\n")
}
