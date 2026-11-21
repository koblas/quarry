package v9fixture

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"slices"
	"testing"

	v9 "github.com/koblas/quarry/internal/quicken/v9"
	"github.com/stretchr/testify/require"
)

// renamePlaceholder parks a name for one statement, so renaming a name onto one that is still in use never collides.
const renamePlaceholder = "__renumbering"

// renumberSchema renames every entity-numbered table and column of db, which carries v9.ReferenceDDL, to the
// numbering the Builder's DDL entities give them, resolved through v9.ResolveName. Columns are renamed first,
// while their tables still carry reference names; each name goes through a placeholder.
func (b *Builder) renumberSchema(tb testing.TB, db *sql.DB) {
	tb.Helper()
	ctx := context.Background()
	reference, err := v9.Reference(ctx)
	require.NoError(tb, err)
	entities := make(map[string][]int64, len(b.ddl))
	for name, number := range b.ddl {
		entities[name] = []int64{number}
	}

	var tableRenames [][2]string
	for _, table := range slices.Sorted(maps.Keys(reference)) {
		var columnRenames [][2]string
		for _, column := range reference[table] {
			if renamed, _ := v9.ResolveName(column, entities); renamed != column {
				columnRenames = append(columnRenames, [2]string{column, renamed})
			}
		}
		for _, r := range columnRenames {
			requireExec(tb, ctx, db, fmt.Sprintf("ALTER TABLE %q RENAME COLUMN %q TO %q", table, r[0], r[0]+renamePlaceholder))
		}
		for _, r := range columnRenames {
			requireExec(tb, ctx, db, fmt.Sprintf("ALTER TABLE %q RENAME COLUMN %q TO %q", table, r[0]+renamePlaceholder, r[1]))
		}
		if renamed, _ := v9.ResolveName(table, entities); renamed != table {
			tableRenames = append(tableRenames, [2]string{table, renamed})
		}
	}
	for _, r := range tableRenames {
		requireExec(tb, ctx, db, fmt.Sprintf("ALTER TABLE %q RENAME TO %q", r[0], r[0]+renamePlaceholder))
	}
	for _, r := range tableRenames {
		requireExec(tb, ctx, db, fmt.Sprintf("ALTER TABLE %q RENAME TO %q", r[0]+renamePlaceholder, r[1]))
	}
}
