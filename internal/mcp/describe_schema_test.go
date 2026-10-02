package mcp_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// schemaWith is a store schema of the given counts of accounts and categories, listed in descending order.
func schemaWith(accounts, categories int) store.Schema {
	var schema store.Schema
	for i := accounts - 1; i >= 0; i-- {
		schema.Accounts = append(schema.Accounts, store.Account{ID: fmt.Sprintf("acct-%04d", i), Name: fmt.Sprintf("Account %04d", i)})
	}
	for i := categories - 1; i >= 0; i-- {
		schema.Categories = append(schema.Categories, store.Category{ID: fmt.Sprintf("cat-%04d", i), FullPath: fmt.Sprintf("Category %04d", i)})
	}
	return schema
}

func Test_describe_schema_reads_the_store_once_per_call_through_a_fresh_report_server(t *testing.T) {
	h := newHarness(t, &fakeStore{}, nil)

	h.describeSchema(t)
	h.describeSchema(t)

	assert.Equal(t, 2, h.store.schemaReads)
	assert.Equal(t, []string{"mcp", "mcp"}, h.commands)
}

func Test_describe_schema_returns_what_the_store_holds_with_the_conventions(t *testing.T) {
	h := newHarness(t, &fakeStore{schema: store.Schema{
		Relations: []store.Relation{{Name: "accounts", Kind: store.RelationTable, Columns: []store.Column{{Name: "id", Type: "VARCHAR"}}}},
		Accounts:  []store.Account{{ID: "acct-1", Name: "Chequing", Type: "chequing", Currency: "CAD", Closed: true}},
		Categories: []store.Category{
			{ID: "cat-1", FullPath: "Food:Groceries", Kind: "expense", Hidden: true},
		},
		Transactions: store.TransactionRange{First: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), Last: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)},
	}}, nil)

	result := h.describeSchema(t)

	require.False(t, result.IsError, textOf(t, result))
	doc := decodeSchema(t, result)
	assert.Equal(t, report.SQLConventions, doc.Conventions)
	assert.Equal(t, []document.SchemaRelation{{Name: "accounts", Kind: "table", Columns: []document.SchemaColumn{{Name: "id", Type: "VARCHAR"}}}}, doc.Relations)
	assert.Equal(t, []document.SchemaAccount{{ID: "acct-1", Name: "Chequing", Type: "chequing", Currency: "CAD", Closed: true}}, doc.Accounts)
	assert.Equal(t, []document.SchemaCategory{{ID: "cat-1", FullPath: "Food:Groceries", Kind: "expense", Hidden: true}}, doc.Categories)
	assert.Equal(t, document.SchemaDates{First: new("2024-01-02"), Last: new("2026-09-30")}, doc.Dates)
}

// decodeSchema is result's one text block decoded as the describe_schema document.
func decodeSchema(t *testing.T, result *sdk.CallToolResult) document.Schema {
	t.Helper()
	var doc document.Schema
	require.NoError(t, json.Unmarshal([]byte(textOf(t, result)), &doc))
	return doc
}

func Test_describe_schema_keeps_500_accounts_and_categories_without_a_warning(t *testing.T) {
	h := newHarness(t, &fakeStore{schema: schemaWith(500, 500)}, nil)

	result := h.describeSchema(t)

	doc := decodeSchema(t, result)
	assert.Len(t, doc.Accounts, 500)
	assert.Len(t, doc.Categories, 500)
	assert.Equal(t, []string{}, doc.Warnings)
}

func Test_describe_schema_says_which_list_it_cut_with_the_total_grouped_in_thousands(t *testing.T) {
	cases := []struct {
		name   string
		schema store.Schema
		want   []string
	}{
		{
			name: "categories only", schema: schemaWith(1, 1001),
			want: []string{"describe_schema lists the first 500 categories of 1,001; query the categories table for the rest"},
		},
		{
			name: "accounts only", schema: schemaWith(501, 1),
			want: []string{"describe_schema lists the first 500 accounts of 501; query the accounts table for the rest"},
		},
		{
			name: "both, accounts first", schema: schemaWith(501, 502),
			want: []string{
				"describe_schema lists the first 500 accounts of 501; query the accounts table for the rest",
				"describe_schema lists the first 500 categories of 502; query the categories table for the rest",
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, &fakeStore{schema: c.schema}, nil)

			result := h.describeSchema(t)

			assert.Equal(t, c.want, decodeSchema(t, result).Warnings)
		})
	}
}

func Test_describe_schema_refuses_with_the_report_factory_text_when_it_fails(t *testing.T) {
	h := newHarness(t, &fakeStore{}, errNoHome)

	result := h.describeSchema(t)

	assert.True(t, result.IsError)
	assert.Equal(t, errNoHome.Error(), textOf(t, result))
	assert.Zero(t, h.store.schemaReads)
}

func Test_describe_schema_refuses_a_missing_store_with_one_text_line_and_one_stderr_line(t *testing.T) {
	const line = "no store at ~/Library/Application Support/quarry/quarry.duckdb yet; run quarry sync to build it"
	h := newHarness(t, &fakeStore{err: &store.OpenError{Fault: store.OpenFaultMissing, Path: testStorePath}}, nil)

	result := h.describeSchema(t)

	assert.True(t, result.IsError)
	assert.Equal(t, line, textOf(t, result))
	assert.Equal(t, "quarry: mcp: describe_schema: "+line+"\n", h.stderr.String())
}
