package duckstore_test

import (
	"os"
	"slices"
	"testing"

	duckdbdriver "github.com/duckdb/duckdb-go/v2"
	"github.com/koblas/quarry/internal/finding"
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

// withFindingCandidates adds, to minimalRows, uncategorized splits of one payee and of no payee,
// and a from-split for the one-sided transfer xfer-3 (split-3, a transfer leg on txn-5).
func withFindingCandidates() store.Rows {
	rows := minimalRows()
	txn := func(id string, sourceID int64, payee *string, amount int64) store.Transaction {
		return store.Transaction{
			ID: id, SourceID: sourceID, AccountID: "acct-1", Date: day(2026, 3, 16), PayeeID: payee,
			Amount: amount, Currency: "CAD", Status: "uncleared",
		}
	}
	rows.Transactions = append(rows.Transactions,
		txn("txn-2", 2, nil, -500), txn("txn-3", 3, new("payee-1"), -700), txn("txn-4", 4, new("payee-1"), -900), txn("txn-5", 5, nil, -300))
	rows.Splits = append(rows.Splits,
		store.Split{ID: "split-5", SourceID: 5, TransactionID: "txn-2", Amount: -500},
		store.Split{ID: "split-6", SourceID: 6, TransactionID: "txn-3", Amount: -700},
		store.Split{ID: "split-7", SourceID: 7, TransactionID: "txn-4", Amount: -900},
		store.Split{ID: "split-3", SourceID: 3, TransactionID: "txn-5", Amount: -300, TransferAccountID: new("acct-9")})
	return rows
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
