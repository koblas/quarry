package importer

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/koblas/quarry/internal/store"
)

// categoryKindMap maps ZTAG.ZTYPE (a category row) to categories.kind. Any
// other value is unmappable (classCategoryType).
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

// uncategorizedPath is the full_path of Quicken's built-in placeholder
// category, which the store represents as a NULL split category.
const uncategorizedPath = "Uncategorized"

// mapCategories reads every non-deleted ZTAG row of categoryEntity. A row
// with no name is added to off and excluded; a row with no type or an
// unmapped type is likewise excluded,
// but its name still anchors any child's full_path. The second return
// value is every category PK that exists (non-deleted), so a nullable
// reference to a deleted or missing category can be told apart from one
// pointing at a live row. The third is the PK of each system-kind category
// whose full_path is exactly "Uncategorized"; its category row is still
// emitted.
func mapCategories(ctx context.Context, src Source, categoryEntity int64, off *offenders) ([]store.Category, map[int64]bool, map[int64]bool, error) {
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
		return nil, nil, nil, fmt.Errorf("read categories: %w", err)
	}

	var rows []store.Category
	uncategorized := make(map[int64]bool)
	for _, r := range raws {
		if !r.name.Valid || r.name.String == "" {
			off.add(offender{class: classMissingValue, reason: reasonCategoryNoName(r.pk), name: fmt.Sprintf("(source id %d)", r.pk), sourceID: r.pk})
			continue
		}
		fullPath := categoryFullPath(byPK, r)
		if !r.typ.Valid {
			off.add(offender{class: classMissingValue, reason: reasonCategoryNoType(fullPath), name: fullPath, sourceID: r.pk})
			continue
		}
		kind, ok := categoryKindMap[r.typ.Int64]
		if !ok {
			off.add(offender{class: classCategoryType, reason: reasonCategoryType(fullPath, r.typ.Int64), name: fullPath, sourceID: r.pk})
			continue
		}

		var parentID *string
		if r.parent.Valid && r.parent.Int64 != 0 {
			if _, ok := byPK[r.parent.Int64]; ok {
				pid := fmt.Sprintf("cat-%d", r.parent.Int64)
				parentID = &pid
			}
		}
		if kind == "system" && fullPath == uncategorizedPath {
			uncategorized[r.pk] = true
		}
		rows = append(rows, store.Category{
			ID: fmt.Sprintf("cat-%d", r.pk), SourceID: r.pk, ParentID: parentID,
			Name: r.name.String, FullPath: fullPath, Kind: kind, Hidden: r.hidden,
		})
	}
	existing := make(map[int64]bool, len(byPK))
	for pk := range byPK {
		existing[pk] = true
	}
	return rows, existing, uncategorized, nil
}

// categoryFullPath walks r's ZPARENTCATEGORY chain, bounded by the number
// of categories read so a cyclic chain cannot loop, joining ancestor names
// with r's own using ":".
func categoryFullPath(byPK map[int64]rawCategory, r rawCategory) string {
	names := []string{categoryName(r)}
	seen := map[int64]bool{r.pk: true}
	cur := r
	for range len(byPK) {
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

// mapTags reads every non-deleted ZTAG row of tagEntity as a user tag. The
// second return value is every tag PK that exists (non-deleted), for a
// split_tags link's tag-existence check.
func mapTags(ctx context.Context, src Source, tagEntity int64) ([]store.Tag, map[int64]bool, error) {
	var rows []store.Tag
	existing := make(map[int64]bool)
	err := src.QueryRows(ctx, userTagsQuery, []any{tagEntity}, func(scan func(dest ...any) error) error {
		var pk int64
		var name string
		if err := scan(&pk, &name); err != nil {
			return err
		}
		rows = append(rows, store.Tag{ID: fmt.Sprintf("tag-%d", pk), SourceID: pk, Name: name})
		existing[pk] = true
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("read tags: %w", err)
	}
	return rows, existing, nil
}
