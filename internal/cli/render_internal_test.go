// White-box: formatMB, formatThousands and renderSuccess are unexported
// formatting rules best driven directly, rather than through a full command
// run for every rounding and boundary case.
package cli

import (
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
)

func Test_formatThousands(t *testing.T) {
	cases := []struct {
		name string
		n    int
		want string
	}{
		{name: "below the grouping boundary", n: 999, want: "999"},
		{name: "at the grouping boundary", n: 1000, want: "1,000"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, formatThousands(c.n))
		})
	}
}

func Test_formatMB(t *testing.T) {
	cases := []struct {
		name  string
		bytes int64
		want  string
	}{
		{name: "rounds to one decimal place", bytes: 212_400_000, want: "212.4 MB"},
		{name: "rounding crosses the thousands-grouping boundary", bytes: 999_960_000, want: "1,000.0 MB"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, formatMB(c.bytes))
		})
	}
}

func Test_renderSuccess_reports_one_account_in_the_singular(t *testing.T) {
	m := snapshot.Manifest{Snapshot: snapshot.SnapshotInfo{Accounts: 1}}

	got := renderSuccess(m, "/Users/dave")

	assert.Contains(t, got, "1 account\n")
}

func Test_renderSuccess_reports_many_accounts_in_the_plural_with_thousands_grouped(t *testing.T) {
	m := snapshot.Manifest{Snapshot: snapshot.SnapshotInfo{Accounts: 1000}}

	got := renderSuccess(m, "/Users/dave")

	assert.Contains(t, got, "1,000 accounts\n")
}

func Test_schemaLine(t *testing.T) {
	cases := []struct {
		name string
		info snapshot.SchemaInfo
		want string
	}{
		{
			name: "exact match",
			info: snapshot.SchemaInfo{Reference: "ref", Verified: true, ReferenceTables: 71, ReferenceColumns: 1042},
			want: "matches reference ref (71 tables, 1,042 columns)",
		},
		{
			name: "extras only",
			info: snapshot.SchemaInfo{
				Reference: "ref", Verified: true, ReferenceTables: 71, ReferenceColumns: 1042,
				UnexpectedTables: []string{"ZNEWENTITY"},
				UnexpectedColumns: []snapshot.ColumnRef{
					{Table: "ZACCOUNT", Column: "ZNEWFLAG"}, {Table: "ZTAG", Column: "ZCOLOR"},
				},
			},
			want: "matches reference ref (71 tables, 1,042 columns), plus 1 table and 2 columns not in it",
		},
		{
			name: "missing tables only",
			info: snapshot.SchemaInfo{Reference: "ref", Verified: false, MissingTables: []string{"ZLOT"}},
			want: "DIFFERS from reference ref: 1 table missing",
		},
		{
			name: "missing columns only",
			info: snapshot.SchemaInfo{
				Reference: "ref", Verified: false,
				MissingColumns: []snapshot.ColumnRef{{Table: "ZACCOUNT", Column: "ZFAKE"}},
			},
			want: "DIFFERS from reference ref: 1 column missing",
		},
		{
			name: "missing tables and columns",
			info: snapshot.SchemaInfo{
				Reference: "ref", Verified: false,
				MissingTables: []string{"ZLOT"},
				MissingColumns: []snapshot.ColumnRef{
					{Table: "ZCASHFLOWTRANSACTIONENTRY", Column: "ZMEMO"}, {Table: "ZSECURITY", Column: "ZCUSIP"},
				},
			},
			want: "DIFFERS from reference ref: 1 table and 2 columns missing",
		},
		{
			name: "missing plus extras",
			info: snapshot.SchemaInfo{
				Reference: "ref", Verified: false,
				MissingTables: []string{"ZLOT"},
				MissingColumns: []snapshot.ColumnRef{
					{Table: "ZCASHFLOWTRANSACTIONENTRY", Column: "ZMEMO"}, {Table: "ZSECURITY", Column: "ZCUSIP"},
				},
				UnexpectedColumns: []snapshot.ColumnRef{{Table: "ZACCOUNT", Column: "ZNEWFLAG"}},
			},
			want: "DIFFERS from reference ref: 1 table and 2 columns missing, 1 column not in reference",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, schemaLine(c.info))
		})
	}
}

func Test_renderSuccess_writes_diff_rows_missing_before_extras_and_tables_before_columns(t *testing.T) {
	m := snapshot.Manifest{
		Schema: snapshot.SchemaInfo{
			Reference:      "ref",
			Verified:       false,
			MissingTables:  []string{"ZLOT"},
			MissingColumns: []snapshot.ColumnRef{{Table: "ZCASHFLOWTRANSACTIONENTRY", Column: "ZMEMO"}, {Table: "ZSECURITY", Column: "ZCUSIP"}},
			UnexpectedColumns: []snapshot.ColumnRef{
				{Table: "ZACCOUNT", Column: "ZNEWFLAG"},
			},
		},
	}

	got := renderSuccess(m, "/Users/dave")

	assert.True(t, strings.HasSuffix(got,
		"  - table   ZLOT\n"+
			"  - column  ZCASHFLOWTRANSACTIONENTRY.ZMEMO\n"+
			"  - column  ZSECURITY.ZCUSIP\n"+
			"  + column  ZACCOUNT.ZNEWFLAG\n"),
		"got: %q", got)
}

func Test_renderSuccess_writes_no_diff_rows_on_an_exact_match(t *testing.T) {
	m := snapshot.Manifest{Schema: snapshot.SchemaInfo{Reference: "ref", Verified: true}}

	got := renderSuccess(m, "/Users/dave")

	assert.False(t, strings.Contains(got, "  -"))
	assert.False(t, strings.Contains(got, "  +"))
}

func Test_renderSuccess_reports_the_exact_match_schema_line(t *testing.T) {
	m := snapshot.Manifest{
		Snapshot: snapshot.SnapshotInfo{
			Path:     "/Users/dave/Library/Application Support/quarry/snapshots/20260927T143005Z.sqlite",
			Manifest: "/Users/dave/Library/Application Support/quarry/snapshots/20260927T143005Z.json",
			Source:   "/Users/dave/Documents/Home.quicken",
			Bytes:    212_400_000,
			SHA256:   "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
			Accounts: 42,
		},
		Schema: snapshot.SchemaInfo{
			Reference:        "hardkoded/quicken-skills@752107b",
			Verified:         true,
			ReferenceTables:  71,
			ReferenceColumns: 1042,
		},
	}

	got := renderSuccess(m, "/Users/dave")

	assert.Equal(t, "Snapshot  ~/Library/Application Support/quarry/snapshots/20260927T143005Z.sqlite\n"+
		"Manifest  ~/Library/Application Support/quarry/snapshots/20260927T143005Z.json\n"+
		"Source    ~/Documents/Home.quicken\n"+
		"Size      212.4 MB, 42 accounts\n"+
		"SHA-256   9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08\n"+
		"Schema    matches reference hardkoded/quicken-skills@752107b (71 tables, 1,042 columns)\n",
		got)
}
