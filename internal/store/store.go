package store

import (
	"errors"
	"time"
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
// money left from; CrossCurrency is set when the legs' accounts differ in
// currency.
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

// Rows is every row a store build writes, grouped by table.
type Rows struct {
	Accounts     []Account
	Categories   []Category
	Payees       []Payee
	Tags         []Tag
	Transactions []Transaction
	Splits       []Split
	SplitTags    []SplitTag
	Transfers    []Transfer
}

// Counts is the row count of each table after a build; Transfers counts
// paired and one-sided rows alike.
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
type Result struct {
	Path        string
	Built       bool
	Counts      Counts
	Validation  Validation
	NotImported NotImported
}

// NotImported counts source rows a build deliberately leaves out of the
// store.
type NotImported struct {
	InvestmentTransactions int
}

// ErrValidationFailed is Import's error when a build's checks find a mismatch.
var ErrValidationFailed = errors.New("validation failed")

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
// display only; it is never rendered.
type SplitMismatch struct {
	ID, Account, Currency, Payee string
	SourceID                     int64
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

// OneSidedTransfer is one transfer leg with no counterpart. OtherAccount is
// the account name the leg records, nil for a numeric link; OtherAccountID
// is the imported account that name matches, nil when none does.
type OneSidedTransfer struct {
	ID             string
	SourceID       int64
	OtherAccount   *string
	OtherAccountID *string
}
