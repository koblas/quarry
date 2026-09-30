package cli

import (
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
	phrase := "the newest one"
	if p.Keep > 1 {
		phrase = "the newest " + humanize.Thousands(p.Keep)
	}
	if p.StoreKept != nil {
		phrase += " and " + p.StoreKept.ID + ", the store's snapshot"
	}
	return phrase
}

// snapshotCount renders n as "1 snapshot" or "N snapshots".
func snapshotCount(n int) string {
	return humanize.Count(n, "snapshot", "snapshots")
}
