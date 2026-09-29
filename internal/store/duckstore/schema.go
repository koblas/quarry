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
	active BOOLEAN NOT NULL
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
	cheque_number VARCHAR
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
	cross_currency BOOLEAN NOT NULL
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
	transfers_cross_currency BIGINT
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
SELECT a.id, a.source_id, a.name, a.type, a.currency, a.institution, a.closed, a.active,
	CASE WHEN a.type IN (` + strings.Join(quoted, ", ") + `) THEN NULL
		ELSE CAST(COALESCE(sum(t.amount), 0) AS DECIMAL(18,2)) END AS balance
FROM accounts a
LEFT JOIN transactions t ON t.account_id = a.id AND t.date <= current_date
GROUP BY a.id, a.source_id, a.name, a.type, a.currency, a.institution, a.closed, a.active;
`
}
