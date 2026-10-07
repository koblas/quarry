package snapshot

import (
	"cmp"
	"io/fs"
	"path/filepath"
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
	// names are the regular snapshot files named as this ID, winner included; manifestNames the regular manifests.
	names, manifestNames []string
}

// folderSelection is selectFolder's decision over a snapshots folder's entries. Every name in it is a name
// the directory listing returned.
type folderSelection struct {
	// snapshots holds one regular snapshot file per ID.
	snapshots []selectedSnapshot
	// orphans are the regular manifests whose snapshot is gone and that no sync is writing.
	orphans []string
	// used holds every ID that some entry of any type is named after, as snapshot or manifest.
	used map[string]bool
}

// uses reports whether some entry of any type in the folder is named as id's snapshot or manifest.
func (f folderSelection) uses(id string) bool { return f.used[id] }

// idGroup is every directory entry that names one snapshot ID.
type idGroup struct {
	stamp, suffix string
	snapshots     []fs.DirEntry
	// named counts the entries of any type named as a snapshot.
	named     int
	manifests []fs.DirEntry
	// manifestNames are the names of the regular files among manifests.
	manifestNames []string
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
			if dirEntry.Type().IsRegular() {
				g.manifestNames = append(g.manifestNames, name)
			}
		} else if match := snapshotFilePattern.FindStringSubmatch(name); match != nil {
			g := group(ID(name))
			g.stamp, g.suffix = match[1], match[2]
			g.named++
			if dirEntry.Type().IsRegular() {
				g.snapshots = append(g.snapshots, dirEntry)
			}
		}
	}
	selection := folderSelection{used: make(map[string]bool, len(ids))}
	for _, id := range ids {
		g := groups[id]
		selection.used[id] = true
		if len(g.snapshots) == 0 {
			continue
		}
		chosen := selectedSnapshot{
			entry: preferred(g.snapshots, id+".sqlite"), id: id, stamp: g.stamp, suffix: g.suffix,
			manifestShared: g.named > 1, names: entryNames(g.snapshots), manifestNames: g.manifestNames,
		}
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

// entryNames are the names of dirEntries, in order.
func entryNames(dirEntries []fs.DirEntry) []string {
	names := make([]string, len(dirEntries))
	for i, dirEntry := range dirEntries {
		names[i] = dirEntry.Name()
	}
	return names
}

// snapshot is the selected snapshot whose ID is exactly id, if the folder has one.
func (f folderSelection) snapshot(id string) (selectedSnapshot, bool) {
	i := slices.IndexFunc(f.snapshots, func(c selectedSnapshot) bool { return c.id == id })
	if i < 0 {
		return selectedSnapshot{}, false
	}
	return f.snapshots[i], true
}

// selectManifest is the name of the manifest of stem in dirEntries: an entry of any type named stem plus .json,
// the exact lowercase name first, else the byte-order first; "" when none.
func selectManifest(dirEntries []fs.DirEntry, stem string) string {
	var candidates []fs.DirEntry
	for _, dirEntry := range dirEntries {
		name := dirEntry.Name()
		if ext := filepath.Ext(name); strings.EqualFold(ext, ".json") && strings.TrimSuffix(name, ext) == stem {
			candidates = append(candidates, dirEntry)
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	return preferred(candidates, stem+".json").Name()
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
