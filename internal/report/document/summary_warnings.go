package document

import (
	"fmt"

	"github.com/koblas/quarry/internal/report"
)

// snapshotTakenLayout is the format of the moment a snapshot was taken.
const snapshotTakenLayout = "2006-01-02 15:04 MST"

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
