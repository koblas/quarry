package duckstore

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/koblas/quarry/internal/finding"
)

// oneSidedTransferQuery lists every transfer with no to-split, with the transaction of its from-split
// (NULL when the from-split is not a stored split); it selects from transfers so a join never shrinks the set.
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

// loadFindings detects the findings in the loaded tables, writes them to findings and finding_items,
// and returns their counts; any fault is a build fault.
func loadFindings(ctx context.Context, db DB, builtAt time.Time) (finding.Counts, error) {
	detected, err := detectFindings(ctx, db)
	if err != nil {
		return finding.Counts{}, err
	}
	findingRows, itemRows, counts := mergeFindings(detected, builtAt)
	if err := appendTable(ctx, db, "findings", findingRows); err != nil {
		return finding.Counts{}, err
	}
	if err := appendTable(ctx, db, "finding_items", itemRows); err != nil {
		return finding.Counts{}, err
	}
	return counts, nil
}

// detectFindings runs every detector against the build connection, one-sided transfers first.
func detectFindings(ctx context.Context, db DB) ([]detectedFinding, error) {
	oneSided, err := detectOneSidedTransfers(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("detect %s findings: %w", finding.OneSidedTransfer, err)
	}
	uncategorized, err := detectUncategorized(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("detect %s findings: %w", finding.Uncategorized, err)
	}
	return append(oneSided, uncategorized...), nil
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
// finding.NoPayee, its items one per split; findings come in the query's payee order.
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

// mergeFindings turns detected findings into findings and finding_items rows, every finding first
// found at builtAt and not fixed, and counts them all open and new.
func mergeFindings(detected []detectedFinding, builtAt time.Time) ([][]any, [][]any, finding.Counts) {
	findingRows := make([][]any, 0, len(detected))
	var itemRows [][]any
	for _, d := range detected {
		findingRows = append(findingRows, []any{d.id, string(d.typ), builtAt, nil})
		for _, item := range d.items {
			itemRows = append(itemRows, []any{d.id, nullableNull(item.transactionID), nullableNull(item.splitID), nil, nil})
		}
	}
	return findingRows, itemRows, finding.Counts{Open: len(detected), New: len(detected)}
}

// nullableNull is s as an appender value: its string, or nil for NULL.
func nullableNull(s sql.NullString) any {
	if !s.Valid {
		return nil
	}
	return s.String
}
