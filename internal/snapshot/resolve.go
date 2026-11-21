package snapshot

import (
	"cmp"
	"maps"
	"slices"

	"github.com/koblas/quarry/internal/platform/sqlschema"
)

// nameResolver returns the function that renames a reference name to the numbering entities gives the file.
func (s *Server) nameResolver(entities map[string][]int64) func(string) (string, bool) {
	return func(name string) (string, bool) { return s.resolver(name, entities) }
}

// keepName is the EntityResolver that reads every name as written.
func keepName(name string, _ map[string][]int64) (string, bool) { return name, true }

// resolvedReference is a reference schema renamed to a file's numbering, with the reference's own spelling of
// each renamed column kept for display.
type resolvedReference struct {
	schema  sqlschema.Schema
	written map[sqlschema.ColumnRef]writtenColumn
}

// writtenColumn is a column as the reference writes it, and whether both its table and column renamed cleanly.
type writtenColumn struct {
	ref      sqlschema.ColumnRef
	resolved bool
}

// resolveReference renames every table and column of reference through resolve. Tables that resolve onto one
// name merge their columns, and columns that resolve onto one name count once, shown as the first spelled it.
func resolveReference(reference sqlschema.Schema, resolve func(string) (string, bool)) resolvedReference {
	out := resolvedReference{
		schema:  make(sqlschema.Schema, len(reference)),
		written: map[sqlschema.ColumnRef]writtenColumn{},
	}
	for _, table := range slices.Sorted(maps.Keys(reference)) {
		resolvedTable, tableOK := resolve(table)
		columns := out.schema[resolvedTable]
		for _, column := range reference[table] {
			resolvedColumn, columnOK := resolve(column)
			if slices.Contains(columns, resolvedColumn) {
				continue
			}
			key := sqlschema.ColumnRef{Table: resolvedTable, Column: resolvedColumn}
			columns = append(columns, resolvedColumn)
			out.written[key] = writtenColumn{
				ref:      sqlschema.ColumnRef{Table: table, Column: column},
				resolved: tableOK && columnOK,
			}
		}
		out.schema[resolvedTable] = columns
	}
	return out
}

// asWritten shows each missing column under the file's numbering when its table and column both resolved, and
// as the reference writes it otherwise, in the order sqlschema.Compare lists them.
func (r resolvedReference) asWritten(missing []sqlschema.ColumnRef) []sqlschema.ColumnRef {
	shown := make([]sqlschema.ColumnRef, len(missing))
	for i, ref := range missing {
		shown[i] = ref
		if w := r.written[ref]; !w.resolved {
			shown[i] = w.ref
		}
	}
	slices.SortFunc(shown, func(a, b sqlschema.ColumnRef) int {
		return cmp.Or(cmp.Compare(a.Table, b.Table), cmp.Compare(a.Column, b.Column))
	})
	return shown
}
