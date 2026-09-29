package importer_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/importer"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/store"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setColumnBlob overwrites column on the row identified by pk with value as
// a real SQLite BLOB, for fixture shapes v9fixture.Builder has no field for.
func setColumnBlob(t *testing.T, dataPath, table, column string, pk int64, value []byte) {
	t.Helper()
	db, err := sql.Open("sqlite3", dataPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	_, err = db.Exec("UPDATE "+table+" SET "+column+" = ? WHERE Z_PK = ?", value, pk)
	require.NoError(t, err)
}

// setColumnEmptyText overwrites column with a zero-length SQLite TEXT
// value — distinct from setColumnBlob's zero-length BLOB.
func setColumnEmptyText(t *testing.T, dataPath, table, column string, pk int64) {
	t.Helper()
	db, err := sql.Open("sqlite3", dataPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	_, err = db.Exec("UPDATE "+table+" SET "+column+" = '' WHERE Z_PK = ?", pk)
	require.NoError(t, err)
}

func Test_import_refuses_a_transaction_with_a_text_amount(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Visa Infinite", Type: "CREDITCARD", Currency: "CAD", Active: true})
	posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "not-a-number", PostedDate: &posted})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	reason := importReason(t, err)
	assert.Equal(t, `a transaction on 2024-03-02 in "Visa Infinite" has an amount that is not a number`, reason)
	assert.NotContains(t, reason, "too large")
}

func Test_import_refuses_a_transaction_with_a_whitespace_amount(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Visa Infinite", Type: "CREDITCARD", Currency: "CAD", Active: true})
	posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "   ", PostedDate: &posted})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t, `a transaction on 2024-03-02 in "Visa Infinite" has an amount that is not a number`, importReason(t, err))
}

func Test_import_refuses_a_transaction_with_an_empty_text_amount(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Visa Infinite", Type: "CREDITCARD", Currency: "CAD", Active: true})
	posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "0.00", PostedDate: &posted})
	bundle := b.WriteBundle(t, t.TempDir())
	setColumnEmptyText(t, bundle.DataPath, "ZTRANSACTION", "ZAMOUNT", txnPK)

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t, `a transaction on 2024-03-02 in "Visa Infinite" has an amount that is not a number`, importReason(t, err))
}

func Test_import_refuses_a_transaction_with_a_blob_amount(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Visa Infinite", Type: "CREDITCARD", Currency: "CAD", Active: true})
	posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "0.00", PostedDate: &posted})
	bundle := b.WriteBundle(t, t.TempDir())
	setColumnBlob(t, bundle.DataPath, "ZTRANSACTION", "ZAMOUNT", txnPK, []byte{0xde, 0xad, 0xbe, 0xef})

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t, `a transaction on 2024-03-02 in "Visa Infinite" has an amount that is not a number`, importReason(t, err))
}

// A text/blob amount with no date falls back to the source-id subject, the
// same shape reason 10's "no date" form uses.
func Test_import_refuses_a_dateless_transaction_with_a_text_amount(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Visa Infinite", Type: "CREDITCARD", Currency: "CAD", Active: true})
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "not-a-number"})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t, `a transaction in "Visa Infinite" (source id `+itoa(txnPK)+`) has an amount that is not a number`, importReason(t, err))
}

func Test_import_refuses_a_split_with_a_text_amount(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Visa Infinite", Type: "CREDITCARD", Currency: "CAD", Active: true})
	posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "12.34", PostedDate: &posted})
	b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "not-a-number"})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t, `a split of a transaction on 2024-03-02 in "Visa Infinite" has an amount that is not a number`, importReason(t, err))
}

func Test_import_refuses_a_split_with_a_blob_amount(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Visa Infinite", Type: "CREDITCARD", Currency: "CAD", Active: true})
	posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: acctPK, Amount: "12.34", PostedDate: &posted})
	entryPK := b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "0.00"})
	bundle := b.WriteBundle(t, t.TempDir())
	setColumnBlob(t, bundle.DataPath, "ZCASHFLOWTRANSACTIONENTRY", "ZAMOUNT", entryPK, []byte{0x01, 0x02})

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t, `a split of a transaction on 2024-03-02 in "Visa Infinite" has an amount that is not a number`, importReason(t, err))
}

// A genuinely NULL amount stays reason 10, never reason 11.
func Test_import_null_amount_is_reason_10_not_reason_11(t *testing.T) {
	t.Parallel()
	b := v9fixture.NewBuilder()
	acctPK := b.Account(v9fixture.AccountRow{Name: "Visa Infinite", Type: "CREDITCARD", Currency: "CAD", Active: true})
	posted := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	b.Transaction(v9fixture.TransactionRow{Account: acctPK, PostedDate: &posted})
	bundle := b.WriteBundle(t, t.TempDir())

	_, err := importer.NewServer(importer.WithStore(&fakeStore{})).Import(t.Context(), store.SnapshotRef{Path: bundle.DataPath})

	assert.Equal(t, `a transaction on 2024-03-02 in "Visa Infinite" has no amount`, importReason(t, err))
}
