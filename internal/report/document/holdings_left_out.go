package document

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
)

// NetWorthWarnings is the unprefixed warning lines for what n leaves out of its balances: that no account has a
// balance on a day listed, the holdings it cannot value, then the balances no exchange rate converts, whose line
// ends with advice on listing them natively; never nil.
func NetWorthWarnings(n report.NetWorth, advice NativeAdvice) []string {
	lines := append([]string{}, emptyNetWorthWarnings(n)...)
	lines = append(lines, unvaluedWarnings(n.Unvalued, n.AsOf, n.Window != nil, n.FirstRate)...)
	return append(lines, rateWarnings(n, advice)...)
}

// AccountsWarnings is the unprefixed warning lines for the holdings l leaves out of its balances; never nil.
func AccountsWarnings(l report.AccountListing) []string {
	return unvaluedWarnings(l.Unvalued, l.AsOf, false, l.FirstRate)
}

// unvaluedWarnings is the lines for rows, holdings left out of a balance: no price, no currency, other
// currency, then no rate. History counts the days instead of naming one; firstRate is zero when the store has none.
func unvaluedWarnings(rows []store.UnvaluedHolding, asOf time.Time, history bool, firstRate time.Time) []string {
	sorted := slices.SortedFunc(slices.Values(rows), compareLeftOut)
	lines := noPriceLines(sorted, asOf, history)
	lines = append(lines, perAccountSecurity(sorted,
		func(r store.UnvaluedHolding) bool { return r.Currency == nil },
		func(r store.UnvaluedHolding) string {
			return fmt.Sprintf("%q has no currency in Quicken, so quarry leaves its value out of %q's balance; "+
				"set its currency in Quicken, then run quarry sync", securityName(r), r.Account)
		})...)
	lines = append(lines, perAccountSecurity(sorted,
		func(r store.UnvaluedHolding) bool {
			return r.Priced && r.Currency != nil && !report.Convertible(store.Holding{Currency: r.Currency})
		},
		func(r store.UnvaluedHolding) string {
			return fmt.Sprintf("%q is priced in %s, which quarry does not convert, so its value is left out of %q's balance",
				securityName(r), *r.Currency, r.Account)
		})...)
	return append(lines, noRateLines(sorted, asOf, history, firstRate)...)
}

// lacksOnlyARate reports whether r is priced in the other of CAD and USD than its CAD or USD account's, so only an
// exchange rate would value it.
func lacksOnlyARate(r store.UnvaluedHolding) bool {
	return r.Priced && report.Convertible(store.Holding{Currency: r.Currency}) &&
		(r.AccountCurrency == money.CAD.String() || r.AccountCurrency == money.USD.String()) && *r.Currency != r.AccountCurrency
}

// noRateLines is one line per account with a holding in sorted that lacksOnlyARate.
func noRateLines(sorted []store.UnvaluedHolding, asOf time.Time, history bool, firstRate time.Time) []string {
	lines := []string{}
	for _, group := range byAccount(sorted, lacksOnlyARate) {
		account, held, left := group[0].Account, *group[0].Currency, group[0].AccountCurrency
		switch {
		case firstRate.IsZero():
			lines = append(lines, fmt.Sprintf("%q holds a %s security and the store has no exchange rates, "+
				"so its %s balance leaves it out; run quarry sync to fetch rates", account, held, left))
		case history:
			lines = append(lines, fmt.Sprintf("%q holds a %s security on %s of the month ends listed, before %s, "+
				"the first exchange rate in the store, so its %s balance leaves it out on those days",
				account, held, humanize.Thousands(distinct(group, dayKey)), firstRate.Format(DateLayout), left))
		default:
			securities := distinct(group, func(r store.UnvaluedHolding) string { return r.SecurityID })
			pronoun := "them"
			if securities == 1 {
				pronoun = "it"
			}
			lines = append(lines, fmt.Sprintf("%q holds %s valued on %s, before %s, the first exchange rate in the store, "+
				"so its %s balance leaves %s out", account, humanize.Count(securities, held+" security", held+" securities"),
				asOf.Format(DateLayout), firstRate.Format(DateLayout), left, pronoun))
		}
	}
	return lines
}

// dayKey is the civil day of r's date.
func dayKey(r store.UnvaluedHolding) string { return r.Date.Format(time.DateOnly) }

// compareLeftOut orders rows by account name ignoring case, then name, then id, and the same for the security.
func compareLeftOut(a, b store.UnvaluedHolding) int {
	return cmp.Or(
		cmp.Compare(strings.ToLower(a.Account), strings.ToLower(b.Account)),
		cmp.Compare(a.Account, b.Account),
		cmp.Compare(a.AccountID, b.AccountID),
		cmp.Compare(strings.ToLower(securityName(a)), strings.ToLower(securityName(b))),
		cmp.Compare(securityName(a), securityName(b)),
		cmp.Compare(a.SecurityID, b.SecurityID),
	)
}

// securityName is r's security's name, or its id when it has none.
func securityName(r store.UnvaluedHolding) string {
	return cmp.Or(r.Security, r.SecurityID)
}

// noPriceLines is one line per account with an unpriced holding in sorted; none when every holding has a price.
func noPriceLines(sorted []store.UnvaluedHolding, asOf time.Time, history bool) []string {
	lines := []string{}
	for _, group := range byAccount(sorted, func(r store.UnvaluedHolding) bool { return !r.Priced }) {
		account := group[0].Account
		if history {
			lines = append(lines, fmt.Sprintf("%q holds a security with no price on %s of the month ends listed, "+
				"so its balance leaves it out on those days; enter prices in Quicken, then run quarry sync",
				account, humanize.Thousands(distinct(group, dayKey))))
			continue
		}
		securities := distinct(group, func(r store.UnvaluedHolding) string { return r.SecurityID })
		if securities == 1 {
			lines = append(lines, fmt.Sprintf("%q holds 1 security with no price on or before %s, so its balance leaves it out; "+
				"enter a price in Quicken, then run quarry sync", account, asOf.Format(DateLayout)))
			continue
		}
		lines = append(lines, fmt.Sprintf("%q holds %s with no price on or before %s, so its balance leaves them out; "+
			"enter prices in Quicken, then run quarry sync", account, humanize.Count(securities, "security", "securities"), asOf.Format(DateLayout)))
	}
	return lines
}

// byAccount is the rows of sorted that qualify, one slice per account, in order. sorted keeps an account's rows together.
func byAccount(sorted []store.UnvaluedHolding, qualifies func(store.UnvaluedHolding) bool) [][]store.UnvaluedHolding {
	var groups [][]store.UnvaluedHolding
	for _, r := range sorted {
		if !qualifies(r) {
			continue
		}
		last := len(groups) - 1
		if last >= 0 && groups[last][0].AccountID == r.AccountID {
			groups[last] = append(groups[last], r)
			continue
		}
		groups = append(groups, []store.UnvaluedHolding{r})
	}
	return groups
}

// distinct is how many different keys rows have.
func distinct(rows []store.UnvaluedHolding, key func(store.UnvaluedHolding) string) int {
	seen := map[string]bool{}
	for _, r := range rows {
		seen[key(r)] = true
	}
	return len(seen)
}

// perAccountSecurity is line for the first row of each distinct account and security among the sorted rows that qualify.
func perAccountSecurity(sorted []store.UnvaluedHolding, qualifies func(store.UnvaluedHolding) bool, line func(store.UnvaluedHolding) string) []string {
	lines := []string{}
	seen := map[[2]string]bool{}
	for _, r := range sorted {
		key := [2]string{r.AccountID, r.SecurityID}
		if !qualifies(r) || seen[key] {
			continue
		}
		seen[key] = true
		lines = append(lines, line(r))
	}
	return lines
}
