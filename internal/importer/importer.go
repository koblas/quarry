package importer

import (
	"context"
	"fmt"
	"time"

	"github.com/koblas/quarry/internal/store"
)

// Server maps a Quicken v9 snapshot into quarry's schema and writes it
// through a Store.
type Server struct {
	store Store
	open  SourceOpener
}

// Option configures a Server built by NewServer.
type Option func(*Server)

// WithStore sets the Store Import writes the mapped rows into.
func WithStore(s Store) Option {
	return func(srv *Server) { srv.store = s }
}

// WithSourceOpener overrides how Import opens the snapshot path, in place
// of the default read-only SQLite opener.
func WithSourceOpener(open SourceOpener) Option {
	return func(srv *Server) { srv.open = open }
}

// NewServer builds a Server from opts, defaulting SourceOpener to a
// read-only SQLite open.
func NewServer(opts ...Option) *Server {
	srv := &Server{open: defaultOpener}
	for _, opt := range opts {
		opt(srv)
	}
	return srv
}

// Import maps snap.Path's v9 database into quarry's schema, validates
// the mapped rows, and writes them, with one import_runs row describing
// the build, through the configured Store only once every check passes.
// It returns an *UnmappableError naming the first-ordered class's first
// offender when one or more values cannot be mapped to quarry's schema, or
// store.ErrValidationFailed with Result.Built false, never calling Replace,
// when the balance or split-sum gate finds a mismatch.
func (srv *Server) Import(ctx context.Context, snap store.SnapshotRef) (store.Result, error) {
	startedAt := time.Now().UTC()
	src, err := srv.open(ctx, snap.Path)
	if err != nil {
		return store.Result{}, fmt.Errorf("open %s: %w", snap.Path, err)
	}
	defer func() { _ = src.Close() }()

	entities, err := resolveEntities(ctx, src)
	if err != nil {
		return store.Result{}, err
	}

	off := &offenders{}

	accounts, accountRefs, existingAccounts, err := mapAccounts(ctx, src, off)
	if err != nil {
		return store.Result{}, err
	}
	statements, err := newestStatements(ctx, src, accountRefs, off)
	if err != nil {
		return store.Result{}, err
	}
	categories, existingCategories, uncategorized, err := mapCategories(ctx, src, entities["CategoryTag"], off)
	if err != nil {
		return store.Result{}, err
	}
	tags, existingTags, err := mapTags(ctx, src, entities["UserTag"])
	if err != nil {
		return store.Result{}, err
	}
	payees, existingPayees, err := mapPayees(ctx, src)
	if err != nil {
		return store.Result{}, err
	}
	investmentEnt, hasInvestment := entities[investmentEntity]
	existingTransactions, investmentsNotImported, err := surveyTransactions(ctx, src, investmentEnt, hasInvestment, accountRefs)
	if err != nil {
		return store.Result{}, err
	}
	transactions, txnRefs, err := mapTransactions(
		ctx, src, entities["CashFlowTransaction"], accountRefs, existingAccounts, existingPayees, off)
	if err != nil {
		return store.Result{}, err
	}
	splits, links, splitIDs, err := mapSplits(ctx, src, txnRefs, existingTransactions, existingCategories, uncategorized, off)
	if err != nil {
		return store.Result{}, err
	}
	splitTags, err := mapSplitTags(ctx, src, splitIDs, existingTags)
	if err != nil {
		return store.Result{}, err
	}

	if err := off.firstError(); err != nil {
		return store.Result{}, err
	}

	transfers, transferCheck := pairTransfers(splits, links, transactions, accounts)
	if err := checkTransferTotals(transfers, transferCheck); err != nil {
		// unreachable: pairTransfers (transfers.go) has three rows appends (name-form leg, unmatched numeric link, paired legs), each with one OneSided append or one Paired++, so counts equal rows
		return store.Result{}, err
	}

	rows := store.Rows{
		Accounts: accounts, Categories: categories, Payees: payees, Tags: tags,
		Transactions: transactions, Splits: splits, SplitTags: splitTags, Transfers: transfers,
	}
	counts := store.Counts{
		Accounts: len(accounts), Categories: len(categories), Payees: len(payees), Tags: len(tags),
		Transactions: len(transactions), Splits: len(splits), SplitTags: len(splitTags), Transfers: len(transfers),
	}
	notImported := store.NotImported{InvestmentTransactions: investmentsNotImported}

	validation := validate(rows, statements, transferCheck)
	if validation.Failed() {
		return store.Result{Built: false, Counts: counts, Validation: validation, NotImported: notImported}, store.ErrValidationFailed
	}

	rows.ImportRuns = []store.ImportRun{newImportRun(startedAt, snap, counts, validation, notImported)}
	path, err := srv.store.Replace(ctx, rows)
	if err != nil {
		return store.Result{}, fmt.Errorf("replace store: %w", err)
	}

	return store.Result{Path: path, Built: true, Counts: counts, Validation: validation, NotImported: notImported}, nil
}

// importRunID is the import_runs id of a store's only run: every build
// writes a fresh store, so no earlier run is carried into it.
const importRunID = 1

// newImportRun describes a build that passed validation, stamping its
// finish time now, before the store is written.
func newImportRun(startedAt time.Time, snap store.SnapshotRef, counts store.Counts, v store.Validation, n store.NotImported) store.ImportRun {
	return store.ImportRun{
		ID: importRunID, StartedAt: startedAt, FinishedAt: time.Now().UTC(), Snapshot: snap, Counts: counts,
		BalancesChecked: v.Balances.Checked, BalancesMismatched: len(v.Balances.Mismatched),
		SplitsMismatched: len(v.Splits.Mismatched), TransfersOneSided: len(v.Transfers.OneSided),
		InvestmentTransactionsNotImported: n.InvestmentTransactions,
		BalancesNeverReconciled:           len(v.Balances.NeverReconciled),
		InvestmentAccounts:                v.Balances.InvestmentAccounts,
		TransfersPaired:                   v.Transfers.Paired,
		TransfersCrossCurrency:            v.Transfers.CrossCurrency,
	}
}
