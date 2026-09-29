package sqlschema

import "github.com/koblas/quarry/internal/platform/humanize"

// CountPhrase renders tables and columns as "N table(s) and M column(s)",
// omitting either clause when its count is zero. Callers never pass two
// zero counts: no diff clause is rendered when both are zero.
func CountPhrase(tables, columns int) string {
	switch {
	case tables == 0:
		return countNoun(columns, "column")
	case columns == 0:
		return countNoun(tables, "table")
	default:
		return countNoun(tables, "table") + " and " + countNoun(columns, "column")
	}
}

// countNoun renders n as "1 <noun>" or "N <noun>s", N thousands-grouped.
func countNoun(n int, noun string) string {
	return humanize.Count(n, noun, noun+"s")
}
