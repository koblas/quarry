package sqlschema_test

import (
	"testing"

	"github.com/koblas/quarry/internal/platform/sqlschema"
	"github.com/stretchr/testify/assert"
)

func Test_Compare(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		reference sqlschema.Schema
		actual    sqlschema.Schema
		want      sqlschema.Diff
	}{
		{
			name:      "identical schemas report no differences",
			reference: sqlschema.Schema{"ZA": {"X", "Y"}},
			actual:    sqlschema.Schema{"ZA": {"X", "Y"}},
			want: sqlschema.Diff{
				MissingTables: []string{}, MissingColumns: []sqlschema.ColumnRef{},
				UnexpectedTables: []string{}, UnexpectedColumns: []sqlschema.ColumnRef{},
			},
		},
		{
			name:      "a table missing entirely does not repeat its columns",
			reference: sqlschema.Schema{"ZA": {"X"}, "ZLOT": {"P", "Q"}},
			actual:    sqlschema.Schema{"ZA": {"X"}},
			want: sqlschema.Diff{
				MissingTables: []string{"ZLOT"}, MissingColumns: []sqlschema.ColumnRef{},
				UnexpectedTables: []string{}, UnexpectedColumns: []sqlschema.ColumnRef{},
			},
		},
		{
			name:      "a missing column is reported when its table is present",
			reference: sqlschema.Schema{"ZA": {"X", "Y", "Z"}},
			actual:    sqlschema.Schema{"ZA": {"X", "Y"}},
			want: sqlschema.Diff{
				MissingTables: []string{}, MissingColumns: []sqlschema.ColumnRef{{Table: "ZA", Column: "Z"}},
				UnexpectedTables: []string{}, UnexpectedColumns: []sqlschema.ColumnRef{},
			},
		},
		{
			name:      "two missing columns in the same table sort bytewise by column",
			reference: sqlschema.Schema{"ZA": {"X", "Y", "Z"}},
			actual:    sqlschema.Schema{"ZA": {"X"}},
			want: sqlschema.Diff{
				MissingTables: []string{}, MissingColumns: []sqlschema.ColumnRef{{Table: "ZA", Column: "Y"}, {Table: "ZA", Column: "Z"}},
				UnexpectedTables: []string{}, UnexpectedColumns: []sqlschema.ColumnRef{},
			},
		},
		{
			name:      "an extra table does not repeat its columns",
			reference: sqlschema.Schema{"ZA": {"X"}},
			actual:    sqlschema.Schema{"ZA": {"X"}, "ZNEW": {"A", "B"}},
			want: sqlschema.Diff{
				MissingTables: []string{}, MissingColumns: []sqlschema.ColumnRef{},
				UnexpectedTables: []string{"ZNEW"}, UnexpectedColumns: []sqlschema.ColumnRef{},
			},
		},
		{
			name:      "an extra column is reported when its table is expected",
			reference: sqlschema.Schema{"ZA": {"X", "Y"}},
			actual:    sqlschema.Schema{"ZA": {"X", "Y", "NEW"}},
			want: sqlschema.Diff{
				MissingTables: []string{}, MissingColumns: []sqlschema.ColumnRef{},
				UnexpectedTables: []string{}, UnexpectedColumns: []sqlschema.ColumnRef{{Table: "ZA", Column: "NEW"}},
			},
		},
		{
			name:      "multiple missing tables sort bytewise",
			reference: sqlschema.Schema{"ZB": {"X"}, "ZA": {"X"}},
			actual:    sqlschema.Schema{},
			want: sqlschema.Diff{
				MissingTables: []string{"ZA", "ZB"}, MissingColumns: []sqlschema.ColumnRef{},
				UnexpectedTables: []string{}, UnexpectedColumns: []sqlschema.ColumnRef{},
			},
		},
		{
			name:      "missing columns from different tables sort by table first",
			reference: sqlschema.Schema{"ZB": {"X"}, "ZA": {"X"}},
			actual:    sqlschema.Schema{"ZB": {}, "ZA": {}},
			want: sqlschema.Diff{
				MissingTables: []string{}, MissingColumns: []sqlschema.ColumnRef{{Table: "ZA", Column: "X"}, {Table: "ZB", Column: "X"}},
				UnexpectedTables: []string{}, UnexpectedColumns: []sqlschema.ColumnRef{},
			},
		},
		{
			name:      "missing and unexpected columns are both reported in the same compare",
			reference: sqlschema.Schema{"ZA": {"X", "Y"}},
			actual:    sqlschema.Schema{"ZA": {"X", "NEW"}},
			want: sqlschema.Diff{
				MissingTables: []string{}, MissingColumns: []sqlschema.ColumnRef{{Table: "ZA", Column: "Y"}},
				UnexpectedTables: []string{}, UnexpectedColumns: []sqlschema.ColumnRef{{Table: "ZA", Column: "NEW"}},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := sqlschema.Compare(c.reference, c.actual)

			assert.Equal(t, c.want, got)
		})
	}
}
