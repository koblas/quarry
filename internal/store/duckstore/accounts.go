package duckstore

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/koblas/quarry/internal/store"
)

// accountsQuery reads today's date and every account's balance in cents; as_of survives zero accounts.
const accountsQuery = `
SELECT d.as_of, v.id, v.source_id, v.name, v.type, v.currency, v.institution, v.closed, v.active,
	CAST(v.balance * 100 AS BIGINT)
FROM (SELECT current_date AS as_of) d
LEFT JOIN v_account_balances v ON true
ORDER BY lower(v.name), v.name, v.source_id`

// Accounts reads every account, closed ones included, with its balance
// and the store's today as AsOf, sorted by name ignoring case, then name,
// then source id. It fails when the store cannot be opened or read.
func (s *Store) Accounts(ctx context.Context) (store.AccountList, error) {
	db, err := s.openRead(ctx)
	if err != nil {
		return store.AccountList{}, fmt.Errorf("read accounts: %w", err)
	}
	defer func() { _ = db.Close() }()

	var list store.AccountList
	err = db.QueryRows(ctx, accountsQuery, nil, func(scan func(dest ...any) error) error {
		var asOf time.Time
		var id, name, typ, currency, institution sql.NullString
		var sourceID, balance sql.NullInt64
		var closed, active sql.NullBool
		if err := scan(&asOf, &id, &sourceID, &name, &typ, &currency, &institution, &closed, &active, &balance); err != nil {
			return err
		}
		list.AsOf = asOf
		if !id.Valid {
			return nil
		}
		acct := store.AccountBalance{
			ID: id.String, SourceID: sourceID.Int64, Name: name.String, Type: typ.String, Currency: currency.String,
			Institution: nullStringPtr(institution), Closed: closed.Bool, Active: active.Bool,
		}
		if balance.Valid {
			acct.Balance = &balance.Int64
		}
		list.Accounts = append(list.Accounts, acct)
		return nil
	})
	if err != nil {
		return store.AccountList{}, fmt.Errorf("read accounts: %w", err)
	}
	return list, nil
}

// nullStringPtr returns s's value, or nil when s is NULL.
func nullStringPtr(s sql.NullString) *string {
	if !s.Valid {
		return nil
	}
	return &s.String
}
