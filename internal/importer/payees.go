package importer

import (
	"context"
	"fmt"

	"github.com/koblas/quarry/internal/store"
)

const payeesQuery = `
SELECT Z_PK, COALESCE(ZNAME, '')
FROM ZUSERPAYEE
WHERE COALESCE(ZDELETIONCOUNT, 0) = 0
ORDER BY ZNAME, Z_PK
`

// mapPayees reads every non-deleted ZUSERPAYEE row, as recorded (never
// merged with another payee of the same name). The second return value is
// every payee PK that exists (non-deleted), for a transaction's payee_id
// existence check.
func mapPayees(ctx context.Context, src Source) ([]store.Payee, map[int64]bool, error) {
	var rows []store.Payee
	existing := make(map[int64]bool)
	err := src.QueryRows(ctx, payeesQuery, nil, func(scan func(dest ...any) error) error {
		var pk int64
		var name string
		if err := scan(&pk, &name); err != nil {
			return err
		}
		rows = append(rows, store.Payee{ID: fmt.Sprintf("payee-%d", pk), SourceID: pk, Name: name})
		existing[pk] = true
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("read payees: %w", err)
	}
	return rows, existing, nil
}
