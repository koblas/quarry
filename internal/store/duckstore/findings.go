package duckstore

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"time"

	"github.com/koblas/quarry/internal/finding"
)

// duplicateQuery lists each pair of transactions in one account with the same non-zero amount at most ? days
// apart, unless both are reconciled, the lower source id first so each pair appears once.
const duplicateQuery = `SELECT a.id, b.id
FROM transactions a JOIN transactions b
  ON a.account_id = b.account_id AND a.amount = b.amount AND (a.source_id, a.id) < (b.source_id, b.id)
WHERE a.amount <> 0 AND abs(a.date - b.date) <= ? AND NOT (a.status = 'reconciled' AND b.status = 'reconciled')
ORDER BY a.id, b.id`

// oneSidedTransferQuery lists every transfer with no to-split, with its from-split's transaction (NULL when not stored).
const oneSidedTransferQuery = `SELECT x.id, x.from_split_id, s.transaction_id
FROM transfers x LEFT JOIN splits s ON s.id = x.from_split_id
WHERE x.to_split_id IS NULL
ORDER BY x.id`

// uncategorizedQuery lists the v_cash_flow splits with no category, so the set is what cashflow counts.
const uncategorizedQuery = `SELECT split_id, transaction_id, payee_id
FROM v_cash_flow
WHERE category_id IS NULL
ORDER BY payee_id NULLS FIRST, split_id`

// findingItem is one row of finding_items; a NULL column is an invalid sql.NullString.
type findingItem struct {
	transactionID, splitID sql.NullString
}

// detectedFinding is one finding a build's detection produced, with the items that name what it is about.
type detectedFinding struct {
	id    string
	typ   finding.Type
	items []findingItem
}

// loadFindings detects the findings in the loaded tables, merges them with the carried ones, writes
// findings and finding_items, and returns the counts; any fault is a build fault.
func loadFindings(ctx context.Context, db DB, carried []carriedFinding, builtAt time.Time) (finding.Counts, error) {
	detected, err := detectFindings(ctx, db)
	if err != nil {
		return finding.Counts{}, err
	}
	findingRows, itemRows, counts := mergeFindings(detected, carried, builtAt)
	if err := appendTable(ctx, db, "findings", findingRows); err != nil {
		return finding.Counts{}, err
	}
	if err := appendTable(ctx, db, "finding_items", itemRows); err != nil {
		return finding.Counts{}, err
	}
	return counts, nil
}

// detectFindings runs every detector against the build connection, in finding.Types order.
func detectFindings(ctx context.Context, db DB) ([]detectedFinding, error) {
	duplicates, err := detectDuplicates(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("detect %s findings: %w", finding.Duplicate, err)
	}
	oneSided, err := detectOneSidedTransfers(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("detect %s findings: %w", finding.OneSidedTransfer, err)
	}
	uncategorized, err := detectUncategorized(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("detect %s findings: %w", finding.Uncategorized, err)
	}
	return slices.Concat(duplicates, oneSided, uncategorized), nil
}

// detectDuplicates returns one finding per duplicate pair, its items the two transactions.
func detectDuplicates(ctx context.Context, db DB) ([]detectedFinding, error) {
	var found []detectedFinding
	err := db.QueryRows(ctx, duplicateQuery, []any{finding.MatchDays}, func(scan func(dest ...any) error) error {
		var first, second string
		if err := scan(&first, &second); err != nil {
			return err
		}
		found = append(found, detectedFinding{
			id: finding.PairID(finding.Duplicate, first, second), typ: finding.Duplicate,
			items: []findingItem{
				{transactionID: sql.NullString{String: first, Valid: true}},
				{transactionID: sql.NullString{String: second, Valid: true}},
			},
		})
		return nil
	})
	return found, err //nolint:wrapcheck // detectFindings names the detector
}

// detectOneSidedTransfers returns one finding per transfer with no to-split, its item the from-split.
func detectOneSidedTransfers(ctx context.Context, db DB) ([]detectedFinding, error) {
	var found []detectedFinding
	err := db.QueryRows(ctx, oneSidedTransferQuery, nil, func(scan func(dest ...any) error) error {
		var transferID string
		var item findingItem
		if err := scan(&transferID, &item.splitID, &item.transactionID); err != nil {
			return err
		}
		found = append(found, detectedFinding{
			id: finding.ID(finding.OneSidedTransfer, transferID), typ: finding.OneSidedTransfer, items: []findingItem{item},
		})
		return nil
	})
	return found, err //nolint:wrapcheck // detectFindings names the detector
}

// detectUncategorized returns one finding per payee with uncategorized splits, or per
// finding.NoPayee, its items one per split.
func detectUncategorized(ctx context.Context, db DB) ([]detectedFinding, error) {
	var found []detectedFinding
	byID := map[string]int{}
	err := db.QueryRows(ctx, uncategorizedQuery, nil, func(scan func(dest ...any) error) error {
		var payee sql.NullString
		var item findingItem
		if err := scan(&item.splitID, &item.transactionID, &payee); err != nil {
			return err
		}
		entity := finding.NoPayee
		if payee.Valid {
			entity = payee.String
		}
		id := finding.ID(finding.Uncategorized, entity)
		i, seen := byID[id]
		if !seen {
			i = len(found)
			byID[id] = i
			found = append(found, detectedFinding{id: id, typ: finding.Uncategorized})
		}
		found[i].items = append(found[i].items, item)
		return nil
	})
	return found, err //nolint:wrapcheck // detectFindings names the detector
}

// mergeFindings turns detected and carried findings into findings and finding_items rows; a carried finding keeps
// its first_found_at and type, and is fixed at builtAt the first build that no longer detects it.
func mergeFindings(detected []detectedFinding, carried []carriedFinding, builtAt time.Time) ([][]any, [][]any, finding.Counts) {
	prior := make(map[string]carriedFinding, len(carried))
	for _, c := range carried {
		prior[c.id] = c
	}
	var counts finding.Counts
	findingRows := make([][]any, 0, len(detected))
	var itemRows [][]any
	found := make(map[string]bool, len(detected))
	for _, d := range detected {
		found[d.id] = true
		firstFoundAt := builtAt
		if c, ok := prior[d.id]; ok {
			firstFoundAt = c.firstFoundAt
		} else {
			counts.New++
		}
		counts.Open++
		findingRows = append(findingRows, []any{d.id, string(d.typ), firstFoundAt, nil})
		for _, item := range d.items {
			itemRows = append(itemRows, []any{d.id, nullableNull(item.transactionID), nullableNull(item.splitID), nil, nil})
		}
	}
	for _, c := range carried {
		if found[c.id] {
			continue
		}
		fixedAt := c.fixedAt
		if !fixedAt.Valid {
			fixedAt = sql.NullTime{Time: builtAt, Valid: true}
			counts.NewlyFixed++
		}
		counts.Fixed++
		findingRows = append(findingRows, []any{c.id, c.typ, c.firstFoundAt, fixedAt.Time})
	}
	return findingRows, itemRows, counts
}

// nullableNull is s as an appender value: its string, or nil for NULL.
func nullableNull(s sql.NullString) any {
	if !s.Valid {
		return nil
	}
	return s.String
}
