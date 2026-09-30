package duckstore

import (
	"context"
	"database/sql"
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/store"
)

// findingsQuery reads every finding with one row per item (one NULL-item row if none); new and newly fixed mean found or fixed at the build time.
const findingsQuery = `
SELECT f.id, f.type, f.first_found_at, f.fixed_at,
	COALESCE(f.first_found_at = i.built_at, false), COALESCE(f.fixed_at = i.built_at, false),
	fi.finding_id IS NOT NULL, fi.transaction_id, fi.split_id, fi.payee_id, fi.category_id,
	t.date, a.id, a.name, a.currency, a.closed, a.active, p.name,
	CAST(COALESCE(s.amount, t.amount) * 100 AS BIGINT), x.other_account, s.transfer_account_id
FROM findings f
CROSS JOIN store_info i
LEFT JOIN finding_items fi ON fi.finding_id = f.id
LEFT JOIN transactions t ON t.id = fi.transaction_id
LEFT JOIN accounts a ON a.id = t.account_id
LEFT JOIN payees p ON p.id = t.payee_id
LEFT JOIN splits s ON s.id = fi.split_id
LEFT JOIN transfers x ON x.from_split_id = fi.split_id
ORDER BY f.id, fi.rowid`

// Findings reads every finding, open and fixed, with its items, sorted by id; a fixed finding has no items. It
// refuses a store it cannot open or read with *store.OpenError.
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
		)
		err := scan(&id, &typ, &firstFoundAt, &fixedAt, &isNew, &newlyFixed, &hasItem, &txnID, &splitID, &payeeID, &categoryID,
			&date, &accountID, &account, &currency, &closed, &active, &payee, &amount, &otherAccount, &otherAccountID)
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
			Closed: closed.Bool, Active: active.Bool, Payee: payee.String, Amount: amount.Int64,
			OtherAccount: nullStringPtr(otherAccount), OtherAccountID: nullStringPtr(otherAccountID),
		})
		return nil
	})
	if err != nil {
		return store.FindingList{}, openFault(s.Path(), err)
	}
	return list, nil
}
