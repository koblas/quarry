package importer

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/koblas/quarry/internal/store"
)

// categoryKindMap maps ZTAG.ZTYPE (a category row) to categories.kind. Any
// other value is unmappable (S4 reason 9).
var categoryKindMap = map[int64]string{2: "income", 1: "expense", 0: "system"}

// rawCategory is one ZTAG row of the category entity, before mapping.
type rawCategory struct {
	pk     int64
	name   sql.NullString
	typ    sql.NullInt64
	hidden bool
	parent sql.NullInt64
}

const categoriesQuery = `
SELECT Z_PK, ZNAME, ZTYPE, COALESCE(ZHIDDEN, 0), ZPARENTCATEGORY
FROM ZTAG
WHERE Z_ENT = ? AND COALESCE(ZDELETIONCOUNT, 0) = 0
ORDER BY ZNAME, Z_PK
`

// mapCategories reads every non-deleted ZTAG row of categoryEntity. A row
// with no name (S4 reason 10) is added to off and excluded; a row with no
// type (reason 10) or an unmapped type (reason 9) is likewise excluded,
// but its name still anchors any child's full_path.
func mapCategories(ctx context.Context, src Source, categoryEntity int64, off *offenders) ([]store.Category, error) {
	var raws []rawCategory
	byPK := make(map[int64]rawCategory)

	err := src.QueryRows(ctx, categoriesQuery, []any{categoryEntity}, func(scan func(dest ...any) error) error {
		var r rawCategory
		if err := scan(&r.pk, &r.name, &r.typ, &r.hidden, &r.parent); err != nil {
			return err
		}
		raws = append(raws, r)
		byPK[r.pk] = r
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read categories: %w", err)
	}

	var rows []store.Category
	for _, r := range raws {
		if !r.name.Valid || r.name.String == "" {
			off.add(offender{class: 10, reason: reasonCategoryNoName(r.pk), name: fmt.Sprintf("(source id %d)", r.pk), sourceID: r.pk})
			continue
		}
		fullPath := categoryFullPath(byPK, r)
		if !r.typ.Valid {
			off.add(offender{class: 10, reason: reasonCategoryNoType(fullPath), name: fullPath, sourceID: r.pk})
			continue
		}
		kind, ok := categoryKindMap[r.typ.Int64]
		if !ok {
			off.add(offender{class: 9, reason: reasonCategoryType(fullPath, r.typ.Int64), name: fullPath, sourceID: r.pk})
			continue
		}

		var parentID *string
		if r.parent.Valid && r.parent.Int64 != 0 {
			if _, ok := byPK[r.parent.Int64]; ok {
				pid := fmt.Sprintf("cat-%d", r.parent.Int64)
				parentID = &pid
			}
		}
		rows = append(rows, store.Category{
			ID: fmt.Sprintf("cat-%d", r.pk), SourceID: r.pk, ParentID: parentID,
			Name: r.name.String, FullPath: fullPath, Kind: kind, Hidden: r.hidden,
		})
	}
	return rows, nil
}

// categoryFullPath walks r's ZPARENTCATEGORY chain, bounded by the number
// of categories read so a cyclic chain cannot loop, joining ancestor names
// with r's own using ":".
func categoryFullPath(byPK map[int64]rawCategory, r rawCategory) string {
	names := []string{categoryName(r)}
	seen := map[int64]bool{r.pk: true}
	cur := r
	for i := 0; i < len(byPK); i++ {
		if !cur.parent.Valid || cur.parent.Int64 == 0 {
			break
		}
		parent, ok := byPK[cur.parent.Int64]
		if !ok || seen[parent.pk] {
			break
		}
		names = append([]string{categoryName(parent)}, names...)
		seen[parent.pk] = true
		cur = parent
	}
	return strings.Join(names, ":")
}

func categoryName(r rawCategory) string {
	if r.name.Valid && r.name.String != "" {
		return r.name.String
	}
	return fmt.Sprintf("(source id %d)", r.pk)
}

const userTagsQuery = `
SELECT Z_PK, COALESCE(ZNAME, '')
FROM ZTAG
WHERE Z_ENT = ? AND COALESCE(ZDELETIONCOUNT, 0) = 0
ORDER BY ZNAME, Z_PK
`

// mapTags reads every non-deleted ZTAG row of tagEntity as a user tag.
func mapTags(ctx context.Context, src Source, tagEntity int64) ([]store.Tag, error) {
	var rows []store.Tag
	err := src.QueryRows(ctx, userTagsQuery, []any{tagEntity}, func(scan func(dest ...any) error) error {
		var pk int64
		var name string
		if err := scan(&pk, &name); err != nil {
			return err
		}
		rows = append(rows, store.Tag{ID: fmt.Sprintf("tag-%d", pk), SourceID: pk, Name: name})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read tags: %w", err)
	}
	return rows, nil
}
