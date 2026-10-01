// White-box: csvField and renderSQLCSV are unexported quoting rules whose
// many edge inputs are best driven directly.
package cli

import (
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

func Test_csvField_quoting_matrix(t *testing.T) {
	cases := []struct {
		name string
		cell csvCell
		want string
	}{
		{name: "plain text is unquoted", cell: csvCell{Text: "Payee"}, want: "Payee"},
		{name: "a comma quotes", cell: csvCell{Text: "a,b"}, want: `"a,b"`},
		{name: "a quote quotes and doubles", cell: csvCell{Text: `say "hi"`}, want: `"say ""hi"""`},
		{name: "a line feed quotes and stays verbatim", cell: csvCell{Text: "a\nb"}, want: "\"a\nb\""},
		{name: "a carriage return quotes and stays verbatim", cell: csvCell{Text: "a\rb"}, want: "\"a\rb\""},
		{name: "CRLF stays verbatim inside the quotes", cell: csvCell{Text: "a\r\nb"}, want: "\"a\r\nb\""},
		{name: "a lone tab is unquoted and verbatim", cell: csvCell{Text: "a\tb"}, want: "a\tb"},
		{name: "a tab beside a comma quotes and keeps the tab", cell: csvCell{Text: "a\t,b"}, want: "\"a\t,b\""},
		{name: "the empty string is two quotes", cell: csvCell{Text: ""}, want: `""`},
		{name: "NULL is nothing", cell: csvCell{Null: true}, want: ""},
		{name: "NULL ignores its text", cell: csvCell{Text: "NULL", Null: true}, want: ""},
		{name: "the word NULL is text", cell: csvCell{Text: "NULL"}, want: "NULL"},
		{name: "padding spaces stay unquoted", cell: csvCell{Text: " a "}, want: " a "},
		{name: "a formula is not escaped", cell: csvCell{Text: "=1+1"}, want: "=1+1"},
		{name: "a leading plus is not escaped", cell: csvCell{Text: "+x"}, want: "+x"},
		{name: "a leading minus is not escaped", cell: csvCell{Text: "-x"}, want: "-x"},
		{name: "a leading at sign is not escaped", cell: csvCell{Text: "@x"}, want: "@x"},
		{name: "non-ASCII text is unquoted", cell: csvCell{Text: "Café ✓"}, want: "Café ✓"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, csvField(c.cell))
		})
	}
}

func Test_renderSQLCSV(t *testing.T) {
	text := func(name string) store.QueryColumn { return store.QueryColumn{Name: name, Type: "VARCHAR"} }
	cases := []struct {
		name   string
		result store.QueryResult
		want   string
	}{
		{
			name:   "a column name is quoted by the same rule as a value",
			result: store.QueryResult{Columns: []store.QueryColumn{text("a,b"), text(`say "hi"`), text("")}},
			want:   "\"a,b\",\"say \"\"hi\"\"\",\"\"\n",
		},
		{
			name:   "no rows is the header alone",
			result: store.QueryResult{Columns: []store.QueryColumn{text("id"), text("payee")}},
			want:   "id,payee\n",
		},
		{
			name:   "a one-column NULL row is two quotes, never a blank line",
			result: store.QueryResult{Columns: []store.QueryColumn{text("memo")}, Rows: [][]store.QueryValue{{{Text: "NULL", Null: true}}}},
			want:   "memo\n\"\"\n",
		},
		{
			name:   "a one-column empty string row is two quotes",
			result: store.QueryResult{Columns: []store.QueryColumn{text("memo")}, Rows: [][]store.QueryValue{sqlRow("")}},
			want:   "memo\n\"\"\n",
		},
		{
			name:   "every line ends in a line feed, a carriage return inside a field included",
			result: store.QueryResult{Columns: []store.QueryColumn{text("a"), text("b")}, Rows: [][]store.QueryValue{sqlRow("x\ry", "z"), sqlRow("1", "2")}},
			want:   "a,b\n\"x\ry\",z\n1,2\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, renderSQLCSV(c.result))
		})
	}
}
