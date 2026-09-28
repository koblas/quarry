package duckstore

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
	investment_transactions_not_imported BIGINT NOT NULL
);
`
