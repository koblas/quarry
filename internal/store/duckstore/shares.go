package duckstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"time"

	"github.com/koblas/quarry/internal/store"
)

// shareTolerance is how far, in shares, a derived count may sit from Quicken's and still match.
var shareTolerance = big.NewRat(1, 1_000_000)

// sharesPerMillionth is the factor from shares to the millionths ShareMismatch reports.
var sharesPerMillionth = big.NewInt(1_000_000)

// errSplitRatio is the fault behind a split row whose new or old side is missing or not positive.
var errSplitRatio = errors.New("ratio is not positive")

// errNotDecimal is the fault behind DECIMAL column text that is not a decimal number.
var errNotDecimal = errors.New("not a decimal number")

// holdingWalkQuery lists the investment transactions of every holding, a holding's together and in walk order.
const holdingWalkQuery = `
SELECT account_id, security_id, date, action, CAST(shares AS VARCHAR), CAST(split_new_shares AS VARCHAR), CAST(split_old_shares AS VARCHAR)
FROM investment_transactions
WHERE security_id IS NOT NULL
ORDER BY account_id, security_id, date, source_id
`

// holdingKey is a holding by its account and security ids.
type holdingKey struct {
	account, security string
}

// shareSpan is the stretch of days a holding's rounded share count stays at millionths;
// open is true for the span that has no end.
type shareSpan struct {
	from, to   time.Time
	open       bool
	millionths int64
}

// holdingWalk is one holding's exact final count and the spans of its rounded count.
type holdingWalk struct {
	final *big.Rat
	spans []shareSpan
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
	walks, err := holdingSpans(ctx, db)
	if err != nil {
		return nil, err
	}
	finals := make(map[holdingKey]*big.Rat, len(walks))
	for key, walk := range walks {
		finals[key] = walk.final
	}
	return finals, nil
}

// holdingSpans walks every holding with an investment transaction that has a security, returning its
// exact final count and its spans. It is the single owner of that derivation.
func holdingSpans(ctx context.Context, db rowQuerier) (map[holdingKey]*holdingWalk, error) {
	walks := make(map[holdingKey]*holdingWalk)
	var current *holdingWalker
	err := db.QueryRows(ctx, holdingWalkQuery, nil, func(scan func(dest ...any) error) error {
		var account, security, action string
		var date time.Time
		var shares, splitNew, splitOld sql.NullString
		if err := scan(&account, &security, &date, &action, &shares, &splitNew, &splitOld); err != nil {
			return err
		}
		key := holdingKey{account: account, security: security}
		if current == nil || current.key != key {
			if current != nil {
				current.endDay()
			}
			current = &holdingWalker{key: key, walk: &holdingWalk{final: new(big.Rat)}}
			walks[key] = current.walk
		}
		current.endDayBefore(date)
		if action == store.ActionSplit {
			// a split scales the running count by new over old and adds nothing
			ratio, err := splitRatio(splitNew, splitOld)
			if err != nil {
				return fmt.Errorf("split of %s in %s: %w", security, account, err)
			}
			current.walk.final.Mul(current.walk.final, ratio)
			return nil
		}
		if !shares.Valid {
			return nil
		}
		added, err := parseDecimal(shares.String)
		if err != nil {
			return err
		}
		current.walk.final.Add(current.walk.final, added)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk holdings: %w", err)
	}
	if current != nil {
		current.endDay()
	}
	return walks, nil
}

// loadHoldingShares walks the investment transactions already loaded into db and appends every
// holding's spans to holding_shares.
func loadHoldingShares(ctx context.Context, db DB) error {
	walks, err := holdingSpans(ctx, db)
	if err != nil {
		return err
	}
	keys := make([]holdingKey, 0, len(walks))
	for key := range walks {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].account != keys[j].account {
			return keys[i].account < keys[j].account
		}
		return keys[i].security < keys[j].security
	})
	var rows [][]any
	for _, key := range keys {
		for _, span := range walks[key].spans {
			shares, err := decimalCell("shares", &span.millionths, sharesWidth, sharesScale)
			if err != nil {
				return fmt.Errorf("holding_shares %s in %s: %w", key.security, key.account, err)
			}
			rows = append(rows, []any{key.account, key.security, span.from, nullableSpanEnd(span), shares})
		}
	}
	return appendTable(ctx, db, "holding_shares", rows)
}

// nullableSpanEnd is the to_date cell of span: NULL for the open span.
func nullableSpanEnd(span shareSpan) any {
	if span.open {
		return nil
	}
	return span.to
}

// holdingWalker turns one holding's running count into spans: the count is rounded to millionths
// at the end of each date, and a changed rounded count closes the open span and opens the next.
type holdingWalker struct {
	key        holdingKey
	walk       *holdingWalk
	day        time.Time // the date whose rows are being folded in; zero before the first row
	millionths int64     // the rounded count at the end of the last ended date
}

// endDayBefore ends the pending date when date is a later one, then makes date the pending one.
func (w *holdingWalker) endDayBefore(date time.Time) {
	if !w.day.IsZero() && !date.Equal(w.day) {
		w.endDay()
	}
	w.day = date
}

// endDay closes the pending date: a rounded count unlike the last one ends the open span the day before
// and, unless it is zero, opens the next.
func (w *holdingWalker) endDay() {
	rounded := millionthsOf(w.walk.final)
	if rounded == w.millionths {
		return
	}
	if w.millionths != 0 {
		last := &w.walk.spans[len(w.walk.spans)-1]
		last.to, last.open = w.day.AddDate(0, 0, -1), false
	}
	w.millionths = rounded
	if rounded != 0 {
		w.walk.spans = append(w.walk.spans, shareSpan{from: w.day, open: true, millionths: rounded})
	}
}

// splitRatio returns newShares over oldShares, or errSplitRatio unless both are present and positive.
func splitRatio(newShares, oldShares sql.NullString) (*big.Rat, error) {
	if !newShares.Valid || !oldShares.Valid {
		return nil, errSplitRatio
	}
	numerator, err := parseDecimal(newShares.String)
	if err != nil {
		return nil, err
	}
	denominator, err := parseDecimal(oldShares.String)
	if err != nil {
		return nil, err
	}
	if numerator.Sign() <= 0 || denominator.Sign() <= 0 {
		return nil, errSplitRatio
	}
	return numerator.Quo(numerator, denominator), nil
}

// parseDecimal reads the text of a DECIMAL column, or fails with errNotDecimal.
func parseDecimal(text string) (*big.Rat, error) {
	value, ok := new(big.Rat).SetString(text)
	if !ok {
		return nil, fmt.Errorf("%q: %w", text, errNotDecimal)
	}
	return value, nil
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
