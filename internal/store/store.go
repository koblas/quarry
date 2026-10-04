package store

import (
	"errors"
	"math/big"
	"slices"
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/platform/money"
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
	// OtherAccount is the account name a one-sided leg recorded in name form;
	// nil for a pair and for a numeric link, which records no name.
	OtherAccount *string
}

// SplitTag links one split to one tag (the split_tags table).
type SplitTag struct {
	SplitID string
	TagID   string
}

// Security is one row of the securities table, recorded as Quicken has it.
// Ticker is nil when Quicken records none, Currency when it records no currency.
type Security struct {
	ID       string
	SourceID int64
	Name     string
	Ticker   *string
	Currency *string
}

// Price is one row of the prices table: a security's closing price on one day,
// in millionths of its currency's unit (DECIMAL(18,6)). SourceID is the
// ZSECURITYQUOTE.Z_PK of the quote kept for that day.
type Price struct {
	SecurityID string
	SourceID   int64
	Date       time.Time
	Price      int64
}

// InvestmentTransaction is one row of the investment_transactions table. Shares,
// SplitNewShares and SplitOldShares are millionths (DECIMAL(18,6)); Amount and
// Commission are cents. SecurityID is nil for a cash-only action, Shares nil
// when Quicken records none, and the split sides are set only for a split.
type InvestmentTransaction struct {
	ID             string
	SourceID       int64
	AccountID      string
	SecurityID     *string
	Date           time.Time
	Action         string
	Shares         *int64
	Amount         int64
	Commission     *int64
	Currency       string
	Memo           *string
	SplitNewShares *int64
	SplitOldShares *int64
}

// QuickenShare is Quicken's own share count of one holding: the sum of its
// counting lots' units, in millionths. A holding with a lot of zero units is
// listed.
type QuickenShare struct {
	AccountID, SecurityID string
	Millionths            *big.Int
}

// Rows is every row a store build writes, grouped by table. ImportRuns holds
// the new build's run only; the store carries earlier runs forward itself.
// ReferencedCategoryIDs is not a table: sorted unique ids of categories that rows
// not stored as splits use (split entries under transactions the import does not keep,
// budgets, loans, rules), nil when none; never persisted. QuickenShares is not a
// table either: the reference each holding's derived share count is checked against.
type Rows struct {
	Accounts     []Account
	Categories   []Category
	Payees       []Payee
	Tags         []Tag
	Transactions []Transaction
	Splits       []Split
	SplitTags    []SplitTag
	Transfers    []Transfer
	Securities   []Security
	Prices       []Price
	ImportRuns   []ImportRun

	InvestmentTransactions []InvestmentTransaction

	ReferencedCategoryIDs []string
	QuickenShares         []QuickenShare
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
	SharesChecked                     int
	BalancesNeverReconciled           int
	InvestmentAccounts                int
	TransfersPaired                   int
	TransfersCrossCurrency            int
}

// Status is what a built store says about itself: its path, the format and
// build that wrote it, its import run and the dates its transactions cover.
// A zero Run.Snapshot.TakenAt or empty Source means NULL was recorded; the
// dates are zero when there are no transactions. Findings holds each recorded
// finding's id, type and state without its items, read from the same build as the rest.
type Status struct {
	Path                string
	FormatVersion       int
	QuarryVersion       string
	BuiltAt             time.Time
	Run                 ImportRun
	FirstDate, LastDate time.Time
	Findings            []Finding
	Rates               StatusRates
}

// StatusRates is the exchange-rate coverage Status reports: the first and last
// dates in fx_rates (zero when it is empty) and the reason the latest sync's
// fetch fell short, empty when it did not.
type StatusRates struct {
	First, Last time.Time
	FetchError  string
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
	Securities   int
	Prices       int

	InvestmentTransactions int
}

// Result is what a store build returns. Built is false when a check failed:
// Path is then empty (Replace never ran) but Counts, Validation and
// NotImported still describe the rows the build would have written.
// HistoryFault is why the previous store's import runs were not carried
// into a built store; nil when they were, or no store existed. Findings
// tallies FindingStates with no findings.ignore list, zero when Built is false;
// FindingStates is the state of each finding the build recorded;
// FindingsCarried is true iff the previous store's findings were read.
// FindingsFault is why a previous store that opened had findings that could not be read.
// StoreUnreadable is true iff the previous store could not be opened at all.
// RatesFault is why a previous store that opened had exchange rates that could not be read.
type Result struct {
	Path         string
	Built        bool
	Counts       Counts
	Validation   Validation
	NotImported  NotImported
	HistoryFault *OpenError
	Findings     finding.Counts

	FindingStates   []finding.State
	FindingsCarried bool
	FindingsFault   *OpenError
	StoreUnreadable bool
	Rates           RatesSummary
	RatesFault      *OpenError
}

// Replaced is what Store.Replace reports: the path it wrote, the fault that
// kept the previous store's import runs from being carried, if any, the counts
// of the findings it recorded, their states, and whether the previous store's findings were carried.
// FindingsFault, StoreUnreadable and RatesFault mean what they do on Result.
type Replaced struct {
	Path         string
	HistoryFault *OpenError
	Findings     finding.Counts

	FindingStates   []finding.State
	FindingsCarried bool
	FindingsFault   *OpenError
	StoreUnreadable bool
	Rates           RatesSummary
	RatesFault      *OpenError
}

// RatesSummary is the exchange rates a build stored: the first and last
// dates in fx_rates (zero when none), how many were fetched this build, and
// the reason a fetch fell short, if it did. Partial is set when it fell short
// after fetching some rates: Added is then the rates kept, not all of Need.
type RatesSummary struct {
	First, Last time.Time
	Added       int
	FetchError  string
	Partial     bool
}

// The series a stored Rate comes from: the current Bank of Canada series and the discontinued one before it.
const (
	SeriesCurrent = "FXUSDCAD"
	SeriesLegacy  = "IEXE0101"
)

// Rate is one day's USD/CAD exchange rate and the series it came from.
type Rate struct {
	Date   time.Time
	USDCAD money.Rate
	Series string
}

// MaxRate is the largest rate fx_rates.usd_cad holds, in millionths: DECIMAL(10,6)'s 9999.999999.
const MaxRate = money.Rate(9_999_999_999)

// DateSpan is an inclusive run of dates; the zero value is empty.
type DateSpan struct {
	First, Last time.Time
}

// RatesRequest is what a build needs: the dates Need covers, of which Have
// is already stored.
type RatesRequest struct {
	Need, Have DateSpan
}

// RatesRefresh is the rates a fetch returned, how many are new, the reason
// the fetch fell short if it did, and whether Rates is only part of Need.
type RatesRefresh struct {
	Rates      []Rate
	Added      int
	FetchError string
	Partial    bool
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
	Shares    ShareCheck
	Transfers TransferCheck
}

// Failed reports whether the balance, split-sum or share-count check found a
// mismatch. Transfers never fail a build: a one-sided leg is stored, not refused.
func (v Validation) Failed() bool {
	return len(v.Balances.Mismatched) > 0 || len(v.Splits.Mismatched) > 0 || len(v.Shares.Mismatched) > 0
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

// ShareCheck is the share-count gate's result across every holding: Checked
// counts the holdings compared, Mismatched lists those whose counts differ.
type ShareCheck struct {
	Checked    int
	Mismatched []ShareMismatch
}

// ShareMismatch is one holding whose derived share count differs from
// Quicken's, in millionths of a share; Difference is Quarry - Quicken, clamped
// to the int64 range. The importer fills the display fields from the ids, and
// Ticker is nil when Quicken records none.
type ShareMismatch struct {
	AccountID, SecurityID       string
	Quarry, Quicken, Difference int64
	Account, Currency, Security string
	Closed, Active              bool
	AccountSourceID             int64
	SecuritySourceID            int64
	Ticker                      *string
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

// FindingList is every finding the store holds, open and fixed alike; ignored
// is not stored, so callers derive it from the config.
type FindingList struct {
	Findings []Finding
}

// Finding is one row of the findings table with its items. FixedAt is nil
// while the finding is open; a fixed finding has no Items. New and NewlyFixed
// mean first found or fixed by the store's own build.
type Finding struct {
	ID           string
	Type         finding.Type
	FirstFoundAt time.Time
	FixedAt      *time.Time
	New          bool
	NewlyFixed   bool
	Items        []FindingItem
}

// FindingItem is what a finding is about: a transaction or split with its account and payee, or a payee or
// category. Amount is in cents, the split's when SplitID is set; OtherAccount* describe a one-sided leg as
// OneSidedTransfer does.
type FindingItem struct {
	TransactionID  *string
	SplitID        *string
	PayeeID        *string
	CategoryID     *string
	Date           time.Time
	AccountID      string
	Account        string
	Currency       string
	Closed, Active bool
	Payee          string
	Category       *string // an unlinked-transfer item's sole split's full path, nil if none or several; a mixed-categories, similar-categories or unused-category item's category path
	Splits         int     // an unlinked-transfer item's split count; a similar-categories item's category's splits; 0 for an unused-category item
	Transactions   int     // a mixed-categories item's payee's transactions in its category, a payee-variants item's payee's transactions; 0 for every other type
	Amount         int64
	OtherAccount   *string
	OtherAccountID *string
}

// AccountBalance is one account with its balance in cents; Balance is nil
// when the store cannot compute it.
type AccountBalance struct {
	Account

	Balance *int64

	// BalanceCAD and BalanceUSD are Balance in cents in that currency at the latest rate dated on or before AsOf;
	// nil when Balance is nil or no rate converts it.
	BalanceCAD, BalanceUSD *int64
}

// AccountList is every account with its balance as of the store's today.
type AccountList struct {
	AsOf     time.Time
	Accounts []AccountBalance

	// FirstRate is the date of the store's earliest exchange rate; zero when it holds none.
	FirstRate time.Time
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

// SpendingGroups lists every SpendingGroup, in the order the --by vocabulary names them.
func SpendingGroups() []SpendingGroup {
	return []SpendingGroup{SpendByCategory, SpendByPayee, SpendByTag, SpendByMonth}
}

// ParseSpendingGroup is the grouping whose String is name; the bool is false, and the grouping zero,
// for a name no grouping has.
func ParseSpendingGroup(name string) (SpendingGroup, bool) {
	for _, g := range SpendingGroups() {
		if g.String() == name {
			return g, true
		}
	}
	return 0, false
}

// String is the word that names g as a --by value: category, payee, tag or month.
// A value outside the constants reads as "".
func (g SpendingGroup) String() string {
	switch g {
	case SpendByCategory:
		return "category"
	case SpendByPayee:
		return "payee"
	case SpendByTag:
		return "tag"
	case SpendByMonth:
		return "month"
	}
	return ""
}

// SpendingParams is everything a spending read varies by: the Window,
// the grouping, the accounts to count (every account in reports when
// AccountIDs is empty) and the currency to report in.
type SpendingParams struct {
	Window     Window
	By         SpendingGroup
	AccountIDs []string
	// Currency is the currency every split is converted to at its date's rate;
	// a split with no rate stays in its own currency, on rows of that currency.
	// The zero value, money.Native, converts nothing.
	Currency money.Currency
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

// Unconverted is the transactions a CAD or USD report lists in their own currency because the
// store has no rate for their date; the zero value means none, and a native report never fills it.
type Unconverted struct {
	// Transactions counts the distinct CAD and USD transactions in the report's own view and window
	// whose amount stayed unconverted.
	Transactions int
	// FirstRate is the earliest exchange rate in the store, held as UTC midnight; zero means the store has none.
	FirstRate time.Time
}

// Spending is the rows of a spending read in display order, and one Total
// per currency present, CAD before USD.
type Spending struct {
	Rows   []SpendingRow
	Totals []SpendingTotal
	// Unconverted is the transactions left in their own currency; see Unconverted.
	Unconverted Unconverted
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

// CashFlowPeriods lists every CashFlowPeriod, in the order the --by vocabulary names them.
func CashFlowPeriods() []CashFlowPeriod {
	return []CashFlowPeriod{CashFlowByMonth, CashFlowByYear}
}

// ParseCashFlowPeriod is the period whose String is name; the bool is false, and the period zero,
// for a name no period has.
func ParseCashFlowPeriod(name string) (CashFlowPeriod, bool) {
	for _, p := range CashFlowPeriods() {
		if p.String() == name {
			return p, true
		}
	}
	return 0, false
}

// String is the word that names p as a --by value: month or year.
// A value outside the constants reads as "".
func (p CashFlowPeriod) String() string {
	switch p {
	case CashFlowByMonth:
		return "month"
	case CashFlowByYear:
		return "year"
	}
	return ""
}

// CashFlowParams is everything a cash-flow read varies by: the Window, the period
// unit, and the accounts to count (every account in reports when AccountIDs is empty).
type CashFlowParams struct {
	Window     Window
	By         CashFlowPeriod
	AccountIDs []string
	// Currency is the currency to report in; the zero value, money.Native, converts nothing.
	Currency money.Currency
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
	// Unconverted is the transactions left in their own currency; see Unconverted.
	Unconverted Unconverted
	// Transactions is set only when the window holds no income or spending (no Totals), as for Spending.
	Transactions TransactionRange
}

// ChargeParams is what a charges read varies by: Through, the last civil day it
// reads, held as UTC midnight (rows dated later are not charges yet), and the accounts
// whose transaction span it gives.
type ChargeParams struct {
	Through time.Time
	// AccountIDs scopes only Charges.Transactions, as for Spending; empty spans every account.
	AccountIDs []string
}

// ChargeCategory is the category of a charge's expense rows.
type ChargeCategory struct {
	ID   string
	Path string
}

// Charge is one transaction's expense: the sum of its v_spending rows, in cents,
// kept only when positive.
type Charge struct {
	TransactionID string
	SourceID      int64
	Date          time.Time
	// Account holds only ID, Name, Currency, Closed and Active; its other fields are zero.
	Account Account
	// PayeeID and Payee are nil when the transaction has no payee.
	PayeeID, Payee *string
	Currency       string
	Amount         int64
	// AmountCAD and AmountUSD are Amount converted at the charge date's rate, each the sum of its
	// v_spending rows' converted cells; nil when the date has no rate.
	AmountCAD, AmountUSD *int64
	// USDCAD is the rate on the charge's date, or 0 when no rate is on or before it.
	USDCAD money.Rate
	// Category is set only when every expense row has the same non-NULL category.
	Category *ChargeCategory
	// ExpenseSplits is how many v_spending rows the transaction has.
	ExpenseSplits int
}

// Charges is every charge dated through ChargeParams.Through, ordered by date
// then numeric source id, and the span of the transactions Transactions describes.
type Charges struct {
	Rows []Charge
	// Transactions is the span of every transaction in the store, or of those of the reported
	// accounts ChargeParams.AccountIDs names; Rows are never filtered by account.
	Transactions TransactionRange
	// FirstRate is the date of the store's first exchange rate; zero when it has none.
	FirstRate time.Time
}

// Relation kinds: a relation is a base table or a view.
const (
	RelationTable = "table"
	RelationView  = "view"
)

// Column is one column of a Relation: its name and the type DuckDB declares it.
type Column struct {
	Name, Type string
}

// Relation is one table or view of the store's main schema, with its columns in declared order.
type Relation struct {
	Name, Kind string
	Columns    []Column
}

// Schema is what a store holds, read in one pass: its relations, and every account and category
// with only the fields a schema description needs (ID, Name, Type, Currency and Closed; ID, Name,
// FullPath, Kind and Hidden). Transactions is the zero value when the store has none.
type Schema struct {
	Relations    []Relation
	Accounts     []Account
	Categories   []Category
	Transactions TransactionRange
}

// SearchWindow is the dates a search lists; a nil bound is open, so the zero value lists every date.
// Both bounds are civil days at UTC midnight, inclusive.
type SearchWindow struct {
	Since, Until *time.Time
}

// SearchParams is what a search varies by: dates, accounts (empty is every account), text ("" keeps all),
// category full path (nil keeps all; "" names nothing), unsigned amount range in cents (nil is open) and limit (0 is all).
type SearchParams struct {
	Window     SearchWindow
	AccountIDs []string
	Text       string
	Category   *string
	Min, Max   *int64
	Limit      int
}

// SearchSplit is one split of a SearchRow. Category is the category's full path, nil for an uncategorized
// split and for a transfer leg; Memo is nil for NULL and for "".
type SearchSplit struct {
	Category *string
	Memo     *string
	Amount   int64
	Transfer bool
}

// SearchRow is one transaction of a Search, in its own currency. Account holds only ID, Name, Currency,
// Closed and Active; Payee and Memo are nil when absent, Memo for both NULL and "". Transfer and Excluded
// are the flags v_cash_flow's own exclusions give; Splits are in split source id order.
type SearchRow struct {
	TransactionID string
	Date          time.Time
	Account       Account
	Payee, Memo   *string
	Amount        int64
	Currency      string
	Transfer      bool
	Excluded      bool
	Splits        []SearchSplit
}

// Search is the newest SearchParams.Limit transactions the search matched, newest first, ties by
// descending source id.
type Search struct {
	Rows []SearchRow
	// Matched counts every match, including those the limit cut.
	Matched int
	// Transactions is the span of every transaction in the store, or of the named accounts' transactions.
	Transactions TransactionRange
	// UnknownCategory is set only when SearchParams.Category is non-nil and equals no category's full path in any letter case.
	UnknownCategory bool
}
