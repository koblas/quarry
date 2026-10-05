package document

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/platform/tomlstr"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
)

// ACBWarnings is a's warnings in the ruled order: the config's adjustment lines, which name the file as
// configShown, then one line for the possible superficial losses, then one per security with shares acquired
// with no cost, then one per removal of shares with no sale, then one per security with a trade quarry cannot
// convert to CAD, then one per return of capital above the ACB; all but the first two by security then date.
func ACBWarnings(a report.ACB, configShown string) []string {
	warnings := adjustmentWarnings(a, configShown)
	warnings = append(warnings, superficialLossWarnings(a)...)
	warnings = append(warnings, noCostWarnings(a)...)
	warnings = append(warnings, removalWarnings(a)...)
	warnings = append(warnings, unconvertedTradeWarnings(a)...)

	return append(warnings, returnOfCapitalWarnings(a)...)
}

// adjustmentWarnings is one line for each issue of a's adjustment items, by item number.
func adjustmentWarnings(a report.ACB, configShown string) []string {
	var warnings []string
	for _, issue := range a.AdjustmentIssues {
		prefix := fmt.Sprintf("%s: acb.adjustment item %d", configShown, issue.Item)
		switch issue.Kind {
		case report.ACBAdjustmentUnknownSecurity:
			warnings = append(warnings, fmt.Sprintf("%s names %s, which is not a security in quarry's store; quarry skips it",
				prefix, tomlstr.BasicString(issue.SecurityID)))
		case report.ACBAdjustmentNotHeld:
			warnings = append(warnings, fmt.Sprintf(`%s is for "%s", which no non-registered account holds on %s; quarry skips it`,
				prefix, issue.Security, issue.Date.Format(DateLayout)))
		case report.ACBAdjustmentRepeated:
			warnings = append(warnings, fmt.Sprintf("%s: acb.adjustment items %d and %d are both for %s on %s; quarry applies both",
				configShown, issue.First, issue.Item, tomlstr.BasicString(issue.SecurityID), issue.Date.Format(DateLayout)))
		}
	}

	return warnings
}

// superficialLossWarnings is one line for every sale of a marked a possible superficial loss, naming their years
// oldest first; none when no sale is marked.
func superficialLossWarnings(a report.ACB) []string {
	var years []string
	marked := 0
	for _, year := range a.Years {
		if n := year.PossibleSuperficialLosses(); n > 0 {
			marked += n
			years = append(years, strconv.Itoa(year.Year))
		}
	}
	if marked == 0 {
		return nil
	}

	return []string{fmt.Sprintf("%s in %s: the same security was acquired within 30 days before or after the sale, in any account, "+
		"and still held 30 days after; quarry does not deny or adjust these losses; review them with your accountant",
		humanize.Count(marked, "possible superficial loss", "possible superficial losses"), strings.Join(years, ", "))}
}

// noCostWarnings is one line for each security of a that took in shares with no recorded cost, sold out or not.
// The line's cause is which kinds of acquisition had none: added shares, reinvested dividends, or both.
func noCostWarnings(a report.ACB) []string {
	var warnings []string
	for _, security := range a.Securities {
		var added, reinvested bool
		for _, event := range security.Events {
			if !event.UnknownCost {
				continue
			}
			switch event.Action {
			case store.ActionAddShares:
				added = true
			case store.ActionReinvestDividend:
				reinvested = true
			}
		}
		name := security.Security.Name
		switch {
		case added && reinvested:
			warnings = append(warnings, fmt.Sprintf(
				`"%s" has shares added and dividends reinvested with no cost, so its ACB is too low and its gains too high; `+
					"quarry findings --type shares-without-cost lists the added shares; enter the reinvested dividends' cost in Quicken", name))
		case reinvested:
			warnings = append(warnings, fmt.Sprintf(
				`"%s" has reinvested dividends with no cost, so its ACB is too low and its gains too high; `+
					"enter their cost in Quicken; quarry acb --security %s lists them", name, security.Security.ID))
		case added:
			warnings = append(warnings, fmt.Sprintf(
				`"%s" has shares added with no cost, so its ACB is too low and its gains too high; `+
					"quarry findings --type shares-without-cost lists them", name))
		}
	}

	return warnings
}

// removalWarnings is one warning for each remove_shares event of a's securities.
func removalWarnings(a report.ACB) []string {
	var warnings []string
	for _, security := range a.Securities {
		for _, event := range security.Events {
			if event.Action != store.ActionRemoveShares {
				continue
			}
			warnings = append(warnings, fmt.Sprintf(
				`"%s": %s shares left "%s" on %s without a sale; quarry took their share of the ACB out and reports no gain; `+
					"if they went to a registered account or to someone else, that is a disposition at market value; "+
					"check it with your accountant",
				security.Security.Name, humanize.Shares(report.Millionths(event.Shares)), event.Account, event.Date.Format(DateLayout)))
		}
	}

	return warnings
}

// unconvertedTradeWarnings is one line for each security of a with a trade quarry could not convert to CAD, naming the
// earliest. A USD trade is told apart by whether the store has rates at all; any other currency by its code.
func unconvertedTradeWarnings(a report.ACB) []string {
	const left = "so its ACB is incomplete and its gains are left out of the year totals"
	var warnings []string
	for _, security := range a.Securities {
		noRate := security.NoRate
		if noRate == nil {
			continue
		}
		name, date := security.Security.Name, noRate.Date.Format(DateLayout)
		switch currency, _ := money.ParseCurrency(noRate.Currency); {
		case currency != money.USD:
			warnings = append(warnings, fmt.Sprintf(`"%s" has a trade on %s in a currency quarry cannot convert to CAD ("%s"), %s`,
				name, date, noRate.Currency, left))
		case a.FirstRate.IsZero():
			warnings = append(warnings, fmt.Sprintf(`"%s" has a USD trade on %s, and the store has no exchange rates, %s; run quarry sync to fetch rates`,
				name, date, left))
		default:
			warnings = append(warnings, fmt.Sprintf(`"%s" has a USD trade on %s, before %s, the first exchange rate in the store, %s`,
				name, date, a.FirstRate.Format(DateLayout), left))
		}
	}

	return warnings
}

// returnOfCapitalWarnings is one warning for each return of capital of a's securities that exceeds the ACB.
func returnOfCapitalWarnings(a report.ACB) []string {
	var warnings []string
	for _, security := range a.Securities {
		for _, event := range security.Events {
			if event.Action != report.ACBActionReturnOfCapital || !event.Realized {
				continue
			}
			excess := humanize.Money(event.Gain)
			warnings = append(warnings, fmt.Sprintf(
				`"%s": return of capital on %s is %s more than its ACB, so its ACB is 0.00 and %s is a capital gain in %d`,
				security.Security.Name, event.Date.Format(DateLayout), excess, excess, event.Date.Year()))
		}
	}

	return warnings
}
