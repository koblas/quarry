package cli

import (
	"fmt"

	"github.com/koblas/quarry/internal/platform/homepath"
	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/snapshot"
)

// pruneRowIndent sets a Deleted block's rows under its header line.
const pruneRowIndent = "  "

// renderPruned is p as stdout text: the Deleted or Would delete block, else the Nothing to delete
// line when nothing failed and the run was not interrupted, else "". home abbreviates the folder.
func renderPruned(p snapshot.Pruned, home string) string {
	switch {
	case len(p.WouldDelete) > 0:
		return renderSnapshotBlock("Would delete", p.WouldDelete, p)
	case len(p.Deleted) > 0:
		return renderSnapshotBlock("Deleted", p.Deleted, p)
	case len(p.Failed) > 0 || p.NotDeleted > 0:
		return ""
	case p.Snapshots == 0:
		return "Nothing to delete: no snapshots in " + homepath.Abbreviate(home, p.Dir) + "\n"
	}
	return "Nothing to delete: " + snapshotCount(p.Snapshots) + ", within " + keepPhrase(p) + "\n"
}

// renderSnapshotBlock is the block headed by verb: a header, then a row per entry in entries' order.
func renderSnapshotBlock(verb string, entries []snapshot.Entry, p snapshot.Pruned) string {
	var total int64
	rows := make([][]string, len(entries))
	for i, e := range entries {
		total += e.Bytes
		rows[i] = []string{e.ID, snapshotTaken(e), formatMB(e.Bytes)}
	}
	return verb + " " + snapshotCount(len(entries)) + " (" + formatMB(total) + "), keeping " + keepPhrase(p) + ":\n" +
		alignSnapshotRows(rows, pruneRowIndent)
}

// keepPhrase names what p kept: the newest one or N, and the store's snapshot when it lies outside them.
func keepPhrase(p snapshot.Pruned) string {
	phrase := newestPhrase(p.Keep)
	if p.StoreKept != nil {
		phrase += " and " + p.StoreKept.ID + ", the store's snapshot"
	}
	return phrase
}

// newestPhrase is "the newest one" for keep 1, else "the newest N".
func newestPhrase(keep int) string {
	if keep > 1 {
		return "the newest " + humanize.Thousands(keep)
	}
	return "the newest one"
}

// renderPrunedLine is the Pruned line sync prints after its Transfers block: what auto-prune
// deleted, or "" when it deleted nothing. Deletes that failed are not counted.
func renderPrunedLine(p snapshot.Pruned) string {
	if len(p.Deleted) == 0 {
		return ""
	}
	var total int64
	for _, e := range p.Deleted {
		total += e.Bytes
	}
	beyond := newestPhrase(p.Keep)
	if p.StoreKept != nil {
		beyond += " and the store's own"
	}
	return fmt.Sprintf("%-10s%s beyond %s (%s)\n", "Pruned", snapshotCount(len(p.Deleted)), beyond, formatMB(total))
}

// snapshotCount renders n as "1 snapshot" or "N snapshots".
func snapshotCount(n int) string {
	return humanize.Count(n, "snapshot", "snapshots")
}
