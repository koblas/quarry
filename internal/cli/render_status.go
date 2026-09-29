package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/koblas/quarry/internal/platform/homepath"
	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
)

// Layouts for status's times: the snapshot's local moment, and a store date
// (a calendar day, never converted between zones).
const (
	takenLayout = "2006-01-02 15:04 MST"
	dateLayout  = "2006-01-02"
)

// renderStatus renders st's Store, Snapshot, Source, Dates, Rows, Balances,
// Splits and Transfers lines, ages measured against now.
func renderStatus(st store.Status, home string, now time.Time) string {
	run := st.Run
	var b strings.Builder
	fmt.Fprintf(&b, "%-10s%s\n", "Store", homepath.Abbreviate(home, st.Path))
	fmt.Fprintf(&b, "%-10s%s\n", "Snapshot", snapshotLine(run.Snapshot, now))
	fmt.Fprintf(&b, "%-10s%s\n", "Source", sourceLine(run.Snapshot.Source, home))
	fmt.Fprintf(&b, "%-10s%s\n", "Dates", datesLine(st.FirstDate, st.LastDate))
	fmt.Fprintf(&b, "%-10s%s\n", "Rows", rowsPhrase(run.Counts, store.NotImported{InvestmentTransactions: run.InvestmentTransactionsNotImported}))
	fmt.Fprintf(&b, "%-10s%s\n", "Balances", balancesPhrase(run.BalancesChecked, run.BalancesNeverReconciled, run.InvestmentAccounts))
	fmt.Fprintf(&b, "%-10s%s\n", "Splits", splitsPhrase(run.Counts.Transactions))
	fmt.Fprintf(&b, "%-10s%s\n", "Transfers", transfersPhrase(run.TransfersPaired, run.TransfersOneSided))
	return b.String()
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
	return first.Format(dateLayout) + " to " + last.Format(dateLayout)
}

// snapshotTakenPhrase renders takenAt in the local zone with its age at now
// in parentheses.
func snapshotTakenPhrase(now, takenAt time.Time) string {
	local := takenAt.In(time.Local) //nolint:gosmopolitan // status shows the user's own clock; the store keeps UTC
	return local.Format(takenLayout) + " (" + snapshotAge(now, takenAt) + ")"
}

// snapshotAge renders how long before now takenAt was, rounding down to the
// unit: "just now" under a minute (or when takenAt is ahead of the clock),
// then minutes below an hour, hours below 48 hours, and days after.
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
