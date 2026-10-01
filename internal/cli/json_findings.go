package cli

import (
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
)

// findingsListDocument is findings's --json stdout shape; Type is null when no --type filter applies.
type findingsListDocument struct {
	Status   string                 `json:"status"`
	Type     *string                `json:"type"`
	Counts   findingsDocument       `json:"counts"`
	Findings []findingEntryDocument `json:"findings"`
	Warnings []string               `json:"warnings"`
}

// findingEntryDocument is one entry of "findings"; FixedAt is null while the finding is open, and a fixed finding has no items.
type findingEntryDocument struct {
	ID           string                `json:"id"`
	Type         string                `json:"type"`
	Status       string                `json:"status"`
	FirstFoundAt string                `json:"first_found_at"`
	FixedAt      *string               `json:"fixed_at"`
	Fix          string                `json:"fix"`
	Items        []findingItemDocument `json:"items"`
}

// findingItemDocument is one entry of a finding's "items": every key is always present and null where it does not
// apply. Date, AccountID, Account, Currency and Amount are null for an item that is a payee or category, not a transaction or split.
type findingItemDocument struct {
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

// renderFindingsJSON renders listing as findings's --json document for view with warnings as given;
// findings and warnings are [] rather than null when empty.
func renderFindingsJSON(listing report.FindingsListing, view findingsView, warnings []string) ([]byte, error) {
	entries := []findingEntryDocument{}
	for _, group := range listing.Groups {
		for _, f := range group.Findings {
			entries = append(entries, newFindingEntryDocument(f))
		}
	}
	return marshalDocument(findingsListDocument{
		Status:   string(view.status),
		Type:     jsonNullString(string(view.typ)),
		Counts:   newFindingCountsDocument(listing.Counts),
		Findings: entries,
		Warnings: append([]string{}, warnings...),
	})
}

// newFindingEntryDocument converts f into its --json entry: fixed_at is set only for a fixed finding, which has no items.
func newFindingEntryDocument(f report.ListedFinding) findingEntryDocument {
	items := make([]findingItemDocument, len(f.Items))
	for i, item := range f.Items {
		items[i] = newFindingItemDocument(item)
	}
	doc := findingEntryDocument{
		ID: f.ID, Type: string(f.Type), Status: string(f.Status),
		FirstFoundAt: jsonTimestamp(f.FirstFoundAt),
		Fix:          f.Type.Fix().Sentence, Items: items,
	}
	if f.FixedAt != nil {
		fixedAt := jsonTimestamp(*f.FixedAt)
		doc.FixedAt = &fixedAt
	}
	return doc
}

// newFindingItemDocument converts item into its --json entry: date, account, currency and amount are null for
// an item with neither transaction nor split, category is its path or null, and transactions is null where 0.
func newFindingItemDocument(item store.FindingItem) findingItemDocument {
	doc := findingItemDocument{
		TransactionID: item.TransactionID, SplitID: item.SplitID, PayeeID: item.PayeeID, CategoryID: item.CategoryID,
		Payee: jsonNullString(item.Payee), Category: item.Category,
		OtherAccount: item.OtherAccount, OtherAccountID: item.OtherAccountID,
	}
	if item.Transactions > 0 {
		doc.Transactions = &item.Transactions
	}
	if item.TransactionID == nil && item.SplitID == nil {
		return doc
	}
	date, amount := item.Date.Format(jsonDateLayout), jsonMoney(item.Amount)
	doc.Date, doc.AccountID, doc.Account, doc.Currency, doc.Amount = &date, &item.AccountID, &item.Account, &item.Currency, &amount
	return doc
}
