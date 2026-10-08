package document_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewSchema_writes_its_keys_in_the_ruled_order(t *testing.T) {
	schema := report.Schema{
		Relations:    []store.Relation{{Name: "accounts", Kind: store.RelationTable, Columns: []store.Column{{Name: "id", Type: "VARCHAR"}}}},
		Accounts:     []store.Account{{ID: "acct-1", Name: "Chequing", Type: "chequing", Currency: "CAD", Closed: true}},
		Categories:   []store.Category{{ID: "cat-1", FullPath: "Food", Kind: "expense", Hidden: true}},
		Transactions: store.TransactionRange{First: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), Last: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)},
	}

	out, err := json.Marshal(document.NewSchema(schema, []string{"cut"}))

	require.NoError(t, err)
	want := `{"conventions":` + quoted(t, report.SQLConventions) +
		`,"relations":[{"name":"accounts","kind":"table","columns":[{"name":"id","type":"VARCHAR"}]}]` +
		`,"accounts":[{"id":"acct-1","name":"Chequing","type":"chequing","currency":"CAD","closed":true}]` +
		`,"categories":[{"id":"cat-1","full_path":"Food","kind":"expense","hidden":true}]` +
		`,"dates":{"first":"2024-01-02","last":"2026-09-30"},"warnings":["cut"]}`
	assert.Equal(t, want, string(out))
}

func Test_NewSchema_of_an_empty_store_has_empty_lists_and_null_dates(t *testing.T) {
	out, err := json.Marshal(document.NewSchema(report.Schema{}, nil))

	require.NoError(t, err)
	want := `{"conventions":` + quoted(t, report.SQLConventions) +
		`,"relations":[],"accounts":[],"categories":[],"dates":{"first":null,"last":null},"warnings":[]}`
	assert.Equal(t, want, string(out))
}

// quoted is s as the JSON string encoding/json writes for it.
func quoted(tb testing.TB, s string) string {
	tb.Helper()
	out, err := json.Marshal(s)
	require.NoError(tb, err)
	return string(out)
}
