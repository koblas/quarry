package duckstore

import (
	"context"
	"database/sql"
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/store"
)

// findingsQuery reads every finding with one row per item (one NULL-item row if none), new and newly fixed being at the build time.
const findingsQuery = `
SELECT f.id, f.type, f.first_found_at, f.fixed_at,
	COALESCE(f.first_found_at = i.built_at, false), COALESCE(f.fixed_at = i.built_at, false),
	fi.finding_id IS NOT NULL, fi.transaction_id, fi.split_id, fi.payee_id, fi.category_id,
	t.date, a.id, a.name, a.currency, a.closed, a.active, p.name,
	CAST(COALESCE(s.amount, t.amount) * 100 AS BIGINT), x.other_account, s.transfer_account_id,
	c.full_path, COALESCE(ts.n, cs.n, 0), COALESCE(mc.n, pc.n, 0)
FROM findings f
CROSS JOIN store_info i
LEFT JOIN finding_items fi ON fi.finding_id = f.id
LEFT JOIN transactions t ON t.id = fi.transaction_id
LEFT JOIN accounts a ON a.id = t.account_id
LEFT JOIN payees p ON p.id = COALESCE(t.payee_id, fi.payee_id)
LEFT JOIN splits s ON s.id = fi.split_id
LEFT JOIN transfers x ON x.from_split_id = fi.split_id
LEFT JOIN (SELECT transaction_id, count(*) AS n, min(category_id) AS category_id FROM splits GROUP BY transaction_id) ts
	ON ts.transaction_id = fi.transaction_id AND f.type = 'unlinked-transfer'
LEFT JOIN categories c ON c.id = COALESCE(fi.category_id, CASE WHEN ts.n = 1 THEN ts.category_id END)
LEFT JOIN (SELECT payee_id, category_id, count(*) AS n FROM (` + mixedTransactions + `) GROUP BY payee_id, category_id) mc
	ON mc.payee_id = fi.payee_id AND mc.category_id = fi.category_id AND f.type = 'mixed-categories'
LEFT JOIN (` + payeeTransactions + `) pc ON pc.payee_id = fi.payee_id AND f.type = 'payee-variants'
LEFT JOIN (` + categorySplits + `) cs ON cs.category_id = fi.category_id AND f.type = 'similar-categories'
ORDER BY f.id, fi.rowid`

// findingAccountsQuery reads every account the findings and status reads carry beside the findings.
const findingAccountsQuery = `SELECT id, name, type, currency, closed, active FROM accounts ORDER BY id`

// Findings reads every finding, open and fixed, with its items, sorted by id; a fixed finding has no items. It
// reads every account and the investment rows in the same open. It refuses a store it cannot open or read with *store.OpenError.
func (s *Store) Findings(ctx context.Context) (store.FindingList, error) {
	db, err := s.openRead(ctx)
	if err != nil {
		return store.FindingList{}, err
	}
	defer func() { _ = db.Close() }()

	var list store.FindingList
	err = db.QueryRows(ctx, findingsQuery, nil, func(scan func(dest ...any) error) error {
		var (
			id, typ                             string
			firstFoundAt                        time.Time
			fixedAt                             sql.NullTime
			isNew, newlyFixed, hasItem          bool
			txnID, splitID, payeeID, categoryID sql.NullString
			date                                sql.NullTime
			accountID, account, currency, payee sql.NullString
			closed, active                      sql.NullBool
			amount                              sql.NullInt64
			otherAccount, otherAccountID        sql.NullString
			category                            sql.NullString
			splits, transactions                int
		)
		err := scan(&id, &typ, &firstFoundAt, &fixedAt, &isNew, &newlyFixed, &hasItem, &txnID, &splitID, &payeeID, &categoryID,
			&date, &accountID, &account, &currency, &closed, &active, &payee, &amount, &otherAccount, &otherAccountID, &category, &splits, &transactions)
		if err != nil {
			return err
		}
		if n := len(list.Findings); n == 0 || list.Findings[n-1].ID != id {
			f := store.Finding{ID: id, Type: finding.Type(typ), FirstFoundAt: firstFoundAt, New: isNew, NewlyFixed: newlyFixed}
			if fixedAt.Valid {
				f.FixedAt = &fixedAt.Time
			}
			list.Findings = append(list.Findings, f)
		}
		if !hasItem {
			return nil
		}
		last := &list.Findings[len(list.Findings)-1]
		last.Items = append(last.Items, store.FindingItem{
			TransactionID: nullStringPtr(txnID), SplitID: nullStringPtr(splitID),
			PayeeID: nullStringPtr(payeeID), CategoryID: nullStringPtr(categoryID),
			Date: date.Time, AccountID: accountID.String, Account: account.String, Currency: currency.String,
			Closed: closed.Bool, Active: active.Bool, Payee: payee.String, Category: nullStringPtr(category), Splits: splits, Transactions: transactions, Amount: amount.Int64,
			OtherAccount: nullStringPtr(otherAccount), OtherAccountID: nullStringPtr(otherAccountID),
		})
		return nil
	})
	if err != nil {
		return store.FindingList{}, openFault(s.Path(), err)
	}
	list.Accounts, err = readAccounts(ctx, db)
	if err != nil {
		return store.FindingList{}, openFault(s.Path(), err)
	}
	list.Investments, err = readInvestments(ctx, db)
	if err != nil {
		return store.FindingList{}, openFault(s.Path(), err)
	}
	return list, nil
}

// readAccounts reads every account, closed included, sorted by id.
func readAccounts(ctx context.Context, db ReadDB) ([]store.Account, error) {
	var accounts []store.Account
	err := db.QueryRows(ctx, findingAccountsQuery, nil, func(scan func(dest ...any) error) error {
		var a store.Account
		if err := scan(&a.ID, &a.Name, &a.Type, &a.Currency, &a.Closed, &a.Active); err != nil {
			return err
		}
		accounts = append(accounts, a)
		return nil
	})
	return accounts, err //nolint:wrapcheck // callers classify the driver's own error with openFault
}
