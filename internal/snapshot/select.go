package snapshot

import (
	"cmp"
	"io/fs"
	"slices"
	"strings"
)

// selectedSnapshot is the entry chosen as one snapshot, with the name of the manifest chosen beside it.
type selectedSnapshot struct {
	entry             fs.DirEntry
	id, stamp, suffix string
	manifest          string
	// manifestShared is true when another entry of any type is named as this ID's snapshot, so the manifest may describe it.
	manifestShared bool
}

// folderSelection is selectFolder's decision over a snapshots folder's entries. Every name in it is a name
// the directory listing returned.
type folderSelection struct {
	// snapshots holds one regular snapshot file per ID.
	snapshots []selectedSnapshot
	// strays are the regular snapshot files that lost to another file of the same ID.
	strays []string
	// orphans are the regular manifests whose snapshot is gone and that no sync is writing.
	orphans []string
}

// idGroup is every directory entry that names one snapshot ID.
type idGroup struct {
	stamp, suffix string
	snapshots     []fs.DirEntry
	// named counts the entries of any type named as a snapshot.
	named     int
	manifests []fs.DirEntry
}

// selectFolder decides which entry is the snapshot and which the manifest of each ID, folding the extension's
// letter case and nothing else. An entry of any type named as a snapshot blocks an orphan but never wins.
func selectFolder(dirEntries []fs.DirEntry) folderSelection {
	groups := map[string]*idGroup{}
	var ids []string
	names := map[string]bool{}
	group := func(id string) *idGroup {
		if groups[id] == nil {
			groups[id] = &idGroup{}
			ids = append(ids, id)
		}
		return groups[id]
	}
	for _, dirEntry := range dirEntries {
		name := dirEntry.Name()
		names[name] = true
		if match := manifestFilePattern.FindStringSubmatch(name); match != nil {
			g := group(match[1])
			g.manifests = append(g.manifests, dirEntry)
		} else if match := snapshotFilePattern.FindStringSubmatch(name); match != nil {
			g := group(ID(name))
			g.stamp, g.suffix = match[1], match[2]
			g.named++
			if dirEntry.Type().IsRegular() {
				g.snapshots = append(g.snapshots, dirEntry)
			}
		}
	}
	var selection folderSelection
	for _, id := range ids {
		g := groups[id]
		if len(g.snapshots) == 0 {
			continue
		}
		winner := preferred(g.snapshots, id+".sqlite")
		for _, dirEntry := range g.snapshots {
			if dirEntry != winner {
				selection.strays = append(selection.strays, dirEntry.Name())
			}
		}
		chosen := selectedSnapshot{entry: winner, id: id, stamp: g.stamp, suffix: g.suffix, manifestShared: g.named > 1}
		if len(g.manifests) > 0 {
			chosen.manifest = preferred(g.manifests, id+".json").Name()
		}
		selection.snapshots = append(selection.snapshots, chosen)
	}
	for _, dirEntry := range dirEntries {
		match := manifestFilePattern.FindStringSubmatch(dirEntry.Name())
		if match == nil || !dirEntry.Type().IsRegular() {
			continue
		}
		// A manifest beside a partial snapshot is a sync in flight: it commits the manifest first.
		if groups[match[1]].named == 0 && !names["."+match[1]+".sqlite.partial"] {
			selection.orphans = append(selection.orphans, dirEntry.Name())
		}
	}
	return selection
}

// preferred is the entry named exactly want when there is one, else the one whose name sorts first by bytes.
func preferred(candidates []fs.DirEntry, want string) fs.DirEntry {
	return slices.MinFunc(candidates, func(a, b fs.DirEntry) int {
		return cmp.Or(cmp.Compare(rankName(a.Name(), want), rankName(b.Name(), want)), strings.Compare(a.Name(), b.Name()))
	})
}

// rankName is 0 for want itself and 1 for any other name.
func rankName(name, want string) int {
	if name == want {
		return 0
	}
	return 1
}
