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
	fixed := report.ListedFinding{
		Status: finding.StatusFixed,
		ID:     "uncategorized:x", Type: finding.Uncategorized,
		FirstFoundAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), FixedAt: &fixedAt,
	}
	similar := report.ListedFinding{
		Status: finding.StatusOpen,
		ID:     "similar-categories:y", Type: finding.SimilarCategories,
		Items: []store.FindingItem{{Splits: 3}},
	}

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

func Test_NewFindingEntry_encodes_an_unclassified_account_with_no_first_found_time_and_only_its_account_keys(t *testing.T) {
	listed := report.ListedFinding{
		Status: finding.StatusOpen,
		ID:     "unclassified-account:acct-12", Type: finding.UnclassifiedAccount,
		Items: []store.FindingItem{{
			AccountID: "acct-12", Account: "Questrade TFSA", AccountType: store.AccountTypeBrokerage, Currency: "CAD", Closed: true,
		}},
	}

	data, err := json.Marshal(document.NewFindingEntry(listed))

	require.NoError(t, err)
	assert.JSONEq(t, `{
		"id": "unclassified-account:acct-12", "type": "unclassified-account", "status": "open",
		"first_found_at": null, "fixed_at": null, "fix": `+string(mustJSON(t, finding.UnclassifiedAccount.Fix().Sentence))+`,
		"items": [{
			"transaction_id": null, "split_id": null, "payee_id": null, "category_id": null, "date": null,
			"account_id": "acct-12", "account": "Questrade TFSA", "currency": "CAD", "payee": null, "category": null,
			"amount": null, "other_account": null, "other_account_id": null, "transactions": null, "splits": null,
			"investment_transaction_id": null, "security_id": null, "security": null, "shares": null
		}]
	}`, string(data))
}

func Test_NewFindingEntry_gives_a_stored_finding_its_first_found_time(t *testing.T) {
	stored := report.ListedFinding{
		Status: finding.StatusOpen,
		ID:     "uncategorized:x", Type: finding.Uncategorized, FirstFoundAt: time.Date(2026, 9, 1, 8, 30, 0, 0, time.UTC),
	}

	got := document.NewFindingEntry(stored)

	require.NotNil(t, got.FirstFoundAt)
	assert.Equal(t, "2026-09-01T08:30:00Z", *got.FirstFoundAt)
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}

func Test_NewFindingEntry_encodes_shares_without_cost_with_its_date_account_and_investment_keys(t *testing.T) {
	listed := report.ListedFinding{
		Status: finding.StatusOpen,
		ID:     "shares-without-cost:itxn-7", Type: finding.SharesWithoutCost,
		Items: []store.FindingItem{{
			Date: time.Date(2016, time.March, 1, 0, 0, 0, 0, time.UTC), AccountID: "acct-3", Account: "Questrade Margin", Currency: "CAD",
			InvestmentTransactionID: new("itxn-7"), SecurityID: new("sec-4"), Security: "XEQT", Shares: 100_000_000,
		}},
	}

	data, err := json.Marshal(document.NewFindingEntry(listed))

	require.NoError(t, err)
	assert.JSONEq(t, `{
		"id": "shares-without-cost:itxn-7", "type": "shares-without-cost", "status": "open",
		"first_found_at": null, "fixed_at": null, "fix": `+string(mustJSON(t, finding.SharesWithoutCost.Fix().Sentence))+`,
		"items": [{
			"transaction_id": null, "split_id": null, "payee_id": null, "category_id": null, "date": "2016-03-01",
			"account_id": "acct-3", "account": "Questrade Margin", "currency": "CAD", "payee": null, "category": null,
			"amount": null, "other_account": null, "other_account_id": null, "transactions": null, "splits": null,
			"investment_transaction_id": "itxn-7", "security_id": "sec-4", "security": "XEQT", "shares": "100.000000"
		}]
	}`, string(data))
}

func Test_UnmatchedAccountWarnings_names_the_config_and_the_list_of_each_id_registered_first(t *testing.T) {
	got := document.UnmatchedAccountWarnings(configShown, report.UnmatchedAccounts{
		Registered: []string{"acct-99"}, NonRegistered: []string{"acct-98"},
	})

	assert.Equal(t, []string{
		`~/config.toml: accounts.registered lists "acct-99", which is not an account in quarry's store; quarry skips it`,
		`~/config.toml: accounts.non-registered lists "acct-98", which is not an account in quarry's store; quarry skips it`,
	}, got)
}

func Test_UnmatchedAccountWarnings_masks_an_account_number_before_quoting_it(t *testing.T) {
	got := document.UnmatchedAccountWarnings(configShown, report.UnmatchedAccounts{NonRegistered: []string{"12345678"}})

	assert.Equal(t, []string{`~/config.toml: accounts.non-registered lists "****5678", which is not an account in quarry's store; quarry skips it`}, got)
}

func Test_UnmatchedAccountWarnings_escapes_an_id_holding_a_quote(t *testing.T) {
	got := document.UnmatchedAccountWarnings(configShown, report.UnmatchedAccounts{Registered: []string{`a"b`}})

	assert.Equal(t, []string{`~/config.toml: accounts.registered lists "a\"b", which is not an account in quarry's store; quarry skips it`}, got)
}

func Test_UnmatchedAccountWarnings_repeats_a_repeated_id(t *testing.T) {
	got := document.UnmatchedAccountWarnings(configShown, report.UnmatchedAccounts{Registered: []string{"acct-99", "acct-99"}})

	assert.Len(t, got, 2)
}

func Test_UnmatchedAccountWarnings_is_an_empty_list_not_nil_when_nothing_is_unmatched(t *testing.T) {
	got := document.UnmatchedAccountWarnings(configShown, report.UnmatchedAccounts{})

	assert.NotNil(t, got)
	assert.Empty(t, got)
}
