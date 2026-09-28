package v9fixture

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9"
	"github.com/stretchr/testify/require"
)

// Default Z_PRIMARYKEY entity numbers for the row kinds Builder seeds,
// overridable per Builder via WithEntity.
const (
	EntCategoryTag              = 75
	EntUserTag                  = 76
	EntCashFlowTransaction      = 79
	EntSmartCashFlowTransaction = 80
	EntInvestmentTransaction    = 81
)

// coreDataEpoch is Core Data's reference date: TIMESTAMP columns store
// seconds relative to this instant, not the Unix epoch.
var coreDataEpoch = time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)

// CoreDataEpochSeconds converts t to the value a v9 TIMESTAMP column
// stores: seconds since 2001-01-01 UTC.
func CoreDataEpochSeconds(t time.Time) float64 {
	return t.Sub(coreDataEpoch).Seconds()
}

// AccountRow is one ZACCOUNT row.
type AccountRow struct {
	Name     string
	Type     string // ZTYPENAME
	Currency string
	Closed   bool
	Active   bool
	Deleted  bool
}

// TransactionRow is one ZTRANSACTION row. Entity defaults to the Builder's
// CashFlowTransaction entity number when zero.
type TransactionRow struct {
	Entity      int64
	Account     int64
	Amount      string // decimal string bound as text
	PostedDate  *time.Time
	EnteredDate *time.Time
	Status      *int64 // ZRECONCILESTATUS: nil, 0 uncleared, 1 cleared, 2 reconciled
	Payee       int64
	Note        string
	CheckNumber string
	Deleted     bool
}

// EntryRow is one ZCASHFLOWTRANSACTIONENTRY row (a split of a transaction).
type EntryRow struct {
	Parent      int64
	Amount      string
	CategoryTag int64
	Transfer    string // ZTRANSFER: a counterpart ZQUICKENID as text, or an account name
	QuickenID   int64
	Note        string
	Deleted     bool
}

// ReconcileRow is one ZRECONCILERECORD row.
type ReconcileRow struct {
	Account       int64
	EndDate       time.Time
	EndingBalance string
	Deleted       bool
}

// TagRow is one ZTAG row: a category or a user tag, told apart by Entity.
type TagRow struct {
	Entity         int64
	Name           string
	Type           int64
	Hidden         bool
	ParentCategory int64
	Deleted        bool
}

// PayeeRow is one ZUSERPAYEE row.
type PayeeRow struct {
	Name    string
	Deleted bool
}

type pkRow[T any] struct {
	pk  int64
	row T
}

type userTagLink struct{ entryPK, tagPK int64 }

// Builder accumulates v9 fixture rows, assigning each a Z_PK on add. Seed
// or WriteBundle executes the accumulated rows against a database carrying
// v9.ReferenceDDL.
type Builder struct {
	entities map[string]int64
	nextPK   map[string]int64

	accounts     []pkRow[AccountRow]
	transactions []pkRow[TransactionRow]
	entries      []pkRow[EntryRow]
	reconciles   []pkRow[ReconcileRow]
	tags         []pkRow[TagRow]
	payees       []pkRow[PayeeRow]
	userTagLinks []userTagLink
}

// NewBuilder returns a Builder seeded with the reference schema's default
// Z_PRIMARYKEY entity numbers (EntCategoryTag, EntUserTag,
// EntCashFlowTransaction, EntSmartCashFlowTransaction,
// EntInvestmentTransaction), each overridable via WithEntity.
func NewBuilder() *Builder {
	return &Builder{
		entities: map[string]int64{
			"CategoryTag":              EntCategoryTag,
			"UserTag":                  EntUserTag,
			"CashFlowTransaction":      EntCashFlowTransaction,
			"SmartCashFlowTransaction": EntSmartCashFlowTransaction,
			"InvestmentTransaction":    EntInvestmentTransaction,
		},
		nextPK: make(map[string]int64),
	}
}

// WithEntity overrides name's Z_PRIMARYKEY entity number. name is one of
// "CategoryTag", "UserTag", "CashFlowTransaction",
// "SmartCashFlowTransaction" or "InvestmentTransaction".
func (b *Builder) WithEntity(name string, ent int64) *Builder {
	b.entities[name] = ent
	return b
}

func (b *Builder) nextPKFor(table string) int64 {
	b.nextPK[table]++
	return b.nextPK[table]
}

// Account adds row and returns its assigned ZACCOUNT.Z_PK.
func (b *Builder) Account(row AccountRow) int64 {
	pk := b.nextPKFor("ZACCOUNT")
	b.accounts = append(b.accounts, pkRow[AccountRow]{pk: pk, row: row})
	return pk
}

// Transaction adds row and returns its assigned ZTRANSACTION.Z_PK.
func (b *Builder) Transaction(row TransactionRow) int64 {
	if row.Entity == 0 {
		row.Entity = b.entities["CashFlowTransaction"]
	}
	pk := b.nextPKFor("ZTRANSACTION")
	b.transactions = append(b.transactions, pkRow[TransactionRow]{pk: pk, row: row})
	return pk
}

// Entry adds row and returns its assigned ZCASHFLOWTRANSACTIONENTRY.Z_PK.
func (b *Builder) Entry(row EntryRow) int64 {
	pk := b.nextPKFor("ZCASHFLOWTRANSACTIONENTRY")
	b.entries = append(b.entries, pkRow[EntryRow]{pk: pk, row: row})
	return pk
}

// Reconcile adds row and returns its assigned ZRECONCILERECORD.Z_PK.
func (b *Builder) Reconcile(row ReconcileRow) int64 {
	pk := b.nextPKFor("ZRECONCILERECORD")
	b.reconciles = append(b.reconciles, pkRow[ReconcileRow]{pk: pk, row: row})
	return pk
}

// Category adds row as a category tag (Entity defaults to the Builder's
// CategoryTag entity number) and returns its assigned ZTAG.Z_PK.
func (b *Builder) Category(row TagRow) int64 {
	if row.Entity == 0 {
		row.Entity = b.entities["CategoryTag"]
	}
	pk := b.nextPKFor("ZTAG")
	b.tags = append(b.tags, pkRow[TagRow]{pk: pk, row: row})
	return pk
}

// UserTag adds row as a user tag (Entity defaults to the Builder's UserTag
// entity number) and returns its assigned ZTAG.Z_PK.
func (b *Builder) UserTag(row TagRow) int64 {
	if row.Entity == 0 {
		row.Entity = b.entities["UserTag"]
	}
	pk := b.nextPKFor("ZTAG")
	b.tags = append(b.tags, pkRow[TagRow]{pk: pk, row: row})
	return pk
}

// LinkUserTag adds a Z_15USERTAGS row pairing entryPK (an EntryRow's Z_PK)
// with tagPK (a UserTag's Z_PK). The join table's own columns
// (Z_15CASHFLOWTRANSACTIONENTRIES, Z_76USERTAGS) are fixed by the schema's
// DDL and do not follow a WithEntity("UserTag", ...) override.
func (b *Builder) LinkUserTag(entryPK, tagPK int64) {
	b.userTagLinks = append(b.userTagLinks, userTagLink{entryPK: entryPK, tagPK: tagPK})
}

// Payee adds row and returns its assigned ZUSERPAYEE.Z_PK.
func (b *Builder) Payee(row PayeeRow) int64 {
	pk := b.nextPKFor("ZUSERPAYEE")
	b.payees = append(b.payees, pkRow[PayeeRow]{pk: pk, row: row})
	return pk
}

func deletionCount(deleted bool) int {
	if deleted {
		return 1
	}
	return 0
}

func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return CoreDataEpochSeconds(*t)
}

func nullableInt(n *int64) any {
	if n == nil {
		return nil
	}
	return *n
}

// Seed executes every row accumulated on Builder against db, which must
// already carry v9.ReferenceDDL, then writes a Z_PRIMARYKEY row for each of
// Builder's five entity kinds, with Z_MAX set to the highest Z_PK Builder
// assigned that entity (0 when none were added).
func (b *Builder) Seed(tb testing.TB, db *sql.DB) {
	tb.Helper()
	ctx := context.Background()

	for _, a := range b.accounts {
		exec(tb, ctx, db,
			"INSERT INTO ZACCOUNT (Z_PK, ZNAME, ZTYPENAME, ZCURRENCY, ZCLOSED, ZACTIVE, ZDELETIONCOUNT) VALUES (?, ?, ?, ?, ?, ?, ?)",
			a.pk, a.row.Name, a.row.Type, a.row.Currency, a.row.Closed, a.row.Active, deletionCount(a.row.Deleted))
	}

	for _, x := range b.transactions {
		exec(tb, ctx, db,
			`INSERT INTO ZTRANSACTION
				(Z_PK, Z_ENT, ZACCOUNT, ZAMOUNT, ZPOSTEDDATE, ZENTEREDDATE, ZRECONCILESTATUS, ZUSERPAYEE, ZNOTE, ZCHECKNUMBER, ZDELETIONCOUNT)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			x.pk, x.row.Entity, x.row.Account, x.row.Amount, nullableTime(x.row.PostedDate), nullableTime(x.row.EnteredDate),
			nullableInt(x.row.Status), x.row.Payee, x.row.Note, x.row.CheckNumber, deletionCount(x.row.Deleted))
	}

	for _, e := range b.entries {
		exec(tb, ctx, db,
			"INSERT INTO ZCASHFLOWTRANSACTIONENTRY (Z_PK, ZPARENT, ZAMOUNT, ZCATEGORYTAG, ZTRANSFER, ZQUICKENID, ZNOTE, ZDELETIONCOUNT) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			e.pk, e.row.Parent, e.row.Amount, e.row.CategoryTag, e.row.Transfer, e.row.QuickenID, e.row.Note, deletionCount(e.row.Deleted))
	}

	for _, r := range b.reconciles {
		exec(tb, ctx, db,
			"INSERT INTO ZRECONCILERECORD (Z_PK, ZACCOUNT, ZENDDATE, ZENDINGBALANCE, ZDELETIONCOUNT) VALUES (?, ?, ?, ?, ?)",
			r.pk, r.row.Account, CoreDataEpochSeconds(r.row.EndDate), r.row.EndingBalance, deletionCount(r.row.Deleted))
	}

	for _, g := range b.tags {
		exec(tb, ctx, db,
			"INSERT INTO ZTAG (Z_PK, Z_ENT, ZNAME, ZTYPE, ZHIDDEN, ZPARENTCATEGORY, ZDELETIONCOUNT) VALUES (?, ?, ?, ?, ?, ?, ?)",
			g.pk, g.row.Entity, g.row.Name, g.row.Type, g.row.Hidden, g.row.ParentCategory, deletionCount(g.row.Deleted))
	}

	for _, p := range b.payees {
		exec(tb, ctx, db,
			"INSERT INTO ZUSERPAYEE (Z_PK, ZNAME, ZDELETIONCOUNT) VALUES (?, ?, ?)",
			p.pk, p.row.Name, deletionCount(p.row.Deleted))
	}

	for _, link := range b.userTagLinks {
		exec(tb, ctx, db,
			`INSERT INTO "Z_15USERTAGS" (Z_15CASHFLOWTRANSACTIONENTRIES, Z_76USERTAGS) VALUES (?, ?)`,
			link.entryPK, link.tagPK)
	}

	for _, name := range []string{
		"CategoryTag", "UserTag", "CashFlowTransaction", "SmartCashFlowTransaction", "InvestmentTransaction",
	} {
		exec(tb, ctx, db,
			"INSERT INTO Z_PRIMARYKEY (Z_ENT, Z_NAME, Z_SUPER, Z_MAX) VALUES (?, ?, 0, ?)",
			b.entities[name], name, b.maxPKForEntity(name))
	}
}

// maxPKForEntity returns the highest Z_PK Builder assigned to name's Z_ENT
// number, or 0 when nothing carrying that entity was added.
func (b *Builder) maxPKForEntity(name string) int64 {
	ent := b.entities[name]
	var max int64
	switch name {
	case "CategoryTag", "UserTag":
		for _, g := range b.tags {
			if g.row.Entity == ent && g.pk > max {
				max = g.pk
			}
		}
	case "CashFlowTransaction", "SmartCashFlowTransaction", "InvestmentTransaction":
		for _, x := range b.transactions {
			if x.row.Entity == ent && x.pk > max {
				max = x.pk
			}
		}
	}
	return max
}

func exec(tb testing.TB, ctx context.Context, db *sql.DB, query string, args ...any) {
	tb.Helper()
	_, err := db.ExecContext(ctx, query, args...)
	require.NoError(tb, err)
}

// WriteBundle creates a closed, non-WAL Home.quicken bundle under dir with
// v9.ReferenceDDL's schema and every row accumulated on Builder.
func (b *Builder) WriteBundle(tb testing.TB, dir string) Bundle {
	tb.Helper()
	ctx := context.Background()

	bundleDir := filepath.Join(dir, "Home.quicken")
	require.NoError(tb, os.MkdirAll(bundleDir, 0o700))
	dataPath := filepath.Join(bundleDir, "data")

	conn, err := sql.Open("sqlite3", dataPath)
	require.NoError(tb, err)
	exec(tb, ctx, conn, v9.ReferenceDDL)
	b.Seed(tb, conn)
	require.NoError(tb, conn.Close())

	return Bundle{Dir: bundleDir, DataPath: dataPath}
}
