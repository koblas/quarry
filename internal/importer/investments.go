package importer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/koblas/quarry/internal/store"
)

// investmentIDFormat renders an investment transaction's quarry id from its ZTRANSACTION.Z_PK.
const investmentIDFormat = "itxn-%d"

// splitAction is the action whose ZNUMERATOR and ZDENOMINATOR become the split columns.
const splitAction = "split"

// investmentActions maps ZTRANSACTION.ZTYPE to investment_transactions.action; any other code is unmappable.
var investmentActions = map[int64]string{
	2:  "add_shares",
	3:  "buy",
	6:  "margin_interest",
	7:  "misc_expense",
	8:  "capital_gain_long",
	9:  "capital_gain_short",
	10: "dividend",
	11: "interest",
	12: "misc_income",
	15: "reinvest_dividend",
	17: "remove_shares",
	19: "sell",
	23: splitAction,
}

// positionsQuery reads each non-deleted position of one entity that has an account and a security.
const positionsQuery = `
SELECT Z_PK, ZACCOUNT, ZSECURITY
FROM ZPOSITION
WHERE Z_ENT = ? AND COALESCE(ZDELETIONCOUNT, 0) = 0 AND ZACCOUNT IS NOT NULL AND ZSECURITY IS NOT NULL
ORDER BY Z_PK
`

// investmentsQuery reads each non-deleted investment transaction of one entity. Dates are cast to REAL
// as in transactionsQuery; numbers come as typeof() and text so each is parsed exactly.
const investmentsQuery = `
SELECT t.Z_PK, t.ZACCOUNT, CAST(t.ZPOSTEDDATE AS REAL), CAST(t.ZENTEREDDATE AS REAL), t.ZTYPE, t.ZPOSITION, t.ZNOTE,
       typeof(t.ZUNITS), CAST(t.ZUNITS AS TEXT), typeof(t.ZAMOUNT), CAST(t.ZAMOUNT AS TEXT),
       typeof(t.ZCOMMISSION), CAST(t.ZCOMMISSION AS TEXT),
       typeof(t.ZNUMERATOR), CAST(t.ZNUMERATOR AS TEXT), typeof(t.ZDENOMINATOR), CAST(t.ZDENOMINATOR AS TEXT)
FROM ZTRANSACTION t
WHERE t.Z_ENT = ? AND COALESCE(t.ZDELETIONCOUNT, 0) = 0
ORDER BY t.Z_PK
`

// positionRef is the account and security (both source Z_PKs) of one non-deleted position.
type positionRef struct {
	Account  int64
	Security int64
}

// mapPositions reads the non-deleted positions of positionEnt that sit in an imported account,
// by source Z_PK; with no entity in the snapshot it returns nil.
func mapPositions(
	ctx context.Context, src Source, positionEnt int64, hasEntity bool, accounts map[int64]accountRef,
) (map[int64]positionRef, error) {
	positions := make(map[int64]positionRef)
	if !hasEntity {
		return positions, nil
	}
	err := src.QueryRows(ctx, positionsQuery, []any{positionEnt}, func(scan func(dest ...any) error) error {
		var pk, account, security int64
		if err := scan(&pk, &account, &security); err != nil {
			return err
		}
		if _, ok := accounts[account]; ok {
			positions[pk] = positionRef{Account: account, Security: security}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read positions: %w", err)
	}
	return positions, nil
}

// numberColumn is one numeric column as the query returns it: its typeof() and its text.
type numberColumn struct {
	typ  string
	text sql.NullString
}

// investmentRow is one scanned ZTRANSACTION row of the investment entity.
type investmentRow struct {
	pk                        int64
	account, position         sql.NullInt64
	posted, entered           sql.NullFloat64
	code                      sql.NullInt64
	note                      sql.NullString
	units, amount, commission numberColumn
	numerator, denominator    numberColumn
}

// mapInvestmentTransactions reads the non-deleted investment transactions of investmentEnt in an imported
// account, dated by posted day else entered day. A row with no date, no action code or an unmapped one is
// added to off and excluded; its security is that of its position when the security was imported, else NULL.
func mapInvestmentTransactions(
	ctx context.Context, src Source, investmentEnt int64, hasEntity bool,
	accounts map[int64]accountRef, positions map[int64]positionRef, securities map[int64]store.Security, off *offenders,
) ([]store.InvestmentTransaction, error) {
	if !hasEntity {
		return nil, nil
	}
	var rows []store.InvestmentTransaction
	err := src.QueryRows(ctx, investmentsQuery, []any{investmentEnt}, func(scan func(dest ...any) error) error {
		var r investmentRow
		if err := scan(&r.pk, &r.account, &r.posted, &r.entered, &r.code, &r.position, &r.note,
			&r.units.typ, &r.units.text, &r.amount.typ, &r.amount.text, &r.commission.typ, &r.commission.text,
			&r.numerator.typ, &r.numerator.text, &r.denominator.typ, &r.denominator.text); err != nil {
			return err
		}
		acct, ok := accounts[r.account.Int64]
		if !r.account.Valid || !ok {
			return nil
		}
		txn, ok, err := buildInvestmentTransaction(r, acct, positions, securities, off)
		if err != nil {
			return err
		}
		if ok {
			rows = append(rows, txn)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read investment transactions: %w", err)
	}
	return rows, nil
}

// buildInvestmentTransaction maps one row of an imported account, reporting false
// after adding an offender when the row cannot be mapped.
func buildInvestmentTransaction(
	r investmentRow, acct accountRef, positions map[int64]positionRef, securities map[int64]store.Security, off *offenders,
) (store.InvestmentTransaction, bool, error) {
	if !r.posted.Valid && !r.entered.Valid {
		off.add(offender{class: classMissingValue, reason: reasonInvestmentNoDate(acct.Name, r.pk), name: acct.Name, sourceID: r.pk})
		return store.InvestmentTransaction{}, false, nil
	}
	seconds := r.entered.Float64
	if r.posted.Valid {
		seconds = r.posted.Float64
	}
	date := coreDataToDate(seconds)
	dateStr := date.Format(dateLayout)

	if !r.code.Valid {
		off.add(offender{class: classMissingValue, reason: reasonInvestmentNoActionCode(dateStr, acct.Name), dated: true, date: date, account: acct.Name, sourceID: r.pk})
		return store.InvestmentTransaction{}, false, nil
	}
	action, ok := investmentActions[r.code.Int64]
	if !ok {
		off.add(offender{class: classTransactionStatus, reason: reasonInvestmentActionCode(dateStr, acct.Name, r.code.Int64), dated: true, date: date, account: acct.Name, sourceID: r.pk})
		return store.InvestmentTransaction{}, false, nil
	}

	txn := store.InvestmentTransaction{
		ID: fmt.Sprintf(investmentIDFormat, r.pk), SourceID: r.pk, AccountID: acct.ID,
		Date: date, Action: action, Currency: acct.Currency,
	}
	shares, err := shareColumn(r.pk, "shares", r.units)
	if err != nil {
		return txn, false, err
	}
	txn.Shares = nullableInt(shares)
	if r.amount.typ == "null" {
		return txn, false, unreadableInvestmentValue(r.pk, "amount")
	}
	cents, fault := parseMoney(r.amount.typ, r.amount.text.String)
	if fault != moneyOK {
		return txn, false, unreadableInvestmentValue(r.pk, "amount")
	}
	txn.Amount = cents
	commission, err := commissionColumn(r.pk, r.commission)
	if err != nil {
		return txn, false, err
	}
	txn.Commission = nullableInt(commission)
	if r.note.Valid && r.note.String != "" {
		txn.Memo = &r.note.String
	}
	if pos, ok := positions[r.position.Int64]; ok && r.position.Valid {
		if sec, ok := securities[pos.Security]; ok {
			txn.SecurityID = &sec.ID
		}
	}
	if action == splitAction {
		newShares, err := shareColumn(r.pk, "split numerator", r.numerator)
		if err != nil {
			return txn, false, err
		}
		oldShares, err := shareColumn(r.pk, "split denominator", r.denominator)
		if err != nil {
			return txn, false, err
		}
		txn.SplitNewShares, txn.SplitOldShares = nullableInt(newShares), nullableInt(oldShares)
	}
	return txn, true, nil
}

// shareColumn returns the millionths of a share column; invalid when the column is NULL.
func shareColumn(pk int64, name string, col numberColumn) (sql.NullInt64, error) {
	if col.typ == "null" {
		return sql.NullInt64{}, nil
	}
	millionths, fault := parsePrice(col.typ, col.text.String)
	if fault != moneyOK {
		return sql.NullInt64{}, unreadableInvestmentValue(pk, name)
	}
	return sql.NullInt64{Int64: millionths, Valid: true}, nil
}

// commissionColumn returns the cents of a commission column; invalid when it is NULL or zero.
func commissionColumn(pk int64, col numberColumn) (sql.NullInt64, error) {
	if col.typ == "null" {
		return sql.NullInt64{}, nil
	}
	cents, fault := parseMoney(col.typ, col.text.String)
	if fault != moneyOK {
		return sql.NullInt64{}, unreadableInvestmentValue(pk, "commission")
	}
	return sql.NullInt64{Int64: cents, Valid: cents != 0}, nil
}

// nullableInt returns a pointer to n's value, nil when n is NULL.
func nullableInt(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	return &n.Int64
}

// errUnreadableInvestmentValue stands in for the ruled refusal of a share, amount or commission quarry cannot read.
var errUnreadableInvestmentValue = errors.New("an investment value quarry cannot read")

func unreadableInvestmentValue(pk int64, name string) error {
	return fmt.Errorf("investment transaction (source id %d) %s: %w", pk, name, errUnreadableInvestmentValue)
}
