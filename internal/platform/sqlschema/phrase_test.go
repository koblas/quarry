package sqlschema_test

import (
	"testing"

	"github.com/koblas/quarry/internal/platform/sqlschema"
	"github.com/stretchr/testify/assert"
)

func Test_CountPhrase(t *testing.T) {
	cases := []struct {
		name    string
		tables  int
		columns int
		want    string
	}{
		{name: "one table, zero columns", tables: 1, columns: 0, want: "1 table"},
		{name: "many tables, zero columns", tables: 3, columns: 0, want: "3 tables"},
		{name: "zero tables, one column", tables: 0, columns: 1, want: "1 column"},
		{name: "zero tables, many columns", tables: 0, columns: 3, want: "3 columns"},
		{name: "one table, one column", tables: 1, columns: 1, want: "1 table and 1 column"},
		{name: "one table, many columns", tables: 1, columns: 2, want: "1 table and 2 columns"},
		{name: "many tables, one column", tables: 2, columns: 1, want: "2 tables and 1 column"},
		{name: "many tables, many columns", tables: 2, columns: 3, want: "2 tables and 3 columns"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, sqlschema.CountPhrase(c.tables, c.columns))
		})
	}
}
