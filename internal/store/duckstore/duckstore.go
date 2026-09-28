package duckstore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/store"
)

// FileName is quarry's store filename inside a Store's directory.
const FileName = "quarry.duckdb"

// partialNameLayout formats a partial build file's UTC timestamp, e.g. .quarry-20260927T143005Z.duckdb.partial.
const partialNameLayout = "20060102T150405Z"

// moneyWidth and moneyScale match schemaDDL's DECIMAL(18,2) money columns.
const moneyWidth, moneyScale = 18, 2

// Store builds quarry's DuckDB file inside one directory.
type Store struct {
	dir string
}

// New returns a Store that builds quarry.duckdb inside dir.
func New(dir string) *Store {
	return &Store{dir: dir}
}

// Replace builds rows into a new DuckDB file and swaps it in over
// quarry.duckdb, returning the path it wrote. On any failure the partial
// build file (and its .wal) is removed and the existing store, if any, is
// untouched: nothing is renamed until the build and its checkpoint both
// succeed.
func (s *Store) Replace(ctx context.Context, rows store.Rows) (string, error) {
	finalPath := filepath.Join(s.dir, FileName)
	partialPath := filepath.Join(s.dir, fmt.Sprintf(".quarry-%s.duckdb.partial", time.Now().UTC().Format(partialNameLayout)))

	db, err := duckdb.Create(ctx, partialPath)
	if err != nil {
		removePartial(partialPath)
		return "", fmt.Errorf("build store: %w", err)
	}

	if err := build(ctx, db, rows); err != nil {
		_ = db.Close()
		removePartial(partialPath)
		return "", fmt.Errorf("build store: %w", err)
	}

	if err := db.CheckpointClose(ctx); err != nil {
		// unreachable: CheckpointClose's own close/no-WAL failure paths are
		// unreachable in platform/duckdb already; its remaining fault (ctx
		// cancelled during CHECKPOINT) needs ctx cancelled between build and
		// this call, which share one ctx parameter with no race-free hook to
		// cancel only here.
		_ = db.Close()
		removePartial(partialPath)
		return "", fmt.Errorf("build store: %w", err)
	}

	if err := os.Rename(partialPath, finalPath); err != nil {
		removePartial(partialPath)
		return "", fmt.Errorf("build store: %w", err)
	}

	return finalPath, nil
}

// removePartial removes path and path+".wal", ignoring either being
// already absent.
func removePartial(path string) {
	_ = os.Remove(path)
	_ = os.Remove(path + ".wal")
}

// build creates quarry's schema in db and bulk-loads every table in rows.
func build(ctx context.Context, db *duckdb.DB, rows store.Rows) error {
	if _, err := db.Exec(ctx, schemaDDL); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}

	if err := db.AppendRows(ctx, "accounts", accountRows(rows.Accounts)); err != nil {
		return err
	}
	if err := db.AppendRows(ctx, "categories", categoryRows(rows.Categories)); err != nil {
		return err
	}
	if err := db.AppendRows(ctx, "payees", payeeRows(rows.Payees)); err != nil {
		return err
	}
	if err := db.AppendRows(ctx, "tags", tagRows(rows.Tags)); err != nil {
		return err
	}
	txnRows, err := transactionRows(rows.Transactions)
	if err != nil {
		return err
	}
	if err := db.AppendRows(ctx, "transactions", txnRows); err != nil {
		return err
	}
	splitRows, err := splitRows(rows.Splits)
	if err != nil {
		return err
	}
	if err := db.AppendRows(ctx, "splits", splitRows); err != nil {
		return err
	}
	if err := db.AppendRows(ctx, "split_tags", splitTagRows(rows.SplitTags)); err != nil {
		return err
	}
	return db.AppendRows(ctx, "transfers", transferRows(rows.Transfers))
}

func nullableStr(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func accountRows(accounts []store.Account) [][]any {
	out := make([][]any, len(accounts))
	for i, a := range accounts {
		out[i] = []any{a.ID, a.SourceID, a.Name, a.Type, a.Currency, nullableStr(a.Institution), a.Closed, a.Active}
	}
	return out
}

func categoryRows(categories []store.Category) [][]any {
	out := make([][]any, len(categories))
	for i, c := range categories {
		out[i] = []any{c.ID, c.SourceID, nullableStr(c.ParentID), c.Name, c.FullPath, c.Kind, c.Hidden}
	}
	return out
}

func payeeRows(payees []store.Payee) [][]any {
	out := make([][]any, len(payees))
	for i, p := range payees {
		out[i] = []any{p.ID, p.SourceID, p.Name}
	}
	return out
}

func tagRows(tags []store.Tag) [][]any {
	out := make([][]any, len(tags))
	for i, tg := range tags {
		out[i] = []any{tg.ID, tg.SourceID, tg.Name}
	}
	return out
}

func transactionRows(transactions []store.Transaction) ([][]any, error) {
	out := make([][]any, len(transactions))
	for i, t := range transactions {
		amount, err := duckdb.Decimal(t.Amount, moneyWidth, moneyScale)
		if err != nil {
			return nil, fmt.Errorf("transaction %s: %w", t.ID, err)
		}
		out[i] = []any{
			t.ID, t.SourceID, t.AccountID, t.Date, nullableStr(t.PayeeID), nullableStr(t.Memo),
			amount, t.Currency, t.Status, nullableStr(t.ChequeNumber),
		}
	}
	return out, nil
}

func splitRows(splits []store.Split) ([][]any, error) {
	out := make([][]any, len(splits))
	for i, s := range splits {
		amount, err := duckdb.Decimal(s.Amount, moneyWidth, moneyScale)
		if err != nil {
			return nil, fmt.Errorf("split %s: %w", s.ID, err)
		}
		out[i] = []any{
			s.ID, s.SourceID, s.TransactionID, nullableStr(s.CategoryID), amount, nullableStr(s.Memo), nullableStr(s.TransferAccountID),
		}
	}
	return out, nil
}

func splitTagRows(splitTags []store.SplitTag) [][]any {
	out := make([][]any, len(splitTags))
	for i, st := range splitTags {
		out[i] = []any{st.SplitID, st.TagID}
	}
	return out
}

func transferRows(transfers []store.Transfer) [][]any {
	out := make([][]any, len(transfers))
	for i, tr := range transfers {
		out[i] = []any{tr.ID, tr.FromSplitID, nullableStr(tr.ToSplitID), tr.CrossCurrency}
	}
	return out
}
