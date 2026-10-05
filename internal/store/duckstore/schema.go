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
	posted_date DATE,
	investment_transaction_id VARCHAR
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
CREATE TABLE securities (
	id VARCHAR PRIMARY KEY,
	source_id BIGINT NOT NULL,
	name VARCHAR NOT NULL,
	ticker VARCHAR,
	currency VARCHAR
);
CREATE TABLE prices (
	security_id VARCHAR NOT NULL,
	source_id BIGINT NOT NULL,
	date DATE NOT NULL,
	price DECIMAL(18,6) NOT NULL,
	PRIMARY KEY (security_id, date)
);
CREATE TABLE investment_transactions (
	id VARCHAR PRIMARY KEY,
	source_id BIGINT NOT NULL,
	account_id VARCHAR NOT NULL,
	security_id VARCHAR,
	date DATE NOT NULL,
	action VARCHAR NOT NULL,
	shares DECIMAL(18,6),
	amount DECIMAL(18,2) NOT NULL,
	commission DECIMAL(18,4),
	currency VARCHAR NOT NULL,
	memo VARCHAR,
	split_new_shares DECIMAL(18,6),
	split_old_shares DECIMAL(18,6)
);
CREATE TABLE holding_shares (
	account_id VARCHAR NOT NULL,
	security_id VARCHAR NOT NULL,
	from_date DATE NOT NULL,
	to_date DATE,
	shares DECIMAL(18,6) NOT NULL,
	PRIMARY KEY (account_id, security_id, from_date)
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
	snapshot_taken_at TIMESTAMP,
	source_path VARCHAR,
	balances_never_reconciled BIGINT,
	investment_accounts BIGINT,
	transfers_paired BIGINT,
	transfers_cross_currency BIGINT,
	rates_checked_from DATE,
	rates_last DATE,
	rates_fetch_error VARCHAR,
	securities_rows BIGINT,
	prices_rows BIGINT,
	investment_transactions_rows BIGINT,
	shares_checked BIGINT
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

// investmentTypesSQL is the SQL list of the accounts.type values that hold securities.
func investmentTypesSQL() string {
	quoted := make([]string, 0, len(store.InvestmentAccountTypes()))
	for _, t := range store.InvestmentAccountTypes() {
		quoted = append(quoted, "'"+strings.ReplaceAll(t, "'", "''")+"'")
	}
	return strings.Join(quoted, ", ")
}

// accountBalancesViewDDL creates v_account_balances: each account with its cash, holdings value (NULL
// outside investment accounts) and balance as v_balances_daily has them today.
func accountBalancesViewDDL() string {
	// The join to v_balances_daily is a LEFT JOIN, so an account with no row there (no transactions, or
	// only future-dated ones) is still listed with 0.00.
	return `
CREATE VIEW v_account_balances AS
WITH b AS (
	SELECT a.id, a.source_id, a.name, a.type, a.currency, a.institution, a.closed, a.active,
		CAST(coalesce(d.cash, 0) AS DECIMAL(18,2)) AS cash,
		CASE WHEN a.type IN (` + investmentTypesSQL() + `) THEN CAST(coalesce(d.holdings_value, 0) AS DECIMAL(38,2)) END AS holdings_value,
		CAST(coalesce(d.balance, 0) AS DECIMAL(38,2)) AS balance
	FROM accounts a
	LEFT JOIN v_balances_daily d ON d.account_id = a.id AND d.date = current_date
), r AS (
	SELECT usd_cad FROM fx_rates WHERE date <= current_date ORDER BY date DESC LIMIT 1
)
SELECT b.id, b.source_id, b.name, b.type, b.currency, b.institution, b.closed, b.active, b.cash, b.holdings_value, b.balance,
	` + convertedToWide("CAD", "b.balance", "b.currency", "r.usd_cad", 38) + ` AS balance_cad,
	` + convertedToWide("USD", "b.balance", "b.currency", "r.usd_cad", 38) + ` AS balance_usd
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

// holdingsViewComment is the COMMENT ON VIEW text of v_holdings.
const holdingsViewComment = "one row per holding per day it is held, through today, so filter by date; " +
	"value is shares times price rounded to the cent, value_cad and value_usd convert it at the rate for date as quarry holdings does; " +
	"cash in investment accounts is not included."

// lastHeldDay is the SQL for the last day of a holding_shares span: its to_date, or today while open, and never after today.
const lastHeldDay = "least(coalesce(to_date, current_date), current_date)"

// holdingsViewDDL creates v_holdings: one row per holding per day held, through today, valued at the latest
// price on or before the day and converted at the latest rate on or before it.
func holdingsViewDDL() string {
	// The DECIMAL(19,6) operands make the product DECIMAL(38,12); two DECIMAL(18,6) operands overflow 64 bits at the largest holdings.
	return `
CREATE VIEW v_holdings AS
WITH days AS (
	SELECT account_id, security_id, shares,
		CAST(unnest(generate_series(from_date, ` + lastHeldDay + `, INTERVAL 1 DAY)) AS DATE) AS date
	FROM holding_shares
), priced AS (
	SELECT d.date, d.account_id, d.security_id, d.shares, p.price, p.date AS price_date,
		CAST(CAST(d.shares AS DECIMAL(19,6)) * CAST(p.price AS DECIMAL(19,6)) AS DECIMAL(38,2)) AS value
	FROM days d
	ASOF LEFT JOIN prices p ON d.security_id = p.security_id AND d.date >= p.date
)
SELECT v.date, v.account_id, v.security_id, s.name AS security, s.ticker, v.shares, v.price, v.price_date, s.currency, v.value,
	` + convertedToWide("CAD", "v.value", "s.currency", "r.usd_cad", 38) + ` AS value_cad,
	` + convertedToWide("USD", "v.value", "s.currency", "r.usd_cad", 38) + ` AS value_usd,
	r.usd_cad
FROM priced v
LEFT JOIN securities s ON s.id = v.security_id
ASOF LEFT JOIN fx_rates r ON v.date >= r.date;
COMMENT ON VIEW v_holdings IS '` + holdingsViewComment + `';
`
}

// balancesDailyViewComment is the COMMENT ON VIEW text of v_balances_daily.
const balancesDailyViewComment = "one row per account per day from its first transaction through today; " +
	"cash is the sum of its transactions to that day, holdings_value its holdings' value in its own currency " +
	"(NULL outside brokerage and retirement accounts), balance is cash plus holdings_value, " +
	"as quarry accounts and quarry networth use; filter by date."

// valuedInAccountCurrencySQL is the SQL for a v_holdings row h's value in the currency of its account a, NULL when
// it has none: the holding is left out of balances. v_balances_daily and the unvalued-holdings reads share it.
func valuedInAccountCurrencySQL() string {
	return `CASE WHEN h.currency = a.currency THEN h.value
		WHEN a.currency = 'CAD' THEN h.value_cad
		WHEN a.currency = 'USD' THEN h.value_usd END`
}

// balancesDailyViewDDL creates v_balances_daily: one row per account per day, its cash, the value of its holdings
// and the balance in CAD and USD at the day's rate.
func balancesDailyViewDDL() string {
	return `
CREATE VIEW v_balances_daily AS
WITH firsts AS (
	SELECT account_id, min(d) AS first_day
	FROM (SELECT account_id, date AS d FROM transactions UNION ALL SELECT account_id, from_date FROM holding_shares)
	GROUP BY account_id
), days AS (
	SELECT account_id, CAST(unnest(generate_series(first_day, current_date, INTERVAL 1 DAY)) AS DATE) AS date
	FROM firsts
), flow AS (
	SELECT account_id, date, sum(amount) AS amount FROM transactions GROUP BY account_id, date
), cashed AS (
	SELECT d.account_id, d.date,
		CAST(sum(coalesce(f.amount, 0)) OVER (PARTITION BY d.account_id ORDER BY d.date) AS DECIMAL(18,2)) AS cash
	FROM days d
	LEFT JOIN flow f ON f.account_id = d.account_id AND f.date = d.date
), valued AS (
	SELECT h.account_id, h.date, ` + valuedInAccountCurrencySQL() + ` AS value
	FROM v_holdings h
	JOIN accounts a ON a.id = h.account_id
), held AS (
	SELECT account_id, date, sum(value) AS value, count(*) - count(value) AS unvalued
	FROM valued
	GROUP BY account_id, date
), parts AS (
	SELECT c.date, a.id AS account_id, a.name AS account, a.type, a.currency, c.cash,
		a.type IN (` + investmentTypesSQL() + `) AS investment,
		coalesce(h.value, 0) AS held_value, coalesce(h.unvalued, 0) AS held_unvalued
	FROM cashed c
	JOIN accounts a ON a.id = c.account_id
	LEFT JOIN held h ON h.account_id = c.account_id AND h.date = c.date
)
SELECT b.date, b.account_id, b.account, b.type, b.currency, b.cash,
	CASE WHEN b.investment THEN CAST(b.held_value AS DECIMAL(38,2)) END AS holdings_value,
	CASE WHEN b.investment THEN b.held_unvalued END AS holdings_unvalued,
	b.balance,
	` + convertedToWide("CAD", "b.balance", "b.currency", "r.usd_cad", 38) + ` AS balance_cad,
	` + convertedToWide("USD", "b.balance", "b.currency", "r.usd_cad", 38) + ` AS balance_usd,
	r.usd_cad
FROM (
	SELECT *, CAST(cash + CASE WHEN investment THEN held_value ELSE 0 END AS DECIMAL(38,2)) AS balance
	FROM parts
) b
ASOF LEFT JOIN fx_rates r ON b.date >= r.date;
COMMENT ON VIEW v_balances_daily IS '` + strings.ReplaceAll(balancesDailyViewComment, "'", "''") + `';
`
}

// netWorthViewComment is the COMMENT ON VIEW text of v_net_worth.
const netWorthViewComment = "net worth by day, account type and currency over the accounts Quicken's reports count, as quarry networth does; " +
	"sum balance_cad or balance_usd over one date for the total; a NULL there means no exchange rate for that day."

// netWorthViewDDL creates v_net_worth: net worth by day, account type and currency over the accounts Quicken's reports count.
func netWorthViewDDL() string {
	// Rate presence depends only on (currency, day), so within a row every account has a rate or none does and a plain sum is NULL exactly when one is missing.
	return `
CREATE VIEW v_net_worth AS
SELECT b.date, b.type, b.currency, count(*) AS accounts,
	CAST(sum(b.balance) AS DECIMAL(38,2)) AS balance,
	CAST(sum(b.balance_cad) AS DECIMAL(38,2)) AS balance_cad,
	CAST(sum(b.balance_usd) AS DECIMAL(38,2)) AS balance_usd
FROM v_balances_daily b
JOIN accounts a ON a.id = b.account_id
WHERE ` + reportedAccount + `
GROUP BY b.date, b.type, b.currency;
COMMENT ON VIEW v_net_worth IS '` + strings.ReplaceAll(netWorthViewComment, "'", "''") + `';
`
}
