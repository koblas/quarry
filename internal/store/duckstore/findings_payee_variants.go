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

// payeeTransactions counts each payee's transactions in every account; a multi-split transaction counts once.
const payeeTransactions = `SELECT payee_id, count(*) AS n FROM transactions WHERE payee_id IS NOT NULL GROUP BY payee_id`

// payeeVariantsQuery lists each payee a transaction uses, with its name and transaction count, in id order.
const payeeVariantsQuery = `SELECT p.id, p.name, pc.n
FROM payees p JOIN (` + payeeTransactions + `) pc ON pc.payee_id = p.id
ORDER BY p.id`

// payeeVariantMin is the fewest payees sharing a key that make a payee-variants finding.
const payeeVariantMin = 2

// payeeUse is one payee a transaction uses.
type payeeUse struct {
	id, name     string
	transactions int64
}

// detectPayeeVariants returns one finding per payee-name key shared by at least two used payees, its items one per payee.
func detectPayeeVariants(ctx context.Context, db DB) ([]detectedFinding, error) {
	byKey := map[string][]payeeUse{}
	err := db.QueryRows(ctx, payeeVariantsQuery, nil, func(scan func(dest ...any) error) error {
		var use payeeUse
		if err := scan(&use.id, &use.name, &use.transactions); err != nil {
			return err
		}
		if key := finding.PayeeKey(use.name); key != "" {
			byKey[key] = append(byKey[key], use)
		}
		return nil
	})
	if err != nil {
		return nil, err //nolint:wrapcheck // detectFindings names the detector
	}
	return payeeVariantFindings(byKey), nil
}

// payeeVariantFindings turns the payees grouped by key into findings in key order, each group's payees by
// transactions descending, then name case-insensitively, then id.
func payeeVariantFindings(byKey map[string][]payeeUse) []detectedFinding {
	var found []detectedFinding
	for _, key := range slices.Sorted(maps.Keys(byKey)) {
		uses := byKey[key]
		if len(uses) < payeeVariantMin {
			continue
		}
		slices.SortFunc(uses, func(a, b payeeUse) int {
			if byCount := cmp.Compare(b.transactions, a.transactions); byCount != 0 {
				return byCount
			}
			if byName := strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name)); byName != 0 {
				return byName
			}
			return strings.Compare(a.id, b.id)
		})
		f := detectedFinding{id: finding.ID(finding.PayeeVariants, key), typ: finding.PayeeVariants}
		for _, use := range uses {
			f.items = append(f.items, findingItem{payeeID: sql.NullString{String: use.id, Valid: true}})
		}
		found = append(found, f)
	}
	return found
}
