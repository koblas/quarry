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

// AccountRow is one ZACCOUNT row. Name, Type and Currency write SQL NULL
// when "" (a real v9 file never has 0 or "" for a zero ref or a required
// value — those are Go zero values standing in for NULL). Institution is a
// zero ref: 0 writes NULL (no financial institution).
type AccountRow struct {
	Name        string
	Type        string // ZTYPENAME
	Currency    string
	Institution int64 // zero ref to a Builder.Institution row
	Closed      bool
	Active      bool
	Deleted     bool
}

// TransactionRow is one ZTRANSACTION row. Entity defaults to the Builder's
// CashFlowTransaction entity number when zero. Account and Payee are zero
// refs (0 writes NULL); Amount, Note and CheckNumber write NULL when "".
type TransactionRow struct {
	Entity      int64
	Account     int64
	Amount      string // decimal string bound as text; "" writes NULL
	PostedDate  *time.Time
	EnteredDate *time.Time
	Status      *int64 // ZRECONCILESTATUS: nil, 0 uncleared, 1 cleared, 2 reconciled
	Payee       int64
	Note        string
	CheckNumber string
	Deleted     bool
}

// EntryRow is one ZCASHFLOWTRANSACTIONENTRY row (a split of a transaction).
// Parent and CategoryTag are zero refs (0 writes NULL); Amount, Transfer and
// Note write NULL when "".
type EntryRow struct {
	Parent      int64
	Amount      string
	CategoryTag int64
	Transfer    string // ZTRANSFER: a counterpart ZQUICKENID as text, or an account name
	QuickenID   int64
	Note        string
	Deleted     bool
}

// ReconcileRow is one ZRECONCILERECORD row. Account is a zero ref (0 writes
// NULL); EndDate writes NULL when nil; EndingBalance writes NULL when "".
type ReconcileRow struct {
	Account       int64
	EndDate       *time.Time
	EndingBalance string
	Deleted       bool
}

// TagRow is one ZTAG row: a category or a user tag, told apart by Entity.
// ParentCategory is a zero ref (0 writes NULL). Type is a pointer because 0
// is a valid ZTYPE (system); nil is the only way to request NULL.
type TagRow struct {
	Entity         int64
	Name           string
	Type           *int64
	Hidden         bool
	ParentCategory int64
	Deleted        bool
}

// PayeeRow is one ZUSERPAYEE row.
type PayeeRow struct {
	Name    string
	Deleted bool
}

// InstitutionRow is one ZFINANCIALINSTITUTION row.
type InstitutionRow struct {
	Name string
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
	omitted  map[string]bool
	nextPK   map[string]int64

	accounts     []pkRow[AccountRow]
	transactions []pkRow[TransactionRow]
	entries      []pkRow[EntryRow]
	reconciles   []pkRow[ReconcileRow]
	tags         []pkRow[TagRow]
	payees       []pkRow[PayeeRow]
	institutions []pkRow[InstitutionRow]
	userTagLinks []userTagLink
}

// Int64Ptr returns a pointer to v, for fixture fields (such as
// TagRow.Type) that must distinguish a present zero value from NULL.
func Int64Ptr(v int64) *int64 { return &v }

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
		omitted: make(map[string]bool),
		nextPK:  make(map[string]int64),
	}
}

// WithoutEntity omits name's Z_PRIMARYKEY row entirely, simulating a
// snapshot whose schema carries an entity kind quarry cannot resolve.
func (b *Builder) WithoutEntity(name string) *Builder {
	b.omitted[name] = true
	return b
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
// with tagPK (a UserTag's Z_PK); either as 0 writes NULL. The join table's own columns
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

// Institution adds row and returns its assigned ZFINANCIALINSTITUTION.Z_PK,
// for use as an AccountRow.Institution ref.
func (b *Builder) Institution(row InstitutionRow) int64 {
	pk := b.nextPKFor("ZFINANCIALINSTITUTION")
	b.institutions = append(b.institutions, pkRow[InstitutionRow]{pk: pk, row: row})
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

// nullableString stands in for a v9 column that has no way to be "" in
// real data: "" is the Go zero value a fixture uses to mean NULL.
func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// nullableRef stands in for a zero ref (a foreign key field whose Go zero
// value, 0, means "no row"): real v9 stores NULL there, never 0.
func nullableRef(ref int64) any {
	if ref == 0 {
		return nil
	}
	return ref
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
			"INSERT INTO ZACCOUNT (Z_PK, ZNAME, ZTYPENAME, ZCURRENCY, ZFINANCIALINSTITUTION, ZCLOSED, ZACTIVE, ZDELETIONCOUNT) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			a.pk, nullableString(a.row.Name), nullableString(a.row.Type), nullableString(a.row.Currency),
			nullableRef(a.row.Institution), a.row.Closed, a.row.Active, deletionCount(a.row.Deleted))
	}

	for _, i := range b.institutions {
		exec(tb, ctx, db,
			"INSERT INTO ZFINANCIALINSTITUTION (Z_PK, ZNAME) VALUES (?, ?)",
			i.pk, i.row.Name)
	}

	for _, x := range b.transactions {
		exec(tb, ctx, db,
			`INSERT INTO ZTRANSACTION
				(Z_PK, Z_ENT, ZACCOUNT, ZAMOUNT, ZPOSTEDDATE, ZENTEREDDATE, ZRECONCILESTATUS, ZUSERPAYEE, ZNOTE, ZCHECKNUMBER, ZDELETIONCOUNT)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			x.pk, x.row.Entity, nullableRef(x.row.Account), nullableString(x.row.Amount), nullableTime(x.row.PostedDate), nullableTime(x.row.EnteredDate),
			nullableInt(x.row.Status), nullableRef(x.row.Payee), nullableString(x.row.Note), nullableString(x.row.CheckNumber), deletionCount(x.row.Deleted))
	}

	for _, e := range b.entries {
		exec(tb, ctx, db,
			"INSERT INTO ZCASHFLOWTRANSACTIONENTRY (Z_PK, ZPARENT, ZAMOUNT, ZCATEGORYTAG, ZTRANSFER, ZQUICKENID, ZNOTE, ZDELETIONCOUNT) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			e.pk, nullableRef(e.row.Parent), nullableString(e.row.Amount), nullableRef(e.row.CategoryTag), nullableString(e.row.Transfer),
			e.row.QuickenID, nullableString(e.row.Note), deletionCount(e.row.Deleted))
	}

	for _, r := range b.reconciles {
		exec(tb, ctx, db,
			"INSERT INTO ZRECONCILERECORD (Z_PK, ZACCOUNT, ZENDDATE, ZENDINGBALANCE, ZDELETIONCOUNT) VALUES (?, ?, ?, ?, ?)",
			r.pk, nullableRef(r.row.Account), nullableTime(r.row.EndDate), nullableString(r.row.EndingBalance), deletionCount(r.row.Deleted))
	}

	for _, g := range b.tags {
		exec(tb, ctx, db,
			"INSERT INTO ZTAG (Z_PK, Z_ENT, ZNAME, ZTYPE, ZHIDDEN, ZPARENTCATEGORY, ZDELETIONCOUNT) VALUES (?, ?, ?, ?, ?, ?, ?)",
			g.pk, g.row.Entity, nullableString(g.row.Name), nullableInt(g.row.Type), g.row.Hidden, nullableRef(g.row.ParentCategory), deletionCount(g.row.Deleted))
	}

	for _, p := range b.payees {
		exec(tb, ctx, db,
			"INSERT INTO ZUSERPAYEE (Z_PK, ZNAME, ZDELETIONCOUNT) VALUES (?, ?, ?)",
			p.pk, p.row.Name, deletionCount(p.row.Deleted))
	}

	for _, link := range b.userTagLinks {
		exec(tb, ctx, db,
			`INSERT INTO "Z_15USERTAGS" (Z_15CASHFLOWTRANSACTIONENTRIES, Z_76USERTAGS) VALUES (?, ?)`,
			nullableRef(link.entryPK), nullableRef(link.tagPK))
	}

	for _, name := range []string{
		"CategoryTag", "UserTag", "CashFlowTransaction", "SmartCashFlowTransaction", "InvestmentTransaction",
	} {
		if b.omitted[name] {
			continue
		}
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
