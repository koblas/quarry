package duckstore_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// builtSchemaStore is a store of rows built in a fresh directory.
func builtSchemaStore(t *testing.T, rows store.Rows) *duckstore.Store {
	t.Helper()
	st := duckstore.New(t.TempDir())
	_, err := st.Replace(t.Context(), rows)
	require.NoError(t, err)
	return st
}

func Test_schema_lists_every_relation_with_the_columns_the_catalog_declares(t *testing.T) {
	t.Parallel()
	st := builtSchemaStore(t, minimalRows())
	db := openReadOnly(t, st.Path())
	var want []store.Relation
	err := db.QueryRows(t.Context(), `SELECT table_name, table_type, column_name, data_type
		FROM information_schema.tables JOIN information_schema.columns USING (table_catalog, table_schema, table_name)
		WHERE table_catalog = current_database() AND table_schema = 'main'
		ORDER BY table_type, table_name, ordinal_position`, nil, func(scan func(dest ...any) error) error {
		var name, tableType string
		var column store.Column
		if err := scan(&name, &tableType, &column.Name, &column.Type); err != nil {
			return err
		}
		kind := store.RelationTable
		if tableType == "VIEW" {
			kind = store.RelationView
		}
		if len(want) == 0 || want[len(want)-1].Name != name {
			want = append(want, store.Relation{Name: name, Kind: kind})
		}
		want[len(want)-1].Columns = append(want[len(want)-1].Columns, column)
		return nil
	})
	require.NoError(t, err)

	got, err := st.Schema(t.Context())

	require.NoError(t, err)
	assert.Equal(t, want, got.Relations)
}

func Test_schema_lists_a_relations_columns_in_the_order_the_ddl_declares_them(t *testing.T) {
	t.Parallel()
	st := builtSchemaStore(t, minimalRows())

	got, err := st.Schema(t.Context())

	require.NoError(t, err)
	var names []string
	for _, r := range got.Relations {
		if r.Name == "categories" {
			for _, c := range r.Columns {
				names = append(names, c.Name)
			}
		}
	}
	assert.Equal(t, []string{"id", "source_id", "parent_id", "name", "full_path", "kind", "hidden"}, names)
}

func Test_schema_reads_the_accounts_and_categories_it_lists(t *testing.T) {
	t.Parallel()
	rows := minimalRows()
	rows.Accounts = []store.Account{
		{ID: "acct-open", SourceID: 1, Name: "Chequing", Type: "chequing", Currency: "CAD", Active: true},
		{ID: "acct-shut", SourceID: 2, Name: "Old", Type: "savings", Currency: "USD", Closed: true, Active: true},
	}
	rows.Transactions, rows.Splits, rows.SplitTags, rows.Transfers = nil, nil, nil, nil
	rows.Categories = []store.Category{
		{ID: "cat-shown", SourceID: 1, Name: "Groceries", FullPath: "Food:Groceries", Kind: "expense"},
		{ID: "cat-hidden", SourceID: 2, Name: "Old", FullPath: "Misc:Old", Kind: "income", Hidden: true},
	}
	st := builtSchemaStore(t, rows)

	got, err := st.Schema(t.Context())

	require.NoError(t, err)
	assert.ElementsMatch(t, []store.Account{
		{ID: "acct-open", Name: "Chequing", Type: "chequing", Currency: "CAD"},
		{ID: "acct-shut", Name: "Old", Type: "savings", Currency: "USD", Closed: true},
	}, got.Accounts)
	assert.ElementsMatch(t, []store.Category{
		{ID: "cat-shown", Name: "Groceries", FullPath: "Food:Groceries", Kind: "expense"},
		{ID: "cat-hidden", Name: "Old", FullPath: "Misc:Old", Kind: "income", Hidden: true},
	}, got.Categories)
}

func Test_schema_reads_the_first_and_last_transaction_dates(t *testing.T) {
	t.Parallel()
	rows := minimalRows()
	rows.Transactions = append(rows.Transactions, store.Transaction{
		ID: "txn-2", SourceID: 2, AccountID: "acct-1", Date: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		Amount: 500, Currency: "CAD", Status: "uncleared",
	})
	st := builtSchemaStore(t, rows)

	got, err := st.Schema(t.Context())

	require.NoError(t, err)
	assert.Equal(t, store.TransactionRange{First: day(2024, 1, 2), Last: day(2026, 3, 15)}, got.Transactions)
}

func Test_schema_of_a_store_with_nothing_in_it_has_no_accounts_categories_or_dates(t *testing.T) {
	t.Parallel()
	rows := minimalRows()
	rows.Accounts, rows.Categories, rows.Payees, rows.Tags = nil, nil, nil, nil
	rows.Transactions, rows.Splits, rows.SplitTags, rows.Transfers = nil, nil, nil, nil
	st := builtSchemaStore(t, rows)

	got, err := st.Schema(t.Context())

	require.NoError(t, err)
	assert.Empty(t, got.Accounts)
	assert.Empty(t, got.Categories)
	assert.Zero(t, got.Transactions)
	assert.NotEmpty(t, got.Relations)
}

// schemaQueries is how many queries Schema runs after the open's format checks.
const schemaQueries = 4

func Test_schema_returns_the_fault_of_each_query_as_another_fault(t *testing.T) {
	t.Parallel()
	for passed := range schemaQueries {
		t.Run(fmt.Sprintf("after %d queries", passed), func(t *testing.T) {
			t.Parallel()
			fault := ioFault(`query rows "SELECT"`)
			st := newBuiltStore(t, spyOpener(&spyReadDB{queryFault: fault, passQueries: passed}))

			_, err := st.Schema(t.Context())

			assertOtherFault(t, err, "disk read failed")
			assert.ErrorIs(t, err, fault)
		})
	}
}

func Test_schema_returns_the_scan_fault_of_each_query_as_another_fault(t *testing.T) {
	t.Parallel()
	for passed := range schemaQueries {
		t.Run(fmt.Sprintf("after %d queries", passed), func(t *testing.T) {
			t.Parallel()
			st := newBuiltStore(t, spyOpener(&spyReadDB{scanFault: errScanFailed, passQueries: passed}))

			_, err := st.Schema(t.Context())

			assertOtherFault(t, err, errScanFailed.Error())
			assert.ErrorIs(t, err, errScanFailed)
		})
	}
}
