package importer

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
)

// categoryRefs collects the categories the source references from rows the
// import does not store as splits. Only a category that exists is recorded.
type categoryRefs struct {
	existing map[int64]bool
	pks      map[int64]bool
}

func newCategoryRefs(existing map[int64]bool) *categoryRefs {
	return &categoryRefs{existing: existing, pks: make(map[int64]bool)}
}

// add records category when it is set and exists.
func (r *categoryRefs) add(category sql.NullInt64) {
	if category.Valid && r.existing[category.Int64] {
		r.pks[category.Int64] = true
	}
}

// ids returns the recorded categories as store ids, deduplicated and sorted by
// source id; nil when none were recorded.
func (r *categoryRefs) ids() []string {
	if len(r.pks) == 0 {
		return nil
	}
	pks := make([]int64, 0, len(r.pks))
	for pk := range r.pks {
		pks = append(pks, pk)
	}
	slices.Sort(pks)
	ids := make([]string, len(pks))
	for i, pk := range pks {
		ids[i] = fmt.Sprintf("cat-%d", pk)
	}
	return ids
}

// categoryRefSource is one non-split table column that references a category.
// A deleted row is gone in Quicken, so its reference does not count.
type categoryRefSource struct {
	name  string
	query string
}

var categoryRefSources = []categoryRefSource{
	{"budget line items", `
SELECT ZCATEGORYTAG FROM ZBUDGETLINEITEM
WHERE ZCATEGORYTAG IS NOT NULL AND COALESCE(ZDELETIONCOUNT, 0) = 0`},
	{"loan split entries", `
SELECT ZCATEGORY FROM ZLOANSPLITENTRY
WHERE ZCATEGORY IS NOT NULL AND COALESCE(ZDELETIONCOUNT, 0) = 0`},
	{"loan interest categories", `
SELECT ZLOANINTERESTCATEGORY FROM ZACCOUNT
WHERE ZLOANINTERESTCATEGORY IS NOT NULL AND COALESCE(ZDELETIONCOUNT, 0) = 0`},
	{"quickfill rule split entries", `
SELECT ZCATEGORYTAG FROM ZQUICKFILLRULESPLITENTRY
WHERE ZCATEGORYTAG IS NOT NULL AND COALESCE(ZDELETIONCOUNT, 0) = 0`},
	{"product and service categories", `
SELECT ZCATEGORY FROM ZPRODUCTSERVICE
WHERE ZCATEGORY IS NOT NULL AND COALESCE(ZDELETIONCOUNT, 0) = 0`},
	{"customer credit line item categories", `
SELECT ZCATEGORY FROM ZCUSTOMERCREDITLINEITEM
WHERE ZCATEGORY IS NOT NULL AND COALESCE(ZDELETIONCOUNT, 0) = 0`},
}

// readCategoryRefs adds to refs the category every non-deleted row of
// categoryRefSources names.
func readCategoryRefs(ctx context.Context, src Source, refs *categoryRefs) error {
	for _, s := range categoryRefSources {
		err := src.QueryRows(ctx, s.query, nil, func(scan func(dest ...any) error) error {
			var category sql.NullInt64
			if err := scan(&category); err != nil {
				return err
			}
			refs.add(category)
			return nil
		})
		if err != nil {
			return fmt.Errorf("read %s: %w", s.name, err)
		}
	}
	return nil
}
