package duckstore

import (
	"context"
	"database/sql"
	"errors"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/store"
)

// errNoImportRuns refuses a store whose import_runs holds no run; its text is the phrase a user reads.
var errNoImportRuns = errors.New("the store has no import history")

// statusQuery reads store_info, the import run, the transaction dates and the rate coverage in one pass; NULL check counts read as zero.
const statusQuery = `
SELECT i.format_version, i.quarry_version, i.built_at,
	r.id, r.started_at, r.finished_at, r.snapshot_path, r.snapshot_sha256, r.schema_fingerprint,
	r.accounts_rows, r.categories_rows, r.payees_rows, r.tags_rows,
	r.transactions_rows, r.splits_rows, r.split_tags_rows, r.transfers_rows,
	r.balances_checked, r.balances_mismatched, r.splits_mismatched, r.transfers_one_sided,
	r.snapshot_taken_at, r.source_path,
	COALESCE(r.balances_never_reconciled, 0), COALESCE(r.investment_accounts, 0),
	COALESCE(r.transfers_paired, 0), COALESCE(r.transfers_cross_currency, 0),
	COALESCE(r.securities_rows, 0), COALESCE(r.prices_rows, 0), COALESCE(r.investment_transactions_rows, 0), COALESCE(r.shares_checked, 0),
	(SELECT min(date) FROM transactions), (SELECT max(date) FROM transactions),
	(SELECT min(date) FROM fx_rates), (SELECT max(date) FROM fx_rates), r.rates_fetch_error
FROM store_info i CROSS JOIN import_runs r
ORDER BY r.id DESC LIMIT 1`

// statusFindingsQuery reads each finding's id, type and state without its items; new and newly fixed are at the build time.
const statusFindingsQuery = `
SELECT f.id, f.type, f.fixed_at,
	COALESCE(f.first_found_at = i.built_at, false), COALESCE(f.fixed_at = i.built_at, false)
FROM findings f CROSS JOIN store_info i
ORDER BY f.id`

// Status reads back what the store records about itself, from the highest-id
// import run. It refuses a store it cannot open or read, whose format is not
// this build's, or whose import_runs is empty, with *store.OpenError, and
// reads the findings and every account on the same connection so all describe one build; a NULL
// snapshot_taken_at or source_path reads as the zero value. Rates takes its
// first and last dates from fx_rates and its fetch error from that run, in the
// same query as the rest.
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
	var source, fetchError sql.NullString
	var firstRate, lastRate sql.NullTime
	found := false
	err = db.QueryRows(ctx, statusQuery, nil, func(scan func(dest ...any) error) error {
		found = true
		return scan(&st.FormatVersion, &st.QuarryVersion, &st.BuiltAt,
			&run.ID, &run.StartedAt, &run.FinishedAt, &run.Snapshot.Path, &run.Snapshot.SHA256, &run.Snapshot.SchemaFingerprint,
			&c.Accounts, &c.Categories, &c.Payees, &c.Tags, &c.Transactions, &c.Splits, &c.SplitTags, &c.Transfers,
			&run.BalancesChecked, &run.BalancesMismatched, &run.SplitsMismatched, &run.TransfersOneSided,
			&takenAt, &source,
			&run.BalancesNeverReconciled, &run.InvestmentAccounts, &run.TransfersPaired, &run.TransfersCrossCurrency,
			&c.Securities, &c.Prices, &c.InvestmentTransactions, &run.SharesChecked,
			&first, &last, &firstRate, &lastRate, &fetchError)
	})
	if err != nil {
		return store.Status{}, openFault(st.Path, err)
	}
	if !found {
		return store.Status{}, openFault(st.Path, errNoImportRuns)
	}

	err = db.QueryRows(ctx, statusFindingsQuery, nil, func(scan func(dest ...any) error) error {
		var f store.Finding
		var typ string
		var fixedAt sql.NullTime
		if err := scan(&f.ID, &typ, &fixedAt, &f.New, &f.NewlyFixed); err != nil {
			return err
		}
		f.Type = finding.Type(typ)
		if fixedAt.Valid {
			f.FixedAt = &fixedAt.Time
		}
		st.Findings = append(st.Findings, f)
		return nil
	})
	if err != nil {
		return store.Status{}, openFault(st.Path, err)
	}

	st.Accounts, err = readAccounts(ctx, db)
	if err != nil {
		return store.Status{}, openFault(st.Path, err)
	}

	run.Snapshot.TakenAt = takenAt.Time
	run.Snapshot.Source = source.String
	st.FirstDate, st.LastDate = first.Time, last.Time
	st.Rates = store.StatusRates{First: firstRate.Time, Last: lastRate.Time, FetchError: fetchError.String}
	return st, nil
}
