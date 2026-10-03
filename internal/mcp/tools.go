package mcp

import (
	"fmt"
	"strconv"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tool names, as clients call them.
const (
	toolQuery       = "query"
	toolDescribe    = "describe_schema"
	toolSyncStatus  = "sync_status"
	toolDataQuality = "data_quality"
	toolSpending    = "spending"
	toolCashFlow    = "cash_flow"
	toolRecurring   = "recurring_charges"
	toolAnomalies   = "anomalies"
	toolSearch      = "search_transactions"
)

// Row limits: the most any tool returns, data_quality's default, and the most items it lists per finding.
const (
	maxRows          = 500
	defaultFindLimit = 50
	maxItems         = 25
)

// callTimeout is the deadline of a tool call unless WithTimeout sets another.
const callTimeout = 30 * time.Second

// instructions is sent to every client at initialize.
const instructions = `quarry serves David's Quicken Classic for Mac data from a local, read-only
store. Call sync_status first and tell the user how old the snapshot is
(snapshot.taken_at). For spending, income, recurring charges and unusually
large charges call spending, cash_flow, recurring_charges and anomalies:
they apply quarry's rules for transfers, refunds and currencies. For other
questions call describe_schema before writing SQL for query: its
conventions say which views already leave out transfers and how amounts,
signs and currencies work. Every number you report must come from a tool
result; never estimate. quarry cannot change data: fixes are made in
Quicken, then the user runs quarry sync.`

const queryDescription = `Run one read-only SQL query (DuckDB dialect) against quarry's store and
return its columns and rows. Call describe_schema first for the tables,
views and conventions. For spending and income totals call spending or
cash_flow instead; in SQL use v_spending and v_cash_flow, which already
leave out transfers between the user's own accounts. Returns at most
` + "`limit`" + ` rows (default 500, the most allowed); aggregate in SQL rather
than paging through rows. The store cannot be changed, and other files,
databases and extensions are off.
Send one statement; if you send several, only the last one's rows come back.`

const describeSchemaDescription = `Describe quarry's store: every table and view with its columns and types,
the conventions for amounts, signs, transfers and currencies, the
accounts, the category tree, and the first and last transaction dates.
Call this before writing SQL for query.`

const syncStatusDescription = `Report how fresh quarry's data is: the snapshot the store was built from
and when it was taken, the dates its transactions cover, the checks sync
ran (balances reconciled to Quicken, splits, transfers), open findings,
and Bank of Canada rate coverage. quarry cannot refresh the data; if it
is old, ask the user to run quarry sync.`

const dataQualityDescription = `List the data-quality findings quarry's last sync found: problems to fix
in Quicken (duplicates, one-sided or unlinked transfers, uncategorized
splits, payees in mixed categories, payee name variants, similar or
unused categories). Each finding has an id, the suggested fix, and the
transactions, payees or categories it is about. quarry never fixes them:
the user fixes them in Quicken and runs quarry sync, and fixed findings
drop off. To ignore a finding the user adds its id to findings.ignore in
quarry's config file.`

const spendingDescription = `Total the user's spending for a period, grouped by category, payee, tag
or month, with a total per currency. quarry's spending rules apply:
transfers between the user's own accounts, Quicken's system categories,
transactions marked "exclude from reports" and accounts Quicken leaves out
of reports are not counted, and refunds are netted, so a category can come
out negative. Each split is converted at the Bank of Canada rate for its
date. Use this rather than query for spending totals. Returns at most 500
rows; totals always count every row.`

const cashFlowDescription = `Report income, spending, net and savings rate for each month or year of a
period, with totals per currency. The rules are spending's: transfers
between the user's own accounts are neither income nor spending, and spent
equals spending's total for the same period, accounts and currency.
Savings rate is net divided by income, null when income is zero or less.
A period that since or until cuts short is marked partial.`

const recurringDescription = `List charges that repeat every week, month, quarter or year at a steady
amount (subscriptions, memberships, insurance), found in all of the
user's history and listed when they were running during the period. Each
series has its cadence, latest amount, cost per year while active, price
changes and accounts. A series is found in its account's own currency, so
an exchange-rate change is never a price change. Charges dated after today
never count. Bills whose amount changes most times, such as utilities, are
not listed; use spending with by payee for those. Returns at most 500
series; totals count every series.`

const anomaliesDescription = `List charges in the period that are unusually large: more than 2 times the
median of the payee's earlier charges (when it has at least 3), else more
than 5 times the median of the category's earlier charges (at least 10).
Charges under 100.00 in their account's own currency are never listed.
Each charge is compared with all earlier history, whatever the period.
Possible duplicates are not listed here; data_quality lists them. Charges
dated after today are never listed. Returns at most 500 charges, newest
first.`

const searchDescription = `Find the user's transactions by text, date, account, category or amount,
newest first. text matches payee names, transaction memos and split
memos, ignoring letter case; % and _ are plain characters. Every
transaction is searched, including transfers between the user's own
accounts (flagged transfer) and transactions Quicken's reports leave out
(flagged excluded); spending and cash_flow do not count those, so call
them for totals rather than adding up these rows. Amounts are in each
account's own currency and are never converted. Returns at most limit
transactions (default 500); matched counts every match.`

// The descriptions of the parameters query and data_quality take.
const (
	querySQLDescription       = "One read-only SQL statement in DuckDB's dialect over quarry's tables and views; describe_schema lists them."
	queryLimitDescription     = "Most rows to return, 1 to 500. Defaults to 500."
	findingsStatusDescription = "Which findings to list: open (the default), ignored (the user listed the id in findings.ignore), fixed (no longer found since a later sync), or all."
	findingsTypeDescription   = "List only findings of this type. Omit it to list every type."
	findingsLimitDescription  = "Most findings to return, 1 to 500. Defaults to 50. counts always covers every finding, and each finding lists at most 25 items."
)

// The descriptions of the parameters spending shares with the other report tools.
const (
	sinceDescription    = "First day to count: YYYY, YYYY-MM or YYYY-MM-DD; a year or month starts on its first day. Defaults to January 1 of this year."
	untilDescription    = "Last day to count: YYYY, YYYY-MM or YYYY-MM-DD; a year or month ends on its last day. Defaults to today; future-dated transactions count only when until is later than today."
	accountsDescription = "Count only these accounts, each given by id or by name in any letter case. Omit it to count every account."
	currencyDescription = "Currency for amounts: CAD, USD, or native to list each account's own currency separately. Defaults to reporting.currency in quarry's config file, else CAD."
	spendingByDesc      = "Group by category (the default), payee, tag or month. A split with several tags counts under each tag."
	cashFlowByDesc      = "One row per month (the default) or per year."
)

// The descriptions of the parameters recurring_charges and anomalies word for themselves.
const (
	recurringSinceDescription    = "List series still running on or after this date: YYYY, YYYY-MM or YYYY-MM-DD. Defaults to January 1 of this year."
	recurringUntilDescription    = "List series that started on or before this date: YYYY, YYYY-MM or YYYY-MM-DD; a year or month ends on its last day. Defaults to today."
	recurringAccountsDescription = "List only series with a charge in one of these accounts, each given by id or by name in any letter case. Omit it for every account."
	anomaliesSinceDescription    = "List charges dated on or after this date: YYYY, YYYY-MM or YYYY-MM-DD. Defaults to January 1 of this year."
	anomaliesUntilDescription    = "List charges dated on or before this date: YYYY, YYYY-MM or YYYY-MM-DD; a year or month ends on its last day. Defaults to today."
	anomaliesAccountsDescription = "List only charges in these accounts, each given by id or by name in any letter case; the payee's charges in other accounts still count as history."
)

// The descriptions of the parameters search_transactions takes.
const (
	searchTextDescription     = "Words to find in payee names, transaction memos and split memos, in any letter case; every character is literal. Omit it to search by the other parameters alone."
	searchSinceDescription    = "Earliest date to list: YYYY, YYYY-MM or YYYY-MM-DD; a year or month starts on its first day. Omit it to search from the first transaction."
	searchUntilDescription    = "Latest date to list: YYYY, YYYY-MM or YYYY-MM-DD; a year or month ends on its last day. Omit it to search every later date, future-dated transactions included."
	searchAccountsDescription = "Search only these accounts, each given by id or by name in any letter case. Omit it to search every account."
	searchCategoryDescription = "List only transactions with a split in this category or one under it, given by its full path (such as Food:Groceries) in any letter case."
	searchMinDescription      = `Smallest amount to list, as a string such as "25" or "19.99", compared without its sign in the account's own currency.`
	searchMaxDescription      = `Largest amount to list, as a string such as "100" or "250.50", compared without its sign in the account's own currency. ` +
		"Give min and max the same value to find one amount."
	searchLimitDescription = "Most transactions to return, newest first, 1 to 500. Defaults to 500. matched always counts every match."
)

type (
	// queryInput is the query tool's arguments.
	queryInput struct {
		SQL   string `json:"sql"`
		Limit int    `json:"limit"`
	}
	// dataQualityInput is the data_quality tool's arguments.
	dataQualityInput struct {
		Status string `json:"status"`
		Type   string `json:"type"`
		Limit  int    `json:"limit"`
	}
	// spendingInput is the spending tool's arguments; Since and Until are nil when absent.
	spendingInput struct {
		Since    *string  `json:"since"`
		Until    *string  `json:"until"`
		Accounts []string `json:"accounts"`
		Currency string   `json:"currency"`
		By       string   `json:"by"`
	}
	// cashFlowInput is the cash_flow tool's arguments; Since and Until are nil when absent.
	cashFlowInput struct {
		Since    *string  `json:"since"`
		Until    *string  `json:"until"`
		Accounts []string `json:"accounts"`
		Currency string   `json:"currency"`
		By       string   `json:"by"`
	}
	// recurringInput is the recurring_charges tool's arguments; Since and Until are nil when absent.
	recurringInput struct {
		Since    *string  `json:"since"`
		Until    *string  `json:"until"`
		Accounts []string `json:"accounts"`
		Currency string   `json:"currency"`
	}
	// anomaliesInput is the anomalies tool's arguments; Since and Until are nil when absent.
	anomaliesInput struct {
		Since    *string  `json:"since"`
		Until    *string  `json:"until"`
		Accounts []string `json:"accounts"`
		Currency string   `json:"currency"`
	}
	// searchInput is the search_transactions tool's arguments. The string fields are pointers so absent (nil) stays
	// apart from given and empty, which the handler refuses.
	searchInput struct {
		Text     *string  `json:"text"`
		Since    *string  `json:"since"`
		Until    *string  `json:"until"`
		Accounts []string `json:"accounts"`
		Category *string  `json:"category"`
		Min      *string  `json:"min"`
		Max      *string  `json:"max"`
		Limit    int      `json:"limit"`
	}
	// noInput is the arguments of a tool that takes none.
	noInput struct{}
)

// addTools registers quarry's tools on srv.
func (s *Server) addTools(srv *sdk.Server) {
	sdk.AddTool(srv, tool(toolDescribe, describeSchemaDescription, objectSchema(nil)), handler(s.timeout, stoppedLine(toolDescribe), s.describeSchema))
	sdk.AddTool(srv, tool(toolQuery, queryDescription, objectSchema(map[string]*jsonschema.Schema{
		"sql":   described(querySQLDescription, &jsonschema.Schema{Type: "string", MinLength: new(1)}),
		"limit": described(queryLimitDescription, limitSchema(maxRows)),
	}, "sql")), handler(s.timeout, queryStoppedLine, s.query))
	sdk.AddTool(srv, tool(toolSyncStatus, syncStatusDescription, objectSchema(nil)), handler(s.timeout, stoppedLine(toolSyncStatus), s.syncStatus))
	sdk.AddTool(srv, tool(toolDataQuality, dataQualityDescription, objectSchema(map[string]*jsonschema.Schema{
		"status": described(findingsStatusDescription, &jsonschema.Schema{Type: "string", Enum: findingStatuses(), Default: []byte(`"open"`)}),
		"type":   described(findingsTypeDescription, &jsonschema.Schema{Type: "string", Enum: findingTypes()}),
		"limit":  described(findingsLimitDescription, limitSchema(defaultFindLimit)),
	})), handler(s.timeout, stoppedLine(toolDataQuality), s.dataQuality))
	sdk.AddTool(srv, tool(toolSpending, spendingDescription, objectSchema(map[string]*jsonschema.Schema{
		"since":    described(sinceDescription, &jsonschema.Schema{Type: "string"}),
		"until":    described(untilDescription, &jsonschema.Schema{Type: "string"}),
		"accounts": accountsSchema(accountsDescription),
		"currency": currencySchema(),
		"by":       described(spendingByDesc, &jsonschema.Schema{Type: "string", Enum: spendingGroups(), Default: []byte(`"category"`)}),
	})), handler(s.timeout, stoppedLine(toolSpending), s.spending))
	sdk.AddTool(srv, tool(toolCashFlow, cashFlowDescription, objectSchema(map[string]*jsonschema.Schema{
		"since":    described(sinceDescription, &jsonschema.Schema{Type: "string"}),
		"until":    described(untilDescription, &jsonschema.Schema{Type: "string"}),
		"accounts": accountsSchema(accountsDescription),
		"currency": currencySchema(),
		"by":       described(cashFlowByDesc, &jsonschema.Schema{Type: "string", Enum: cashFlowPeriods(), Default: []byte(`"month"`)}),
	})), handler(s.timeout, stoppedLine(toolCashFlow), s.cashFlow))
	sdk.AddTool(srv, tool(toolRecurring, recurringDescription, objectSchema(map[string]*jsonschema.Schema{
		"since":    described(recurringSinceDescription, &jsonschema.Schema{Type: "string"}),
		"until":    described(recurringUntilDescription, &jsonschema.Schema{Type: "string"}),
		"accounts": accountsSchema(recurringAccountsDescription),
		"currency": currencySchema(),
	})), handler(s.timeout, stoppedLine(toolRecurring), s.recurringCharges))
	sdk.AddTool(srv, tool(toolAnomalies, anomaliesDescription, objectSchema(map[string]*jsonschema.Schema{
		"since":    described(anomaliesSinceDescription, &jsonschema.Schema{Type: "string"}),
		"until":    described(anomaliesUntilDescription, &jsonschema.Schema{Type: "string"}),
		"accounts": accountsSchema(anomaliesAccountsDescription),
		"currency": currencySchema(),
	})), handler(s.timeout, stoppedLine(toolAnomalies), s.anomalies))
	sdk.AddTool(srv, tool(toolSearch, searchDescription, objectSchema(map[string]*jsonschema.Schema{
		"text":     described(searchTextDescription, &jsonschema.Schema{Type: "string"}),
		"since":    described(searchSinceDescription, &jsonschema.Schema{Type: "string"}),
		"until":    described(searchUntilDescription, &jsonschema.Schema{Type: "string"}),
		"accounts": accountsSchema(searchAccountsDescription),
		"category": described(searchCategoryDescription, &jsonschema.Schema{Type: "string"}),
		"min":      described(searchMinDescription, &jsonschema.Schema{Type: "string"}),
		"max":      described(searchMaxDescription, &jsonschema.Schema{Type: "string"}),
		"limit":    described(searchLimitDescription, limitSchema(maxRows)),
	})), handler(s.timeout, stoppedLine(toolSearch), s.searchTransactions))
}

// tool describes one tool; its result is a JSON object.
func tool(name, description string, input *jsonschema.Schema) *sdk.Tool {
	return &sdk.Tool{
		Name:         name,
		Description:  description,
		InputSchema:  input,
		OutputSchema: &jsonschema.Schema{Type: "object"},
	}
}

// objectSchema is the schema of an arguments object that refuses unknown properties.
func objectSchema(properties map[string]*jsonschema.Schema, required ...string) *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:                 "object",
		Properties:           properties,
		Required:             required,
		AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
	}
}

// limitSchema is a row limit between 1 and maxRows that defaults to def.
func limitSchema(def int) *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:    "integer",
		Minimum: new(1.0),
		Maximum: new(float64(maxRows)),
		Default: []byte(strconv.Itoa(def)),
	}
}

// described is schema with description set.
func described(description string, schema *jsonschema.Schema) *jsonschema.Schema {
	schema.Description = description
	return schema
}

// accountsSchema is the schema of an accounts list described by description; absent and [] both mean every account.
func accountsSchema(description string) *jsonschema.Schema {
	return described(description, &jsonschema.Schema{Type: "array", Items: &jsonschema.Schema{Type: "string"}})
}

// currencySchema is the schema of the currency parameter. It has no default, which would override reporting.currency,
// and its enum is exact-case where the CLI's flag is not.
func currencySchema() *jsonschema.Schema {
	return described(currencyDescription, &jsonschema.Schema{Type: "string", Enum: []any{money.CAD.String(), money.USD.String(), money.Native.String()}})
}

// spendingGroups lists the groupings spending accepts for by, in the order the store declares them.
func spendingGroups() []any { return stringEnum(store.SpendingGroups()) }

// cashFlowPeriods lists the periods cash_flow accepts for by, in the order the store declares them.
func cashFlowPeriods() []any { return stringEnum(store.CashFlowPeriods()) }

// stringEnum is the String of each value, as a schema enum.
func stringEnum[T fmt.Stringer](values []T) []any {
	enum := make([]any, len(values))
	for i, v := range values {
		enum[i] = v.String()
	}
	return enum
}

// findingStatuses lists the statuses data_quality accepts.
func findingStatuses() []any {
	return []any{string(finding.StatusOpen), string(finding.StatusIgnored), string(finding.StatusFixed), string(report.FindingsAll)}
}

// findingTypes lists the finding types data_quality accepts.
func findingTypes() []any {
	types := finding.Types()
	enum := make([]any, len(types))
	for i, t := range types {
		enum[i] = string(t)
	}
	return enum
}
