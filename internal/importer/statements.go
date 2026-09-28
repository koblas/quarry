package importer

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// investmentTypes are the accounts.type values the balance gate counts but
// never checks (P1-3): quarry imports their cash-flow transactions but has
// no register balance to compare them against.
var investmentTypes = map[string]bool{"brokerage": true, "retirement": true}

// parsedStatement is the one reconcile record the balance gate uses for a
// non-investment account: its newest non-deleted ZRECONCILERECORD, already
// parsed to cents.
type parsedStatement struct {
	Date  time.Time
	Cents int64
}

// rawReconcile is one non-deleted ZRECONCILERECORD row, before newest
// selection and balance parsing.
type rawReconcile struct {
	pk      int64
	account int64
	endDate sql.NullFloat64
	balType string
	balText sql.NullString
}

const reconcileRecordsQuery = `
SELECT Z_PK, ZACCOUNT, CAST(ZENDDATE AS REAL), typeof(ZENDINGBALANCE), CAST(ZENDINGBALANCE AS TEXT)
FROM ZRECONCILERECORD
WHERE COALESCE(ZDELETIONCOUNT, 0) = 0
`

// newestStatements reads every non-deleted ZRECONCILERECORD row and
// returns, per imported non-investment account id, the newest record by
// the gate's ranking: a NULL ZENDDATE ranks newest (it refuses with reason
// 10 rather than falling back to an older statement), else the latest
// ZENDDATE, ties broken by the higher Z_PK. A record on an investment,
// deleted or missing account is skipped silently. Only the newest-ranked
// record per account is parsed: a missing, too-precise, too-large or
// non-numeric balance is added to off (reason 5/6/11); a missing date is
// added separately (reason 10), so it is reported once any balance fault
// on the same record is fixed.
func newestStatements(ctx context.Context, src Source, accounts map[int64]accountRef, off *offenders) (map[string]parsedStatement, error) {
	newest := make(map[int64]rawReconcile)
	err := src.QueryRows(ctx, reconcileRecordsQuery, nil, func(scan func(dest ...any) error) error {
		var r rawReconcile
		if err := scan(&r.pk, &r.account, &r.endDate, &r.balType, &r.balText); err != nil {
			return err
		}
		acct, ok := accounts[r.account]
		if !ok || investmentTypes[acct.Type] {
			return nil
		}
		if cur, ok := newest[r.account]; !ok || newerReconcile(r, cur) {
			newest[r.account] = r
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read reconcile records: %w", err)
	}

	statements := make(map[string]parsedStatement, len(newest))
	for acctPK, r := range newest {
		acct := accounts[acctPK]
		if stmt, ok := parseStatement(acct, r, off); ok {
			statements[acct.ID] = stmt
		}
	}
	return statements, nil
}

// newerReconcile reports whether a ranks newer than b: a NULL end date
// ranks newest, else the later end date, ties broken by the higher Z_PK.
func newerReconcile(a, b rawReconcile) bool {
	if a.endDate.Valid != b.endDate.Valid {
		return !a.endDate.Valid
	}
	if a.endDate.Valid && a.endDate.Float64 != b.endDate.Float64 {
		return a.endDate.Float64 > b.endDate.Float64
	}
	return a.pk > b.pk
}

// parseStatement validates r's date and balance, adding an offender for
// each missing or unparseable field, and returns the parsed statement only
// when both are usable.
func parseStatement(acct accountRef, r rawReconcile, off *offenders) (parsedStatement, bool) {
	hasDate := r.endDate.Valid
	var date time.Time
	var dateStr string
	if hasDate {
		date = coreDataToDate(r.endDate.Float64)
		dateStr = date.Format(dateLayout)
	} else {
		off.add(offender{class: 10, reason: reasonStatementNoDate(acct.Name, r.pk), name: acct.Name, sourceID: r.pk})
	}

	if r.balType == "null" {
		if hasDate {
			off.add(offender{class: 10, reason: reasonStatementNoBalance(dateStr, acct.Name), dated: true, date: date, account: acct.Name, sourceID: r.pk})
		}
		return parsedStatement{}, false
	}

	cents, fault := parseMoney(r.balType, r.balText.String)
	switch fault {
	case moneyNotANumber:
		if hasDate {
			off.add(offender{class: 11, reason: reasonStatementNotANumber(dateStr, acct.Name), dated: true, date: date, account: acct.Name, sourceID: r.pk})
		} else {
			off.add(offender{class: 11, reason: reasonStatementNotANumberNoDate(acct.Name, r.pk), name: acct.Name, sourceID: r.pk})
		}
		return parsedStatement{}, false
	case moneyPrecision:
		if hasDate {
			off.add(offender{class: 5, reason: reasonStatementPrecision(dateStr, acct.Name, r.balText.String), dated: true, date: date, account: acct.Name, sourceID: r.pk})
		} else {
			off.add(offender{class: 5, reason: reasonStatementPrecisionNoDate(acct.Name, r.pk, r.balText.String), name: acct.Name, sourceID: r.pk})
		}
		return parsedStatement{}, false
	case moneyTooLarge:
		if hasDate {
			off.add(offender{class: 6, reason: reasonStatementTooLarge(dateStr, acct.Name, r.balText.String), dated: true, date: date, account: acct.Name, sourceID: r.pk})
		} else {
			off.add(offender{class: 6, reason: reasonStatementTooLargeNoDate(acct.Name, r.pk, r.balText.String), name: acct.Name, sourceID: r.pk})
		}
		return parsedStatement{}, false
	}

	if !hasDate {
		return parsedStatement{}, false
	}
	return parsedStatement{Date: date, Cents: cents}, true
}
