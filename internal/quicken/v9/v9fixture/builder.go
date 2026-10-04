package v9fixture

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	EntSecurity                 = 66
	EntSecurityQuote            = 68
	EntPosition                 = 49
	EntLot                      = 44
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
	// UsedInReports is ZUSEDINREPORTS: nil writes NULL, 0 off, 1 on.
	UsedInReports *int64
	// SimpleInvesting is ZSIMPLEINVESTING (linked account tracking): nil
	// writes NULL, 0 off, 1 on.
	SimpleInvesting *int64
	// LoanInterestCategory is ZLOANINTERESTCATEGORY, a zero ref to a category
	// tag (0 writes NULL).
	LoanInterestCategory int64
}

// TransactionRow is one ZTRANSACTION row. Entity defaults to the Builder's
// CashFlowTransaction entity number when zero. Account and Payee are zero
// refs (0 writes NULL); Amount, Note, CheckNumber, Units, Numerator,
// Denominator and Commission write NULL when "". The decimal strings are
// bound as text, so SQLite's affinity picks integer, real or text.
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
	// ExcludeFromReports is ZEXCLUDEFROMREPORTS: nil writes NULL, 0 off, 1 on.
	ExcludeFromReports *int64
	// Type is ZTYPE, the investment action code: nil writes NULL.
	Type *int64
	// Position is ZPOSITION, a zero ref to a Builder.Position row.
	Position    int64
	Units       string // ZUNITS
	Numerator   string // ZNUMERATOR, a split's new shares
	Denominator string // ZDENOMINATOR, a split's old shares
	Commission  string // ZCOMMISSION
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

// BudgetLineItemRow is one ZBUDGETLINEITEM row. Category is a zero ref to a
// category tag (0 writes NULL).
type BudgetLineItemRow struct {
	Category int64
	Deleted  bool
}

// LoanSplitEntryRow is one ZLOANSPLITENTRY row. Category is a zero ref to a
// category tag (0 writes NULL).
type LoanSplitEntryRow struct {
	Category int64
	Deleted  bool
}

// QuickfillRuleSplitEntryRow is one ZQUICKFILLRULESPLITENTRY row. Category is
// a zero ref to a category tag (0 writes NULL).
type QuickfillRuleSplitEntryRow struct {
	Category int64
	Deleted  bool
}

// ProductServiceRow is one ZPRODUCTSERVICE row. Category is a zero ref to a
// category tag (0 writes NULL).
type ProductServiceRow struct {
	Category int64
	Deleted  bool
}

// CustomerCreditLineItemRow is one ZCUSTOMERCREDITLINEITEM row. Category is a
// zero ref to a category tag (0 writes NULL).
type CustomerCreditLineItemRow struct {
	Category int64
	Deleted  bool
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

// SecurityRow is one ZSECURITY row. Entity defaults to the Builder's Security
// entity number when zero. Name, Ticker and Currency write NULL when "".
type SecurityRow struct {
	Entity   int64
	Name     string
	Ticker   string
	Currency string
	Deleted  bool
}

// SecurityQuoteRow is one ZSECURITYQUOTE row. Entity defaults to the Builder's
// SecurityQuote entity number when zero. Security is a zero ref to a Security
// row (0 writes NULL); QuoteDate writes NULL when nil; ClosingPrice is a
// decimal string bound as text, so SQLite's affinity picks integer, real or
// text, and writes NULL when "".
type SecurityQuoteRow struct {
	Entity       int64
	Security     int64
	QuoteDate    *time.Time
	ClosingPrice string
	Deleted      bool
}

// PositionRow is one ZPOSITION row. Entity defaults to the Builder's Position
// entity number when zero. Account and Security are zero refs (0 writes NULL).
type PositionRow struct {
	Entity   int64
	Account  int64
	Security int64
	Deleted  bool
}

// LotRow is one ZLOT row. Entity defaults to the Builder's Lot entity number
// when zero. Position is a zero ref to a Position row (0 writes NULL);
// LatestUnits is ZLATESTUNITS, a decimal string bound as text so SQLite's
// affinity picks integer, real or text, and writes NULL when "".
type LotRow struct {
	Entity      int64
	Position    int64
	LatestUnits string
	Deleted     bool
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
	budgetLines  []pkRow[BudgetLineItemRow]
	loanSplits   []pkRow[LoanSplitEntryRow]
	quickfills   []pkRow[QuickfillRuleSplitEntryRow]
	products     []pkRow[ProductServiceRow]
	creditLines  []pkRow[CustomerCreditLineItemRow]
	tags         []pkRow[TagRow]
	payees       []pkRow[PayeeRow]
	institutions []pkRow[InstitutionRow]
	securities   []pkRow[SecurityRow]
	quotes       []pkRow[SecurityQuoteRow]
	positions    []pkRow[PositionRow]
	lots         []pkRow[LotRow]
	userTagLinks []userTagLink
}

// NewBuilder returns a Builder seeded with the reference schema's default
// Z_PRIMARYKEY entity numbers (EntCategoryTag, EntUserTag,
// EntCashFlowTransaction, EntSmartCashFlowTransaction,
// EntInvestmentTransaction, EntSecurity, EntSecurityQuote, EntPosition, EntLot),
// each overridable via WithEntity.
func NewBuilder() *Builder {
	return &Builder{
		entities: map[string]int64{
			"CategoryTag":              EntCategoryTag,
			"UserTag":                  EntUserTag,
			"CashFlowTransaction":      EntCashFlowTransaction,
			"SmartCashFlowTransaction": EntSmartCashFlowTransaction,
			"InvestmentTransaction":    EntInvestmentTransaction,
			"Security":                 EntSecurity,
			"SecurityQuote":            EntSecurityQuote,
			"Position":                 EntPosition,
			"Lot":                      EntLot,
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
// "SmartCashFlowTransaction", "InvestmentTransaction", "Security",
// "SecurityQuote", "Position" or "Lot".
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

// InvestmentTransaction adds row to ZTRANSACTION, Entity defaulting to the
// Builder's InvestmentTransaction entity number, and returns its Z_PK.
func (b *Builder) InvestmentTransaction(row TransactionRow) int64 {
	if row.Entity == 0 {
		row.Entity = b.entities["InvestmentTransaction"]
	}
	return b.Transaction(row)
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

// BudgetLineItem adds row and returns its assigned ZBUDGETLINEITEM.Z_PK.
func (b *Builder) BudgetLineItem(row BudgetLineItemRow) int64 {
	pk := b.nextPKFor("ZBUDGETLINEITEM")
	b.budgetLines = append(b.budgetLines, pkRow[BudgetLineItemRow]{pk: pk, row: row})
	return pk
}

// LoanSplitEntry adds row and returns its assigned ZLOANSPLITENTRY.Z_PK.
func (b *Builder) LoanSplitEntry(row LoanSplitEntryRow) int64 {
	pk := b.nextPKFor("ZLOANSPLITENTRY")
	b.loanSplits = append(b.loanSplits, pkRow[LoanSplitEntryRow]{pk: pk, row: row})
	return pk
}

// QuickfillRuleSplitEntry adds row and returns its assigned
// ZQUICKFILLRULESPLITENTRY.Z_PK.
func (b *Builder) QuickfillRuleSplitEntry(row QuickfillRuleSplitEntryRow) int64 {
	pk := b.nextPKFor("ZQUICKFILLRULESPLITENTRY")
	b.quickfills = append(b.quickfills, pkRow[QuickfillRuleSplitEntryRow]{pk: pk, row: row})
	return pk
}

// ProductService adds row and returns its assigned ZPRODUCTSERVICE.Z_PK.
func (b *Builder) ProductService(row ProductServiceRow) int64 {
	pk := b.nextPKFor("ZPRODUCTSERVICE")
	b.products = append(b.products, pkRow[ProductServiceRow]{pk: pk, row: row})
	return pk
}

// CustomerCreditLineItem adds row and returns its assigned
// ZCUSTOMERCREDITLINEITEM.Z_PK.
func (b *Builder) CustomerCreditLineItem(row CustomerCreditLineItemRow) int64 {
	pk := b.nextPKFor("ZCUSTOMERCREDITLINEITEM")
	b.creditLines = append(b.creditLines, pkRow[CustomerCreditLineItemRow]{pk: pk, row: row})
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

// Security adds row and returns its assigned ZSECURITY.Z_PK.
func (b *Builder) Security(row SecurityRow) int64 {
	if row.Entity == 0 {
		row.Entity = b.entities["Security"]
	}
	pk := b.nextPKFor("ZSECURITY")
	b.securities = append(b.securities, pkRow[SecurityRow]{pk: pk, row: row})
	return pk
}

// SecurityQuote adds row and returns its assigned ZSECURITYQUOTE.Z_PK.
func (b *Builder) SecurityQuote(row SecurityQuoteRow) int64 {
	if row.Entity == 0 {
		row.Entity = b.entities["SecurityQuote"]
	}
	pk := b.nextPKFor("ZSECURITYQUOTE")
	b.quotes = append(b.quotes, pkRow[SecurityQuoteRow]{pk: pk, row: row})
	return pk
}

// Position adds row and returns its assigned ZPOSITION.Z_PK.
func (b *Builder) Position(row PositionRow) int64 {
	if row.Entity == 0 {
		row.Entity = b.entities["Position"]
	}
	pk := b.nextPKFor("ZPOSITION")
	b.positions = append(b.positions, pkRow[PositionRow]{pk: pk, row: row})
	return pk
}

// Lot adds row and returns its assigned ZLOT.Z_PK.
func (b *Builder) Lot(row LotRow) int64 {
	if row.Entity == 0 {
		row.Entity = b.entities["Lot"]
	}
	pk := b.nextPKFor("ZLOT")
	b.lots = append(b.lots, pkRow[LotRow]{pk: pk, row: row})
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
// Builder's nine entity kinds, with Z_MAX set to the highest Z_PK Builder
// assigned that entity (0 when none were added).
func (b *Builder) Seed(tb testing.TB, db *sql.DB) {
	tb.Helper()
	ctx := context.Background()

	for _, a := range b.accounts {
		exec(tb, ctx, db,
			`INSERT INTO ZACCOUNT
				(Z_PK, ZNAME, ZTYPENAME, ZCURRENCY, ZFINANCIALINSTITUTION, ZCLOSED, ZACTIVE, ZDELETIONCOUNT, ZUSEDINREPORTS, ZSIMPLEINVESTING, ZLOANINTERESTCATEGORY)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			a.pk, nullableString(a.row.Name), nullableString(a.row.Type), nullableString(a.row.Currency),
			nullableRef(a.row.Institution), a.row.Closed, a.row.Active, deletionCount(a.row.Deleted),
			nullableInt(a.row.UsedInReports), nullableInt(a.row.SimpleInvesting), nullableRef(a.row.LoanInterestCategory))
	}

	for _, i := range b.institutions {
		exec(tb, ctx, db,
			"INSERT INTO ZFINANCIALINSTITUTION (Z_PK, ZNAME) VALUES (?, ?)",
			i.pk, i.row.Name)
	}

	for _, x := range b.transactions {
		exec(tb, ctx, db,
			`INSERT INTO ZTRANSACTION
				(Z_PK, Z_ENT, ZACCOUNT, ZAMOUNT, ZPOSTEDDATE, ZENTEREDDATE, ZRECONCILESTATUS, ZUSERPAYEE, ZNOTE, ZCHECKNUMBER, ZDELETIONCOUNT, ZEXCLUDEFROMREPORTS,
				 ZTYPE, ZPOSITION, ZUNITS, ZNUMERATOR, ZDENOMINATOR, ZCOMMISSION)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			x.pk, x.row.Entity, nullableRef(x.row.Account), nullableString(x.row.Amount), nullableTime(x.row.PostedDate), nullableTime(x.row.EnteredDate),
			nullableInt(x.row.Status), nullableRef(x.row.Payee), nullableString(x.row.Note), nullableString(x.row.CheckNumber), deletionCount(x.row.Deleted), nullableInt(x.row.ExcludeFromReports),
			nullableInt(x.row.Type), nullableRef(x.row.Position), nullableString(x.row.Units), nullableString(x.row.Numerator),
			nullableString(x.row.Denominator), nullableString(x.row.Commission))
	}

	for _, e := range b.entries {
		exec(tb, ctx, db,
			"INSERT INTO ZCASHFLOWTRANSACTIONENTRY (Z_PK, ZPARENT, ZAMOUNT, ZCATEGORYTAG, ZTRANSFER, ZQUICKENID, ZNOTE, ZDELETIONCOUNT) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			e.pk, nullableRef(e.row.Parent), nullableString(e.row.Amount), nullableRef(e.row.CategoryTag), nullableString(e.row.Transfer),
			e.row.QuickenID, nullableString(e.row.Note), deletionCount(e.row.Deleted))
	}

	for _, l := range b.budgetLines {
		exec(tb, ctx, db,
			"INSERT INTO ZBUDGETLINEITEM (Z_PK, ZCATEGORYTAG, ZDELETIONCOUNT) VALUES (?, ?, ?)",
			l.pk, nullableRef(l.row.Category), deletionCount(l.row.Deleted))
	}

	for _, l := range b.loanSplits {
		exec(tb, ctx, db,
			"INSERT INTO ZLOANSPLITENTRY (Z_PK, ZCATEGORY, ZDELETIONCOUNT) VALUES (?, ?, ?)",
			l.pk, nullableRef(l.row.Category), deletionCount(l.row.Deleted))
	}

	for _, q := range b.quickfills {
		exec(tb, ctx, db,
			"INSERT INTO ZQUICKFILLRULESPLITENTRY (Z_PK, ZCATEGORYTAG, ZDELETIONCOUNT) VALUES (?, ?, ?)",
			q.pk, nullableRef(q.row.Category), deletionCount(q.row.Deleted))
	}

	for _, p := range b.products {
		exec(tb, ctx, db,
			"INSERT INTO ZPRODUCTSERVICE (Z_PK, ZCATEGORY, ZDELETIONCOUNT) VALUES (?, ?, ?)",
			p.pk, nullableRef(p.row.Category), deletionCount(p.row.Deleted))
	}

	for _, c := range b.creditLines {
		exec(tb, ctx, db,
			"INSERT INTO ZCUSTOMERCREDITLINEITEM (Z_PK, ZCATEGORY, ZDELETIONCOUNT) VALUES (?, ?, ?)",
			c.pk, nullableRef(c.row.Category), deletionCount(c.row.Deleted))
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

	for _, sec := range b.securities {
		exec(tb, ctx, db,
			"INSERT INTO ZSECURITY (Z_PK, Z_ENT, ZNAME, ZTICKER, ZCURRENCY, ZDELETIONCOUNT) VALUES (?, ?, ?, ?, ?, ?)",
			sec.pk, sec.row.Entity, nullableString(sec.row.Name), nullableString(sec.row.Ticker), nullableString(sec.row.Currency), deletionCount(sec.row.Deleted))
	}

	for _, q := range b.quotes {
		exec(tb, ctx, db,
			"INSERT INTO ZSECURITYQUOTE (Z_PK, Z_ENT, ZSECURITY, ZQUOTEDATE, ZCLOSINGPRICE, ZDELETIONCOUNT) VALUES (?, ?, ?, ?, ?, ?)",
			q.pk, q.row.Entity, nullableRef(q.row.Security), nullableTime(q.row.QuoteDate), nullableString(q.row.ClosingPrice), deletionCount(q.row.Deleted))
	}

	for _, pos := range b.positions {
		exec(tb, ctx, db,
			"INSERT INTO ZPOSITION (Z_PK, Z_ENT, ZACCOUNT, ZSECURITY, ZDELETIONCOUNT) VALUES (?, ?, ?, ?, ?)",
			pos.pk, pos.row.Entity, nullableRef(pos.row.Account), nullableRef(pos.row.Security), deletionCount(pos.row.Deleted))
	}

	for _, lot := range b.lots {
		exec(tb, ctx, db,
			"INSERT INTO ZLOT (Z_PK, Z_ENT, ZPOSITION, ZLATESTUNITS, ZDELETIONCOUNT) VALUES (?, ?, ?, ?, ?)",
			lot.pk, lot.row.Entity, nullableRef(lot.row.Position), nullableString(lot.row.LatestUnits), deletionCount(lot.row.Deleted))
	}

	for _, link := range b.userTagLinks {
		exec(tb, ctx, db,
			`INSERT INTO "Z_15USERTAGS" (Z_15CASHFLOWTRANSACTIONENTRIES, Z_76USERTAGS) VALUES (?, ?)`,
			nullableRef(link.entryPK), nullableRef(link.tagPK))
	}

	for _, name := range []string{
		"CategoryTag", "UserTag", "CashFlowTransaction", "SmartCashFlowTransaction", "InvestmentTransaction",
		"Security", "SecurityQuote", "Position", "Lot",
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
	var maxPK int64
	switch name {
	case "CategoryTag", "UserTag":
		for _, g := range b.tags {
			if g.row.Entity == ent {
				maxPK = max(maxPK, g.pk)
			}
		}
	case "CashFlowTransaction", "SmartCashFlowTransaction", "InvestmentTransaction":
		for _, x := range b.transactions {
			if x.row.Entity == ent {
				maxPK = max(maxPK, x.pk)
			}
		}
	case "Security":
		for _, sec := range b.securities {
			if sec.row.Entity == ent {
				maxPK = max(maxPK, sec.pk)
			}
		}
	case "SecurityQuote":
		for _, q := range b.quotes {
			if q.row.Entity == ent {
				maxPK = max(maxPK, q.pk)
			}
		}
	case "Position":
		for _, pos := range b.positions {
			if pos.row.Entity == ent {
				maxPK = max(maxPK, pos.pk)
			}
		}
	case "Lot":
		for _, lot := range b.lots {
			if lot.row.Entity == ent {
				maxPK = max(maxPK, lot.pk)
			}
		}
	}
	return maxPK
}

//nolint:revive // testing.TB leads, by convention
func exec(tb testing.TB, ctx context.Context, db *sql.DB, query string, args ...any) {
	tb.Helper()
	_, err := db.ExecContext(ctx, query, args...)
	require.NoError(tb, err)
}

// WriteBundle creates a closed, non-WAL Home.quicken bundle under dir with
// v9.ReferenceDDL's schema and every row accumulated on Builder.
func (b *Builder) WriteBundle(tb testing.TB, dir string) Bundle {
	tb.Helper()

	bundleDir := filepath.Join(dir, "Home.quicken")
	require.NoError(tb, os.MkdirAll(bundleDir, 0o700))
	dataPath := filepath.Join(bundleDir, "data")
	writeReferenceSchema(tb, dataPath)

	conn, err := sql.Open("sqlite3", dataPath)
	require.NoError(tb, err)
	b.Seed(tb, conn)
	require.NoError(tb, conn.Close())

	return Bundle{Dir: bundleDir, DataPath: dataPath}
}
