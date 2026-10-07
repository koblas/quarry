package duckstore_test

import (
	"context"
	"fmt"
	"os"
	"slices"
	"testing"

	duckdbdriver "github.com/duckdb/duckdb-go/v2"
	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_replace_creates_the_findings_tables(t *testing.T) {
	t.Parallel()
	replaced, err := duckstore.New(t.TempDir()).Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	db := openReadOnly(t, replaced.Path)
	columns := `SELECT string_agg(column_name || ' ' || data_type || ' ' || is_nullable, ', ' ORDER BY ordinal_position)
		FROM information_schema.columns WHERE table_name = '`

	assertScalar(t, db, columns+`findings'`,
		"id VARCHAR NO, type VARCHAR NO, first_found_at TIMESTAMP NO, fixed_at TIMESTAMP YES")
	assertScalar(t, db, columns+`finding_items'`,
		"finding_id VARCHAR NO, transaction_id VARCHAR YES, split_id VARCHAR YES, payee_id VARCHAR YES, category_id VARCHAR YES")
	assertScalar(t, db, `SELECT CAST(constraint_column_names AS VARCHAR) FROM duckdb_constraints()
		WHERE table_name = 'findings' AND constraint_type = 'PRIMARY KEY'`, "[id]")
}

func Test_replace_records_each_finding_with_its_items(t *testing.T) {
	t.Parallel()
	replaced, err := duckstore.New(t.TempDir()).Replace(t.Context(), withFindingCandidates())
	require.NoError(t, err)
	db := openReadOnly(t, replaced.Path)

	assertScalar(t, db, `SELECT string_agg(f.id || '|' || f.type || '|' || COALESCE(i.transaction_id, 'NULL') || '|' || COALESCE(i.split_id, 'NULL') || '|' ||
		COALESCE(i.payee_id, 'NULL') || '|' || COALESCE(i.category_id, 'NULL'), '; ' ORDER BY f.id, i.split_id)
		FROM findings f JOIN finding_items i ON i.finding_id = f.id`,
		"one-sided-transfer:xfer-3|one-sided-transfer|txn-5|split-3|NULL|NULL; "+
			"uncategorized:no-payee|uncategorized|txn-2|split-5|NULL|NULL; "+
			"uncategorized:payee-1|uncategorized|txn-3|split-6|NULL|NULL; "+
			"uncategorized:payee-1|uncategorized|txn-4|split-7|NULL|NULL")
	assertScalar(t, db, `SELECT CAST(count(*) AS VARCHAR) || ' ' || CAST(count(fixed_at) AS VARCHAR) FROM findings`, "3 0")
}

func Test_replace_records_a_one_sided_transfer_whose_from_split_is_missing(t *testing.T) {
	t.Parallel()
	replaced, err := duckstore.New(t.TempDir()).Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	db := openReadOnly(t, replaced.Path)

	assertScalar(t, db, `SELECT string_agg(finding_id || ' ' || COALESCE(transaction_id, 'NULL') || ' ' || split_id, '; ') FROM finding_items`,
		"one-sided-transfer:xfer-3 NULL split-3")
}

func Test_replace_reports_the_findings_counts(t *testing.T) {
	t.Parallel()

	replaced, err := duckstore.New(t.TempDir()).Replace(t.Context(), withFindingCandidates())

	require.NoError(t, err)
	assert.Equal(t, finding.Counts{Open: 3, New: 3}, replaced.Findings)
}

func Test_replace_reports_no_findings_when_nothing_is_detected(t *testing.T) {
	t.Parallel()
	rows := minimalRows()
	rows.Transfers = rows.Transfers[:1]

	replaced, err := duckstore.New(t.TempDir()).Replace(t.Context(), rows)

	require.NoError(t, err)
	assert.Equal(t, finding.Counts{}, replaced.Findings)
}

func Test_replace_keeps_the_previous_store_when_detection_fails(t *testing.T) {
	t.Parallel()
	queryFault := ioFault(`query rows "SELECT"`)
	appendFault := &duckdbdriver.Error{Type: duckdbdriver.ErrorTypeConstraint, Msg: `Constraint Error: Duplicate key "id: x" violates primary key constraint`}
	cases := []struct {
		name  string
		fault *faultDB
		want  string
	}{
		{"the duplicate query", &faultDB{queryFaultOn: duckstore.DuplicateQuery, queryFault: queryFault}, "detect duplicate findings"},
		{"scanning a duplicate row", &faultDB{queryFaultOn: duckstore.DuplicateQuery, scanFault: queryFault}, "detect duplicate findings"},
		{"the one-sided query", &faultDB{queryFaultOn: duckstore.OneSidedTransferQuery, queryFault: queryFault}, "detect one-sided-transfer findings"},
		{"the unlinked-transfer query", &faultDB{queryFaultOn: duckstore.UnlinkedTransferQuery, queryFault: queryFault}, "detect unlinked-transfer findings"},
		{"scanning an unlinked-transfer row", &faultDB{queryFaultOn: duckstore.UnlinkedTransferQuery, scanFault: queryFault}, "detect unlinked-transfer findings"},
		{"the mixed-categories query", &faultDB{queryFaultOn: duckstore.MixedCategoriesQuery, queryFault: queryFault}, "detect mixed-categories findings"},
		{"scanning a mixed-categories row", &faultDB{queryFaultOn: duckstore.MixedCategoriesQuery, scanFault: queryFault}, "detect mixed-categories findings"},
		{"the payee-variants query", &faultDB{queryFaultOn: duckstore.PayeeVariantsQuery, queryFault: queryFault}, "detect payee-variants findings"},
		{"scanning a payee-variants row", &faultDB{queryFaultOn: duckstore.PayeeVariantsQuery, scanFault: queryFault}, "detect payee-variants findings"},
		{"the similar-categories query", &faultDB{queryFaultOn: duckstore.SimilarCategoriesQuery, queryFault: queryFault}, "detect similar-categories findings"},
		{"scanning a similar-categories row", &faultDB{queryFaultOn: duckstore.SimilarCategoriesQuery, scanFault: queryFault}, "detect similar-categories findings"},
		{"the unused-category query", &faultDB{queryFaultOn: duckstore.UnusedCategoryQuery, queryFault: queryFault}, "detect unused-category findings"},
		{"scanning an unused-category row", &faultDB{queryFaultOn: duckstore.UnusedCategoryQuery, scanFault: queryFault}, "detect unused-category findings"},
		{"the uncategorized query", &faultDB{queryFaultOn: duckstore.UncategorizedQuery, queryFault: queryFault}, "detect uncategorized findings"},
		{"scanning a one-sided row", &faultDB{queryFaultOn: duckstore.OneSidedTransferQuery, scanFault: queryFault}, "detect one-sided-transfer findings"},
		{"scanning an uncategorized row", &faultDB{queryFaultOn: duckstore.UncategorizedQuery, scanFault: queryFault}, "detect uncategorized findings"},
		{"appending findings", &faultDB{appendFaultTable: "findings", appendFault: appendFault}, "load findings"},
		{"appending finding items", &faultDB{appendFaultTable: "finding_items", appendFault: appendFault}, "load finding_items"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			replaced, err := duckstore.New(dir).Replace(t.Context(), minimalRows())
			require.NoError(t, err)
			before, err := os.ReadFile(replaced.Path)
			require.NoError(t, err)
			rows := withFindingCandidates()
			rows.Transactions[0].Amount = 999

			_, err = newFaultStore(dir, c.fault).Replace(t.Context(), rows)

			require.ErrorContains(t, err, c.want)
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			assert.Equal(t, []string{"quarry.duckdb"}, direntNames(entries))
			after, err := os.ReadFile(replaced.Path)
			require.NoError(t, err)
			assert.Equal(t, before, after)
		})
	}
}

// findingTimes is every findings row as id|type|first_found_at|fixed_at text, ordered by id.
func findingTimes(t *testing.T, st *duckstore.Store) string {
	t.Helper()
	db := openReadOnly(t, st.Path())
	var out string
	require.NoError(t, db.QueryRows(t.Context(), `SELECT COALESCE(string_agg(id || '|' || type || '|' || CAST(first_found_at AS VARCHAR) || '|' ||
		COALESCE(CAST(fixed_at AS VARCHAR), 'NULL'), '; ' ORDER BY id), '') FROM findings`, nil,
		func(scan func(dest ...any) error) error { return scan(&out) }))
	require.NoError(t, db.Close())
	return out
}

func Test_replace_marks_a_carried_finding_not_detected_fixed_at_the_build_time(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := duckstore.New(dir).Replace(t.Context(), withFindingCandidates())
	require.NoError(t, err)

	replaced, err := duckstore.New(dir).Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	db := openReadOnly(t, replaced.Path)
	assertScalar(t, db, `SELECT CAST(count(*) AS VARCHAR) FROM findings WHERE id LIKE 'uncategorized:%'
		AND fixed_at = (SELECT built_at FROM store_info)`, "2")
	assertScalar(t, db, `SELECT CAST(count(*) AS VARCHAR) FROM finding_items WHERE finding_id LIKE 'uncategorized:%'`, "0")
	assertScalar(t, db, `SELECT CAST(count(*) AS VARCHAR) FROM findings WHERE id = 'one-sided-transfer:xfer-3' AND fixed_at IS NULL`, "1")
}

func Test_replace_keeps_the_type_of_a_carried_finding_that_is_no_longer_detected(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := duckstore.New(dir).Replace(t.Context(), withFindingCandidates())
	require.NoError(t, err)
	rows := withFindingCandidates()
	rows.Transfers = slices.DeleteFunc(rows.Transfers, func(x store.Transfer) bool { return x.ID == "xfer-3" })

	replaced, err := duckstore.New(dir).Replace(t.Context(), rows)

	require.NoError(t, err)
	db := openReadOnly(t, replaced.Path)
	assertScalar(t, db, `SELECT type || ' ' || CAST(fixed_at = (SELECT built_at FROM store_info) AS VARCHAR)
		FROM findings WHERE id = 'one-sided-transfer:xfer-3'`, "one-sided-transfer true")
}

func Test_replace_keeps_a_fixed_finding_fixed_at_its_first_fix_time(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := duckstore.New(dir).Replace(t.Context(), withFindingCandidates())
	require.NoError(t, err)
	_, err = duckstore.New(dir).Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	st := duckstore.New(dir)
	afterFirstFix := findingTimes(t, st)

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.Equal(t, afterFirstFix, findingTimes(t, st))
	assert.Equal(t, finding.Counts{Open: 1, Fixed: 2}, replaced.Findings)
}

func Test_replace_reopens_a_fixed_finding_keeping_its_first_found_at(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := duckstore.New(dir).Replace(t.Context(), withFindingCandidates())
	require.NoError(t, err)
	st := duckstore.New(dir)
	firstFound := findingTimes(t, st)
	_, err = st.Replace(t.Context(), minimalRows())
	require.NoError(t, err)

	replaced, err := st.Replace(t.Context(), withFindingCandidates())

	require.NoError(t, err)
	assert.Equal(t, firstFound, findingTimes(t, st))
	assert.Equal(t, finding.Counts{Open: 3}, replaced.Findings)
}

func Test_replace_counts_new_and_newly_fixed_as_the_findings_stamped_with_the_build_time(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := duckstore.New(dir).Replace(t.Context(), withFindingCandidates())
	require.NoError(t, err)
	rows := withFindingCandidates()
	rows.Transactions = slices.DeleteFunc(rows.Transactions, func(x store.Transaction) bool { return x.ID == "txn-2" })
	rows.Splits = slices.DeleteFunc(rows.Splits, func(x store.Split) bool { return x.ID == "split-5" })
	rows.Payees = append(rows.Payees, store.Payee{ID: "payee-2", SourceID: 2, Name: "Bakery"})
	rows.Transactions = append(rows.Transactions, store.Transaction{
		ID: "txn-6", SourceID: 6, AccountID: "acct-1", Date: day(2026, 3, 16), PayeeID: new("payee-2"),
		Amount: -100, Currency: "CAD", Status: "uncleared",
	})
	rows.Splits = append(rows.Splits, store.Split{ID: "split-8", SourceID: 8, TransactionID: "txn-6", Amount: -100})

	replaced, err := duckstore.New(dir).Replace(t.Context(), rows)

	require.NoError(t, err)
	assert.Equal(t, finding.Counts{Open: 3, Fixed: 1, New: 1, NewlyFixed: 1}, replaced.Findings)
	db := openReadOnly(t, replaced.Path)
	assertScalar(t, db, `SELECT CAST(count(*) AS VARCHAR) FROM findings WHERE first_found_at = (SELECT built_at FROM store_info) AND fixed_at IS NULL`, "1")
	assertScalar(t, db, `SELECT CAST(count(*) AS VARCHAR) FROM findings WHERE fixed_at = (SELECT built_at FROM store_info)`, "1")
}

func Test_replace_reports_each_findings_state_new_carried_reopened_and_newly_fixed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := duckstore.New(dir).Replace(t.Context(), withFindingCandidates())
	require.NoError(t, err)
	_, err = duckstore.New(dir).Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	rows := withFindingCandidates()
	rows.Transfers = slices.DeleteFunc(rows.Transfers, func(x store.Transfer) bool { return x.ID == "xfer-3" })
	rows.Transactions = slices.DeleteFunc(rows.Transactions, func(x store.Transaction) bool { return x.ID == "txn-2" })
	rows.Splits = slices.DeleteFunc(rows.Splits, func(x store.Split) bool { return x.ID == "split-5" })
	rows.Payees = append(rows.Payees, store.Payee{ID: "payee-2", SourceID: 2, Name: "Bakery"})
	rows.Transactions = append(rows.Transactions, store.Transaction{
		ID: "txn-6", SourceID: 6, AccountID: "acct-1", Date: day(2026, 3, 16), PayeeID: new("payee-2"),
		Amount: -100, Currency: "CAD", Status: "uncleared",
	})
	rows.Splits = append(rows.Splits, store.Split{ID: "split-8", SourceID: 8, TransactionID: "txn-6", Amount: -100})

	replaced, err := duckstore.New(dir).Replace(t.Context(), rows)

	require.NoError(t, err)
	assert.ElementsMatch(t, []finding.State{
		{ID: "uncategorized:payee-2", New: true},
		{ID: "uncategorized:payee-1"},
		{ID: "one-sided-transfer:xfer-3", Fixed: true, NewlyFixed: true},
		{ID: "uncategorized:no-payee"},
	}, replaced.FindingStates)
}

func Test_replace_keeps_a_carried_finding_of_an_unknown_type_out_of_the_findings_states(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	withUnknown := duckstore.WithCreate(func(ctx context.Context, path string) (duckstore.DB, error) {
		db, err := duckdb.Create(ctx, path)
		if err != nil {
			return nil, err
		}
		return &editDB{DB: db, statements: []string{`INSERT INTO findings VALUES ('future-kind:x', 'future-kind', now(), NULL)`}}, nil
	})
	_, err := duckstore.New(dir, withUnknown).Replace(t.Context(), withFindingCandidates())
	require.NoError(t, err)

	replaced, err := duckstore.New(dir).Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.ElementsMatch(t, []finding.State{
		{ID: "one-sided-transfer:xfer-3"},
		{ID: "uncategorized:payee-1", Fixed: true, NewlyFixed: true},
		{ID: "uncategorized:no-payee", Fixed: true, NewlyFixed: true},
	}, replaced.FindingStates)
	assert.Equal(t, finding.Counts{Open: 1, Fixed: 2, NewlyFixed: 2}, replaced.Findings)
	assert.Contains(t, findingTimes(t, duckstore.New(dir)), "future-kind:x|future-kind|")
}

// investmentCash is txn as an investment cash row, the way sync stores a brokerage dividend or sale.
func investmentCash(txn store.Transaction) store.Transaction {
	txn.InvestmentTransactionID = new(fmt.Sprintf("itxn-%d", txn.SourceID))
	return txn
}

// identity is the register-row arm of a case that varies which side is an investment cash row.
func identity(txn store.Transaction) store.Transaction { return txn }

// duplicateIDs replaces the transactions of minimalRows with txns (and the extra closed account acct-2) and returns the
// duplicate finding ids of the built store, comma-joined in id order.
func duplicateIDs(t *testing.T, mutate func(*store.Rows), txns ...store.Transaction) string {
	t.Helper()
	rows := minimalRows()
	rows.Accounts = append(rows.Accounts, store.Account{
		ID: "acct-2", SourceID: 2, Name: "Old Visa", Type: "credit", Currency: "CAD", Closed: true, NotInReports: true,
	})
	rows.Transactions = txns
	rows.Splits, rows.SplitTags, rows.Transfers = nil, nil, nil
	if mutate != nil {
		mutate(&rows)
	}
	replaced, err := duckstore.New(t.TempDir()).Replace(t.Context(), rows)
	require.NoError(t, err)
	var ids string
	require.NoError(t, openReadOnly(t, replaced.Path).QueryRows(t.Context(),
		`SELECT COALESCE(string_agg(id, ',' ORDER BY id), '') FROM findings WHERE type = 'duplicate'`, nil,
		func(scan func(dest ...any) error) error { return scan(&ids) }))
	return ids
}

func Test_replace_flags_two_same_amount_transactions_up_to_three_days_apart_as_a_duplicate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		// lowerIDApart is the lower-id transaction's date minus the higher-id one's, in days.
		lowerIDApart int
		want         string
	}{
		{"the same day", 0, "duplicate:txn-1+txn-2"},
		{"three days apart, the last day in", -3, "duplicate:txn-1+txn-2"},
		{"four days apart, the first day out", -4, ""},
		{"three days apart, the lower id dated later", 3, "duplicate:txn-1+txn-2"},
		{"four days apart, the lower id dated later", 4, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			start := day(2026, 8, 10)

			got := duplicateIDs(t, nil,
				dupTxn(1, "acct-1", start.AddDate(0, 0, c.lowerIDApart), duplicateAmount, uncleared),
				dupTxn(2, "acct-1", start, duplicateAmount, uncleared))

			assert.Equal(t, c.want, got)
		})
	}
}

func Test_replace_flags_a_pair_with_one_reconciled_transaction_but_not_a_reconciled_pair(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		first, other string
		want         string
	}{
		{"neither reconciled", uncleared, uncleared, "duplicate:txn-1+txn-2"},
		{"only the first reconciled", reconciled, uncleared, "duplicate:txn-1+txn-2"},
		{"only the second reconciled", uncleared, reconciled, "duplicate:txn-1+txn-2"},
		{"both reconciled", reconciled, reconciled, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := duplicateIDs(t, nil,
				dupTxn(1, "acct-1", day(2026, 8, 3), duplicateAmount, c.first),
				dupTxn(2, "acct-1", day(2026, 8, 4), duplicateAmount, c.other))

			assert.Equal(t, c.want, got)
		})
	}
}

func Test_replace_does_not_flag_transactions_that_differ_in_account_or_amount_or_are_zero(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		firstAccount string
		otherAccount string
		firstAmount  int64
		otherAmount  int64
	}{
		{"in different accounts", "acct-1", "acct-2", duplicateAmount, duplicateAmount},
		{"a cent apart", "acct-1", "acct-1", duplicateAmount, duplicateAmount - 1},
		{"opposite signs", "acct-1", "acct-1", duplicateAmount, -duplicateAmount},
		{"zero amounts", "acct-1", "acct-1", 0, 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := duplicateIDs(t, nil,
				dupTxn(1, c.firstAccount, day(2026, 8, 3), c.firstAmount, uncleared),
				dupTxn(2, c.otherAccount, day(2026, 8, 4), c.otherAmount, uncleared))

			assert.Empty(t, got)
		})
	}
}

func Test_replace_flags_duplicates_in_every_kind_of_account_and_ignores_the_payee(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		mutate func(*store.Rows)
		txns   []store.Transaction
	}{
		{"a closed account outside reports", nil, []store.Transaction{
			dupTxn(1, "acct-2", day(2026, 8, 3), duplicateAmount, uncleared), dupTxn(2, "acct-2", day(2026, 8, 4), duplicateAmount, uncleared),
		}},
		{"a linked-tracking account", func(r *store.Rows) { r.Accounts[0].LinkedTracking = true }, []store.Transaction{
			dupTxn(1, "acct-1", day(2026, 8, 3), duplicateAmount, uncleared), dupTxn(2, "acct-1", day(2026, 8, 4), duplicateAmount, uncleared),
		}},
		{"transactions excluded from reports", nil, []store.Transaction{
			{ID: "txn-1", SourceID: 1, AccountID: "acct-1", Date: day(2026, 8, 3), Amount: duplicateAmount, Currency: "CAD", Status: uncleared, ExcludedFromReports: true},
			dupTxn(2, "acct-1", day(2026, 8, 4), duplicateAmount, uncleared),
		}},
		{"transfer legs", func(r *store.Rows) {
			r.Splits = []store.Split{
				{ID: "split-1", SourceID: 1, TransactionID: "txn-1", Amount: duplicateAmount, TransferAccountID: new("acct-2")},
				{ID: "split-2", SourceID: 2, TransactionID: "txn-2", Amount: duplicateAmount, TransferAccountID: new("acct-2")},
			}
		}, []store.Transaction{
			dupTxn(1, "acct-1", day(2026, 8, 3), duplicateAmount, uncleared), dupTxn(2, "acct-1", day(2026, 8, 4), duplicateAmount, uncleared),
		}},
		{"a payee on only one", nil, []store.Transaction{
			{ID: "txn-1", SourceID: 1, AccountID: "acct-1", Date: day(2026, 8, 3), Amount: duplicateAmount, Currency: "CAD", Status: uncleared, PayeeID: new("payee-1")},
			dupTxn(2, "acct-1", day(2026, 8, 4), duplicateAmount, uncleared),
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := duplicateIDs(t, c.mutate, c.txns...)

			assert.Equal(t, "duplicate:txn-1+txn-2", got)
		})
	}
}

func Test_replace_does_not_flag_a_duplicate_when_either_transaction_is_an_investment_cash_row(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		first, other func(store.Transaction) store.Transaction
	}{
		{"both investment cash rows", investmentCash, investmentCash},
		{"the lower id a register row, the higher id an investment cash row", identity, investmentCash},
		{"the lower id an investment cash row, the higher id a register row", investmentCash, identity},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := duplicateIDs(t, nil,
				c.first(dupTxn(1, "acct-1", day(2026, 8, 3), duplicateAmount, uncleared)),
				c.other(dupTxn(2, "acct-1", day(2026, 8, 3), duplicateAmount, uncleared)))

			assert.Empty(t, got)
		})
	}
}

func Test_replace_records_one_duplicate_finding_per_pair_of_three_matching_transactions(t *testing.T) {
	t.Parallel()

	got := duplicateIDs(t, nil,
		dupTxn(9, "acct-1", day(2026, 8, 3), duplicateAmount, uncleared),
		dupTxn(10, "acct-1", day(2026, 8, 4), duplicateAmount, uncleared),
		dupTxn(11, "acct-1", day(2026, 8, 5), duplicateAmount, uncleared))

	assert.Equal(t, "duplicate:txn-10+txn-11,duplicate:txn-9+txn-10,duplicate:txn-9+txn-11", got)
}

func Test_replace_records_the_two_transactions_of_a_duplicate_as_its_items(t *testing.T) {
	t.Parallel()
	rows := minimalRows()
	rows.Transactions = []store.Transaction{
		dupTxn(9, "acct-1", day(2026, 8, 3), duplicateAmount, uncleared),
		dupTxn(10, "acct-1", day(2026, 8, 4), duplicateAmount, uncleared),
	}
	replaced, err := duckstore.New(t.TempDir()).Replace(t.Context(), rows)
	require.NoError(t, err)
	db := openReadOnly(t, replaced.Path)

	assertScalar(t, db, `SELECT string_agg(COALESCE(transaction_id, 'NULL') || '|' || COALESCE(split_id, 'NULL') || '|' ||
		COALESCE(payee_id, 'NULL') || '|' || COALESCE(category_id, 'NULL'), '; ' ORDER BY transaction_id)
		FROM finding_items WHERE finding_id = 'duplicate:txn-9+txn-10'`,
		"txn-10|NULL|NULL|NULL; txn-9|NULL|NULL|NULL")
}

// unlinkedIDs returns the unlinked-transfer finding ids of the store built from unlinkedRows, comma-joined in id order.
func unlinkedIDs(t *testing.T, mutate func(*store.Rows), txns ...store.Transaction) string {
	t.Helper()
	replaced, err := duckstore.New(t.TempDir()).Replace(t.Context(), unlinkedRows(mutate, txns...))
	require.NoError(t, err)
	var ids string
	require.NoError(t, openReadOnly(t, replaced.Path).QueryRows(t.Context(),
		`SELECT COALESCE(string_agg(id, ',' ORDER BY id), '') FROM findings WHERE type = 'unlinked-transfer'`, nil,
		func(scan func(dest ...any) error) error { return scan(&ids) }))
	return ids
}

func Test_replace_flags_opposite_amounts_in_two_accounts_up_to_three_days_apart_as_an_unlinked_transfer(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		// lowerIDApart is the lower-id transaction's date minus the higher-id one's, in days.
		lowerIDApart int
		want         string
	}{
		{"the same day", 0, "unlinked-transfer:txn-1+txn-2"},
		{"three days apart, the last day in", -3, "unlinked-transfer:txn-1+txn-2"},
		{"four days apart, the first day out", -4, ""},
		{"three days apart, the lower id dated later", 3, "unlinked-transfer:txn-1+txn-2"},
		{"four days apart, the lower id dated later", 4, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			start := day(2026, 8, 10)

			got := unlinkedIDs(t, nil,
				dupTxn(1, "acct-1", start.AddDate(0, 0, c.lowerIDApart), -unlinkedAmount, uncleared),
				dupTxn(2, "acct-2", start, unlinkedAmount, uncleared))

			assert.Equal(t, c.want, got)
		})
	}
}

func Test_replace_flags_an_unlinked_transfer_whose_lower_id_transaction_is_the_deposit(t *testing.T) {
	t.Parallel()

	got := unlinkedIDs(t, nil,
		dupTxn(1, "acct-1", day(2026, 8, 3), unlinkedAmount, uncleared),
		dupTxn(2, "acct-2", day(2026, 8, 6), -unlinkedAmount, uncleared))

	assert.Equal(t, "unlinked-transfer:txn-1+txn-2", got)
}

func Test_replace_does_not_flag_an_unlinked_transfer_between_transactions_that_are_not_opposite_amounts_in_two_accounts_of_one_currency(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		firstAccount string
		otherAccount string
		firstAmount  int64
		otherAmount  int64
	}{
		{"in the same account", "acct-1", "acct-1", -unlinkedAmount, unlinkedAmount},
		{"a cent apart", "acct-1", "acct-2", -unlinkedAmount, unlinkedAmount + 1},
		{"the same sign", "acct-1", "acct-2", -unlinkedAmount, -unlinkedAmount},
		{"zero amounts", "acct-1", "acct-2", 0, 0},
		{"a CAD and a USD account", "acct-1", "acct-3", -unlinkedAmount, unlinkedAmount},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := unlinkedIDs(t, nil,
				dupTxn(1, c.firstAccount, day(2026, 8, 3), c.firstAmount, uncleared),
				dupTxn(2, c.otherAccount, day(2026, 8, 4), c.otherAmount, uncleared))

			assert.Empty(t, got)
		})
	}
}

func Test_replace_flags_an_unlinked_transfer_in_every_kind_of_account_and_beside_unrelated_splits(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		mutate func(*store.Rows)
		first  string
		other  string
	}{
		{"the same currency in both accounts", nil, "acct-1", "acct-2"},
		{"a closed account outside reports on the lower id side", nil, "acct-2", "acct-1"},
		{"two USD accounts", func(r *store.Rows) {
			r.Accounts = append(r.Accounts, store.Account{ID: "acct-4", SourceID: 4, Name: "US Cheque", Type: "chequing", Currency: "USD"})
		}, "acct-3", "acct-4"},
		{"categorized splits that are not transfer legs", func(r *store.Rows) {
			r.Splits = []store.Split{
				{ID: "split-1", SourceID: 1, TransactionID: "txn-1", CategoryID: new("cat-1"), Amount: -unlinkedAmount},
				{ID: "split-2", SourceID: 2, TransactionID: "txn-2", CategoryID: new("cat-1"), Amount: unlinkedAmount},
			}
		}, "acct-1", "acct-2"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := unlinkedIDs(t, c.mutate,
				dupTxn(1, c.first, day(2026, 8, 3), -unlinkedAmount, uncleared),
				dupTxn(2, c.other, day(2026, 8, 4), unlinkedAmount, uncleared))

			assert.Equal(t, "unlinked-transfer:txn-1+txn-2", got)
		})
	}
}

func Test_replace_does_not_flag_an_unlinked_transfer_when_either_transaction_has_a_transfer_leg(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		mutate func(*store.Rows)
	}{
		{"the lower id transaction's split is a transfers leg only", func(r *store.Rows) {
			r.Splits = []store.Split{{ID: "split-1", SourceID: 1, TransactionID: "txn-1", Amount: -unlinkedAmount}}
			r.Transfers = []store.Transfer{{ID: "xfer-1", FromSplitID: "split-1", ToSplitID: new("split-99")}}
		}},
		{"the higher id transaction's split is a transfers to-leg only", func(r *store.Rows) {
			r.Splits = []store.Split{{ID: "split-2", SourceID: 2, TransactionID: "txn-2", Amount: unlinkedAmount}}
			r.Transfers = []store.Transfer{{ID: "xfer-1", FromSplitID: "split-99", ToSplitID: new("split-2")}}
		}},
		{"the lower id transaction's split names a transfer account only", func(r *store.Rows) {
			r.Splits = []store.Split{{ID: "split-1", SourceID: 1, TransactionID: "txn-1", Amount: -unlinkedAmount, TransferAccountID: new("acct-9")}}
		}},
		{"the higher id transaction's split names a transfer account only", func(r *store.Rows) {
			r.Splits = []store.Split{{ID: "split-2", SourceID: 2, TransactionID: "txn-2", Amount: unlinkedAmount, TransferAccountID: new("acct-9")}}
		}},
		{"a second split of the transaction is a leg", func(r *store.Rows) {
			r.Splits = []store.Split{
				{ID: "split-1", SourceID: 1, TransactionID: "txn-1", CategoryID: new("cat-1"), Amount: -30000},
				{ID: "split-4", SourceID: 4, TransactionID: "txn-1", Amount: -20000, TransferAccountID: new("acct-9")},
			}
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := unlinkedIDs(t, c.mutate,
				dupTxn(1, "acct-1", day(2026, 8, 3), -unlinkedAmount, uncleared),
				dupTxn(2, "acct-2", day(2026, 8, 4), unlinkedAmount, uncleared))

			assert.Empty(t, got)
		})
	}
}

func Test_replace_does_not_flag_an_unlinked_transfer_when_either_transaction_is_an_investment_cash_row(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		first, other func(store.Transaction) store.Transaction
	}{
		{"both investment cash rows", investmentCash, investmentCash},
		{"the lower id a register row, the higher id an investment cash row", identity, investmentCash},
		{"the lower id an investment cash row, the higher id a register row", investmentCash, identity},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := unlinkedIDs(t, nil,
				c.first(dupTxn(1, "acct-1", day(2026, 8, 3), -unlinkedAmount, uncleared)),
				c.other(dupTxn(2, "acct-2", day(2026, 8, 3), unlinkedAmount, uncleared)))

			assert.Empty(t, got)
		})
	}
}

func Test_replace_records_one_unlinked_transfer_per_pair_of_three_matching_transactions(t *testing.T) {
	t.Parallel()

	got := unlinkedIDs(t, func(r *store.Rows) {
		r.Accounts = append(r.Accounts, store.Account{ID: "acct-4", SourceID: 4, Name: "Second Visa", Type: "credit", Currency: "CAD"})
	},
		dupTxn(9, "acct-1", day(2026, 8, 3), -unlinkedAmount, uncleared),
		dupTxn(10, "acct-2", day(2026, 8, 4), unlinkedAmount, uncleared),
		dupTxn(11, "acct-4", day(2026, 8, 5), unlinkedAmount, uncleared))

	assert.Equal(t, "unlinked-transfer:txn-9+txn-10,unlinked-transfer:txn-9+txn-11", got)
}

func Test_replace_records_the_two_transactions_of_an_unlinked_transfer_as_its_items_lower_source_id_first(t *testing.T) {
	t.Parallel()
	rows := unlinkedRows(nil,
		dupTxn(10, "acct-2", day(2026, 8, 4), unlinkedAmount, uncleared),
		dupTxn(9, "acct-1", day(2026, 8, 3), -unlinkedAmount, uncleared))
	replaced, err := duckstore.New(t.TempDir()).Replace(t.Context(), rows)
	require.NoError(t, err)
	db := openReadOnly(t, replaced.Path)

	assertScalar(t, db, `SELECT string_agg(COALESCE(transaction_id, 'NULL') || '|' || COALESCE(split_id, 'NULL') || '|' ||
		COALESCE(payee_id, 'NULL') || '|' || COALESCE(category_id, 'NULL'), '; ' ORDER BY rowid)
		FROM finding_items WHERE finding_id = 'unlinked-transfer:txn-9+txn-10'`,
		"txn-9|NULL|NULL|NULL; txn-10|NULL|NULL|NULL")
}
