package document

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/platform/tomlstr"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
)

// ACBAdvice is how a surface tells its caller to list what a no-cost warning is about: the call that lists the
// shares added with no cost, and the one that lists one security's ACB history, which a security id follows.
type ACBAdvice struct {
	SharesWithoutCost string
	Security          string
}

var (
	// ACBAdviceCLI is the advice on the command line, where each listing is a command.
	ACBAdviceCLI = ACBAdvice{SharesWithoutCost: "quarry findings --type shares-without-cost", Security: "quarry acb --security"}
	// ACBAdviceTool is the advice on the MCP server, where each listing is a tool call.
	ACBAdviceTool = ACBAdvice{SharesWithoutCost: "data_quality with type shares-without-cost", Security: "acb with security"}
)

// ACBWarnings is a's warnings in the ruled order: the config's adjustment lines, which name the file as
// configShown, then the report's data-quality lines, one function per kind, appended below in that order.
// The no-cost lines send the caller to the listings advice names.
func ACBWarnings(a report.ACB, configShown string, advice ACBAdvice) []string {
	warnings := adjustmentWarnings(a, configShown)
	warnings = append(warnings, nothingToShowWarnings(a)...)
	warnings = append(warnings, registeredOnlyWarnings(a)...)
	warnings = append(warnings, superficialLossWarnings(a)...)
	warnings = append(warnings, noCostWarnings(a, advice)...)
	warnings = append(warnings, removalWarnings(a)...)
	warnings = append(warnings, unconvertedTradeWarnings(a)...)
	warnings = append(warnings, sameTickerWarnings(a)...)
	warnings = append(warnings, returnOfCapitalWarnings(a)...)

	warnings = append(warnings, decemberSaleWarnings(a)...)

	return append(warnings, oversoldWarnings(a)...)
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

// nothingToShowWarnings is the line saying why a shows nothing: no non-registered account traded at all, or, under a
// year, that year has no sale and no return of capital above the ACB; none otherwise.
func nothingToShowWarnings(a report.ACB) []string {
	// a is the report before any year cut: its Year names the year asked for.
	switch {
	case a.NoPoolEvents():
		return []string{"no non-registered account has bought or sold a security; quarry acb has nothing to show"}
	case !a.YearIsEmpty():
		return nil
	}

	line := fmt.Sprintf("no sales in %d in non-registered accounts", a.Year)
	first, last, found := a.SaleYears()
	switch {
	case !found:
		line += ", nor in any other year"
	case first == last:
		line += fmt.Sprintf("; the sales are in %d", first)
	default:
		line += fmt.Sprintf("; the sales are in %d–%d", first, last)
	}

	return []string{line}
}

// registeredOnlyWarnings is one line for each security the request named that no non-registered account holds,
// in the walk's order.
func registeredOnlyWarnings(a report.ACB) []string {
	warnings := make([]string, 0, len(a.RegisteredOnly))
	for _, security := range a.RegisteredOnly {
		warnings = append(warnings, fmt.Sprintf(`"%s" is held only in registered accounts, so it has no ACB`, security.Name))
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

// noCostWarnings is one line for each security of a that took in shares with no recorded cost, sold out or not,
// naming added shares, reinvested dividends or both, and ending with the listing advice gives.
func noCostWarnings(a report.ACB, advice ACBAdvice) []string {
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
					"%s lists the added shares; enter the reinvested dividends' cost in Quicken", name, advice.SharesWithoutCost))
		case reinvested:
			warnings = append(warnings, fmt.Sprintf(
				`"%s" has reinvested dividends with no cost, so its ACB is too low and its gains too high; `+
					"enter their cost in Quicken; %s %s lists them", name, advice.Security, security.Security.ID))
		case added:
			warnings = append(warnings, fmt.Sprintf(
				`"%s" has shares added with no cost, so its ACB is too low and its gains too high; `+
					"%s lists them", name, advice.SharesWithoutCost))
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

// sameTickerWarnings is one line for each ticker that two or more of a's securities carry.
func sameTickerWarnings(a report.ACB) []string {
	groups := a.SharedTickers()
	warnings := make([]string, 0, len(groups))
	for _, group := range groups {
		names := make([]string, len(group.Securities))
		for i, security := range group.Securities {
			names[i] = security.Name
		}
		warnings = append(warnings, fmt.Sprintf(
			`"%s" is %d securities in Quicken (%s); quarry keeps a separate ACB for each; if they are the same, merge them in Quicken`,
			group.Ticker, len(names), strings.Join(names, ", ")))
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

// firstDecemberSaleDay is the first day of December whose sales may settle in the next tax year.
const firstDecemberSaleDay = 24

// decemberSaleWarnings is one line for each year of a with a sale dated December 24 to 31, oldest year first.
func decemberSaleWarnings(a report.ACB) []string {
	var warnings []string
	for _, year := range a.Years {
		n := 0
		for _, sale := range year.Sales {
			if sale.Date.Month() == time.December && sale.Date.Day() >= firstDecemberSaleDay {
				n++
			}
		}
		if n == 0 {
			continue
		}
		warnings = append(warnings, fmt.Sprintf(
			"%s dated December 24–31, %d: a sale settles a day or two after its trade date and counts for tax in the year it settles; "+
				"check its date on your T5008", humanize.Count(n, "sale", "sales"), year.Year))
	}

	return warnings
}

// oversoldWarnings is one line for each sale or removal of a's securities that left the pool short, in walk order.
// Each names the units short after it, which the next acquisitions only cover.
func oversoldWarnings(a report.ACB) []string {
	var warnings []string
	for _, security := range a.Securities {
		for _, event := range security.Events {
			if event.Oversold == nil {
				continue
			}
			short := humanize.Shares(report.Millionths(event.Oversold))
			date := event.Date.Format(DateLayout)
			if event.Action == store.ActionSell {
				warnings = append(warnings, fmt.Sprintf(
					`"%s": the sale on %s in "%s" sold %s more shares than the non-registered accounts held; quarry counts them at no cost, `+
						"so the sale's gain is too high by what they cost, and the next %s shares acquired only bring the holding back to 0; "+
						"correct the shares in Quicken if they are wrong",
					security.Security.Name, date, event.Account, short, short))

				continue
			}
			warnings = append(warnings, fmt.Sprintf(
				`"%s": %s more shares left "%s" on %s than the non-registered accounts held; `+
					"the next %s shares acquired only bring the holding back to 0; correct the shares in Quicken if they are wrong",
				security.Security.Name, short, event.Account, date, short))
		}
	}

	return warnings
}
