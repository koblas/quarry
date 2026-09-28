package store

import "time"

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
}

// Counts is the row count of each table after a build. Transfers is always
// 0 until the importer builds the transfers table.
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

// Result is what a store build returns: the path it was written to, and the
// row counts it holds.
type Result struct {
	Path   string
	Counts Counts
}
