package document

import (
	"fmt"
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/store"
)

// DateLayout is the format of every calendar date quarry prints, in text and in --json.
const DateLayout = time.DateOnly

// FindingCounts is the --json finding-counts object shared by sync's "store.findings" and findings' "counts".
type FindingCounts struct {
	Open       int `json:"open"`
	Ignored    int `json:"ignored"`
	Fixed      int `json:"fixed"`
	New        int `json:"new"`
	NewlyFixed int `json:"newly_fixed"`
}

// Rows is the --json row-count object: one count per table.
type Rows struct {
	Accounts     int `json:"accounts"`
	Categories   int `json:"categories"`
	Payees       int `json:"payees"`
	Tags         int `json:"tags"`
	Transactions int `json:"transactions"`
	Splits       int `json:"splits"`
	SplitTags    int `json:"split_tags"`
	Transfers    int `json:"transfers"`

	InvestmentTransactions int `json:"investment_transactions"`
	Securities             int `json:"securities"`
	Prices                 int `json:"prices"`
}

// NotImported is the --json "not_imported" object.
type NotImported struct {
	InvestmentTransactions int `json:"investment_transactions"`
}

// NewFindingCounts converts c into its --json shape.
func NewFindingCounts(c finding.Counts) FindingCounts {
	return FindingCounts{Open: c.Open, Ignored: c.Ignored, Fixed: c.Fixed, New: c.New, NewlyFixed: c.NewlyFixed}
}

// NewRows converts c's table counts into their --json shape.
func NewRows(c store.Counts) Rows {
	return Rows{
		Accounts: c.Accounts, Categories: c.Categories, Payees: c.Payees, Tags: c.Tags,
		Transactions: c.Transactions, Splits: c.Splits, SplitTags: c.SplitTags, Transfers: c.Transfers,
		InvestmentTransactions: c.InvestmentTransactions, Securities: c.Securities, Prices: c.Prices,
	}
}

// NullString returns nil for the empty string, else a pointer to s.
func NullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Money renders cents as a 2-decimal amount with a leading "-" for a
// negative value and no thousands grouping.
func Money(cents int64) string {
	negative := cents < 0
	if negative {
		cents = -cents
	}
	s := fmt.Sprintf("%d.%02d", cents/100, cents%100)
	if negative {
		return "-" + s
	}
	return s
}
