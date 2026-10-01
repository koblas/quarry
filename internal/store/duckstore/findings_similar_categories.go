package duckstore

import (
	"cmp"
	"context"
	"database/sql"
	"maps"
	"slices"
	"strings"

	"github.com/koblas/quarry/internal/finding"
)

// categorySplits counts each category's splits in every account, closed and excluded ones included.
const categorySplits = `SELECT category_id, count(*) AS n FROM splits WHERE category_id IS NOT NULL GROUP BY category_id`

// similarCategoriesQuery lists every category, used or not, hidden or not, with its kind, path and split count, in id order.
const similarCategoriesQuery = `SELECT c.id, c.kind, c.full_path, COALESCE(cs.n, 0)
FROM categories c LEFT JOIN (` + categorySplits + `) cs ON cs.category_id = c.id
ORDER BY c.id`

// similarCategoriesMin is the fewest categories sharing a key that make a similar-categories finding.
const similarCategoriesMin = 2

// categoryUse is one category with the splits that use it.
type categoryUse struct {
	id, path string
	splits   int64
}

// detectSimilarCategories returns one finding per category key shared by at least two categories, its items one per category.
func detectSimilarCategories(ctx context.Context, db DB) ([]detectedFinding, error) {
	byKey := map[string][]categoryUse{}
	err := db.QueryRows(ctx, similarCategoriesQuery, nil, func(scan func(dest ...any) error) error {
		var use categoryUse
		var kind string
		if err := scan(&use.id, &kind, &use.path, &use.splits); err != nil {
			return err
		}
		if key := finding.CategoryKey(kind, use.path); key != "" {
			byKey[key] = append(byKey[key], use)
		}
		return nil
	})
	if err != nil {
		return nil, err //nolint:wrapcheck // detectFindings names the detector
	}
	return similarCategoryFindings(byKey), nil
}

// similarCategoryFindings turns the categories grouped by key into findings in key order, each group's categories by
// splits descending, then path case-insensitively, then id.
func similarCategoryFindings(byKey map[string][]categoryUse) []detectedFinding {
	var found []detectedFinding
	for _, key := range slices.Sorted(maps.Keys(byKey)) {
		uses := byKey[key]
		if len(uses) < similarCategoriesMin {
			continue
		}
		slices.SortFunc(uses, func(a, b categoryUse) int {
			if bySplits := cmp.Compare(b.splits, a.splits); bySplits != 0 {
				return bySplits
			}
			if byPath := strings.Compare(strings.ToLower(a.path), strings.ToLower(b.path)); byPath != 0 {
				return byPath
			}
			return strings.Compare(a.id, b.id)
		})
		f := detectedFinding{id: finding.ID(finding.SimilarCategories, key), typ: finding.SimilarCategories}
		for _, use := range uses {
			f.items = append(f.items, findingItem{categoryID: sql.NullString{String: use.id, Valid: true}})
		}
		found = append(found, f)
	}
	return found
}
