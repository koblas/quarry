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
// merged with another payee of the same name).
func mapPayees(ctx context.Context, src Source) ([]store.Payee, error) {
	var rows []store.Payee
	err := src.QueryRows(ctx, payeesQuery, nil, func(scan func(dest ...any) error) error {
		var pk int64
		var name string
		if err := scan(&pk, &name); err != nil {
			return err
		}
		rows = append(rows, store.Payee{ID: fmt.Sprintf("payee-%d", pk), SourceID: pk, Name: name})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read payees: %w", err)
	}
	return rows, nil
}
