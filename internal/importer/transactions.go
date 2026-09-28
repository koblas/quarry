package importer

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/koblas/quarry/internal/store"
)

// coreDataEpoch is Core Data's reference date: a v9 TIMESTAMP column stores
// seconds relative to this instant, not the Unix epoch.
var coreDataEpoch = time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)

// coreDataToDate converts a v9 TIMESTAMP value (seconds since
// coreDataEpoch) to its UTC calendar date: quarry's date columns hold no
// time of day.
func coreDataToDate(seconds float64) time.Time {
	t := coreDataEpoch.Add(time.Duration(seconds * float64(time.Second)))
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// dateLayout is the S4 subject date format ("2024-03-02").
const dateLayout = "2006-01-02"

// txnRef is what mapSplits needs about an imported transaction, without
// re-reading ZTRANSACTION: its quarry id, date and account name (for S4
// subjects).
type txnRef struct {
	ID          string
	Date        time.Time
	AccountName string
}

// ZPOSTEDDATE/ZENTEREDDATE are cast to REAL: their declared type is
// TIMESTAMP, and the sqlite3 driver auto-converts a TIMESTAMP column to a
// Unix-epoch time.Time unless the query's own result type says otherwise.
// quarry needs the raw Core Data epoch-seconds value, so REAL is forced.
const transactionsQuery = `
SELECT t.Z_PK, t.ZACCOUNT, CAST(t.ZPOSTEDDATE AS REAL), CAST(t.ZENTEREDDATE AS REAL),
       typeof(t.ZAMOUNT), CAST(t.ZAMOUNT AS TEXT),
       t.ZRECONCILESTATUS, t.ZUSERPAYEE, t.ZNOTE, t.ZCHECKNUMBER
FROM ZTRANSACTION t
WHERE t.Z_ENT = ? AND COALESCE(t.ZDELETIONCOUNT, 0) = 0
ORDER BY COALESCE(t.ZPOSTEDDATE, t.ZENTEREDDATE), t.ZACCOUNT, t.Z_PK
`

// existingTransactionPKs reads every ZTRANSACTION row's Z_PK, of any
// entity and deletion state, so a split's dangling parent reference (no
// row at all) can be told apart from one pointing at a row this importer
// excludes for its own reason (deleted, Smart/Investment, or otherwise).
func existingTransactionPKs(ctx context.Context, src Source) (map[int64]bool, error) {
	existing := make(map[int64]bool)
	err := src.QueryRows(ctx, "SELECT Z_PK FROM ZTRANSACTION", nil, func(scan func(dest ...any) error) error {
		var pk int64
		if err := scan(&pk); err != nil {
			return err
		}
		existing[pk] = true
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read transaction ids: %w", err)
	}
	return existing, nil
}

// mapTransactions reads every non-deleted transactionEntity row of
// ZTRANSACTION. A row in a deleted account, or whose account was itself
// excluded, is silently skipped. A row whose account reference points to
// no row at all (S4 reason 10), whose date is missing (reason 10), whose
// amount is missing (reason 10), stored as text or blob (reason 11), has
// too much precision (reason 3) or is too large (reason 6), or whose
// reconcile status is unmapped (reason 8), is added to off and excluded.
// A payee reference to a deleted or missing payee stores NULL.
func mapTransactions(
	ctx context.Context, src Source, transactionEntity int64,
	accounts map[int64]accountRef, existingAccounts, existingPayees map[int64]bool, off *offenders,
) ([]store.Transaction, map[int64]txnRef, error) {
	var rows []store.Transaction
	refs := make(map[int64]txnRef)

	err := src.QueryRows(ctx, transactionsQuery, []any{transactionEntity}, func(scan func(dest ...any) error) error {
		var pk int64
		var account sql.NullInt64
		var posted, entered sql.NullFloat64
		var amtType string
		var amtText sql.NullString
		var status sql.NullInt64
		var payee sql.NullInt64
		var note, cheque sql.NullString
		if err := scan(&pk, &account, &posted, &entered, &amtType, &amtText, &status, &payee, &note, &cheque); err != nil {
			return err
		}

		if !account.Valid {
			off.add(offender{class: 10, reason: reasonTransactionNoAccount(pk), name: fmt.Sprintf("(source id %d)", pk), sourceID: pk})
			return nil
		}
		if !existingAccounts[account.Int64] {
			off.add(offender{class: 10, reason: reasonTransactionNoAccount(pk), name: fmt.Sprintf("(source id %d)", pk), sourceID: pk})
			return nil
		}
		acct, ok := accounts[account.Int64]
		if !ok {
			return nil // account was deleted, or itself excluded (P1-5d)
		}

		hasDate := posted.Valid || entered.Valid
		var date time.Time
		var dateStr string
		if hasDate {
			seconds := entered.Float64
			if posted.Valid {
				seconds = posted.Float64
			}
			date = coreDataToDate(seconds)
			dateStr = date.Format(dateLayout)
		}

		if amtType == "null" {
			if !hasDate {
				off.add(offender{class: 10, reason: reasonTransactionNoDate(acct.Name, pk), name: acct.Name, sourceID: pk})
				return nil
			}
			off.add(offender{class: 10, reason: reasonTransactionNoAmount(dateStr, acct.Name), dated: true, date: date, account: acct.Name, sourceID: pk})
			return nil
		}

		cents, fault := parseMoney(amtType, amtText.String)
		if fault == moneyNotANumber {
			if hasDate {
				off.add(offender{class: 11, reason: reasonTransactionNotANumber(dateStr, acct.Name), dated: true, date: date, account: acct.Name, sourceID: pk})
			} else {
				off.add(offender{class: 11, reason: reasonTransactionNotANumberNoDate(acct.Name, pk), name: acct.Name, sourceID: pk})
			}
			return nil
		}
		if !hasDate {
			off.add(offender{class: 10, reason: reasonTransactionNoDate(acct.Name, pk), name: acct.Name, sourceID: pk})
			return nil
		}
		switch fault {
		case moneyPrecision:
			off.add(offender{class: 3, reason: reasonTransactionPrecision(dateStr, acct.Name, amtText.String), dated: true, date: date, account: acct.Name, sourceID: pk})
			return nil
		case moneyTooLarge:
			off.add(offender{class: 6, reason: reasonTransactionTooLarge(dateStr, acct.Name, amtText.String), dated: true, date: date, account: acct.Name, sourceID: pk})
			return nil
		}

		var statusStr string
		switch {
		case !status.Valid || status.Int64 == 0:
			statusStr = "uncleared"
		case status.Int64 == 1:
			statusStr = "cleared"
		case status.Int64 == 2:
			statusStr = "reconciled"
		default:
			off.add(offender{class: 8, reason: reasonTransactionStatus(dateStr, acct.Name, status.Int64), dated: true, date: date, account: acct.Name, sourceID: pk})
			return nil
		}

		id := fmt.Sprintf("txn-%d", pk)
		txn := store.Transaction{
			ID: id, SourceID: pk, AccountID: acct.ID, Date: date,
			Amount: cents, Currency: acct.Currency, Status: statusStr,
		}
		if payee.Valid && existingPayees[payee.Int64] {
			pid := fmt.Sprintf("payee-%d", payee.Int64)
			txn.PayeeID = &pid
		}
		if note.Valid && note.String != "" {
			memo := note.String
			txn.Memo = &memo
		}
		if cheque.Valid && cheque.String != "" {
			num := cheque.String
			txn.ChequeNumber = &num
		}
		rows = append(rows, txn)
		refs[pk] = txnRef{ID: id, Date: date, AccountName: acct.Name}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("read transactions: %w", err)
	}
	return rows, refs, nil
}
