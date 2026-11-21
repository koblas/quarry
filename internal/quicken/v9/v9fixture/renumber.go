package v9fixture

import (
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
	ctx := tb.Context()
	reference, err := v9.Reference(ctx)
	require.NoError(tb, err)
	entities := b.ddlEntities()
	renamed := func(name string) (rename, bool) {
		to, _ := v9.ResolveName(name, entities)
		return rename{from: name, to: to}, to != name
	}

	var tableRenames []rename
	for _, table := range slices.Sorted(maps.Keys(reference)) {
		var columnRenames []rename
		for _, column := range reference[table] {
			if r, ok := renamed(column); ok {
				columnRenames = append(columnRenames, r)
			}
		}
		renameAll(tb, db, columnRenames, func(from, to string) string {
			return fmt.Sprintf("ALTER TABLE %q RENAME COLUMN %q TO %q", table, from, to)
		})
		if r, ok := renamed(table); ok {
			tableRenames = append(tableRenames, r)
		}
	}
	renameAll(tb, db, tableRenames, func(from, to string) string {
		return fmt.Sprintf("ALTER TABLE %q RENAME TO %q", from, to)
	})
}

// rename is one table or column name and the name it becomes.
type rename struct{ from, to string }

// renameAll runs statement for each rename, parking every name under renamePlaceholder first so no rename lands
// on a name another rename has yet to vacate.
func renameAll(tb testing.TB, db *sql.DB, renames []rename, statement func(from, to string) string) {
	tb.Helper()
	ctx := tb.Context()
	for _, r := range renames {
		requireExec(tb, ctx, db, statement(r.from, r.from+renamePlaceholder))
	}
	for _, r := range renames {
		requireExec(tb, ctx, db, statement(r.from+renamePlaceholder, r.to))
	}
}
