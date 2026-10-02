package document_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const configShown = "~/config.toml"

func Test_UnmatchedIgnoreWarnings_names_the_config_and_quotes_a_plain_id(t *testing.T) {
	got := document.UnmatchedIgnoreWarnings(configShown, []string{"duplicate:a+b"})

	assert.Equal(t, []string{`~/config.toml: findings.ignore lists "duplicate:a+b", which is not a finding in quarry's store; quarry skips it`}, got)
}

func Test_UnmatchedIgnoreWarnings_escapes_an_id_holding_a_quote_and_a_backslash(t *testing.T) {
	got := document.UnmatchedIgnoreWarnings(configShown, []string{`a"b\c`})

	assert.Equal(t, []string{`~/config.toml: findings.ignore lists "a\"b\\c", which is not a finding in quarry's store; quarry skips it`}, got)
}

func Test_UnmatchedIgnoreWarnings_is_an_empty_list_not_nil_when_nothing_is_unmatched(t *testing.T) {
	got := document.UnmatchedIgnoreWarnings(configShown, nil)

	assert.NotNil(t, got)
	assert.Empty(t, got)
}

func Test_NewFindingsList_encodes_empty_findings_and_warnings_as_arrays_and_no_type_as_null(t *testing.T) {
	doc := document.NewFindingsList(report.FindingsListing{}, finding.StatusOpen, "", nil)

	data, err := json.Marshal(doc)

	require.NoError(t, err)
	assert.JSONEq(t, `{"status":"open","type":null,"counts":{"open":0,"ignored":0,"fixed":0,"new":0,"newly_fixed":0},"findings":[],"warnings":[]}`, string(data))
}

func Test_NewFindingEntry_sets_fixed_at_for_a_fixed_finding_and_splits_for_a_similar_categories_item(t *testing.T) {
	fixedAt := time.Date(2026, 10, 2, 3, 0, 0, 0, time.UTC)
	fixed := report.ListedFinding{Status: finding.StatusFixed, Finding: store.Finding{
		ID: "uncategorized:x", Type: finding.Uncategorized,
		FirstFoundAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), FixedAt: &fixedAt,
	}}
	similar := report.ListedFinding{Status: finding.StatusOpen, Finding: store.Finding{
		ID: "similar-categories:y", Type: finding.SimilarCategories,
		Items: []store.FindingItem{{Splits: 3}},
	}}

	fixedEntry := document.NewFindingEntry(fixed)
	similarEntry := document.NewFindingEntry(similar)

	require.NotNil(t, fixedEntry.FixedAt)
	assert.Equal(t, "2026-10-02T03:00:00Z", *fixedEntry.FixedAt)
	assert.Empty(t, fixedEntry.Items)
	assert.Nil(t, similarEntry.FixedAt)
	require.Len(t, similarEntry.Items, 1)
	assert.Equal(t, 3, *similarEntry.Items[0].Splits)
}

func Test_NewFindingItem_leaves_transaction_fields_null_for_an_item_that_is_a_payee(t *testing.T) {
	payeeID := "p1"

	got := document.NewFindingItem(store.FindingItem{PayeeID: &payeeID, Payee: "ACME", Transactions: 2})

	assert.Equal(t, "ACME", *got.Payee)
	assert.Equal(t, 2, *got.Transactions)
	assert.Nil(t, got.Date)
	assert.Nil(t, got.Amount)
}

func Test_NewFindingItem_formats_the_date_and_amount_of_a_transaction_item(t *testing.T) {
	txn := "t1"

	got := document.NewFindingItem(store.FindingItem{
		TransactionID: &txn, Date: time.Date(2012, 1, 5, 0, 0, 0, 0, time.UTC),
		Account: "Chequing", AccountID: "a1", Currency: "CAD", Amount: -12345,
	})

	assert.Equal(t, "2012-01-05", *got.Date)
	assert.Equal(t, "-123.45", *got.Amount)
	assert.Nil(t, got.Transactions)
}
