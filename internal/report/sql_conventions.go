package report

// SQLConventions is the paragraph that tells a query writer what the store's amounts, views and transfers mean;
// quarry sql's help and the describe_schema document both carry it.
const SQLConventions = `Amounts are DECIMAL(18,2) in each account's own currency; negative is money
leaving the account. v_cash_flow and v_spending also carry each amount in
CAD and in USD (amount_cad and amount_usd; spent_cad and spent_usd),
converted per split at the Bank of Canada rate for its date and rounded to
the cent, as quarry spend and quarry cashflow convert; they are NULL for a
date before the first rate. v_account_balances has balance_cad and
balance_usd at today's rate. fx_rates holds one rate per business day:
usd_cad is the Canadian dollars in one US dollar. For spending and income,
query v_spending and v_cash_flow: they already leave out transfers between
your own accounts, Quicken's system categories, transactions excluded from
reports and accounts Quicken leaves out of reports, so their totals match
quarry spend and quarry cashflow. A transfer leg is any split named in
transfers.from_split_id or transfers.to_split_id.`
