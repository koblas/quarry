package duckstore

import (
	"context"
	"database/sql"
	"slices"
	"strings"

	"github.com/koblas/quarry/internal/finding"
)

// mixedTransactions selects the transactions a mixed-categories finding judges: a payee and one categorized v_cash_flow row.
const mixedTransactions = `SELECT payee_id, date, source_id, category_id, category FROM (
  SELECT cf.payee_id, cf.date, t.source_id, cf.category_id, cf.category, count(*) OVER (PARTITION BY cf.transaction_id) AS n
  FROM v_cash_flow cf JOIN transactions t ON t.id = cf.transaction_id
  WHERE cf.category_id IS NOT NULL AND cf.payee_id IS NOT NULL
) WHERE n = 1`

// mixedCategoriesQuery lists each payee's judged transactions as payee id, category id and path, in the order the walk reads them.
const mixedCategoriesQuery = `SELECT payee_id, category_id, category FROM (` + mixedTransactions + `)
ORDER BY payee_id, date, source_id`

// mixedStep is one judged transaction's category.
type mixedStep struct {
	categoryID, path string
}

// detectMixedCategories returns one finding per payee whose category goes back and forth, its items one per category.
func detectMixedCategories(ctx context.Context, db DB) ([]detectedFinding, error) {
	var found []detectedFinding
	var payee string
	var walk []mixedStep
	judge := func() {
		if f, ok := mixedCategoriesFinding(payee, walk); ok {
			found = append(found, f)
		}
		walk = nil
	}
	err := db.QueryRows(ctx, mixedCategoriesQuery, nil, func(scan func(dest ...any) error) error {
		var next string
		var step mixedStep
		if err := scan(&next, &step.categoryID, &step.path); err != nil {
			return err
		}
		if next != payee {
			judge()
			payee = next
		}
		walk = append(walk, step)
		return nil
	})
	if err != nil {
		return nil, err //nolint:wrapcheck // detectFindings names the detector
	}
	judge()
	return found, nil
}

// mixedCategoriesFinding judges one payee's transactions, oldest first: flagged when there are at least
// finding.MixedMin and the category changes more often than one visit to each category needs, so some is revisited.
func mixedCategoriesFinding(payee string, walk []mixedStep) (detectedFinding, bool) {
	if len(walk) < finding.MixedMin {
		return detectedFinding{}, false
	}
	counts := map[mixedStep]int{}
	changes := 0
	for i, step := range walk {
		counts[step]++
		if i > 0 && step.categoryID != walk[i-1].categoryID {
			changes++
		}
	}
	distinct := len(counts)
	if changes <= distinct-1 {
		return detectedFinding{}, false
	}
	steps := make([]mixedStep, 0, distinct)
	for step := range counts {
		steps = append(steps, step)
	}
	slices.SortFunc(steps, func(a, b mixedStep) int {
		if byCount := counts[b] - counts[a]; byCount != 0 {
			return byCount
		}
		if byPath := strings.Compare(strings.ToLower(a.path), strings.ToLower(b.path)); byPath != 0 {
			return byPath
		}
		return strings.Compare(a.categoryID, b.categoryID)
	})
	f := detectedFinding{id: finding.ID(finding.MixedCategories, payee), typ: finding.MixedCategories}
	for _, step := range steps {
		f.items = append(f.items, findingItem{
			payeeID:    sql.NullString{String: payee, Valid: true},
			categoryID: sql.NullString{String: step.categoryID, Valid: true},
		})
	}
	return f, true
}
