package report

import (
	"math/big"
	"time"

	"github.com/koblas/quarry/internal/store"
)

// superficialWindowDays is the days before and after a loss within which an acquisition may make it superficial.
const superficialWindowDays = 30

// markSuperficialLosses marks each sale at a loss that the security, or one of its ticker, was acquired near and
// is still held after, in any account. A loss is marked, never adjusted.
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
		// unreachable: the nil-security arm, since duckstore's InvestmentHistory query selects only rows WHERE security_id IS NOT NULL (internal/store/duckstore/investments.go:20).
		if tx.SecurityID == nil || tx.Date.After(today) {
			continue
		}
		index.byID[*tx.SecurityID] = append(index.byID[*tx.SecurityID], tx)
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
			if isAcquisition(tx.Action) && units(tx.Shares).Sign() > 0 && !tx.Date.Before(from) && !tx.Date.After(to) {
				return true
			}
		}
	}

	return false
}

// heldAt is whether any account's holding of ids, its walked actions' signed shares and own splits, is above
// nothing at the end of day.
func (x superficialIndex) heldAt(ids []string, day time.Time) bool {
	for _, id := range ids {
		counts := make(map[string]*big.Rat)
		for _, tx := range x.byID[id] {
			if _, walked := acbTiers[tx.Action]; !walked {
				continue
			}
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

// isAcquisition is whether action is one the walk puts in the acquisition tier; an action it skips is none.
func isAcquisition(action string) bool {
	tier, walked := acbTiers[action]

	return walked && tier == acbAcquisition
}

// splitShares multiplies count by newShares over oldShares.
func splitShares(count *big.Rat, newShares, oldShares *int64) {
	if newShares == nil || oldShares == nil || *newShares <= 0 || *oldShares <= 0 {
		// unreachable: the importer refuses such a split (internal/importer/investments.go:245 splitSides), so no stored split has a nil or non-positive side.
		return
	}
	count.Mul(count, big.NewRat(*newShares, *oldShares))
}
