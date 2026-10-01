package importer

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/koblas/quarry/internal/store"
)

const entriesQuery = `
SELECT e.Z_PK, e.ZPARENT, typeof(e.ZAMOUNT), CAST(e.ZAMOUNT AS TEXT), e.ZCATEGORYTAG, e.ZNOTE,
       e.ZQUICKENID, e.ZTRANSFER
FROM ZCASHFLOWTRANSACTIONENTRY e
WHERE COALESCE(e.ZDELETIONCOUNT, 0) = 0
ORDER BY e.ZPARENT, e.Z_PK
`

// mapSplits reads every non-deleted ZCASHFLOWTRANSACTIONENTRY row. An entry
// with no imported parent transaction (NULL, dangling, deleted, Smart/Investment
// or itself excluded) is skipped, its category recorded in refs. An entry under
// an imported transaction whose amount is missing, stored as text or blob, has
// more than 2 decimals beyond the snap tolerance or is too large, is added to
// off and excluded. A category reference to a deleted or missing category, or
// to a PK in uncategorized, stores NULL.
func mapSplits(
	ctx context.Context, src Source, txns map[int64]txnRef, existingCategories, uncategorized map[int64]bool, refs *categoryRefs, off *offenders,
) ([]store.Split, []transferLink, map[int64]string, error) {
	var rows []store.Split
	var links []transferLink
	ids := make(map[int64]string)

	err := src.QueryRows(ctx, entriesQuery, nil, func(scan func(dest ...any) error) error {
		var pk int64
		var parent sql.NullInt64
		var amtType string
		var amtText sql.NullString
		var category sql.NullInt64
		var note sql.NullString
		var quickenID sql.NullInt64
		var transfer sql.NullString
		if err := scan(&pk, &parent, &amtType, &amtText, &category, &note, &quickenID, &transfer); err != nil {
			return err
		}

		if !parent.Valid {
			refs.add(category)
			return nil
		}
		txn, ok := txns[parent.Int64]
		if !ok {
			refs.add(category) // parent is missing, deleted, Smart/Investment, or itself excluded
			return nil
		}
		dateStr := txn.Date.Format(dateLayout)

		if amtType == "null" {
			off.add(offender{class: classMissingValue, reason: reasonSplitNoAmount(dateStr, txn.AccountName), dated: true, date: txn.Date, account: txn.AccountName, sourceID: pk})
			return nil
		}
		cents, fault := parseMoney(amtType, amtText.String)
		switch fault {
		case moneyNotANumber:
			off.add(offender{class: classNotANumber, reason: reasonSplitNotANumber(dateStr, txn.AccountName), dated: true, date: txn.Date, account: txn.AccountName, sourceID: pk})
			return nil
		case moneyPrecision:
			off.add(offender{class: classSplitPrecision, reason: reasonSplitPrecision(dateStr, txn.AccountName, amtText.String), dated: true, date: txn.Date, account: txn.AccountName, sourceID: pk})
			return nil
		case moneyTooLarge:
			off.add(offender{class: classTooLarge, reason: reasonSplitTooLarge(dateStr, txn.AccountName, amtText.String), dated: true, date: txn.Date, account: txn.AccountName, sourceID: pk})
			return nil
		case moneyOK:
		}

		id := fmt.Sprintf("split-%d", pk)
		split := store.Split{ID: id, SourceID: pk, TransactionID: txn.ID, Amount: cents}
		if category.Valid && existingCategories[category.Int64] && !uncategorized[category.Int64] {
			cid := fmt.Sprintf("cat-%d", category.Int64)
			split.CategoryID = &cid
		}
		if note.Valid && note.String != "" {
			memo := note.String
			split.Memo = &memo
		}
		rows = append(rows, split)
		links = append(links, transferLink{quickenID: quickenID, link: transfer.String}) // links[i] belongs to rows[i]
		ids[pk] = id
		return nil
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read splits: %w", err)
	}
	return rows, links, ids, nil
}

const splitTagsQuery = `SELECT Z_15CASHFLOWTRANSACTIONENTRIES, Z_76USERTAGS FROM Z_15USERTAGS`

// mapSplitTags reads Z_15USERTAGS, keeping only links whose split is in
// splitIDs (an entry whose transaction was skipped has no split to link)
// and whose tag exists (non-deleted); a link missing either end is dropped.
func mapSplitTags(ctx context.Context, src Source, splitIDs map[int64]string, existingTags map[int64]bool) ([]store.SplitTag, error) {
	var rows []store.SplitTag
	err := src.QueryRows(ctx, splitTagsQuery, nil, func(scan func(dest ...any) error) error {
		var entryPK, tagPK sql.NullInt64
		if err := scan(&entryPK, &tagPK); err != nil {
			return err
		}
		if !entryPK.Valid || !tagPK.Valid {
			return nil
		}
		splitID, ok := splitIDs[entryPK.Int64]
		if !ok || !existingTags[tagPK.Int64] {
			return nil
		}
		rows = append(rows, store.SplitTag{SplitID: splitID, TagID: fmt.Sprintf("tag-%d", tagPK.Int64)})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read split tags: %w", err)
	}
	return rows, nil
}
