package importer

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"
	"sort"

	"github.com/koblas/quarry/internal/store"
)

// lotsQuery reads each non-deleted lot of one entity; its units come as typeof() and text.
const lotsQuery = `
SELECT Z_PK, ZPOSITION, typeof(ZLATESTUNITS), CAST(ZLATESTUNITS AS TEXT)
FROM ZLOT
WHERE Z_ENT = ? AND COALESCE(ZDELETIONCOUNT, 0) = 0
ORDER BY Z_PK
`

// holdingKey is a holding by its quarry account and security ids.
type holdingKey struct {
	account, security string
}

// mapLots sums each holding's lot units over the positions mapPositions kept, for imported, named securities.
// A lot whose units quarry cannot read goes to off; with no Lot entity it returns nil.
func mapLots(
	ctx context.Context, src Source, lotEnt int64, hasEntity bool,
	positions map[int64]positionRef, accounts map[int64]accountRef, securities map[int64]store.Security, off *offenders,
) ([]store.QuickenShare, error) {
	if !hasEntity {
		return nil, nil
	}
	totals := make(map[holdingKey]*big.Int)
	err := src.QueryRows(ctx, lotsQuery, []any{lotEnt}, func(scan func(dest ...any) error) error {
		var pk int64
		var position sql.NullInt64
		var units numberColumn
		if err := scan(&pk, &position, &units.typ, &units.text); err != nil {
			return err
		}
		pos, ok := positions[position.Int64]
		if !position.Valid || !ok {
			return nil
		}
		// a nameless security refuses the import; its lots could only be named ""
		sec, ok := securities[pos.Security]
		if !ok || sec.Name == "" {
			return nil
		}
		account := accounts[pos.Account]
		millionths, ok := lotUnits(units, pk, sec.Name, account.Name, off)
		if !ok {
			return nil
		}
		key := holdingKey{account: account.ID, security: sec.ID}
		if totals[key] == nil {
			totals[key] = new(big.Int)
		}
		totals[key].Add(totals[key], big.NewInt(millionths))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read lots: %w", err)
	}

	shares := make([]store.QuickenShare, 0, len(totals))
	for key, millionths := range totals {
		shares = append(shares, store.QuickenShare{AccountID: key.account, SecurityID: key.security, Millionths: millionths})
	}
	sort.Slice(shares, func(i, j int) bool {
		if shares[i].AccountID != shares[j].AccountID {
			return shares[i].AccountID < shares[j].AccountID
		}
		return shares[i].SecurityID < shares[j].SecurityID
	})
	return shares, nil
}

// lotUnits returns the millionths of a lot's units column, reporting false after adding an
// offender when quarry cannot read it. NULL units are a refusal, never 0.
func lotUnits(units numberColumn, pk int64, security, account string, off *offenders) (int64, bool) {
	refuse := func(class unmappableClass, reason string) {
		off.add(offender{class: class, reason: reason, name: security, sourceID: pk})
	}
	if units.typ == "null" {
		refuse(classMissingValue, reasonLotNoShareCount(security, account))
		return 0, false
	}
	text := units.text.String
	millionths, fault := parseShares(units.typ, text)
	switch fault {
	case moneyOK:
		return millionths, true
	case moneyPrecision:
		refuse(classTransactionPrecision, reasonLotSharesPrecision(security, account, text))
	case moneyTooLarge:
		refuse(classTooLarge, reasonLotSharesTooLarge(security, account, text))
	case moneyNotANumber:
		refuse(classNotANumber, reasonLotNotANumber(security, account))
	}
	return 0, false
}

// requireLots adds the refusal of imported investment transactions when the snapshot has no Lot
// entity to check their share counts against.
func requireLots(hasEntity bool, investments []store.InvestmentTransaction, off *offenders) {
	if hasEntity || len(investments) == 0 {
		return
	}
	off.add(offender{class: classMissingEntity, reason: reasonNoLotEntity})
}
