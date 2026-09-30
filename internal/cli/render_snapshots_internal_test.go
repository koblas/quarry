// White-box: renderSnapshots and snapshotStatus are unexported formatting
// rules whose cell and precedence edges are best driven directly.
package cli

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
)

func snapshotEntry(id string, bytes int64, source string, takenAt time.Time, verified bool) snapshot.Entry {
	return snapshot.Entry{
		ID: id, Bytes: bytes, TakenAt: takenAt,
		Manifest: &snapshot.Manifest{Snapshot: snapshot.Info{Source: source}, Schema: snapshot.SchemaInfo{Verified: verified}},
	}
}

func Test_renderSnapshots_lays_out_the_ruled_table(t *testing.T) {
	useZone(t, time.FixedZone("EDT", -4*60*60))
	stored := snapshotEntry("20260930T141502Z", 55_200_000, "/Users/x/Documents/Home.quicken", time.Date(2026, 9, 30, 14, 15, 2, 0, time.UTC), true)
	stored.Store = true
	differs := snapshotEntry("20260929T090011Z", 55_200_000, "/Users/x/Documents/Home.quicken", time.Date(2026, 9, 29, 9, 0, 11, 0, time.UTC), false)
	bare := snapshot.Entry{ID: "20260927T143005Z", Bytes: 55_100_000}
	listing := snapshot.Listing{Entries: []snapshot.Entry{stored, differs, bare}, TotalBytes: 165_500_000}

	got := renderSnapshots(listing)

	assert.Equal(t, ""+
		"ID                Taken                     Size  Source        Status\n"+
		"20260930T141502Z  2026-09-30 10:15 EDT   55.2 MB  Home.quicken  store\n"+
		"20260929T090011Z  2026-09-29 05:00 EDT   55.2 MB  Home.quicken  schema differs\n"+
		"20260927T143005Z  unknown                55.1 MB  unknown       no manifest\n"+
		"Total                                   165.5 MB\n", got)
}

func Test_renderSnapshots_prints_only_the_header_when_there_are_no_snapshots(t *testing.T) {
	got := renderSnapshots(snapshot.Listing{Entries: []snapshot.Entry{}})

	assert.Equal(t, "ID  Taken  Size  Source  Status\n", got)
}

func Test_renderSnapshots_leaves_no_trailing_space_on_rows_without_a_status(t *testing.T) {
	useZone(t, time.UTC)
	entry := snapshotEntry("20260927T143005Z", 1_240_000, "/Users/x/Home.quicken", time.Date(2026, 9, 27, 14, 30, 5, 0, time.UTC), true)
	listing := snapshot.Listing{Entries: []snapshot.Entry{entry}, TotalBytes: 1_240_000}

	got := renderSnapshots(listing)

	assert.Equal(t, ""+
		"ID                Taken                   Size  Source        Status\n"+
		"20260927T143005Z  2026-09-27 14:30 UTC  1.2 MB  Home.quicken\n"+
		"Total                                   1.2 MB\n", got)
}

func Test_renderSnapshots_shows_unknown_for_an_unparsable_taken_at_and_an_empty_source(t *testing.T) {
	entry := snapshotEntry("20260927T143005Z", 1_240_000, "", time.Time{}, false)

	got := renderSnapshots(snapshot.Listing{Entries: []snapshot.Entry{entry}, TotalBytes: 1_240_000})

	assert.Equal(t, ""+
		"ID                Taken      Size  Source   Status\n"+
		"20260927T143005Z  unknown  1.2 MB  unknown  schema differs\n"+
		"Total                      1.2 MB\n", got)
}

func Test_renderSnapshots_pads_a_non_ASCII_source_by_characters_not_bytes(t *testing.T) {
	useZone(t, time.UTC)
	cafe := snapshotEntry("20260930T141502Z", 1_240_000, "/Users/x/Café.quicken", time.Date(2026, 9, 30, 14, 15, 2, 0, time.UTC), true)
	cafe.Store = true
	family := snapshotEntry("20260929T090011Z", 1_240_000, "/Users/x/Family.quicken", time.Date(2026, 9, 29, 9, 0, 11, 0, time.UTC), false)

	got := renderSnapshots(snapshot.Listing{Entries: []snapshot.Entry{cafe, family}, TotalBytes: 2_480_000})

	assert.Equal(t, ""+
		"ID                Taken                   Size  Source          Status\n"+
		"20260930T141502Z  2026-09-30 14:15 UTC  1.2 MB  Café.quicken    store\n"+
		"20260929T090011Z  2026-09-29 09:00 UTC  1.2 MB  Family.quicken  schema differs\n"+
		"Total                                   2.5 MB\n", got)
}

func Test_snapshotStatus_puts_store_first(t *testing.T) {
	cases := []struct {
		name  string
		entry snapshot.Entry
		want  string
	}{
		{name: "store beats no manifest", entry: snapshot.Entry{Store: true}, want: "store"},
		{name: "store beats schema differs", entry: snapshot.Entry{Store: true, Manifest: &snapshot.Manifest{}}, want: "store"},
		{name: "no manifest", entry: snapshot.Entry{}, want: "no manifest"},
		{name: "schema differs", entry: snapshot.Entry{Manifest: &snapshot.Manifest{}}, want: "schema differs"},
		{name: "usable", entry: snapshot.Entry{Manifest: &snapshot.Manifest{Schema: snapshot.SchemaInfo{Verified: true}}}, want: ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, snapshotStatus(c.entry))
		})
	}
}
