package document

import (
	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/platform/tomlstr"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
)

// FindingsList is findings's --json document and the findings tool's structured result; Type is null when no type filter applies.
type FindingsList struct {
	Status   string         `json:"status"`
	Type     *string        `json:"type"`
	Counts   FindingCounts  `json:"counts"`
	Findings []FindingEntry `json:"findings"`
	Warnings []string       `json:"warnings"`
}

// FindingEntry is one entry of "findings"; FixedAt is null while the finding is open, and a fixed finding has no items.
type FindingEntry struct {
	ID           string        `json:"id"`
	Type         string        `json:"type"`
	Status       string        `json:"status"`
	FirstFoundAt string        `json:"first_found_at"`
	FixedAt      *string       `json:"fixed_at"`
	Fix          string        `json:"fix"`
	Items        []FindingItem `json:"items"`
}

// FindingItem is one entry of a finding's "items": every key is always present and null where it does not
// apply. Date, AccountID, Account, Currency and Amount are null for an item that is a payee or category, not a transaction or split.
type FindingItem struct {
	TransactionID  *string `json:"transaction_id"`
	SplitID        *string `json:"split_id"`
	PayeeID        *string `json:"payee_id"`
	CategoryID     *string `json:"category_id"`
	Date           *string `json:"date"`
	AccountID      *string `json:"account_id"`
	Account        *string `json:"account"`
	Currency       *string `json:"currency"`
	Payee          *string `json:"payee"`
	Category       *string `json:"category"`
	Amount         *string `json:"amount"`
	OtherAccount   *string `json:"other_account"`
	OtherAccountID *string `json:"other_account_id"`
	Transactions   *int    `json:"transactions"`
	Splits         *int    `json:"splits"`
}

// NewFindingsList builds the findings document for listing as filtered by status and typ (empty for no type
// filter); findings and warnings are [] rather than null when empty.
func NewFindingsList(listing report.FindingsListing, status finding.Status, typ finding.Type, warnings []string) FindingsList {
	entries := []FindingEntry{}
	for _, group := range listing.Groups {
		for _, f := range group.Findings {
			entries = append(entries, NewFindingEntry(f))
		}
	}
	return FindingsList{
		Status:   string(status),
		Type:     NullString(string(typ)),
		Counts:   NewFindingCounts(listing.Counts),
		Findings: entries,
		Warnings: append([]string{}, warnings...),
	}
}

// NewFindingEntry converts f into its --json entry: fixed_at is set only for a fixed finding, which has no items,
// and splits only for the items of a similar-categories finding.
func NewFindingEntry(f report.ListedFinding) FindingEntry {
	items := make([]FindingItem, len(f.Items))
	for i, item := range f.Items {
		items[i] = NewFindingItem(item)
		if f.Type == finding.SimilarCategories {
			items[i].Splits = &item.Splits
		}
	}
	doc := FindingEntry{
		ID: f.ID, Type: string(f.Type), Status: string(f.Status),
		FirstFoundAt: timestamp(f.FirstFoundAt),
		Fix:          f.Type.Fix().Sentence, Items: items,
	}
	if f.FixedAt != nil {
		fixedAt := timestamp(*f.FixedAt)
		doc.FixedAt = &fixedAt
	}
	return doc
}

// NewFindingItem converts item into its --json entry: date, account, currency and amount are null for
// an item with neither transaction nor split, category is its path or null, and transactions is null where 0.
func NewFindingItem(item store.FindingItem) FindingItem {
	doc := FindingItem{
		TransactionID: item.TransactionID, SplitID: item.SplitID, PayeeID: item.PayeeID, CategoryID: item.CategoryID,
		Payee: NullString(item.Payee), Category: item.Category,
		OtherAccount: item.OtherAccount, OtherAccountID: item.OtherAccountID,
	}
	if item.Transactions > 0 {
		doc.Transactions = &item.Transactions
	}
	if item.TransactionID == nil && item.SplitID == nil {
		return doc
	}
	date, amount := item.Date.Format(DateLayout), Money(item.Amount)
	doc.Date, doc.AccountID, doc.Account, doc.Currency, doc.Amount = &date, &item.AccountID, &item.Account, &item.Currency, &amount
	return doc
}

// UnmatchedIgnoreWarnings is one line per findings.ignore element that names no finding, each
// naming the config file as configShown (~-abbreviated for stderr, absolute for --json); [] when none.
func UnmatchedIgnoreWarnings(configShown string, unmatched []string) []string {
	lines := make([]string, len(unmatched))
	for i, id := range unmatched {
		lines[i] = configShown + ": findings.ignore lists " + tomlstr.BasicString(id) +
			", which is not a finding in quarry's store; quarry skips it"
	}
	return lines
}
