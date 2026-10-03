package duckstore

import (
	"strings"

	"github.com/koblas/quarry/internal/store"
)

// schemaDDL creates quarry's own tables. Primary keys are quarry's stable
// <prefix>-<source id> ids; a build that would write two rows under the
// same id fails here, at the Appender's Close.
const schemaDDL = `
CREATE TABLE accounts (
	id VARCHAR PRIMARY KEY,
	source_id BIGINT NOT NULL,
	name VARCHAR NOT NULL,
	type VARCHAR NOT NULL,
	currency VARCHAR NOT NULL,
	institution VARCHAR,
	closed BOOLEAN NOT NULL,
	active BOOLEAN NOT NULL,
	in_reports BOOLEAN NOT NULL,
	linked_tracking BOOLEAN NOT NULL
);
CREATE TABLE categories (
	id VARCHAR PRIMARY KEY,
	source_id BIGINT NOT NULL,
	parent_id VARCHAR,
	name VARCHAR NOT NULL,
	full_path VARCHAR NOT NULL,
	kind VARCHAR NOT NULL,
	hidden BOOLEAN NOT NULL
);
CREATE TABLE payees (
	id VARCHAR PRIMARY KEY,
	source_id BIGINT NOT NULL,
	name VARCHAR NOT NULL
);
CREATE TABLE tags (
	id VARCHAR PRIMARY KEY,
	source_id BIGINT NOT NULL,
	name VARCHAR NOT NULL
);
CREATE TABLE transactions (
	id VARCHAR PRIMARY KEY,
	source_id BIGINT NOT NULL,
	account_id VARCHAR NOT NULL,
	date DATE NOT NULL,
	payee_id VARCHAR,
	memo VARCHAR,
	amount DECIMAL(18,2) NOT NULL,
	currency VARCHAR NOT NULL,
	status VARCHAR NOT NULL,
	cheque_number VARCHAR,
	excluded_from_reports BOOLEAN NOT NULL,
	posted_date DATE
);
CREATE TABLE splits (
	id VARCHAR PRIMARY KEY,
	source_id BIGINT NOT NULL,
	transaction_id VARCHAR NOT NULL,
	category_id VARCHAR,
	amount DECIMAL(18,2) NOT NULL,
	memo VARCHAR,
	transfer_account_id VARCHAR
);
CREATE TABLE transfers (
	id VARCHAR PRIMARY KEY,
	from_split_id VARCHAR NOT NULL,
	to_split_id VARCHAR,
	cross_currency BOOLEAN NOT NULL,
	other_account VARCHAR
);
CREATE TABLE split_tags (
	split_id VARCHAR NOT NULL,
	tag_id VARCHAR NOT NULL,
	PRIMARY KEY (split_id, tag_id)
);
CREATE TABLE import_runs (
	id BIGINT PRIMARY KEY,
	started_at TIMESTAMP NOT NULL,
	finished_at TIMESTAMP NOT NULL,
	snapshot_path VARCHAR NOT NULL,
	snapshot_sha256 VARCHAR NOT NULL,
	schema_fingerprint VARCHAR NOT NULL,
	accounts_rows BIGINT NOT NULL,
	categories_rows BIGINT NOT NULL,
	payees_rows BIGINT NOT NULL,
	tags_rows BIGINT NOT NULL,
	transactions_rows BIGINT NOT NULL,
	splits_rows BIGINT NOT NULL,
	split_tags_rows BIGINT NOT NULL,
	transfers_rows BIGINT NOT NULL,
	balances_checked BIGINT NOT NULL,
	balances_mismatched BIGINT NOT NULL,
	splits_mismatched BIGINT NOT NULL,
	transfers_one_sided BIGINT NOT NULL,
	investment_transactions_not_imported BIGINT NOT NULL,
	snapshot_taken_at TIMESTAMP,
	source_path VARCHAR,
	balances_never_reconciled BIGINT,
	investment_accounts BIGINT,
	transfers_paired BIGINT,
	transfers_cross_currency BIGINT,
	rates_checked_from DATE,
	rates_last DATE,
	rates_fetch_error VARCHAR
);
CREATE TABLE fx_rates (
	date DATE PRIMARY KEY,
	usd_cad DECIMAL(10,6) NOT NULL CHECK (usd_cad > 0),
	series VARCHAR NOT NULL
);
CREATE TABLE findings (
	id VARCHAR PRIMARY KEY,
	type VARCHAR NOT NULL,
	first_found_at TIMESTAMP NOT NULL,
	fixed_at TIMESTAMP
);
CREATE TABLE finding_items (
	finding_id VARCHAR NOT NULL,
	transaction_id VARCHAR,
	split_id VARCHAR,
	payee_id VARCHAR,
	category_id VARCHAR
);
CREATE TABLE store_info (
	format_version INTEGER NOT NULL,
	quarry_version VARCHAR NOT NULL,
	built_at TIMESTAMP NOT NULL
);
`

// accountBalancesViewDDL creates v_account_balances: each account with the sum of its
// transactions dated today or earlier, NULL for an investment account.
func accountBalancesViewDDL() string {
	quoted := make([]string, 0, len(store.InvestmentAccountTypes()))
	for _, t := range store.InvestmentAccountTypes() {
		quoted = append(quoted, "'"+strings.ReplaceAll(t, "'", "''")+"'")
	}
	// The date test sits in the join, not a WHERE, so an account whose only
	// transactions are future-dated is still listed.
	return `
CREATE VIEW v_account_balances AS
WITH b AS (
	SELECT a.id, a.source_id, a.name, a.type, a.currency, a.institution, a.closed, a.active,
		CASE WHEN a.type IN (` + strings.Join(quoted, ", ") + `) THEN NULL
			ELSE CAST(COALESCE(sum(t.amount), 0) AS DECIMAL(18,2)) END AS balance
	FROM accounts a
	LEFT JOIN transactions t ON t.account_id = a.id AND t.date <= current_date
	GROUP BY a.id, a.source_id, a.name, a.type, a.currency, a.institution, a.closed, a.active
), r AS (
	SELECT usd_cad FROM fx_rates WHERE date <= current_date ORDER BY date DESC LIMIT 1
)
SELECT b.id, b.source_id, b.name, b.type, b.currency, b.institution, b.closed, b.active, b.balance,
	` + convertedTo("CAD", "b.balance", "b.currency", "r.usd_cad") + ` AS balance_cad,
	` + convertedTo("USD", "b.balance", "b.currency", "r.usd_cad") + ` AS balance_usd
FROM b
LEFT JOIN r ON true;
`
}

// reportedAccount is the predicate, over accounts aliased a, for an account Quicken's reports count;
// it is the SQL form of the negation of store.Account.LeftOutOfReports.
const reportedAccount = "a.in_reports AND NOT a.linked_tracking"

// reportedTransaction is the predicate, over transactions aliased t and accounts aliased a, for a
// transaction Quicken's reports count: in a reported account and not marked "exclude from reports".
const reportedTransaction = reportedAccount + " AND NOT t.excluded_from_reports"

// transferLeg is the predicate for the split aliased alias being a leg of a transfer.
func transferLeg(alias string) string {
	return "EXISTS (SELECT 1 FROM transfers x WHERE x.from_split_id = " + alias + ".id OR x.to_split_id = " + alias + ".id)"
}

// cashFlowViewDDL creates v_cash_flow: each split that counts as income or spending in Quicken's reports,
// with its amount in CAD and in USD at the latest fx_rates rate dated on or before the split's date.
func cashFlowViewDDL() string {
	return `
CREATE VIEW v_cash_flow AS
SELECT s.id AS split_id, s.transaction_id, t.account_id, t.date,
	CAST(date_trunc('month', t.date) AS DATE) AS month, t.currency,
	s.category_id, c.full_path AS category, t.payee_id, p.name AS payee,
	CASE WHEN s.category_id IS NOT NULL THEN c.kind
		WHEN s.amount < 0 THEN 'expense'
		ELSE 'income' END AS flow,
	s.amount,
	` + convertedTo("CAD", "s.amount", "t.currency", "r.usd_cad") + ` AS amount_cad,
	` + convertedTo("USD", "s.amount", "t.currency", "r.usd_cad") + ` AS amount_usd,
	r.usd_cad
FROM splits s
JOIN transactions t ON t.id = s.transaction_id
JOIN accounts a ON a.id = t.account_id
LEFT JOIN categories c ON c.id = s.category_id
LEFT JOIN payees p ON p.id = t.payee_id
ASOF LEFT JOIN fx_rates r ON t.date >= r.date
WHERE ` + reportedTransaction + `
	AND c.kind IS DISTINCT FROM 'system'
	AND NOT (s.category_id IS NULL AND s.amount = 0)
	AND NOT ` + transferLeg("s") + `;
COMMENT ON VIEW v_cash_flow IS 'excludes accounts where accounts.in_reports is false or accounts.linked_tracking is true, as Quicken reports do.';
`
}

// spendingViewDDL creates v_spending: the expense rows of v_cash_flow, amount sign flipped.
const spendingViewDDL = `
CREATE VIEW v_spending AS
SELECT split_id, transaction_id, account_id, date, month, currency,
	category_id, category, payee_id, payee, -amount AS spent,
	-amount_cad AS spent_cad, -amount_usd AS spent_usd, usd_cad
FROM v_cash_flow
WHERE flow = 'expense';
COMMENT ON VIEW v_spending IS 'expense splits of v_cash_flow with spent = -amount; same exclusions as v_cash_flow, so totals match quarry spend.';
`
