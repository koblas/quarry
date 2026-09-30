package cli

import (
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/koblas/quarry/internal/snapshot"
)

// unknownCell is the Taken or Source cell of a snapshot whose manifest does not record it.
const unknownCell = "unknown"

// Status words of the snapshots table.
const (
	statusStore        = "store"
	statusNoManifest   = "no manifest"
	statusSchemaDiffer = "schema differs"
)

// snapshotsSizeColumn is the index of the right-aligned Size column.
const snapshotsSizeColumn = 2

// renderSnapshots renders l as the snapshots table: a header row, one row per
// entry in l's order, then a Total row unless there are no entries.
func renderSnapshots(l snapshot.Listing) string {
	rows := [][]string{{"ID", "Taken", "Size", "Source", "Status"}}
	for _, e := range l.Entries {
		rows = append(rows, []string{e.ID, snapshotTaken(e), formatMB(e.Bytes), snapshotSource(e), snapshotStatus(e)})
	}
	if len(l.Entries) > 0 {
		rows = append(rows, []string{tableTotalLabel, "", formatMB(l.TotalBytes), "", ""})
	}

	return alignSnapshotRows(rows, "")
}

// alignSnapshotRows lays rows out in columns behind indent, the Size column right-aligned and no row ending in a space.
func alignSnapshotRows(rows [][]string, indent string) string {
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, cell := range row {
			widths[i] = max(widths[i], utf8.RuneCountInString(cell))
		}
	}

	var b strings.Builder
	for _, row := range rows {
		cells := make([]string, len(row))
		for i, cell := range row {
			cells[i] = padRight(cell, widths[i])
			if i == snapshotsSizeColumn {
				cells[i] = padLeft(cell, widths[i])
			}
		}
		b.WriteString(indent + strings.TrimRight(strings.Join(cells, accountsColumnGap), " ") + "\n")
	}
	return b.String()
}

// snapshotTaken is e's moment in the local zone, or "unknown" when its manifest does not record one.
func snapshotTaken(e snapshot.Entry) string {
	if e.TakenAt.IsZero() {
		return unknownCell
	}
	return e.TakenAt.Local().Format(takenLayout) //nolint:gosmopolitan // the table shows the local zone
}

// snapshotSource is the base name of the Quicken file e came from, or "unknown" when its manifest does not record one.
func snapshotSource(e snapshot.Entry) string {
	if e.Manifest == nil || e.Manifest.Snapshot.Source == "" {
		return unknownCell
	}
	return filepath.Base(e.Manifest.Snapshot.Source)
}

// snapshotStatus is e's Status word: store, else no manifest, else schema differs, else "".
func snapshotStatus(e snapshot.Entry) string {
	switch {
	case e.Store:
		return statusStore
	case e.Manifest == nil:
		return statusNoManifest
	case !e.Manifest.Schema.Verified:
		return statusSchemaDiffer
	}
	return ""
}
