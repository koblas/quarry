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

// dateLayout is the refusal subject date format ("2024-03-02").
const dateLayout = "2006-01-02"

// txnRef is what mapSplits needs about an imported transaction, without
// re-reading ZTRANSACTION: its quarry id, date and account name (for
// refusal subjects).
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
       t.ZRECONCILESTATUS, t.ZUSERPAYEE, t.ZNOTE, t.ZCHECKNUMBER,
       COALESCE(t.ZEXCLUDEFROMREPORTS, 0) <> 0
FROM ZTRANSACTION t
WHERE t.Z_ENT = ? AND COALESCE(t.ZDELETIONCOUNT, 0) = 0
ORDER BY COALESCE(t.ZENTEREDDATE, t.ZPOSTEDDATE), t.ZACCOUNT, t.Z_PK
`

// mapTransactions reads every non-deleted transactionEntity row of
// ZTRANSACTION. A row with no account, or whose account is deleted,
// excluded or points to no row at all, is silently skipped. A row whose
// date or amount is missing, whose amount is stored as text or blob, has
// more than 2 decimals beyond the snap tolerance or is too large, or whose
// reconcile status is unmapped, is added to off and excluded.
// A payee reference to a deleted or missing payee stores NULL. A
// transaction is dated by its entered day (the register date), else its
// posted day; PostedDate keeps the posted day whenever there is one.
func mapTransactions(
	ctx context.Context, src Source, transactionEntity int64,
	accounts map[int64]accountRef, existingPayees map[int64]bool, off *offenders,
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
		var excluded bool
		if err := scan(&pk, &account, &posted, &entered, &amtType, &amtText, &status, &payee, &note, &cheque, &excluded); err != nil {
			return err
		}

		acct, ok := accounts[account.Int64]
		if !account.Valid || !ok {
			return nil
		}

		hasDate := posted.Valid || entered.Valid
		var date time.Time
		var dateStr string
		if hasDate {
			seconds := posted.Float64
			if entered.Valid {
				seconds = entered.Float64
			}
			date = coreDataToDate(seconds)
			dateStr = date.Format(dateLayout)
		}

		if amtType == "null" {
			if !hasDate {
				off.add(offender{class: classMissingValue, reason: reasonTransactionNoDate(acct.Name, pk), name: acct.Name, sourceID: pk})
				return nil
			}
			off.add(offender{class: classMissingValue, reason: reasonTransactionNoAmount(dateStr, acct.Name), dated: true, date: date, account: acct.Name, sourceID: pk})
			return nil
		}

		cents, fault := parseMoney(amtType, amtText.String)
		if fault == moneyNotANumber {
			if hasDate {
				off.add(offender{class: classNotANumber, reason: reasonTransactionNotANumber(dateStr, acct.Name), dated: true, date: date, account: acct.Name, sourceID: pk})
			} else {
				off.add(offender{class: classNotANumber, reason: reasonTransactionNotANumberNoDate(acct.Name, pk), name: acct.Name, sourceID: pk})
			}
			return nil
		}
		if !hasDate {
			off.add(offender{class: classMissingValue, reason: reasonTransactionNoDate(acct.Name, pk), name: acct.Name, sourceID: pk})
			return nil
		}
		switch fault {
		case moneyPrecision:
			off.add(offender{class: classTransactionPrecision, reason: reasonTransactionPrecision(dateStr, acct.Name, amtText.String), dated: true, date: date, account: acct.Name, sourceID: pk})
			return nil
		case moneyTooLarge:
			off.add(offender{class: classTooLarge, reason: reasonTransactionTooLarge(dateStr, acct.Name, amtText.String), dated: true, date: date, account: acct.Name, sourceID: pk})
			return nil
		case moneyOK, moneyNotANumber: // moneyNotANumber returned above
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
			off.add(offender{class: classTransactionStatus, reason: reasonTransactionStatus(dateStr, acct.Name, status.Int64), dated: true, date: date, account: acct.Name, sourceID: pk})
			return nil
		}

		id := fmt.Sprintf("txn-%d", pk)
		txn := store.Transaction{
			ID: id, SourceID: pk, AccountID: acct.ID, Date: date,
			Amount: cents, Currency: acct.Currency, Status: statusStr, ExcludedFromReports: excluded,
		}
		if posted.Valid {
			postedDay := coreDataToDate(posted.Float64)
			txn.PostedDate = &postedDay
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
