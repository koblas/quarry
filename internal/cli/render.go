package cli

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/koblas/quarry/internal/platform/homepath"
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

// renderSuccess renders m's exact-schema-match success block: snapshot and
// manifest paths, source, size and account count, hash, and the Schema line
// naming the reference and its scoped table/column counts.
func renderSuccess(m snapshot.Manifest, home string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-10s%s\n", "Snapshot", homepath.Abbreviate(home, m.Snapshot.Path))
	fmt.Fprintf(&b, "%-10s%s\n", "Manifest", homepath.Abbreviate(home, m.Snapshot.Manifest))
	fmt.Fprintf(&b, "%-10s%s\n", "Source", homepath.Abbreviate(home, m.Snapshot.Source))
	fmt.Fprintf(&b, "%-10s%s, %s\n", "Size", formatMB(m.Snapshot.Bytes), accountsPhrase(m.Snapshot.Accounts))
	fmt.Fprintf(&b, "%-10s%s\n", "SHA-256", m.Snapshot.SHA256)
	fmt.Fprintf(&b, "%-10s%s\n", "Schema", schemaLine(m.Schema))
	return b.String()
}

// schemaLine renders the exact-match Schema line.
func schemaLine(s snapshot.SchemaInfo) string {
	return fmt.Sprintf("matches reference %s (%s tables, %s columns)",
		s.Reference, formatThousands(s.ReferenceTables), formatThousands(s.ReferenceColumns))
}
