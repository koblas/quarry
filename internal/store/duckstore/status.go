package duckstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/koblas/quarry/internal/store"
)

// errImportRunCount refuses a store that does not pair one import run with one store_info row.
var errImportRunCount = errors.New("expected exactly one import run")

// statusQuery reads store_info, the import run and the transaction dates; NULL check counts read as zero.
const statusQuery = `
SELECT i.format_version, i.quarry_version, i.built_at,
	r.id, r.started_at, r.finished_at, r.snapshot_path, r.snapshot_sha256, r.schema_fingerprint,
	r.accounts_rows, r.categories_rows, r.payees_rows, r.tags_rows,
	r.transactions_rows, r.splits_rows, r.split_tags_rows, r.transfers_rows,
	r.balances_checked, r.balances_mismatched, r.splits_mismatched, r.transfers_one_sided,
	r.investment_transactions_not_imported,
	r.snapshot_taken_at, r.source_path,
	COALESCE(r.balances_never_reconciled, 0), COALESCE(r.investment_accounts, 0),
	COALESCE(r.transfers_paired, 0), COALESCE(r.transfers_cross_currency, 0),
	(SELECT min(date) FROM transactions), (SELECT max(date) FROM transactions)
FROM store_info i CROSS JOIN import_runs r`

// Status reads back what the store records about itself. It refuses a store
// it cannot open or read, whose format is not this build's, or that holds
// other than one import run, with *store.OpenError; a NULL
// snapshot_taken_at or source_path reads as the zero value.
func (s *Store) Status(ctx context.Context) (store.Status, error) {
	db, err := s.openRead(ctx)
	if err != nil {
		return store.Status{}, err
	}
	defer func() { _ = db.Close() }()

	st := store.Status{Path: s.Path()}
	run := &st.Run
	c := &run.Counts
	var takenAt, first, last sql.NullTime
	var source sql.NullString
	found := 0
	err = db.QueryRows(ctx, statusQuery, nil, func(scan func(dest ...any) error) error {
		found++
		return scan(&st.FormatVersion, &st.QuarryVersion, &st.BuiltAt,
			&run.ID, &run.StartedAt, &run.FinishedAt, &run.Snapshot.Path, &run.Snapshot.SHA256, &run.Snapshot.SchemaFingerprint,
			&c.Accounts, &c.Categories, &c.Payees, &c.Tags, &c.Transactions, &c.Splits, &c.SplitTags, &c.Transfers,
			&run.BalancesChecked, &run.BalancesMismatched, &run.SplitsMismatched, &run.TransfersOneSided,
			&run.InvestmentTransactionsNotImported,
			&takenAt, &source,
			&run.BalancesNeverReconciled, &run.InvestmentAccounts, &run.TransfersPaired, &run.TransfersCrossCurrency,
			&first, &last)
	})
	if err != nil {
		return store.Status{}, openFault(st.Path, err)
	}
	if found != 1 {
		return store.Status{}, openFault(st.Path, fmt.Errorf("%w, found %d", errImportRunCount, found))
	}

	run.Snapshot.TakenAt = takenAt.Time
	run.Snapshot.Source = source.String
	st.FirstDate, st.LastDate = first.Time, last.Time
	return st, nil
}
