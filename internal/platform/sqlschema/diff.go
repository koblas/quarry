package sqlschema

import "sort"

// Diff reports how an actual schema differs from a reference schema. Each
// list is sorted bytewise and never nil, so it encodes as JSON "[]" rather
// than "null" when empty.
type Diff struct {
	MissingTables     []string
	MissingColumns    []ColumnRef
	UnexpectedTables  []string
	UnexpectedColumns []ColumnRef
}

// Compare reports the tables and columns actual is missing from reference,
// and the tables and columns actual has that reference does not. A table
// missing (or unexpected) entirely is reported once, without repeating its
// columns in the column lists.
func Compare(reference, actual Schema) Diff {
	d := Diff{
		MissingTables:     []string{},
		MissingColumns:    []ColumnRef{},
		UnexpectedTables:  []string{},
		UnexpectedColumns: []ColumnRef{},
	}

	for table, refCols := range reference {
		actualCols, ok := actual[table]
		if !ok {
			d.MissingTables = append(d.MissingTables, table)
			continue
		}
		actualSet := toSet(actualCols)
		for _, col := range refCols {
			if !actualSet[col] {
				d.MissingColumns = append(d.MissingColumns, ColumnRef{Table: table, Column: col})
			}
		}
	}

	for table, actualCols := range actual {
		refCols, ok := reference[table]
		if !ok {
			d.UnexpectedTables = append(d.UnexpectedTables, table)
			continue
		}
		refSet := toSet(refCols)
		for _, col := range actualCols {
			if !refSet[col] {
				d.UnexpectedColumns = append(d.UnexpectedColumns, ColumnRef{Table: table, Column: col})
			}
		}
	}

	sort.Strings(d.MissingTables)
	sort.Strings(d.UnexpectedTables)
	sortColumnRefs(d.MissingColumns)
	sortColumnRefs(d.UnexpectedColumns)
	return d
}

func toSet(items []string) map[string]bool {
	set := make(map[string]bool, len(items))
	for _, item := range items {
		set[item] = true
	}
	return set
}

func sortColumnRefs(refs []ColumnRef) {
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Table != refs[j].Table {
			return refs[i].Table < refs[j].Table
		}
		return refs[i].Column < refs[j].Column
	})
}
