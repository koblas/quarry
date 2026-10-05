package document

import (
	"fmt"

	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/platform/tomlstr"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
)

// ACBWarnings is a's warnings in the ruled order: the config's adjustment lines, which name the file as
// configShown (~-abbreviated for stderr, absolute for --json), then one per removal of shares with no sale,
// then one per return of capital above the ACB; the last two by security then date.
func ACBWarnings(a report.ACB, configShown string) []string {
	warnings := adjustmentWarnings(a, configShown)
	warnings = append(warnings, removalWarnings(a)...)

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
