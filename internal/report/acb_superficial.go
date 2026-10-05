package report

import (
	"cmp"
	"math/big"
	"slices"
	"time"

	"github.com/koblas/quarry/internal/store"
)

// superficialWindowDays is the days before and after a loss within which an acquisition may make it superficial.
const superficialWindowDays = 30

// markSuperficialLosses sets PossibleSuperficialLoss on each sale at a loss when the security (or one with the
// same ticker) was acquired within 30 days either side of it in any account of history, registered included, and
// is still held at the end of the 30th day after. A loss is marked, never adjusted; rows after today are ignored.
func markSuperficialLosses(sales []acbSale, history store.InvestmentHistory, today time.Time) {
	index := newSuperficialIndex(history, today)
	for i := range sales {
		row := &sales[i].row
		row.PossibleSuperficialLoss = row.Gain < 0 && index.possible(*row)
	}
}

// superficialIndex is the file's transactions through today by security, each security's in date then source id
// order, and the securities that share a ticker.
type superficialIndex struct {
	byID       map[string][]store.InvestmentTransaction
	sameTicker map[string][]string
	tickerOf   map[string]string
}

func newSuperficialIndex(history store.InvestmentHistory, today time.Time) superficialIndex {
	index := superficialIndex{
		byID:       make(map[string][]store.InvestmentTransaction),
		sameTicker: make(map[string][]string),
		tickerOf:   make(map[string]string),
	}
	for _, tx := range history.Transactions {
		// unreachable: the nil-security arm, since duckstore's InvestmentHistory reads only rows with a security_id (investments.go:20).
		if tx.SecurityID == nil || tx.Date.After(today) {
			continue
		}
		index.byID[*tx.SecurityID] = append(index.byID[*tx.SecurityID], tx)
	}
	for _, txs := range index.byID {
		slices.SortStableFunc(txs, func(a, b store.InvestmentTransaction) int {
			return cmp.Or(a.Date.Compare(b.Date), cmp.Compare(a.SourceID, b.SourceID))
		})
	}
	for _, security := range history.Securities {
		if security.Ticker != nil && *security.Ticker != "" {
			index.tickerOf[security.ID] = *security.Ticker
			index.sameTicker[*security.Ticker] = append(index.sameTicker[*security.Ticker], security.ID)
		}
	}

	return index
}

// possible is whether the loss sale had an acquisition within the window and its security's group is still held
// at the end of the 30th day after it; the index holds nothing after today, so a window still open ends there.
func (x superficialIndex) possible(sale ACBSale) bool {
	group := x.group(sale.SecurityID)
	from := sale.Date.AddDate(0, 0, -superficialWindowDays)
	to := sale.Date.AddDate(0, 0, superficialWindowDays)

	return x.acquiredBetween(group, from, to) && x.heldAt(group, to)
}

// group is the ids of the securities identical to id: its own, and every other one with the same non-empty,
// case-sensitive ticker.
func (x superficialIndex) group(id string) []string {
	ticker, ok := x.tickerOf[id]
	if !ok {
		return []string{id}
	}

	return x.sameTicker[ticker]
}

// acquiredBetween is whether any of ids gained units in a buy, reinvested dividend or added shares dated from
// through to, both days included.
func (x superficialIndex) acquiredBetween(ids []string, from, to time.Time) bool {
	for _, id := range ids {
		for _, tx := range x.byID[id] {
			if acbTiers[tx.Action] == acbAcquisition && units(tx.Shares).Sign() > 0 && !tx.Date.Before(from) && !tx.Date.After(to) {
				return true
			}
		}
	}

	return false
}

// heldAt is whether any holding of ids, an account's own units of one security, is above nothing at the end of
// day. A holding counts its stored (signed) shares, each split row in its own account scaling that account's
// count, and is rounded to millionths; a holding below nothing never cancels one above.
func (x superficialIndex) heldAt(ids []string, day time.Time) bool {
	for _, id := range ids {
		counts := make(map[string]*big.Rat)
		for _, tx := range x.byID[id] {
			if tx.Date.After(day) {
				break
			}
			count, ok := counts[tx.AccountID]
			if !ok {
				count = new(big.Rat)
				counts[tx.AccountID] = count
			}
			if tx.Action == store.ActionSplit {
				splitShares(count, tx.SplitNewShares, tx.SplitOldShares)
			} else {
				count.Add(count, units(tx.Shares))
			}
		}
		for _, count := range counts {
			if Millionths(count) > 0 {
				return true
			}
		}
	}

	return false
}

// splitShares multiplies count by newShares over oldShares.
func splitShares(count *big.Rat, newShares, oldShares *int64) {
	if newShares == nil || oldShares == nil || *newShares <= 0 || *oldShares <= 0 {
		// unreachable: the importer refuses such a split (investments.go splitSides) and duckstore/shares.go:223 splitRatio, called from shares.go:123, does too.
		return
	}
	count.Mul(count, big.NewRat(*newShares, *oldShares))
}
