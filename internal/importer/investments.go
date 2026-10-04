package importer

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/koblas/quarry/internal/store"
)

// investmentIDFormat renders an investment transaction's quarry id from its ZTRANSACTION.Z_PK.
const investmentIDFormat = "itxn-%d"

// ratioSideNone and ratioSideBlob are how a refusal shows a NULL and a blob split side.
const (
	ratioSideNone = "none"
	ratioSideBlob = "blob"
)

// investmentActions maps ZTRANSACTION.ZTYPE to investment_transactions.action; any other code is unmappable.
var investmentActions = map[int64]string{
	2:  store.ActionAddShares,
	3:  store.ActionBuy,
	6:  store.ActionMarginInterest,
	7:  store.ActionMiscExpense,
	8:  store.ActionCapitalGainLong,
	9:  store.ActionCapitalGainShort,
	10: store.ActionDividend,
	11: store.ActionInterest,
	12: store.ActionMiscIncome,
	15: store.ActionReinvestDividend,
	17: store.ActionRemoveShares,
	19: store.ActionSell,
	23: store.ActionSplit,
}

// positionsQuery reads each non-deleted position of one entity that has an account and a security.
const positionsQuery = `
SELECT Z_PK, ZACCOUNT, ZSECURITY
FROM ZPOSITION
WHERE Z_ENT = ? AND COALESCE(ZDELETIONCOUNT, 0) = 0 AND ZACCOUNT IS NOT NULL AND ZSECURITY IS NOT NULL
ORDER BY Z_PK
`

// investmentsQuery reads each non-deleted investment transaction of one entity; numbers come as typeof() and text.
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
// by source Z_PK; with no entity in the snapshot it returns an empty map.
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

// mapInvestmentTransactions reads the investment transactions of investmentEnt in an imported account, dated posted
// else entered; a row quarry cannot read goes to off. Its security is its position's, when imported.
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
		if txn, ok := buildInvestmentTransaction(r, acct, positions, securities, off); ok {
			rows = append(rows, txn)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read investment transactions: %w", err)
	}
	return rows, nil
}

// investmentSubject is one dated row of an imported account, for adding its offender.
type investmentSubject struct {
	row     investmentRow
	account string
	date    time.Time
	off     *offenders
}

func (s investmentSubject) refuse(class unmappableClass, reason string) {
	s.off.add(offender{class: class, reason: reason, dated: true, date: s.date, account: s.account, sourceID: s.row.pk})
}

func (s investmentSubject) day() string { return s.date.Format(dateLayout) }

// buildInvestmentTransaction maps one row of an imported account, reporting false after adding an
// offender when quarry cannot read it. An undated row is refused before any other fault.
func buildInvestmentTransaction(
	r investmentRow, acct accountRef, positions map[int64]positionRef, securities map[int64]store.Security, off *offenders,
) (store.InvestmentTransaction, bool) {
	if !r.posted.Valid && !r.entered.Valid {
		off.add(offender{class: classMissingValue, reason: reasonInvestmentNoDate(acct.Name, r.pk), name: acct.Name, sourceID: r.pk})
		return store.InvestmentTransaction{}, false
	}
	seconds := r.entered.Float64
	if r.posted.Valid {
		seconds = r.posted.Float64
	}
	s := investmentSubject{row: r, account: acct.Name, date: coreDataToDate(seconds), off: off}

	if !r.code.Valid {
		s.refuse(classMissingValue, reasonInvestmentNoActionCode(s.day(), acct.Name))
		return store.InvestmentTransaction{}, false
	}
	action, ok := investmentActions[r.code.Int64]
	if !ok {
		s.refuse(classTransactionStatus, reasonInvestmentActionCode(s.day(), acct.Name, r.code.Int64))
		return store.InvestmentTransaction{}, false
	}

	txn := store.InvestmentTransaction{
		ID: fmt.Sprintf(investmentIDFormat, r.pk), SourceID: r.pk, AccountID: acct.ID,
		Date: s.date, Action: action, Currency: acct.Currency,
	}
	if !s.readValues(&txn) {
		return txn, false
	}
	if r.note.Valid && r.note.String != "" {
		txn.Memo = &r.note.String
	}
	sec, hasSecurity := resolveSecurity(r, positions, securities)
	if hasSecurity {
		txn.SecurityID = &sec.ID
	}
	if !hasSecurity && txn.Shares != nil && *txn.Shares != 0 {
		s.refuse(classMissingValue, reasonInvestmentSharesWithoutSecurity(s.day(), acct.Name))
		return txn, false
	}
	if action == store.ActionSplit {
		newShares, oldShares, ok := splitSides(r)
		if !ok {
			s.refuse(classMissingValue, reasonSplitRatio(s.day(), acct.Name, sec.Name, ratioSideText(r.numerator), ratioSideText(r.denominator)))
			return txn, false
		}
		txn.SplitNewShares, txn.SplitOldShares = &newShares, &oldShares
	}
	return txn, true
}

// resolveSecurity returns the imported security of r's position; false when r has no position or
// its position or security is not imported.
func resolveSecurity(r investmentRow, positions map[int64]positionRef, securities map[int64]store.Security) (store.Security, bool) {
	pos, ok := positions[r.position.Int64]
	if !ok || !r.position.Valid {
		return store.Security{}, false
	}
	sec, ok := securities[pos.Security]
	return sec, ok
}

// readValues sets txn's shares, amount and commission, reporting false after adding an offender
// for the first one quarry cannot read.
func (s investmentSubject) readValues(txn *store.InvestmentTransaction) bool {
	shares, ok := s.shares()
	if !ok {
		return false
	}
	txn.Shares = nullableInt(shares)

	if s.row.amount.typ == "null" {
		s.refuse(classMissingValue, reasonInvestmentNoAmount(s.day(), s.account))
		return false
	}
	amount, ok := s.money(s.row.amount, "an amount", amountColumn)
	if !ok {
		return false
	}
	txn.Amount = amount

	if s.row.commission.typ == "null" {
		return true
	}
	commission, ok := s.money(s.row.commission, "a commission", commissionColumn)
	if !ok {
		return false
	}
	if commission != 0 {
		txn.Commission = &commission
	}
	return true
}

// shares returns the millionths of the units column; invalid when it is NULL.
func (s investmentSubject) shares() (sql.NullInt64, bool) {
	col := s.row.units
	if col.typ == "null" {
		return sql.NullInt64{}, true
	}
	text := col.text.String
	millionths, fault := parseShares(col.typ, text)
	switch fault {
	case moneyOK:
		return sql.NullInt64{Int64: millionths, Valid: true}, true
	case moneyPrecision:
		s.refuse(classTransactionPrecision, reasonInvestmentSharesPrecision(s.day(), s.account, text))
	case moneyTooLarge:
		s.refuse(classTooLarge, reasonInvestmentSharesTooLarge(s.day(), s.account, text))
	case moneyNotANumber:
		s.refuse(classNotANumber, reasonInvestmentNotANumber(s.day(), s.account, "a share count"))
	}
	return sql.NullInt64{}, false
}

// moneyColumn is how an investment money column is read: its parser and its decimal places.
type moneyColumn struct {
	parse  func(typ, text string) (int64, moneyFault)
	places int
}

var (
	amountColumn     = moneyColumn{parse: parseMoney, places: 2}
	commissionColumn = moneyColumn{parse: parseCommission, places: 4}
)

// money returns the units (cents for an amount, ten-thousandths for a commission) of a non-NULL
// money column, named what in a refusal.
func (s investmentSubject) money(col numberColumn, what string, kind moneyColumn) (int64, bool) {
	text := col.text.String
	units, fault := kind.parse(col.typ, text)
	switch fault {
	case moneyOK:
		return units, true
	case moneyPrecision:
		s.refuse(classTransactionPrecision, reasonInvestmentMoneyPrecision(s.day(), s.account, what, text, kind.places))
	case moneyTooLarge:
		s.refuse(classTooLarge, reasonInvestmentMoneyTooLarge(s.day(), s.account, what, text))
	case moneyNotANumber:
		s.refuse(classNotANumber, reasonInvestmentNotANumber(s.day(), s.account, what))
	}
	return 0, false
}

// splitSides returns the millionths of a split row's numerator and denominator; false when either is
// NULL, zero, negative or unreadable.
func splitSides(r investmentRow) (int64, int64, bool) {
	newShares, ok := splitSide(r.numerator)
	if !ok {
		return 0, 0, false
	}
	oldShares, ok := splitSide(r.denominator)
	return newShares, oldShares, ok
}

// ratioSideText is one side of a split ratio as a refusal shows it: its column text, "none" when NULL
// and "blob" when stored as bytes, which never reach the message.
func ratioSideText(col numberColumn) string {
	switch col.typ {
	case "null":
		return ratioSideNone
	case "blob":
		return ratioSideBlob
	}
	return col.text.String
}

// splitSide returns the millionths of one split ratio side; false unless it is a positive number.
func splitSide(col numberColumn) (int64, bool) {
	if col.typ == "null" {
		return 0, false
	}
	millionths, fault := parseShares(col.typ, col.text.String)
	return millionths, fault == moneyOK && millionths > 0
}

// nullableInt returns a pointer to n's value, nil when n is NULL.
func nullableInt(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	return &n.Int64
}
