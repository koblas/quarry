package importer

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/koblas/quarry/internal/store"
)

const entriesQuery = `
SELECT e.Z_PK, e.ZPARENT, typeof(e.ZAMOUNT), CAST(e.ZAMOUNT AS TEXT), e.ZCATEGORYTAG, e.ZNOTE
FROM ZCASHFLOWTRANSACTIONENTRY e
WHERE COALESCE(e.ZDELETIONCOUNT, 0) = 0
ORDER BY e.ZPARENT, e.Z_PK
`

// mapSplits reads every non-deleted ZCASHFLOWTRANSACTIONENTRY row. An
// entry with no transaction (S4 reason 10) is added to off and excluded;
// one whose amount has too much precision (reason 4) or is too large
// (reason 6) likewise. An entry whose transaction is not in txns — a
// Smart/Investment transaction, or one itself excluded — is silently
// skipped, never refused.
func mapSplits(ctx context.Context, src Source, txns map[int64]txnRef, off *offenders) ([]store.Split, map[int64]string, error) {
	var rows []store.Split
	ids := make(map[int64]string)

	err := src.QueryRows(ctx, entriesQuery, nil, func(scan func(dest ...any) error) error {
		var pk int64
		var parent sql.NullInt64
		var amtType string
		var amtText sql.NullString
		var category sql.NullInt64
		var note sql.NullString
		if err := scan(&pk, &parent, &amtType, &amtText, &category, &note); err != nil {
			return err
		}

		if !parent.Valid {
			off.add(offender{class: 10, reason: reasonSplitNoTransaction(pk), name: fmt.Sprintf("(source id %d)", pk), sourceID: pk})
			return nil
		}
		txn, ok := txns[parent.Int64]
		if !ok {
			return nil
		}
		dateStr := txn.Date.Format(dateLayout)

		if amtType == "null" {
			off.add(offender{class: 10, reason: reasonSplitNoAmount(dateStr, txn.AccountName), dated: true, date: txn.Date, account: txn.AccountName, sourceID: pk})
			return nil
		}
		cents, fault := parseMoney(amtType, amtText.String)
		switch fault {
		case moneyPrecision:
			off.add(offender{class: 4, reason: reasonSplitPrecision(dateStr, txn.AccountName, amtText.String), dated: true, date: txn.Date, account: txn.AccountName, sourceID: pk})
			return nil
		case moneyTooLarge:
			off.add(offender{class: 6, reason: reasonSplitTooLarge(dateStr, txn.AccountName, amtText.String), dated: true, date: txn.Date, account: txn.AccountName, sourceID: pk})
			return nil
		}

		id := fmt.Sprintf("split-%d", pk)
		split := store.Split{ID: id, SourceID: pk, TransactionID: txn.ID, Amount: cents}
		if category.Valid {
			cid := fmt.Sprintf("cat-%d", category.Int64)
			split.CategoryID = &cid
		}
		if note.Valid && note.String != "" {
			memo := note.String
			split.Memo = &memo
		}
		rows = append(rows, split)
		ids[pk] = id
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("read splits: %w", err)
	}
	return rows, ids, nil
}

const splitTagsQuery = `SELECT Z_15CASHFLOWTRANSACTIONENTRIES, Z_76USERTAGS FROM Z_15USERTAGS`

// mapSplitTags reads Z_15USERTAGS, keeping only links whose split is in
// splitIDs (an entry whose transaction was skipped has no split to link).
func mapSplitTags(ctx context.Context, src Source, splitIDs map[int64]string) ([]store.SplitTag, error) {
	var rows []store.SplitTag
	err := src.QueryRows(ctx, splitTagsQuery, nil, func(scan func(dest ...any) error) error {
		var entryPK, tagPK int64
		if err := scan(&entryPK, &tagPK); err != nil {
			return err
		}
		splitID, ok := splitIDs[entryPK]
		if !ok {
			return nil
		}
		rows = append(rows, store.SplitTag{SplitID: splitID, TagID: fmt.Sprintf("tag-%d", tagPK)})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read split tags: %w", err)
	}
	return rows, nil
}
