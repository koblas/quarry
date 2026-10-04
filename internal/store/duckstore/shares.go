package duckstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"

	"github.com/koblas/quarry/internal/store"
)

// shareTolerance is how far, in shares, a derived count may sit from Quicken's and still match.
var shareTolerance = big.NewRat(1, 1_000_000)

// sharesPerMillionth is the factor from shares to the millionths ShareMismatch reports.
var sharesPerMillionth = big.NewInt(1_000_000)

// splitAction is the action whose split sides multiply a holding's running share count.
const splitAction = "split"

// errSplitRatio is the fault behind a split row whose new or old side is missing or not positive.
var errSplitRatio = errors.New("ratio is not positive")

// holdingWalkQuery lists the investment transactions of every holding, a holding's together and in walk order.
const holdingWalkQuery = `
SELECT account_id, security_id, action, CAST(shares AS VARCHAR), CAST(split_new_shares AS VARCHAR), CAST(split_old_shares AS VARCHAR)
FROM investment_transactions
WHERE security_id IS NOT NULL
ORDER BY account_id, security_id, date, source_id
`

// holdingKey is a holding by its account and security ids.
type holdingKey struct {
	account, security string
}

// rowQuerier is the read half of a database: all the walk needs of one.
type rowQuerier interface {
	QueryRows(ctx context.Context, query string, args []any, row func(scan func(dest ...any) error) error) error
}

// CheckShares compares, for every holding, the share count derived from rows.InvestmentTransactions
// with Quicken's in rows.QuickenShares; a holding on either side is checked. The derivation runs
// on a scratch database, so nothing is written to the store.
func (s *Store) CheckShares(ctx context.Context, rows store.Rows) (store.ShareCheck, error) {
	derived, err := s.deriveShares(ctx, rows.InvestmentTransactions)
	if err != nil {
		return store.ShareCheck{}, err
	}
	return compareShares(derived, rows.QuickenShares), nil
}

// deriveShares loads txns into a scratch database built from the store's own schema and walks them.
func (s *Store) deriveShares(ctx context.Context, txns []store.InvestmentTransaction) (map[holdingKey]*big.Rat, error) {
	db, err := s.scratch(ctx)
	if err != nil {
		return nil, fmt.Errorf("create scratch database: %w", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.Exec(ctx, schemaDDL); err != nil {
		return nil, fmt.Errorf("create schema: %w", err)
	}
	invRows, err := investmentTransactionRows(txns)
	if err != nil {
		return nil, err
	}
	if err := appendTable(ctx, db, "investment_transactions", invRows); err != nil {
		return nil, err
	}
	return holdingShares(ctx, db)
}

// holdingShares returns the derived share count of every holding with an investment transaction
// that has a security. Walking a holding in date order, ties by source_id, shares add and a
// split multiplies the running count by its new over old sides and adds nothing. It is the
// single owner of that derivation.
func holdingShares(ctx context.Context, db rowQuerier) (map[holdingKey]*big.Rat, error) {
	counts := make(map[holdingKey]*big.Rat)
	err := db.QueryRows(ctx, holdingWalkQuery, nil, func(scan func(dest ...any) error) error {
		var account, security, action string
		var shares, splitNew, splitOld sql.NullString
		if err := scan(&account, &security, &action, &shares, &splitNew, &splitOld); err != nil {
			return err
		}
		key := holdingKey{account: account, security: security}
		if counts[key] == nil {
			counts[key] = new(big.Rat)
		}
		if action == splitAction {
			ratio := splitRatio(splitNew, splitOld)
			if ratio == nil {
				return fmt.Errorf("split of %s in %s: %w", security, account, errSplitRatio)
			}
			counts[key].Mul(counts[key], ratio)
			return nil
		}
		if added := decimalOf(shares); added != nil {
			counts[key].Add(counts[key], added)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk holdings: %w", err)
	}
	return counts, nil
}

// splitRatio returns newShares over oldShares, nil unless both are present and positive.
func splitRatio(newShares, oldShares sql.NullString) *big.Rat {
	numerator, denominator := decimalOf(newShares), decimalOf(oldShares)
	if numerator == nil || denominator == nil || numerator.Sign() <= 0 || denominator.Sign() <= 0 {
		return nil
	}
	return numerator.Quo(numerator, denominator)
}

// decimalOf returns the value of a DECIMAL column read as text, nil when it is NULL.
func decimalOf(col sql.NullString) *big.Rat {
	if !col.Valid {
		return nil
	}
	value, ok := new(big.Rat).SetString(col.String)
	if !ok {
		// unreachable: the column is a DECIMAL cast to VARCHAR, which DuckDB renders as plain decimal text
		return nil
	}
	return value
}

// compareShares checks every holding on either side: a missing count is 0, and counts match
// when they differ by at most shareTolerance. Mismatches come in account then security order.
func compareShares(derived map[holdingKey]*big.Rat, quicken []store.QuickenShare) store.ShareCheck {
	reference := make(map[holdingKey]*big.Rat, len(quicken))
	holdings := make(map[holdingKey]bool, len(derived)+len(quicken))
	for key := range derived {
		holdings[key] = true
	}
	for _, q := range quicken {
		key := holdingKey{account: q.AccountID, security: q.SecurityID}
		reference[key] = new(big.Rat).SetFrac(q.Millionths, sharesPerMillionth)
		holdings[key] = true
	}

	keys := make([]holdingKey, 0, len(holdings))
	for key := range holdings {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].account != keys[j].account {
			return keys[i].account < keys[j].account
		}
		return keys[i].security < keys[j].security
	})

	check := store.ShareCheck{Checked: len(keys)}
	for _, key := range keys {
		ours, theirs := zeroIfNil(derived[key]), zeroIfNil(reference[key])
		difference := new(big.Rat).Sub(ours, theirs)
		if difference.Abs(difference).Cmp(shareTolerance) > 0 {
			check.Mismatched = append(check.Mismatched, store.ShareMismatch{
				AccountID: key.account, SecurityID: key.security, Quarry: millionthsOf(ours), Quicken: millionthsOf(theirs),
			})
		}
	}
	return check
}

func zeroIfNil(r *big.Rat) *big.Rat {
	if r == nil {
		return new(big.Rat)
	}
	return r
}

var (
	minInt64 = big.NewInt(math.MinInt64)
	maxInt64 = big.NewInt(math.MaxInt64)
)

// millionthsOf returns shares in millionths, rounded half to even; a count beyond an int64 reads as the nearest one.
func millionthsOf(shares *big.Rat) int64 {
	scaled := new(big.Rat).Mul(shares, new(big.Rat).SetInt(sharesPerMillionth))
	magnitude := new(big.Rat).Abs(scaled)
	rounded, remainder := new(big.Int).QuoRem(magnitude.Num(), magnitude.Denom(), new(big.Int))
	switch remainder.Lsh(remainder, 1).Cmp(magnitude.Denom()) {
	case 1:
		rounded.Add(rounded, big.NewInt(1))
	case 0:
		if rounded.Bit(0) == 1 {
			rounded.Add(rounded, big.NewInt(1))
		}
	}
	if scaled.Sign() < 0 {
		rounded.Neg(rounded)
	}
	switch {
	case rounded.Cmp(minInt64) < 0:
		return math.MinInt64
	case rounded.Cmp(maxInt64) > 0:
		return math.MaxInt64
	}
	return rounded.Int64()
}
