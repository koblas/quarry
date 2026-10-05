package document

import (
	"fmt"

	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
)

// ACBWarnings is a's warnings in the ruled order: one per removal of shares with no sale, by security then date.
func ACBWarnings(a report.ACB) []string {
	return removalWarnings(a)
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
