package importer

import (
	"context"
	"fmt"

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

// Import maps snapshotPath's v9 database into quarry's schema, validates
// the mapped rows, and writes them through the configured Store only once
// every check passes. It returns an *UnmappableError naming the
// first-ordered S4 class's first offender when one or more values cannot
// be mapped to quarry's schema, or store.ErrValidationFailed with
// Result.Built false, never calling Replace, when the balance or
// split-sum gate finds a mismatch.
func (srv *Server) Import(ctx context.Context, snapshotPath string) (store.Result, error) {
	src, err := srv.open(ctx, snapshotPath)
	if err != nil {
		return store.Result{}, fmt.Errorf("open %s: %w", snapshotPath, err)
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
	categories, existingCategories, err := mapCategories(ctx, src, entities["CategoryTag"], off)
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
	existingTransactions, err := existingTransactionPKs(ctx, src)
	if err != nil {
		return store.Result{}, err
	}
	transactions, txnRefs, err := mapTransactions(
		ctx, src, entities["CashFlowTransaction"], accountRefs, existingAccounts, existingPayees, off)
	if err != nil {
		return store.Result{}, err
	}
	splits, splitIDs, err := mapSplits(ctx, src, txnRefs, existingTransactions, existingCategories, off)
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

	rows := store.Rows{
		Accounts: accounts, Categories: categories, Payees: payees, Tags: tags,
		Transactions: transactions, Splits: splits, SplitTags: splitTags,
	}
	counts := store.Counts{
		Accounts: len(accounts), Categories: len(categories), Payees: len(payees), Tags: len(tags),
		Transactions: len(transactions), Splits: len(splits), SplitTags: len(splitTags),
	}

	validation := validate(rows, statements)
	if validation.Failed() {
		return store.Result{Built: false, Counts: counts, Validation: validation}, store.ErrValidationFailed
	}

	path, err := srv.store.Replace(ctx, rows)
	if err != nil {
		return store.Result{}, fmt.Errorf("replace store: %w", err)
	}

	return store.Result{Path: path, Built: true, Counts: counts, Validation: validation}, nil
}
