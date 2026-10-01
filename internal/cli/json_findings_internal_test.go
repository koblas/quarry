// White-box: renderFindingsJSON's field mapping is unexported; distinct counts and a null
// payee are driven directly over a report.FindingsListing.
package cli

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_renderFindingsJSON_encodes_each_of_the_five_counts_under_its_own_key(t *testing.T) {
	listing := report.FindingsListing{Counts: finding.Counts{Open: 5, Ignored: 4, Fixed: 12, New: 2, NewlyFixed: 1}}

	data, err := renderFindingsJSON(listing, openView, nil)

	require.NoError(t, err)
	var doc struct {
		Counts json.RawMessage `json:"counts"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	assert.JSONEq(t, `{"open": 5, "ignored": 4, "fixed": 12, "new": 2, "newly_fixed": 1}`, string(doc.Counts))
}

func Test_renderFindingsJSON_encodes_a_finding_item_without_a_payee_as_null(t *testing.T) {
	listing := report.FindingsListing{Groups: []report.FindingsGroup{{
		Type: finding.Uncategorized,
		Findings: []report.ListedFinding{openFinding(store.Finding{
			ID: "uncategorized:no-payee", Type: finding.Uncategorized,
			Items: []store.FindingItem{{Payee: "", Date: findingDay(2012, 1, 1)}},
		})},
	}}}

	data, err := renderFindingsJSON(listing, openView, nil)

	require.NoError(t, err)
	var doc struct {
		Findings []struct {
			Items []map[string]any `json:"items"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	require.Len(t, doc.Findings, 1)
	require.Len(t, doc.Findings[0].Items, 1)
	assert.Contains(t, doc.Findings[0].Items[0], "payee")
	assert.Nil(t, doc.Findings[0].Items[0]["payee"])
}

func Test_renderFindingsJSON_names_the_view_status_and_type(t *testing.T) {
	cases := []struct {
		name       string
		view       findingsView
		wantStatus string
		wantType   any
	}{
		{name: "the default view", view: openView, wantStatus: "open", wantType: nil},
		{name: "all statuses", view: allView, wantStatus: "all", wantType: nil},
		{name: "fixed of one type", view: findingsView{status: finding.StatusFixed, typ: finding.Duplicate}, wantStatus: "fixed", wantType: "duplicate"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data, err := renderFindingsJSON(report.FindingsListing{}, c.view, nil)

			require.NoError(t, err)
			var doc map[string]any
			require.NoError(t, json.Unmarshal(data, &doc))
			assert.Equal(t, c.wantStatus, doc["status"])
			assert.Equal(t, c.wantType, doc["type"])
		})
	}
}

func Test_renderFindingsJSON_entries_follow_each_findings_status_and_fixed_at(t *testing.T) {
	fixedAt := time.Date(2026, 10, 2, 3, 0, 0, 0, time.UTC)
	ignored := withStatus(openFinding(store.Finding{
		ID: "duplicate:txn-1+txn-2", Type: finding.Duplicate,
		Items: []store.FindingItem{{Date: findingDay(2026, 8, 3)}},
	}), finding.StatusIgnored)
	listing := report.FindingsListing{Groups: []report.FindingsGroup{{
		Type:     finding.Duplicate,
		Findings: []report.ListedFinding{ignored, fixedFinding("duplicate:txn-3+txn-4", finding.Duplicate, fixedAt)},
	}}}

	data, err := renderFindingsJSON(listing, allView, nil)

	require.NoError(t, err)
	var doc struct {
		Findings []struct {
			ID      string  `json:"id"`
			Status  string  `json:"status"`
			FixedAt *string `json:"fixed_at"`
			Items   []any   `json:"items"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	require.Len(t, doc.Findings, 2)
	assert.Equal(t, "ignored", doc.Findings[0].Status)
	assert.Nil(t, doc.Findings[0].FixedAt)
	assert.Len(t, doc.Findings[0].Items, 1)
	assert.Equal(t, "fixed", doc.Findings[1].Status)
	require.NotNil(t, doc.Findings[1].FixedAt)
	assert.Equal(t, "2026-10-02T03:00:00Z", *doc.Findings[1].FixedAt)
	assert.Equal(t, []any{}, doc.Findings[1].Items)
}

// itemsJSON renders one open finding of typ with items as --json and returns its items' decoded objects.
func itemsJSON(t *testing.T, typ finding.Type, items ...store.FindingItem) []map[string]any {
	t.Helper()
	listing := report.FindingsListing{Groups: []report.FindingsGroup{{
		Type:     typ,
		Findings: []report.ListedFinding{openFinding(store.Finding{ID: string(typ) + ":x", Type: typ, Items: items})},
	}}}
	data, err := renderFindingsJSON(listing, openView, nil)
	require.NoError(t, err)
	var doc struct {
		Findings []struct {
			Items []map[string]any `json:"items"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	require.Len(t, doc.Findings, 1)
	return doc.Findings[0].Items
}

func unlinkedItemsJSON(t *testing.T, items ...store.FindingItem) []map[string]any {
	t.Helper()
	return itemsJSON(t, finding.UnlinkedTransfer, items...)
}

func Test_renderFindingsJSON_gives_an_unlinked_transfer_item_its_category_path_and_a_null_category_id(t *testing.T) {
	txn := "txn-1"

	items := unlinkedItemsJSON(t,
		store.FindingItem{TransactionID: &txn, Date: findingDay(2026, 7, 2), Category: new("Income:Other"), Splits: 1})

	assert.Equal(t, []any{"Income:Other", nil}, []any{items[0]["category"], items[0]["category_id"]})
}

func Test_renderFindingsJSON_gives_an_unlinked_transfer_item_with_several_splits_or_none_a_null_category(t *testing.T) {
	items := unlinkedItemsJSON(t,
		store.FindingItem{Date: findingDay(2026, 7, 2), Splits: 2},
		store.FindingItem{Date: findingDay(2026, 7, 3), Splits: 0})

	assert.Contains(t, items[0], "category")
	assert.Nil(t, items[0]["category"])
	assert.Contains(t, items[1], "category")
	assert.Nil(t, items[1]["category"])
}

func Test_renderFindingsJSON_gives_a_mixed_categories_item_its_payee_category_and_count_and_null_transaction_fields(t *testing.T) {
	items := itemsJSON(t, finding.MixedCategories, store.FindingItem{
		PayeeID: new("payee-12"), CategoryID: new("cat-3"), Payee: "Costco", Category: new("Groceries"), Transactions: 30,
	})

	assert.Equal(t, map[string]any{
		"transaction_id": nil, "split_id": nil, "payee_id": "payee-12", "category_id": "cat-3",
		"date": nil, "account_id": nil, "account": nil, "currency": nil,
		"payee": "Costco", "category": "Groceries", "amount": nil,
		"other_account": nil, "other_account_id": nil, "transactions": float64(30), "splits": nil,
	}, items[0])
}

func Test_renderFindingsJSON_keeps_the_date_account_and_amount_of_a_transaction_item_and_a_null_transactions(t *testing.T) {
	items := itemsJSON(t, finding.Duplicate, store.FindingItem{
		TransactionID: new("txn-1"), Date: findingDay(2026, 8, 3), AccountID: "acct-3", Account: "Chequing", Currency: "CAD",
		Payee: "Hydro One", Amount: -14217,
	})

	assert.Equal(t, map[string]any{
		"transaction_id": "txn-1", "split_id": nil, "payee_id": nil, "category_id": nil,
		"date": "2026-08-03", "account_id": "acct-3", "account": "Chequing", "currency": "CAD",
		"payee": "Hydro One", "category": nil, "amount": "-142.17",
		"other_account": nil, "other_account_id": nil, "transactions": nil, "splits": nil,
	}, items[0])
}

func Test_renderFindingsJSON_keeps_the_date_account_and_amount_of_an_item_with_only_a_split(t *testing.T) {
	items := itemsJSON(t, finding.Uncategorized, store.FindingItem{
		SplitID: new("split-4"), Date: findingDay(2026, 8, 3), AccountID: "acct-3", Account: "Chequing", Currency: "CAD", Amount: -500,
	})

	assert.Equal(t, map[string]any{
		"transaction_id": nil, "split_id": "split-4", "payee_id": nil, "category_id": nil,
		"date": "2026-08-03", "account_id": "acct-3", "account": "Chequing", "currency": "CAD",
		"payee": nil, "category": nil, "amount": "-5.00",
		"other_account": nil, "other_account_id": nil, "transactions": nil, "splits": nil,
	}, items[0])
}
