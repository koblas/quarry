package store

import (
	"errors"
	"slices"
	"time"

	"github.com/koblas/quarry/internal/finding"
)

// Account is one row of the accounts table.
type Account struct {
	ID          string
	SourceID    int64
	Name        string
	Type        string
	Currency    string
	Institution *string
	Closed      bool
	Active      bool
	// NotInReports is true when Quicken leaves the account out of its
	// reports; the zero value means the account is in reports.
	NotInReports bool
	// LinkedTracking is true when the account uses Quicken's linked account
	// tracking; the zero value means it does not.
	LinkedTracking bool
}

// LeftOutOfReports reports whether spend and cashflow leave the account out, as
// Quicken's reports do: not in reports, or using linked account tracking.
// duckstore's reportedAccount is the SQL form of its negation.
func (a Account) LeftOutOfReports() bool {
	return a.NotInReports || a.LinkedTracking
}

// Investment account types, whose balance quarry cannot compute.
const (
	AccountTypeBrokerage  = "brokerage"
	AccountTypeRetirement = "retirement"
)

// InvestmentAccountTypes returns every accounts.type value
// IsInvestmentAccount accepts, as a fresh slice.
func InvestmentAccountTypes() []string {
	return []string{AccountTypeBrokerage, AccountTypeRetirement}
}

// IsInvestmentAccount reports whether accountType is a brokerage or
// retirement account: sync never checks its balance, and accounts shows it
// as not imported.
func IsInvestmentAccount(accountType string) bool {
	return slices.Contains(InvestmentAccountTypes(), accountType)
}

// Category is one row of the categories table. ParentID is nil for a
// top-level category; FullPath joins every ancestor's name with Category's
// own, separated by ":".
type Category struct {
	ID       string
	SourceID int64
	ParentID *string
	Name     string
	FullPath string
	Kind     string
	Hidden   bool
}

// Payee is one row of the payees table, recorded as Quicken has it — never
// merged with another payee of the same name.
type Payee struct {
	ID       string
	SourceID int64
	Name     string
}

// Tag is one row of the tags table (a Quicken user tag).
type Tag struct {
	ID       string
	SourceID int64
	Name     string
}

// Transaction is one row of the transactions table. Amount is DECIMAL(18,2)
// in native Currency, expressed as cents (int64) — never a float.
type Transaction struct {
	ID           string
	SourceID     int64
	AccountID    string
	Date         time.Time
	PayeeID      *string
	Memo         *string
	Amount       int64
	Currency     string
	Status       string
	ChequeNumber *string
	// ExcludedFromReports is true when Quicken leaves the transaction out
	// of its reports.
	ExcludedFromReports bool
	// PostedDate is the bank's posting day, set whenever Quicken holds one,
	// even when it equals Date.
	PostedDate *time.Time
}

// Split is one row of the splits table, a share of its Transaction's amount
// assigned to a category. TransferAccountID is set when the split is one
// leg of a transfer between the user's own accounts.
type Split struct {
	ID                string
	SourceID          int64
	TransactionID     string
	CategoryID        *string
	Amount            int64
	Memo              *string
	TransferAccountID *string
}

// Transfer is one row of the transfers table: a pair of split legs between
// the user's own accounts, or a one-sided leg when ToSplitID is nil.
// FromSplitID is the leg with the lower numeric source id, not the leg the
// money left from.
type Transfer struct {
	ID            string
	FromSplitID   string
	ToSplitID     *string
	CrossCurrency bool
}

// SplitTag links one split to one tag (the split_tags table).
type SplitTag struct {
	SplitID string
	TagID   string
}

// Rows is every row a store build writes, grouped by table. ImportRuns holds
// the new build's run only; the store carries earlier runs forward itself.
type Rows struct {
	Accounts     []Account
	Categories   []Category
	Payees       []Payee
	Tags         []Tag
	Transactions []Transaction
	Splits       []Split
	SplitTags    []SplitTag
	Transfers    []Transfer
	ImportRuns   []ImportRun
}

// SnapshotRef identifies the snapshot a build reads: its absolute Path,
// its SHA-256, and its schema fingerprint, all as its manifest records them.
// TakenAt (UTC) and Source are the manifest's recorded values, zero unless
// filled from a manifest.
type SnapshotRef struct {
	Path, SHA256, SchemaFingerprint string
	TakenAt                         time.Time
	Source                          string
}

// ImportRun is one row of the import_runs table, describing the build that
// wrote it. FinishedAt is stamped before the store is written and swapped
// in; both times are UTC. A build hands ImportRun.ID unset: the store numbers it.
type ImportRun struct {
	ID                                int64
	StartedAt, FinishedAt             time.Time
	Snapshot                          SnapshotRef
	Counts                            Counts
	BalancesChecked                   int
	BalancesMismatched                int
	SplitsMismatched                  int
	TransfersOneSided                 int
	InvestmentTransactionsNotImported int
	BalancesNeverReconciled           int
	InvestmentAccounts                int
	TransfersPaired                   int
	TransfersCrossCurrency            int
}

// Status is what a built store says about itself: its path, the format and
// build that wrote it, its import run and the dates its transactions cover.
// A zero Run.Snapshot.TakenAt or empty Source means NULL was recorded; the
// dates are zero when there are no transactions.
type Status struct {
	Path                string
	FormatVersion       int
	QuarryVersion       string
	BuiltAt             time.Time
	Run                 ImportRun
	FirstDate, LastDate time.Time
}

// Counts is the row count of each table after a build; Transfers counts
// paired and one-sided rows alike, and import_runs is not counted.
type Counts struct {
	Accounts     int
	Categories   int
	Payees       int
	Tags         int
	Transactions int
	Splits       int
	SplitTags    int
	Transfers    int
}

// Result is what a store build returns. Built is false when a check failed:
// Path is then empty (Replace never ran) but Counts, Validation and
// NotImported still describe the rows the build would have written.
// HistoryFault is why the previous store's import runs were not carried
// into a built store; nil when they were, or no store existed. Findings
// counts what the build's detection recorded, zero when Built is false;
// FindingsCarried is true iff the previous store's findings were read.
type Result struct {
	Path         string
	Built        bool
	Counts       Counts
	Validation   Validation
	NotImported  NotImported
	HistoryFault *OpenError
	Findings     finding.Counts

	FindingsCarried bool
}

// Replaced is what Store.Replace reports: the path it wrote, the fault that
// kept the previous store's import runs from being carried, if any, the counts
// of the findings it recorded, and whether the previous store's findings were carried.
type Replaced struct {
	Path         string
	HistoryFault *OpenError
	Findings     finding.Counts

	FindingsCarried bool
}

// NotImported counts source rows a build deliberately leaves out of the
// store.
type NotImported struct {
	InvestmentTransactions int
}

// ErrValidationFailed is Import's error when a build's checks find a mismatch.
var ErrValidationFailed = errors.New("validation failed")

// ErrStoreNotWritable matches a build error caused by a permission fault writing the store.
var ErrStoreNotWritable = errors.New("store not writable")

// ErrDiskFull matches a build error caused by the disk or quota filling up.
var ErrDiskFull = errors.New("disk full")

// ErrUnmappable matches an import error for a source value quarry cannot map.
var ErrUnmappable = errors.New("unmappable value")

// Validation is the outcome of every check a build runs on its mapped rows,
// before the store is swapped in.
type Validation struct {
	Balances  BalanceCheck
	Splits    SplitCheck
	Transfers TransferCheck
}

// Failed reports whether the balance or split-sum check found a mismatch.
// Transfers never fail a build: a one-sided leg is stored, not refused.
func (v Validation) Failed() bool {
	return len(v.Balances.Mismatched) > 0 || len(v.Splits.Mismatched) > 0
}

// BalanceCheck is the balance gate's result across every non-investment
// account: each reconciled account's imported transactions with status
// reconciled are compared to its last reconciled statement's balance.
type BalanceCheck struct {
	Checked            int
	Mismatched         []BalanceMismatch
	NeverReconciled    []Account
	InvestmentAccounts int
}

// BalanceMismatch is one reconciled account whose reconciled-transaction
// sum does not equal its last reconciled statement's balance, in cents.
// Difference is Quarry minus Quicken. SourceID orders display only; it is
// never rendered.
type BalanceMismatch struct {
	ID, Name, Currency          string
	SourceID                    int64
	Closed, Active              bool
	StatementDate               time.Time
	Quarry, Quicken, Difference int64
}

// SplitCheck is the split-sum gate's result across every imported
// transaction.
type SplitCheck struct {
	Checked    int
	Mismatched []SplitMismatch
}

// SplitMismatch is one transaction whose splits do not sum to its amount,
// in cents. Payee is "" when the transaction has none. SourceID orders
// display only, and Closed/Active label the account for display only;
// none of the three is emitted in --json.
type SplitMismatch struct {
	ID, Account, Currency, Payee string
	SourceID                     int64
	Closed, Active               bool
	Date                         time.Time
	Amount, SplitsTotal          int64
}

// TransferCheck is the transfer-pairing result. CrossCurrency counts pairs
// whose legs' accounts differ in currency; OneSided lists every stored leg
// with no counterpart.
type TransferCheck struct {
	Paired, CrossCurrency int
	OneSided              []OneSidedTransfer
}

// OneSidedTransfer is one transfer leg with no counterpart: its
// transaction's Date and Payee ("" when none), its account's name and
// currency, and the leg's own split Amount in cents. OtherAccount is the
// account name the leg records, nil for a numeric link; OtherAccountID is
// the imported account that name matches, nil when none does. SourceID
// (the split's) orders display only, and Closed/Active label the account
// for display only; none of the three is emitted in --json.
type OneSidedTransfer struct {
	ID                string
	SourceID          int64
	Date              time.Time
	Account, Currency string
	Closed, Active    bool
	Payee             string
	Amount            int64
	OtherAccount      *string
	OtherAccountID    *string
}

// AccountBalance is one account with its balance in cents; Balance is nil
// when the store cannot compute it.
type AccountBalance struct {
	Account

	Balance *int64
}

// AccountList is every account with its balance as of the store's today.
type AccountList struct {
	AsOf     time.Time
	Accounts []AccountBalance
}

// Window is an inclusive range of civil days: Since and Until are each a
// calendar day held as UTC midnight, and both days count.
type Window struct {
	Since, Until time.Time
}

// SpendingGroup names what a spending read groups its rows by.
type SpendingGroup int

// The spending groupings.
const (
	// SpendByCategory groups by the split's category full path.
	SpendByCategory SpendingGroup = iota
	// SpendByPayee groups by the transaction's payee name.
	SpendByPayee
	// SpendByTag groups by tag name; a split with several tags counts under
	// each of them, and once in the Totals.
	SpendByTag
	// SpendByMonth groups by calendar month; the key is the month as
	// YYYY-MM and is never nil.
	SpendByMonth
)

// SpendingParams is everything a spending read varies by: the Window,
// the grouping, and the accounts to count (every account in reports when
// AccountIDs is empty).
type SpendingParams struct {
	Window     Window
	By         SpendingGroup
	AccountIDs []string
}

// SpendingRow is one group's spending in one currency, in cents. Key is nil
// for the group of splits with no category (or no payee); a month row's Key is
// never nil.
type SpendingRow struct {
	Key      *string
	Currency string
	Spent    int64
}

// SpendingTotal is all the spending in one currency, in cents.
type SpendingTotal struct {
	Currency string
	Spent    int64
}

// TransactionRange is the first and last day of a set of transactions, each a
// calendar day held as UTC midnight; the zero value means there are none.
type TransactionRange struct {
	First, Last time.Time
}

// Spending is the rows of a spending read in display order, and one Total
// per currency present, CAD before USD.
type Spending struct {
	Rows   []SpendingRow
	Totals []SpendingTotal
	// MultiTagSplits counts the splits in the window that carry more than one
	// tag; it is set only when grouping by tag.
	MultiTagSplits int
	// Transactions is set only when there are no Totals: the span of the store's transactions, or
	// of the in-report accounts among SpendingParams.AccountIDs; zero means there are none.
	Transactions TransactionRange
}

// CashFlowPeriod names the calendar unit a cash-flow read groups its rows by.
type CashFlowPeriod int

// The cash-flow periods.
const (
	// CashFlowByMonth groups by calendar month; the key is YYYY-MM.
	CashFlowByMonth CashFlowPeriod = iota
	// CashFlowByYear groups by calendar year; the key is YYYY.
	CashFlowByYear
)

// CashFlowParams is everything a cash-flow read varies by: the Window, the period
// unit, and the accounts to count (every account in reports when AccountIDs is empty).
type CashFlowParams struct {
	Window     Window
	By         CashFlowPeriod
	AccountIDs []string
}

// CashFlowRow is one period's income, spending and net in one currency, in cents.
// SavingsRatePct is net over income as a percentage rounded to one decimal, nil
// when income is zero or less.
type CashFlowRow struct {
	Period         string
	Currency       string
	Income, Spent  int64
	Net            int64
	SavingsRatePct *float64
}

// CashFlowTotal is all of one currency's income, spending and net in the window, in cents.
type CashFlowTotal struct {
	Currency       string
	Income, Spent  int64
	Net            int64
	SavingsRatePct *float64
}

// CashFlow is the periods of a cash-flow read, oldest first and CAD before USD within one,
// and one Total per currency present.
type CashFlow struct {
	Rows   []CashFlowRow
	Totals []CashFlowTotal
	// Transactions is set only when the window holds no income or spending (no Totals), as for Spending.
	Transactions TransactionRange
}
