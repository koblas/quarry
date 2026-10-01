package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/platform/homepath"
	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
)

// takenLayout is the format of the snapshot's local moment.
const takenLayout = "2006-01-02 15:04 MST"

// statusFindings is the findings tally status reports; ignoreKnown is false when findings.ignore
// could not be read, so every ignored finding is counted open and "ignored" cannot be said.
type statusFindings struct {
	counts      finding.Counts
	ignoreKnown bool
}

// renderStatus renders st's Store, Snapshot, Source, Dates, Rows, Balances,
// Splits, Transfers, Findings and Rates lines, ages measured against now.
func renderStatus(st store.Status, findings statusFindings, home string, now time.Time) string {
	run := st.Run
	var b strings.Builder
	fmt.Fprintf(&b, "%-10s%s\n", "Store", homepath.Abbreviate(home, st.Path))
	fmt.Fprintf(&b, "%-10s%s\n", "Snapshot", snapshotLine(run.Snapshot, now))
	fmt.Fprintf(&b, "%-10s%s\n", "Source", sourceLine(run.Snapshot.Source, home))
	fmt.Fprintf(&b, "%-10s%s\n", "Dates", datesLine(st.FirstDate, st.LastDate))
	fmt.Fprintf(&b, "%-10s%s\n", "Rows", rowsPhrase(run.Counts, store.NotImported{InvestmentTransactions: run.InvestmentTransactionsNotImported}))
	fmt.Fprintf(&b, "%-10s%s\n", "Balances", balancesPhrase(balanceCounts{Checked: run.BalancesChecked, NeverReconciled: run.BalancesNeverReconciled, InvestmentAccounts: run.InvestmentAccounts}))
	fmt.Fprintf(&b, "%-10s%s\n", "Splits", splitsPhrase(run.Counts.Transactions))
	fmt.Fprintf(&b, "%-10s%s\n", "Transfers", transfersPhrase(run.TransfersPaired, run.TransfersOneSided))
	fmt.Fprintf(&b, "%-10s%s\n", "Findings", statusFindingsPhrase(findings))
	fmt.Fprintf(&b, "%-10s%s\n", "Rates", statusRatesPhrase(st, now))
	return b.String()
}

// statusRatesPhrase is the Rates row text: the stored coverage and its age, or that there is none,
// then a clause when transactions precede the first rate and one when the last sync's fetch failed.
func statusRatesPhrase(st store.Status, now time.Time) string {
	rates := st.Rates
	var b strings.Builder
	if rates.Last.IsZero() {
		b.WriteString("none, so amounts are not converted; run quarry sync to fetch them from the Bank of Canada")
	} else {
		fmt.Fprintf(&b, "USD/CAD from the Bank of Canada, %s to %s (%s)",
			rates.First.Format(jsonDateLayout), rates.Last.Format(jsonDateLayout), rateAge(now, rates.Last))
		if !st.FirstDate.IsZero() && rates.First.After(st.FirstDate) {
			b.WriteString("; transactions before " + rates.First.Format(jsonDateLayout) + " are not converted")
		}
	}
	if rates.FetchError != "" {
		b.WriteString("; the last sync could not fetch new rates: " + rates.FetchError)
	}
	return b.String()
}

// rateAge renders how many calendar days before now's local date the stored day last is: "today",
// "1 day ago" or "N days ago". A day after today reads "today".
func rateAge(now, last time.Time) string {
	local := now.In(time.Local) //nolint:gosmopolitan // the age counts the user's own calendar days; last is a stored day, not an instant
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	days := int(today.Sub(time.Date(last.Year(), last.Month(), last.Day(), 0, 0, 0, 0, time.UTC)) / (24 * time.Hour))
	if days <= 0 {
		return "today"
	}
	return humanize.Count(days, "day", "days") + " ago"
}

// statusFindingsPhrase is the Findings row text: the open count and, when findings.ignore was read,
// the ignored one, without the new and fixed clauses sync prints.
func statusFindingsPhrase(f statusFindings) string {
	c := f.counts
	c.New, c.NewlyFixed = 0, 0
	if !f.ignoreKnown {
		c.Ignored = 0
	}
	return findingsPhrase(c, false)
}

// snapshotLine renders the snapshot's ID with when it was taken, or says the
// manifest did not record it; the time is never guessed from the ID.
func snapshotLine(ref store.SnapshotRef, now time.Time) string {
	id := report.SnapshotID(ref.Path)
	if ref.TakenAt.IsZero() {
		return id + ", time taken not recorded in its manifest"
	}
	return id + ", taken " + snapshotTakenPhrase(now, ref.TakenAt)
}

// sourceLine renders the Quicken file the snapshot came from, or says the
// manifest did not record it.
func sourceLine(source, home string) string {
	if source == "" {
		return "not recorded in the snapshot's manifest"
	}
	return homepath.Abbreviate(home, source)
}

// datesLine renders the transaction date range, or "no transactions" when
// the store holds none.
func datesLine(first, last time.Time) string {
	if first.IsZero() {
		return "no transactions"
	}
	return first.Format(jsonDateLayout) + " to " + last.Format(jsonDateLayout)
}

// snapshotTakenPhrase renders takenAt in the local zone with its age at now
// in parentheses.
func snapshotTakenPhrase(now, takenAt time.Time) string {
	local := takenAt.In(time.Local) //nolint:gosmopolitan // status shows the user's own clock; the store keeps UTC
	return local.Format(takenLayout) + " (" + snapshotAge(now, takenAt) + ")"
}

// snapshotAge renders how long before now takenAt was, or "just now" when
// under a minute or ahead of the clock.
func snapshotAge(now, takenAt time.Time) string {
	age := now.Sub(takenAt)
	switch {
	case age < time.Minute:
		return "just now"
	case age < time.Hour:
		return humanize.Count(int(age/time.Minute), "minute", "minutes") + " ago"
	case age < 48*time.Hour:
		return humanize.Count(int(age/time.Hour), "hour", "hours") + " ago"
	default:
		return humanize.Count(int(age/(24*time.Hour)), "day", "days") + " ago"
	}
}
