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

// Import maps snapshotPath's v9 database into quarry's schema and writes it
// through the configured Store, returning the store.Result Replace
// produced. Every check runs before any refusal (P1-6): it returns an
// *UnmappableError naming the first-ordered S4 class's first offender when
// one or more values cannot be mapped to quarry's schema.
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

	accounts, accountRefs, err := mapAccounts(ctx, src, off)
	if err != nil {
		return store.Result{}, err
	}
	categories, err := mapCategories(ctx, src, entities["CategoryTag"], off)
	if err != nil {
		return store.Result{}, err
	}
	tags, err := mapTags(ctx, src, entities["UserTag"])
	if err != nil {
		return store.Result{}, err
	}
	payees, err := mapPayees(ctx, src)
	if err != nil {
		return store.Result{}, err
	}
	transactions, txnRefs, err := mapTransactions(ctx, src, entities["CashFlowTransaction"], accountRefs, off)
	if err != nil {
		return store.Result{}, err
	}
	splits, splitIDs, err := mapSplits(ctx, src, txnRefs, off)
	if err != nil {
		return store.Result{}, err
	}
	splitTags, err := mapSplitTags(ctx, src, splitIDs)
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
	path, err := srv.store.Replace(ctx, rows)
	if err != nil {
		return store.Result{}, fmt.Errorf("replace store: %w", err)
	}

	return store.Result{
		Path: path,
		Counts: store.Counts{
			Accounts: len(accounts), Categories: len(categories), Payees: len(payees), Tags: len(tags),
			Transactions: len(transactions), Splits: len(splits), SplitTags: len(splitTags),
		},
	}, nil
}
