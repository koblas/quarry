// White-box: renderSQLTable is an unexported layout rule whose widths,
// alignment and escaping are best driven directly.
package cli

import (
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

func sqlRow(texts ...string) []store.QueryValue {
	row := make([]store.QueryValue, len(texts))
	for i, text := range texts {
		row[i] = store.QueryValue{Text: text}
	}
	return row
}

func Test_renderSQLTable(t *testing.T) {
	text := store.QueryColumn{Name: "s", Type: "VARCHAR"}
	integer := store.QueryColumn{Name: "n", Type: "INTEGER"}
	cases := []struct {
		name   string
		result store.QueryResult
		want   string
	}{
		{
			name:   "an escaped control character widens its column",
			result: store.QueryResult{Columns: []store.QueryColumn{text, integer}, Rows: [][]store.QueryValue{sqlRow("a\nb", "1")}},
			want:   "s     n\na\\nb  1\n",
		},
		{
			name:   "tab and carriage return are escaped and nothing else is",
			result: store.QueryResult{Columns: []store.QueryColumn{text}, Rows: [][]store.QueryValue{sqlRow("a\tb\rc\\d'e")}},
			want:   "s\na\\tb\\rc\\d'e\n",
		},
		{
			name:   "a column name is escaped",
			result: store.QueryResult{Columns: []store.QueryColumn{{Name: "a\tb", Type: "VARCHAR"}}},
			want:   "a\\tb\n",
		},
		{
			name: "a numeric column and its header are right-aligned, NULL included",
			result: store.QueryResult{
				Columns: []store.QueryColumn{integer, text},
				Rows:    [][]store.QueryValue{sqlRow("12345", "x"), {{Null: true, Text: "NULL"}, {Text: "y"}}},
			},
			want: "    n  s\n12345  x\n NULL  y\n",
		},
		{
			name:   "a left-aligned last column is not padded",
			result: store.QueryResult{Columns: []store.QueryColumn{text}, Rows: [][]store.QueryValue{sqlRow("long"), sqlRow("x")}},
			want:   "s\nlong\nx\n",
		},
		{
			name:   "an empty last cell leaves no column gap behind",
			result: store.QueryResult{Columns: []store.QueryColumn{integer, text}, Rows: [][]store.QueryValue{sqlRow("1", "")}},
			want:   "n  s\n1\n",
		},
		{
			name:   "a last column value's own trailing space is kept",
			result: store.QueryResult{Columns: []store.QueryColumn{integer, text}, Rows: [][]store.QueryValue{sqlRow("1", "b ")}},
			want:   "n  s\n1  b \n",
		},
		{
			name:   "a middle column value's own edge spaces are kept and widen its column",
			result: store.QueryResult{Columns: []store.QueryColumn{text, integer}, Rows: [][]store.QueryValue{sqlRow(" b ", "1"), sqlRow("x", "2")}},
			want:   "s    n\n b   1\nx    2\n",
		},
		{
			name:   "an all-spaces middle cell is kept before an empty last cell",
			result: store.QueryResult{Columns: []store.QueryColumn{text, {Name: "t", Type: "VARCHAR"}}, Rows: [][]store.QueryValue{sqlRow("  ", "")}},
			want:   "s   t\n  \n",
		},
		{
			name:   "zero rows print the header only",
			result: store.QueryResult{Columns: []store.QueryColumn{text, integer}},
			want:   "s  n\n",
		},
		{
			name:   "width counts runes, not bytes",
			result: store.QueryResult{Columns: []store.QueryColumn{text, integer}, Rows: [][]store.QueryValue{sqlRow("é", "1"), sqlRow("ab", "2")}},
			want:   "s   n\né   1\nab  2\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, renderSQLTable(c.result))
		})
	}
}
