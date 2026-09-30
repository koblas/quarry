package duckstore_test

import (
	"os"
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
		{"the one-sided query", &faultDB{queryFaultOn: duckstore.OneSidedTransferQuery, queryFault: queryFault}, "detect one-sided-transfer findings"},
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
