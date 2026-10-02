package report_test

import (
	"context"
	"fmt"
	"io/fs"
	"testing"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const listCap = 500

func describe(t *testing.T, schema store.Schema, maxListed int) report.Schema {
	t.Helper()
	srv := report.NewServer(report.WithStore(fakeStore{schema: schema}))
	got, err := srv.DescribeSchema(t.Context(), maxListed)
	require.NoError(t, err)
	return got
}

func relationNames(s report.Schema) []string {
	names := make([]string, 0, len(s.Relations))
	for _, r := range s.Relations {
		names = append(names, r.Name)
	}
	return names
}

func accountKeys(s report.Schema) []string {
	keys := make([]string, 0, len(s.Accounts))
	for _, a := range s.Accounts {
		keys = append(keys, a.Name+"/"+a.ID)
	}
	return keys
}

func categoryPaths(s report.Schema) []string {
	paths := make([]string, 0, len(s.Categories))
	for _, c := range s.Categories {
		paths = append(paths, c.FullPath)
	}
	return paths
}

// manyAccounts is n accounts "Account 000" up, listed in descending order.
func manyAccounts(n int) []store.Account {
	accounts := make([]store.Account, 0, n)
	for i := n - 1; i >= 0; i-- {
		accounts = append(accounts, store.Account{ID: fmt.Sprintf("acct-%03d", i), Name: fmt.Sprintf("Account %03d", i)})
	}
	return accounts
}

// manyCategories is n categories "Category 000" up, listed in descending order.
func manyCategories(n int) []store.Category {
	categories := make([]store.Category, 0, n)
	for i := n - 1; i >= 0; i-- {
		categories = append(categories, store.Category{ID: fmt.Sprintf("cat-%03d", i), FullPath: fmt.Sprintf("Category %03d", i)})
	}
	return categories
}

func Test_describe_schema_lists_tables_before_views_then_by_name(t *testing.T) {
	schema := store.Schema{Relations: []store.Relation{
		{Name: "a_view", Kind: store.RelationView},
		{Name: "z_table", Kind: store.RelationTable},
		{Name: "b_table", Kind: store.RelationTable},
		{Name: "c_view", Kind: store.RelationView},
	}}

	got := describe(t, schema, listCap)

	assert.Equal(t, []string{"b_table", "z_table", "a_view", "c_view"}, relationNames(got))
}

func Test_describe_schema_orders_accounts_by_name_ignoring_case_first(t *testing.T) {
	schema := store.Schema{Accounts: []store.Account{{ID: "1", Name: "Zeta"}, {ID: "2", Name: "alpha"}}}

	got := describe(t, schema, listCap)

	assert.Equal(t, []string{"alpha/2", "Zeta/1"}, accountKeys(got))
}

func Test_describe_schema_orders_accounts_whose_names_differ_only_in_case_by_the_name_bytes(t *testing.T) {
	schema := store.Schema{Accounts: []store.Account{{ID: "1", Name: "chequing"}, {ID: "2", Name: "Chequing"}}}

	got := describe(t, schema, listCap)

	assert.Equal(t, []string{"Chequing/2", "chequing/1"}, accountKeys(got))
}

func Test_describe_schema_orders_accounts_with_one_name_by_id(t *testing.T) {
	schema := store.Schema{Accounts: []store.Account{{ID: "b", Name: "Chequing"}, {ID: "a", Name: "Chequing"}}}

	got := describe(t, schema, listCap)

	assert.Equal(t, []string{"Chequing/a", "Chequing/b"}, accountKeys(got))
}

func Test_describe_schema_orders_categories_by_full_path_with_a_grandchild_after_its_parent(t *testing.T) {
	schema := store.Schema{Categories: []store.Category{
		{FullPath: "Food:Groceries:Organic"}, {FullPath: "Misc"}, {FullPath: "Food:Groceries"}, {FullPath: "Auto"},
	}}

	got := describe(t, schema, listCap)

	assert.Equal(t, []string{"Auto", "Food:Groceries", "Food:Groceries:Organic", "Misc"}, categoryPaths(got))
}

func Test_describe_schema_keeps_500_and_cuts_501(t *testing.T) {
	cases := []struct {
		name   string
		schema store.Schema
	}{
		{name: "500 accounts and categories are all kept", schema: store.Schema{Accounts: manyAccounts(listCap), Categories: manyCategories(listCap)}},
		{name: "501 accounts and categories lose the last in order", schema: store.Schema{Accounts: manyAccounts(listCap + 1), Categories: manyCategories(listCap + 1)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := describe(t, c.schema, listCap)

			require.Len(t, got.Accounts, listCap)
			require.Len(t, got.Categories, listCap)
			assert.Equal(t, "Account 000/acct-000", accountKeys(got)[0])
			assert.Equal(t, "Account 499/acct-499", accountKeys(got)[listCap-1])
			assert.Equal(t, "Category 000", categoryPaths(got)[0])
			assert.Equal(t, "Category 499", categoryPaths(got)[listCap-1])
		})
	}
}

func Test_describe_schema_counts_accounts_and_categories_before_the_cut(t *testing.T) {
	schema := store.Schema{Accounts: manyAccounts(listCap + 2), Categories: manyCategories(listCap + 3)}

	got := describe(t, schema, listCap)

	assert.Equal(t, listCap+2, got.AccountsTotal)
	assert.Equal(t, listCap+3, got.CategoriesTotal)
}

func Test_describe_schema_keeps_every_account_and_category_when_the_cap_is_zero(t *testing.T) {
	schema := store.Schema{Accounts: manyAccounts(listCap + 1), Categories: manyCategories(listCap + 1)}

	got := describe(t, schema, 0)

	assert.Len(t, got.Accounts, listCap+1)
	assert.Len(t, got.Categories, listCap+1)
}

func Test_describe_schema_passes_the_relations_and_dates_through(t *testing.T) {
	schema := store.Schema{
		Relations:    []store.Relation{{Name: "accounts", Kind: store.RelationTable, Columns: []store.Column{{Name: "id", Type: "VARCHAR"}}}},
		Transactions: store.TransactionRange{First: day(2024, 1, 2), Last: day(2026, 9, 30)},
	}

	got := describe(t, schema, listCap)

	assert.Equal(t, schema.Relations, got.Relations)
	assert.Equal(t, schema.Transactions, got.Transactions)
}

func Test_describe_schema_reads_the_store_once(t *testing.T) {
	var reads int
	srv := report.NewServer(report.WithStore(fakeStore{schemaReads: &reads}))

	_, err := srv.DescribeSchema(t.Context(), listCap)

	require.NoError(t, err)
	assert.Equal(t, 1, reads)
}

func Test_describe_schema_refuses_a_missing_store_with_the_store_refusal(t *testing.T) {
	openErr := &store.OpenError{Fault: store.OpenFaultMissing, Path: storePath, Err: fs.ErrNotExist}
	srv := report.NewServer(report.WithStore(fakeStore{err: openErr}), report.WithHome(refusalHome))

	_, err := srv.DescribeSchema(t.Context(), listCap)

	assert.EqualError(t, err, "no store at ~/Library/Application Support/quarry/quarry.duckdb yet; run quarry sync to build it")
}

func Test_describe_schema_reports_an_interrupt_before_any_store_refusal(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	openErr := &store.OpenError{Fault: store.OpenFaultMissing, Path: storePath, Err: fs.ErrNotExist}
	srv := report.NewServer(report.WithStore(fakeStore{err: openErr}), report.WithHome(refusalHome))

	_, err := srv.DescribeSchema(ctx, listCap)

	assert.EqualError(t, err, "describe_schema interrupted")
}

func Test_describe_schema_returns_another_store_error_unchanged(t *testing.T) {
	srv := report.NewServer(report.WithStore(fakeStore{err: errDiskRead}))

	_, err := srv.DescribeSchema(t.Context(), listCap)

	assert.Equal(t, errDiskRead, err)
}
