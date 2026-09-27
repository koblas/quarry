package cli

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/koblas/quarry/internal/platform/homepath"
	"github.com/koblas/quarry/internal/platform/sqlschema"
	"github.com/koblas/quarry/internal/snapshot"
)

// formatThousands renders n, which is always non-negative in this package's
// callers (byte counts, account counts, table/column counts), with a comma
// every three digits from the right.
func formatThousands(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// formatMB renders bytes as decimal megabytes with one decimal place and
// thousands-grouped whole MB.
func formatMB(bytes int64) string {
	tenths := int64(math.Round(float64(bytes) / 100000))
	return fmt.Sprintf("%s.%d MB", formatThousands(int(tenths/10)), tenths%10)
}

// accountsPhrase renders n as "1 account" or "N accounts", thousands-grouped.
func accountsPhrase(n int) string {
	if n == 1 {
		return "1 account"
	}
	return formatThousands(n) + " accounts"
}

// renderSuccess renders m's result block, then the diff rows a mismatch or
// extras-only Schema line summarizes.
func renderSuccess(m snapshot.Manifest, home string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-10s%s\n", "Snapshot", homepath.Abbreviate(home, m.Snapshot.Path))
	fmt.Fprintf(&b, "%-10s%s\n", "Manifest", homepath.Abbreviate(home, m.Snapshot.Manifest))
	fmt.Fprintf(&b, "%-10s%s\n", "Source", homepath.Abbreviate(home, m.Snapshot.Source))
	fmt.Fprintf(&b, "%-10s%s, %s\n", "Size", formatMB(m.Snapshot.Bytes), accountsPhrase(m.Snapshot.Accounts))
	fmt.Fprintf(&b, "%-10s%s\n", "SHA-256", m.Snapshot.SHA256)
	fmt.Fprintf(&b, "%-10s%s\n", "Schema", schemaLine(m.Schema))
	writeDiffRows(&b, m.Schema)
	return b.String()
}

// schemaLine renders the Schema line: exact match, extras-only (W1), or a
// DIFFERS line for a mismatch (M1).
func schemaLine(s snapshot.SchemaInfo) string {
	extra := len(s.UnexpectedTables) + len(s.UnexpectedColumns)

	if !s.Verified {
		line := fmt.Sprintf("DIFFERS from reference %s: %s missing",
			s.Reference, sqlschema.CountPhrase(len(s.MissingTables), len(s.MissingColumns)))
		if extra > 0 {
			line += fmt.Sprintf(", %s not in reference",
				sqlschema.CountPhrase(len(s.UnexpectedTables), len(s.UnexpectedColumns)))
		}
		return line
	}

	line := fmt.Sprintf("matches reference %s (%s tables, %s columns)",
		s.Reference, formatThousands(s.ReferenceTables), formatThousands(s.ReferenceColumns))
	if extra > 0 {
		line += fmt.Sprintf(", plus %s not in it",
			sqlschema.CountPhrase(len(s.UnexpectedTables), len(s.UnexpectedColumns)))
	}
	return line
}

// writeDiffRows appends one row per s's diff entries: missing before
// unexpected, tables before columns within each.
func writeDiffRows(b *strings.Builder, s snapshot.SchemaInfo) {
	for _, table := range s.MissingTables {
		writeDiffRow(b, "-", "table", table)
	}
	for _, col := range s.MissingColumns {
		writeDiffRow(b, "-", "column", col.Table+"."+col.Column)
	}
	for _, table := range s.UnexpectedTables {
		writeDiffRow(b, "+", "table", table)
	}
	for _, col := range s.UnexpectedColumns {
		writeDiffRow(b, "+", "column", col.Table+"."+col.Column)
	}
}

// writeDiffRow appends one diff row: two leading spaces, sign, a
// space-padded 8-wide label, then value.
func writeDiffRow(b *strings.Builder, sign, label, value string) {
	fmt.Fprintf(b, "  %s %-8s%s\n", sign, label, value)
}
