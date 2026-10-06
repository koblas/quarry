package document

import (
	"fmt"
	"slices"

	"github.com/koblas/quarry/internal/report"
)

// snapshotTakenLayout is the format of the moment a snapshot was taken.
const snapshotTakenLayout = "2006-01-02 15:04 MST"

// SummaryWarnings is the summary's own warnings, never nil: the snapshot warning, then the unconverted-charge
// note, the unconverted-series note and the net-worth rate note, each only when it applies and none twice.
// The empty-window and left-out lines of the standalone reports are not part of it. Text stderr and --json
// read this one list, so the two formats cannot disagree; again is as for SnapshotWarning, advice as for NetWorthWarnings.
func SummaryWarnings(s report.Summary, again string, advice NativeAdvice) []string {
	var warnings []string
	if warning := SnapshotWarning(s, again); warning != "" {
		warnings = append(warnings, warning)
	}
	warnings = append(warnings, unconvertedWarnings(s.Anomalies.Currency, s.Anomalies.Unconverted, chargesNoun)...)
	warnings = append(warnings, unconvertedWarnings(s.Recurring.Currency, s.Recurring.Unconverted, seriesNoun)...)
	warnings = append(warnings, rateWarnings(s.NetWorth, advice)...)
	return withoutRepeats(warnings)
}

// withoutRepeats is warnings with each line kept at its first place only, never nil.
func withoutRepeats(warnings []string) []string {
	unique := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		if !slices.Contains(unique, warning) {
			unique = append(unique, warning)
		}
	}
	return unique
}

// SnapshotWarning says why s may be missing part of its month: "" when the snapshot covers the month, else
// the warning for a snapshot taken before the month ended or whose time is unknown. again is the surface's
// phrase for repeating the report, such as "run quarry summary again"; the first warning ends with it.
func SnapshotWarning(s report.Summary, again string) string {
	switch s.Coverage {
	case report.SnapshotPredatesMonthEnd:
		taken := s.Status.Run.Snapshot.TakenAt.In(s.Month.Location()).Format(snapshotTakenLayout)
		return fmt.Sprintf("the store was built from a snapshot taken %s, before %s ended, so transactions from the rest "+
			"of the month are missing; open your Quicken file, run quarry sync, then %s", taken, s.Month.Name(), again)
	case report.SnapshotTimeUnknown:
		return fmt.Sprintf("cannot tell whether the store holds all of %s: its snapshot's manifest does not record "+
			"when it was taken; open your Quicken file and run quarry sync to take a new snapshot", s.Month.Name())
	case report.SnapshotCovers:
		return ""
	}
	return ""
}
