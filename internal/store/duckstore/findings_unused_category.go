package duckstore

import (
	"context"
	"database/sql"
	"slices"
	"strings"

	"github.com/koblas/quarry/internal/finding"
)

// unusedCategoryQuery lists every category, in id order, with its tree position, kind, visibility and whether a split uses it.
const unusedCategoryQuery = `SELECT c.id, c.parent_id, c.kind, c.full_path, c.hidden, cs.n IS NOT NULL
FROM categories c LEFT JOIN (` + categorySplits + `) cs ON cs.category_id = c.id
ORDER BY c.id`

// categoryNode is one category in the walk that decides which are unused.
type categoryNode struct {
	id, parent, kind, path string
	hidden                 bool
	// used: this category, or a descendant, is referenced. blocked: this category is hidden, below a hidden one,
	// or has a hidden one below it.
	used, blocked bool
	children      []*categoryNode
}

// categoryTree is the categories by id, in id order.
type categoryTree struct {
	order []*categoryNode
	byID  map[string]*categoryNode
}

// detectUnusedCategories returns one finding per top unused category, its items the category then its subcategories.
// referenced holds the ids of categories rows the import does not store as splits use.
func detectUnusedCategories(ctx context.Context, db DB, referenced []string) ([]detectedFinding, error) {
	tree := categoryTree{byID: map[string]*categoryNode{}}
	err := db.QueryRows(ctx, unusedCategoryQuery, nil, func(scan func(dest ...any) error) error {
		var n categoryNode
		var parent sql.NullString
		var split bool
		if err := scan(&n.id, &parent, &n.kind, &n.path, &n.hidden, &split); err != nil {
			return err
		}
		n.parent, n.used = parent.String, split
		tree.order = append(tree.order, &n)
		tree.byID[n.id] = &n
		return nil
	})
	if err != nil {
		return nil, err //nolint:wrapcheck // detectFindings names the detector
	}
	tree.link()
	tree.markUsed(referenced)
	tree.markBlocked()
	return tree.unusedFindings(), nil
}

// link gives each category its children; a category whose parent is not in the table is a top node.
func (t *categoryTree) link() {
	for _, n := range t.order {
		if parent, ok := t.byID[n.parent]; ok {
			parent.children = append(parent.children, n)
		}
	}
}

// climb calls visit on n, then on each ancestor. The step bound only keeps a parent cycle from looping forever.
func (t *categoryTree) climb(n *categoryNode, visit func(*categoryNode)) {
	for steps := 0; n != nil && steps <= len(t.order); steps++ {
		visit(n)
		n = t.byID[n.parent]
	}
}

// markUsed marks every category a split or a referenced id uses, and each of their ancestors.
func (t *categoryTree) markUsed(referenced []string) {
	for _, id := range referenced {
		if n, ok := t.byID[id]; ok {
			n.used = true
		}
	}
	for _, n := range t.order {
		if n.used {
			t.climb(n, func(a *categoryNode) { a.used = true })
		}
	}
}

// markBlocked marks every hidden category, its ancestors and its descendants.
func (t *categoryTree) markBlocked() {
	blockAncestor := func(a *categoryNode) { a.blocked = true }
	for _, n := range t.order {
		blockUnderHidden := func(a *categoryNode) {
			if a.hidden {
				n.blocked = true
			}
		}
		t.climb(n, blockUnderHidden)
		if n.hidden {
			t.climb(n, blockAncestor)
		}
	}
}

// unusedFindings returns a finding for each income or expense category that is unused and not blocked and whose
// parent is absent or used, in id order.
func (t *categoryTree) unusedFindings() []detectedFinding {
	var found []detectedFinding
	for _, n := range t.order {
		parent, hasParent := t.byID[n.parent]
		if (n.kind != "income" && n.kind != "expense") || n.used || n.blocked || (hasParent && !parent.used) {
			continue
		}
		items := []findingItem{unusedCategoryItem(n)}
		for _, d := range sortedDescendants(n) {
			items = append(items, unusedCategoryItem(d))
		}
		found = append(found, detectedFinding{id: finding.ID(finding.UnusedCategory, n.id), typ: finding.UnusedCategory, items: items})
	}
	return found
}

// unusedCategoryItem is the finding item naming category n; it carries the category id only.
func unusedCategoryItem(n *categoryNode) findingItem {
	return findingItem{categoryID: sql.NullString{String: n.id, Valid: true}}
}

// sortedDescendants returns every category below n by path case-insensitively, then id.
func sortedDescendants(n *categoryNode) []*categoryNode {
	var below []*categoryNode
	for _, c := range n.children {
		below = append(append(below, c), sortedDescendants(c)...)
	}
	slices.SortFunc(below, func(a, b *categoryNode) int {
		if byPath := strings.Compare(strings.ToLower(a.path), strings.ToLower(b.path)); byPath != 0 {
			return byPath
		}
		return strings.Compare(a.id, b.id)
	})
	return below
}
