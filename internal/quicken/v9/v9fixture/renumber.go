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

// ddlEntities is b.ddl in the shape v9.ResolveName reads: each entity's one number in the schema's names.
func (b *Builder) ddlEntities() map[string][]int64 {
	entities := make(map[string][]int64, len(b.ddl))
	for name, number := range b.ddl {
		entities[name] = []int64{number}
	}
	return entities
}

// userTagJoin names the UserTag join table and its two columns as the schema carries them: the reference's
// Z_15USERTAGS, Z_15CASHFLOWTRANSACTIONENTRIES and Z_76USERTAGS resolved through the DDL numbering.
func (b *Builder) userTagJoin() (string, string, string) {
	entities := b.ddlEntities()
	table, _ := v9.ResolveName("Z_15USERTAGS", entities)
	entryColumn, _ := v9.ResolveName("Z_15CASHFLOWTRANSACTIONENTRIES", entities)
	tagColumn, _ := v9.ResolveName("Z_76USERTAGS", entities)
	return table, entryColumn, tagColumn
}

// renumberSchema renames the entity-numbered tables and columns of db, which carries v9.ReferenceDDL, to the
// Builder's DDL numbering through v9.ResolveName.
func (b *Builder) renumberSchema(tb testing.TB, db *sql.DB) {
	tb.Helper()
	ctx := context.Background()
	reference, err := v9.Reference(ctx)
	require.NoError(tb, err)
	entities := b.ddlEntities()

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
