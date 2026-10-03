package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/koblas/quarry/internal/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const mcpInstructions = `quarry serves the user's Quicken Classic for Mac data from a local,
read-only store. Call sync_status first and tell the user how old the
snapshot is (snapshot.taken_at). For spending, income, recurring charges
and unusually large charges call spending, cash_flow, recurring_charges
and anomalies: they apply quarry's rules for transfers, refunds and
currencies. To find particular transactions by payee, memo, amount or
date call search_transactions. For other questions call describe_schema
before writing SQL for query: its conventions say which views already
leave out transfers and how amounts, signs and currencies work. Every
number you report must come from a tool result; never estimate. quarry
cannot change data: fixes are made in Quicken, then the user runs quarry
sync.`

const mcpQueryDescription = `Run one read-only SQL query (DuckDB dialect) against quarry's store and
return its columns and rows. Call describe_schema first for the tables,
views and conventions. For spending and income totals call spending or
cash_flow instead, and to find transactions by payee, memo or amount call
search_transactions; in SQL use v_spending and v_cash_flow, which already
leave out transfers between the user's own accounts. Returns at most
` + "`limit`" + ` rows (default 500, the most allowed); aggregate in SQL rather
than paging through rows. The store cannot be changed, and other files,
databases and extensions are off.
Send one statement; if you send several, only the last one's rows come back.`

const mcpSyncStatusDescription = `Report how fresh quarry's data is: the snapshot the store was built from
and when it was taken, the dates its transactions cover, the checks sync
ran (balances reconciled to Quicken, splits, transfers), open findings,
and Bank of Canada rate coverage. quarry cannot refresh the data; if it
is old, ask the user to run quarry sync.`

const mcpDescribeSchemaDescription = `Describe quarry's store: every table and view with its columns and types,
the conventions for amounts, signs, transfers and currencies, the
accounts, the category tree, and the first and last transaction dates.
Call this before writing SQL for query.`

const mcpDataQualityDescription = `List the data-quality findings quarry's last sync found: problems to fix
in Quicken (duplicates, one-sided or unlinked transfers, uncategorized
splits, payees in mixed categories, payee name variants, similar or
unused categories). Each finding has an id, the suggested fix, and the
transactions, payees or categories it is about. quarry never fixes them:
the user fixes them in Quicken and runs quarry sync, and fixed findings
drop off. To ignore a finding the user adds its id to findings.ignore in
quarry's config file.`

const mcpSpendingDescription = `Total the user's spending for a period, grouped by category, payee, tag
or month, with a total per currency. quarry's spending rules apply:
transfers between the user's own accounts, Quicken's system categories,
transactions marked "exclude from reports" and accounts Quicken leaves out
of reports are not counted, and refunds are netted, so a category can come
out negative. Each split is converted at the Bank of Canada rate for its
date. Use this rather than query for spending totals. Returns at most 500
rows; totals always count every row.`

const mcpCashFlowDescription = `Report income, spending, net and savings rate for each month or year of a
period, with totals per currency. The rules are spending's: transfers
between the user's own accounts are neither income nor spending, and spent
equals spending's total for the same period, accounts and currency.
Savings rate is net divided by income, null when income is zero or less.
A period that since or until cuts short is marked partial.`

const mcpRecurringDescription = `List charges that repeat every week, month, quarter or year at a steady
amount (subscriptions, memberships, insurance), found in all of the
user's history and listed when they were running during the period. Each
series has its cadence, latest amount, cost per year while active, price
changes and accounts. A series is found in its account's own currency, so
an exchange-rate change is never a price change. Charges dated after today
never count. Bills whose amount changes most times, such as utilities, are
not listed; use spending with by payee for those. Returns at most 500
series; totals count every series.`

const mcpAnomaliesDescription = `List charges in the period that are unusually large: more than 2 times the
median of the payee's earlier charges (when it has at least 3), else more
than 5 times the median of the category's earlier charges (at least 10).
Charges under 100.00 in their account's own currency are never listed.
Each charge is compared with all earlier history, whatever the period.
Possible duplicates are not listed here; data_quality lists them. Charges
dated after today are never listed. Returns at most 500 charges, newest
first.`

const mcpSearchDescription = `Find the user's transactions by text, date, account, category or amount,
newest first. text matches payee names, transaction memos and split
memos, ignoring letter case; % and _ are plain characters. Every
transaction is searched, including transfers between the user's own
accounts (flagged transfer) and transactions Quicken's reports leave out
(flagged excluded); spending and cash_flow do not count those, so call
them for totals rather than adding up these rows. Amounts are in each
account's own currency and are never converted. Returns at most limit
transactions (default 500); matched counts every match.`

const (
	mcpRecurringInputSchema = `{
		"type": "object",
		"properties": {
			"since": {"type": "string", "description": "List series still running on or after this date: YYYY, YYYY-MM or YYYY-MM-DD. ` +
		`Defaults to January 1 of this year."},
			"until": {"type": "string", "description": "List series that started on or before this date: YYYY, YYYY-MM or YYYY-MM-DD; a year or month ends on its last day. ` +
		`Defaults to today."},
			"accounts": {"type": "array", "items": {"type": "string"}, "description": "List only series with a charge in one of these accounts, each given by id or by name in any letter case. ` +
		`Omit it for every account."},
			"currency": {"type": "string", "enum": ["CAD", "USD", "native"], "description": "Currency for amounts: CAD, USD, or native to list each account's own currency separately. ` +
		`Defaults to reporting.currency in quarry's config file, else CAD."}
		},
		"additionalProperties": false
	}`
	mcpAnomaliesInputSchema = `{
		"type": "object",
		"properties": {
			"since": {"type": "string", "description": "List charges dated on or after this date: YYYY, YYYY-MM or YYYY-MM-DD. ` +
		`Defaults to January 1 of this year."},
			"until": {"type": "string", "description": "List charges dated on or before this date: YYYY, YYYY-MM or YYYY-MM-DD; a year or month ends on its last day. ` +
		`Defaults to today."},
			"accounts": {"type": "array", "items": {"type": "string"}, "description": "List only charges in these accounts, each given by id or by name in any letter case; ` +
		`the payee's charges in other accounts still count as history."},
			"currency": {"type": "string", "enum": ["CAD", "USD", "native"], "description": "Currency for amounts: CAD, USD, or native to list each account's own currency separately. ` +
		`Defaults to reporting.currency in quarry's config file, else CAD."}
		},
		"additionalProperties": false
	}`
)

const (
	mcpCashFlowInputSchema = `{
		"type": "object",
		"properties": {
			"since": {"type": "string", "description": "First day to count: YYYY, YYYY-MM or YYYY-MM-DD; a year or month starts on its first day. ` +
		`Defaults to January 1 of this year."},
			"until": {"type": "string", "description": "Last day to count: YYYY, YYYY-MM or YYYY-MM-DD; a year or month ends on its last day. ` +
		`Defaults to today; future-dated transactions count only when until is later than today."},
			"accounts": {"type": "array", "items": {"type": "string"}, "description": "Count only these accounts, each given by id or by name in any letter case. ` +
		`Omit it to count every account."},
			"currency": {"type": "string", "enum": ["CAD", "USD", "native"], "description": "Currency for amounts: CAD, USD, or native to list each account's own currency separately. ` +
		`Defaults to reporting.currency in quarry's config file, else CAD."},
			"by": {"type": "string", "enum": ["month", "year"], "default": "month", "description": "One row per month (the default) or per year."}
		},
		"additionalProperties": false
	}`
	mcpSpendingInputSchema = `{
		"type": "object",
		"properties": {
			"since": {"type": "string", "description": "First day to count: YYYY, YYYY-MM or YYYY-MM-DD; a year or month starts on its first day. ` +
		`Defaults to January 1 of this year."},
			"until": {"type": "string", "description": "Last day to count: YYYY, YYYY-MM or YYYY-MM-DD; a year or month ends on its last day. ` +
		`Defaults to today; future-dated transactions count only when until is later than today."},
			"accounts": {"type": "array", "items": {"type": "string"}, "description": "Count only these accounts, each given by id or by name in any letter case. ` +
		`Omit it to count every account."},
			"currency": {"type": "string", "enum": ["CAD", "USD", "native"], "description": "Currency for amounts: CAD, USD, or native to list each account's own currency separately. ` +
		`Defaults to reporting.currency in quarry's config file, else CAD."},
			"by": {"type": "string", "enum": ["category", "payee", "tag", "month"], "default": "category", "description": "Group by category (the default), payee, tag or month. ` +
		`A split with several tags counts under each tag."}
		},
		"additionalProperties": false
	}`
	mcpQueryInputSchema = `{
		"type": "object",
		"properties": {
			"sql":   {"type": "string", "minLength": 1, "description": "One read-only SQL statement in DuckDB's dialect over quarry's tables and views; describe_schema lists them."},
			"limit": {"type": "integer", "minimum": 1, "maximum": 500, "default": 500, "description": "Most rows to return, 1 to 500. Defaults to 500."}
		},
		"required": ["sql"],
		"additionalProperties": false
	}`
	mcpNoInputSchema          = `{"type": "object", "additionalProperties": false}`
	mcpDataQualityInputSchema = `{
		"type": "object",
		"properties": {
			"status": {"type": "string", "enum": ["open", "ignored", "fixed", "all"], "default": "open",
				"description": "Which findings to list: open (the default), ignored (the user listed the id in findings.ignore), fixed (no longer found since a later sync), or all."},
			"type":   {"type": "string", "enum": [
				"duplicate", "one-sided-transfer", "unlinked-transfer", "uncategorized",
				"mixed-categories", "payee-variants", "similar-categories", "unused-category"], "description": "List only findings of this type. Omit it to list every type."},
			"limit":  {"type": "integer", "minimum": 1, "maximum": 500, "default": 50,
				"description": "Most findings to return, 1 to 500. Defaults to 50. counts always covers every finding, and each finding lists at most 25 items."}
		},
		"additionalProperties": false
	}`
	mcpSearchInputSchema = `{
		"type": "object",
		"properties": {
			"text": {"type": "string", "description": "Words to find in payee names, transaction memos and split memos, in any letter case; every character is literal. ` +
		`Omit it to search by the other parameters alone."},
			"since": {"type": "string", "description": "Earliest date to list: YYYY, YYYY-MM or YYYY-MM-DD; a year or month starts on its first day. ` +
		`Omit it to search from the first transaction."},
			"until": {"type": "string", "description": "Latest date to list: YYYY, YYYY-MM or YYYY-MM-DD; a year or month ends on its last day. ` +
		`Omit it to search every later date, future-dated transactions included."},
			"accounts": {"type": "array", "items": {"type": "string"}, "description": "Search only these accounts, each given by id or by name in any letter case. ` +
		`Omit it to search every account."},
			"category": {"type": "string", "description": "List only transactions with a split in this category or one under it, given by its full path (such as Food:Groceries) in any letter case."},
			"min": {"type": "string", "description": "Smallest amount to list, as a string such as \"25\" or \"19.99\", compared without its sign in the account's own currency."},
			"max": {"type": "string", "description": "Largest amount to list, as a string such as \"100\" or \"250.50\", compared without its sign in the account's own currency. ` +
		`Give min and max the same value to find one amount."},
			"limit": {"type": "integer", "minimum": 1, "maximum": 500, "default": 500,
				"description": "Most transactions to return, newest first, 1 to 500. Defaults to 500. matched always counts every match."}
		},
		"additionalProperties": false
	}`
	mcpObjectOutputSchema = `{"type": "object"}`
)

func Test_run_mcp_describes_every_tool(t *testing.T) {
	t.Run("tools/list and instructions", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
		defer cancel()
		peer := startMCP(ctx, t, func(*cli.Env) {})
		session := peer.session

		listed, err := session.ListTools(ctx, nil)
		require.NoError(t, err)
		require.NoError(t, session.Close())
		peer.waitForExit(ctx, t)

		assert.Equal(t, mcpInstructions, session.InitializeResult().Instructions)
		wantTools := map[string]struct{ description, inputSchema string }{
			"query":               {mcpQueryDescription, mcpQueryInputSchema},
			"describe_schema":     {mcpDescribeSchemaDescription, mcpNoInputSchema},
			"sync_status":         {mcpSyncStatusDescription, mcpNoInputSchema},
			"data_quality":        {mcpDataQualityDescription, mcpDataQualityInputSchema},
			"spending":            {mcpSpendingDescription, mcpSpendingInputSchema},
			"cash_flow":           {mcpCashFlowDescription, mcpCashFlowInputSchema},
			"recurring_charges":   {mcpRecurringDescription, mcpRecurringInputSchema},
			"anomalies":           {mcpAnomaliesDescription, mcpAnomaliesInputSchema},
			"search_transactions": {mcpSearchDescription, mcpSearchInputSchema},
		}
		require.Len(t, listed.Tools, len(wantTools))
		for _, tool := range listed.Tools {
			want, known := wantTools[tool.Name]
			require.True(t, known, "unexpected tool %q", tool.Name)
			assert.Equal(t, want.description, tool.Description, tool.Name)
			assertJSONEqualAny(t, want.inputSchema, tool.InputSchema, tool.Name+" input schema")
			assertJSONEqualAny(t, mcpObjectOutputSchema, tool.OutputSchema, tool.Name+" output schema")
		}
	})

	t.Run("mcp --help", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		code := runWith(t.Context(), []string{"mcp", "--help"}, testEnv(&stdout, &stderr))

		assert.Equal(t, 0, code)
		assert.Contains(t, stdout.String(), "SQL runs read-only, and every list a tool returns\nstops at 500 entries.")
		assert.Contains(t, stdout.String(), "Tools: describe_schema, query, sync_status, data_quality, spending,\ncash_flow, recurring_charges, anomalies, search_transactions.")
		assert.Empty(t, stderr.String())
	})
}

// assertJSONEqualAny compares want to got, a decoded JSON value, as JSON.
func assertJSONEqualAny(t *testing.T, want string, got any, msg string) {
	t.Helper()
	encoded, err := json.Marshal(got)
	require.NoError(t, err, msg)
	assert.JSONEq(t, want, string(encoded), msg)
}
