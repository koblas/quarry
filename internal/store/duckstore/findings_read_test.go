package duckstore_test

import (
	"slices"
	"testing"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readFinding builds rows in a fresh store and returns the finding with id from its Findings read.
func readFinding(t *testing.T, rows store.Rows, id string) store.Finding {
	t.Helper()
	dir := t.TempDir()
	_, err := duckstore.New(dir).Replace(t.Context(), rows)
	require.NoError(t, err)
	return findingIn(t, duckstore.New(dir), id)
}

// findingIn is the finding with id in st's Findings read.
func findingIn(t *testing.T, st *duckstore.Store, id string) store.Finding {
	t.Helper()
	list, err := st.Findings(t.Context())
	require.NoError(t, err)
	i := slices.IndexFunc(list.Findings, func(f store.Finding) bool { return f.ID == id })
	require.GreaterOrEqual(t, i, 0, "finding %s not listed", id)
	return list.Findings[i]
}

// oneSidedRows is minimalRows plus a one-sided transfer leg split-3 of -1.20 on txn-5 (-3.00), recording other and linked.
func oneSidedRows(other, linked *string) store.Rows {
	rows := withFindingCandidates()
	i := slices.IndexFunc(rows.Splits, func(s store.Split) bool { return s.ID == "split-3" })
	rows.Splits[i].Amount = -120
	rows.Splits[i].TransferAccountID = linked
	j := slices.IndexFunc(rows.Transfers, func(x store.Transfer) bool { return x.ID == "xfer-3" })
	rows.Transfers[j].OtherAccount = other
	return rows
}

func Test_findings_lists_the_two_transactions_of_a_duplicate_in_lower_id_order(t *testing.T) {
	t.Parallel()
	rows := minimalRows()
	first, second := dupTxn(9, "acct-1", day(2026, 8, 3), duplicateAmount, uncleared), dupTxn(10, "acct-1", day(2026, 8, 4), duplicateAmount, uncleared)
	first.PayeeID = new("payee-1")
	rows.Transactions = []store.Transaction{first, second}
	rows.Splits, rows.SplitTags, rows.Transfers = nil, nil, nil

	got := readFinding(t, rows, "duplicate:txn-9+txn-10")

	assert.Equal(t, finding.Duplicate, got.Type)
	assert.Nil(t, got.FixedAt)
	assert.Equal(t, []store.FindingItem{
		{
			TransactionID: new("txn-9"), Date: day(2026, 8, 3), AccountID: "acct-1", Account: "Chequing", Currency: "CAD",
			Active: true, Payee: "Coffee Shop", Amount: duplicateAmount,
		},
		{
			TransactionID: new("txn-10"), Date: day(2026, 8, 4), AccountID: "acct-1", Account: "Chequing", Currency: "CAD",
			Active: true, Amount: duplicateAmount,
		},
	}, got.Items)
}

func Test_findings_carries_a_closed_foreign_currency_account_on_its_items(t *testing.T) {
	t.Parallel()
	rows := minimalRows()
	rows.Accounts = append(rows.Accounts, store.Account{
		ID: "acct-2", SourceID: 2, Name: "Old Visa", Type: "credit", Currency: "USD", Closed: true,
	})
	first, second := dupTxn(9, "acct-2", day(2026, 8, 3), duplicateAmount, uncleared), dupTxn(10, "acct-2", day(2026, 8, 3), duplicateAmount, uncleared)
	first.Currency, second.Currency = "USD", "USD"
	rows.Transactions = []store.Transaction{first, second}
	rows.Splits, rows.SplitTags, rows.Transfers = nil, nil, nil

	got := readFinding(t, rows, "duplicate:txn-9+txn-10")

	require.Len(t, got.Items, 2)
	assert.Equal(t, []string{"acct-2", "Old Visa", "USD"}, []string{got.Items[0].AccountID, got.Items[0].Account, got.Items[0].Currency})
	assert.True(t, got.Items[0].Closed)
	assert.False(t, got.Items[0].Active)
}

func Test_findings_lists_a_one_sided_leg_with_its_split_amount_and_the_name_not_in_the_file(t *testing.T) {
	t.Parallel()

	got := readFinding(t, oneSidedRows(new("Savings"), nil), "one-sided-transfer:xfer-3")

	assert.Equal(t, finding.OneSidedTransfer, got.Type)
	assert.Equal(t, []store.FindingItem{{
		TransactionID: new("txn-5"), SplitID: new("split-3"), Date: day(2026, 3, 16), AccountID: "acct-1", Account: "Chequing",
		Currency: "CAD", Active: true, Amount: -120, OtherAccount: new("Savings"),
	}}, got.Items)
}

func Test_findings_lists_a_one_sided_leg_whose_name_matches_an_account_with_that_account_id(t *testing.T) {
	t.Parallel()

	got := readFinding(t, oneSidedRows(new("Chequing"), new("acct-1")), "one-sided-transfer:xfer-3")

	require.Len(t, got.Items, 1)
	assert.Equal(t, []*string{new("Chequing"), new("acct-1")}, []*string{got.Items[0].OtherAccount, got.Items[0].OtherAccountID})
}

func Test_findings_lists_a_one_sided_leg_linked_by_number_with_no_other_account(t *testing.T) {
	t.Parallel()

	got := readFinding(t, oneSidedRows(nil, nil), "one-sided-transfer:xfer-3")

	require.Len(t, got.Items, 1)
	assert.Equal(t, []*string{nil, nil}, []*string{got.Items[0].OtherAccount, got.Items[0].OtherAccountID})
}

func Test_findings_lists_one_uncategorized_finding_per_payee_with_an_item_per_split(t *testing.T) {
	t.Parallel()

	got := readFinding(t, withFindingCandidates(), "uncategorized:payee-1")

	assert.Equal(t, finding.Uncategorized, got.Type)
	assert.Equal(t, []store.FindingItem{
		{
			TransactionID: new("txn-3"), SplitID: new("split-6"), Date: day(2026, 3, 16), AccountID: "acct-1", Account: "Chequing",
			Currency: "CAD", Active: true, Payee: "Coffee Shop", Amount: -700,
		},
		{
			TransactionID: new("txn-4"), SplitID: new("split-7"), Date: day(2026, 3, 16), AccountID: "acct-1", Account: "Chequing",
			Currency: "CAD", Active: true, Payee: "Coffee Shop", Amount: -900,
		},
	}, got.Items)
}

func Test_findings_lists_uncategorized_splits_without_a_payee_under_no_payee_with_an_empty_payee(t *testing.T) {
	t.Parallel()

	got := readFinding(t, withFindingCandidates(), "uncategorized:no-payee")

	require.Len(t, got.Items, 1)
	assert.Equal(t, []any{"split-5", "", int64(-500)}, []any{*got.Items[0].SplitID, got.Items[0].Payee, got.Items[0].Amount})
}

func Test_findings_lists_every_finding_sorted_by_id(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := duckstore.New(dir).Replace(t.Context(), withFindingCandidates())
	require.NoError(t, err)

	list, err := duckstore.New(dir).Findings(t.Context())

	require.NoError(t, err)
	ids := make([]string, len(list.Findings))
	for i, f := range list.Findings {
		ids[i] = f.ID
	}
	assert.Equal(t, []string{"one-sided-transfer:xfer-3", "uncategorized:no-payee", "uncategorized:payee-1"}, ids)
}

func Test_findings_lists_no_findings_for_a_store_with_none(t *testing.T) {
	t.Parallel()
	rows := minimalRows()
	rows.Transfers = rows.Transfers[:1]
	dir := t.TempDir()
	_, err := duckstore.New(dir).Replace(t.Context(), rows)
	require.NoError(t, err)

	list, err := duckstore.New(dir).Findings(t.Context())

	require.NoError(t, err)
	assert.Empty(t, list.Findings)
}

func Test_findings_lists_a_fixed_finding_with_its_fix_time_and_no_items(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := duckstore.New(dir).Replace(t.Context(), withFindingCandidates())
	require.NoError(t, err)
	_, err = duckstore.New(dir).Replace(t.Context(), minimalRows())
	require.NoError(t, err)

	got := findingIn(t, duckstore.New(dir), "uncategorized:payee-1")

	assert.NotNil(t, got.FixedAt)
	assert.Empty(t, got.Items)
}

func Test_findings_marks_new_and_newly_fixed_by_the_latest_build_only(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := duckstore.New(dir).Replace(t.Context(), withFindingCandidates())
	require.NoError(t, err)
	_, err = duckstore.New(dir).Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	st := duckstore.New(dir)
	flags := func(id string) []bool {
		f := findingIn(t, st, id)
		return []bool{f.New, f.NewlyFixed}
	}

	assert.Equal(t, []bool{false, true}, flags("uncategorized:payee-1"), "fixed by the latest build")
	assert.Equal(t, []bool{false, false}, flags("one-sided-transfer:xfer-3"), "open since an earlier build")

	_, err = st.Replace(t.Context(), minimalRows())
	require.NoError(t, err)

	assert.Equal(t, []bool{false, false}, flags("uncategorized:payee-1"), "fixed by an earlier build")
}

func Test_findings_marks_a_finding_first_found_by_the_latest_build_as_new(t *testing.T) {
	t.Parallel()

	got := readFinding(t, withFindingCandidates(), "uncategorized:payee-1")

	assert.True(t, got.New)
	assert.False(t, got.NewlyFixed)
}

// categoryAndSplits is the two fields an unlinked-transfer item reads beyond its transaction's columns.
type categoryAndSplits struct {
	Category *string
	Splits   int
}

// unlinkedPair builds an unlinked pair txn-1 (acct-1, -500.00) and txn-2 (acct-2, +500.00) with category cat-2
// "Income:Other" added, applies splits to the rows, and returns what the read gives for each item.
func unlinkedPair(t *testing.T, splits ...store.Split) []categoryAndSplits {
	t.Helper()
	rows := unlinkedRows(func(r *store.Rows) {
		r.Categories = append(r.Categories, store.Category{ID: "cat-2", SourceID: 2, Name: "Other", FullPath: "Income:Other", Kind: "income"})
		r.Splits = splits
	},
		dupTxn(1, "acct-1", day(2026, 8, 3), -unlinkedAmount, uncleared),
		dupTxn(2, "acct-2", day(2026, 8, 4), unlinkedAmount, uncleared))
	got := readFinding(t, rows, "unlinked-transfer:txn-1+txn-2")
	out := make([]categoryAndSplits, len(got.Items))
	for i, item := range got.Items {
		out[i] = categoryAndSplits{item.Category, item.Splits}
	}
	return out
}

func Test_findings_reads_the_category_path_of_the_sole_split_of_each_unlinked_transfer_item(t *testing.T) {
	t.Parallel()

	got := unlinkedPair(t,
		store.Split{ID: "split-1", SourceID: 1, TransactionID: "txn-1", CategoryID: new("cat-2"), Amount: -unlinkedAmount},
		store.Split{ID: "split-2", SourceID: 2, TransactionID: "txn-2", CategoryID: new("cat-1"), Amount: unlinkedAmount})

	assert.Equal(t, []categoryAndSplits{{new("Income:Other"), 1}, {new("Groceries"), 1}}, got)
}

func Test_findings_reads_no_category_for_an_unlinked_transfer_item_whose_only_split_has_none_or_that_has_no_split(t *testing.T) {
	t.Parallel()

	got := unlinkedPair(t, store.Split{ID: "split-1", SourceID: 1, TransactionID: "txn-1", Amount: -unlinkedAmount})

	assert.Equal(t, []categoryAndSplits{{nil, 1}, {nil, 0}}, got)
}

func Test_findings_reads_the_split_count_and_no_category_for_an_unlinked_transfer_item_with_two_categorized_splits(t *testing.T) {
	t.Parallel()

	got := unlinkedPair(t,
		store.Split{ID: "split-1", SourceID: 1, TransactionID: "txn-1", CategoryID: new("cat-1"), Amount: -30000},
		store.Split{ID: "split-4", SourceID: 4, TransactionID: "txn-1", CategoryID: new("cat-2"), Amount: -20000},
		store.Split{ID: "split-2", SourceID: 2, TransactionID: "txn-2", CategoryID: new("cat-1"), Amount: unlinkedAmount})

	assert.Equal(t, []categoryAndSplits{{nil, 2}, {new("Groceries"), 1}}, got)
}

func Test_findings_reads_no_category_or_split_count_for_the_items_of_a_duplicate(t *testing.T) {
	t.Parallel()
	rows := minimalRows()
	rows.Transactions = []store.Transaction{
		dupTxn(9, "acct-1", day(2026, 8, 3), duplicateAmount, uncleared), dupTxn(10, "acct-1", day(2026, 8, 4), duplicateAmount, uncleared),
	}
	rows.Splits = []store.Split{
		{ID: "split-9", SourceID: 9, TransactionID: "txn-9", CategoryID: new("cat-1"), Amount: duplicateAmount},
		{ID: "split-10", SourceID: 10, TransactionID: "txn-10", CategoryID: new("cat-1"), Amount: duplicateAmount},
	}
	rows.SplitTags, rows.Transfers = nil, nil

	got := readFinding(t, rows, "duplicate:txn-9+txn-10")

	require.Len(t, got.Items, 2)
	assert.Equal(t, []categoryAndSplits{{nil, 0}, {nil, 0}},
		[]categoryAndSplits{{got.Items[0].Category, got.Items[0].Splits}, {got.Items[1].Category, got.Items[1].Splits}})
}
