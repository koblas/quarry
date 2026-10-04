package report

// SQLConventions is the paragraph that tells a query writer what the store's amounts, views and transfers mean;
// quarry sql's help and the describe_schema document both carry it.
const SQLConventions = `Amounts are DECIMAL(18,2) in each account's own currency; negative is money
leaving the account. v_cash_flow and v_spending also carry each amount in
CAD and in USD (amount_cad and amount_usd; spent_cad and spent_usd),
converted per split at the Bank of Canada rate for its date and rounded to
the cent, as quarry spend and quarry cashflow convert; they are NULL for a
date before the first rate. v_account_balances (v_balances_daily for
today) has balance_cad and balance_usd at today's rate. fx_rates holds
one rate per business day: usd_cad is the Canadian dollars in one US
dollar. For spending and income, query v_spending and v_cash_flow: they
already leave out transfers between your own accounts, Quicken's system
categories, transactions excluded from reports and accounts Quicken
leaves out of reports, so their totals match quarry spend and quarry
cashflow. A transfer leg is any split named in transfers.from_split_id
or transfers.to_split_id.

Each investment transaction that moves cash also has a row in transactions
(investment_transaction_id names it; NULL for a register entry), one split per
Quicken entry, so an account's cash is the sum of its transactions. In
v_cash_flow dividends, interest and capital-gain distributions are income;
buys, sells and share moves are neither. investment_transactions holds each
one's action, security and shares: Their amount is DECIMAL(18,2) in the account's
own currency, negative when cash leaves the account; commission is
DECIMAL(18,4) in the account's own currency as Quicken recorded it (some
brokers charge fractions of a cent), NULL when there is none; shares is
DECIMAL(18,6) as Quicken recorded each transaction, negative when shares
leave. A split row carries split_new_shares and split_old_shares instead, so
a sum of shares is not a holding. prices holds each security's closing price
per day as Quicken recorded it, rounded to 6 decimals, in the security's
currency (securities.currency, NULL when Quicken records none).
holding_shares holds each account's count of each security, one row per span
of days it is unchanged and not zero (from_date through to_date, NULL while
still held), splits applied; these are the counts quarry sync checks against
Quicken. v_holdings has one row per holding per day held, through today:
price is the latest on or before date and price_date its day (NULL when
none), value is shares times price rounded to the cent, value_cad and
value_usd convert it at the rate for date, as quarry holdings does; filter
it by date. Neither includes cash in investment accounts. action is one of
add_shares, buy, capital_gain_long, capital_gain_short, dividend, interest,
margin_interest, misc_expense, misc_income, reinvest_dividend,
remove_shares, sell, split.

v_balances_daily has one row per account per day from its first transaction
through today; cash is the sum of its transactions to that day,
holdings_value its holdings' value in its own currency (NULL outside
brokerage and retirement accounts), balance is cash plus holdings_value, as
quarry accounts and quarry networth use; filter by date. v_net_worth has
net worth by day, account type and currency over the accounts Quicken's
reports count, as quarry networth does; sum balance_cad or balance_usd over
one date for the total; a NULL there means no exchange rate for that day.`
