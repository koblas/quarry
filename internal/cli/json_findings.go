package cli

import (
	"github.com/koblas/quarry/internal/finding"
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

// findingItemDocument is one entry of a finding's "items": every key is always present and null where it does not apply.
type findingItemDocument struct {
	TransactionID  *string `json:"transaction_id"`
	SplitID        *string `json:"split_id"`
	PayeeID        *string `json:"payee_id"`
	CategoryID     *string `json:"category_id"`
	Date           string  `json:"date"`
	AccountID      string  `json:"account_id"`
	Account        string  `json:"account"`
	Currency       string  `json:"currency"`
	Payee          *string `json:"payee"`
	Category       *string `json:"category"`
	Amount         string  `json:"amount"`
	OtherAccount   *string `json:"other_account"`
	OtherAccountID *string `json:"other_account_id"`
	Transactions   *int    `json:"transactions"`
	Splits         *int    `json:"splits"`
}

// renderFindingsJSON renders listing as findings's --json document with warnings as given; findings
// and warnings are [] rather than null when empty.
func renderFindingsJSON(listing report.FindingsListing, warnings []string) ([]byte, error) {
	entries := []findingEntryDocument{}
	for _, group := range listing.Groups {
		for _, f := range group.Findings {
			entries = append(entries, newFindingEntryDocument(f))
		}
	}
	c := listing.Counts
	return marshalDocument(findingsListDocument{
		Status:   string(finding.StatusOpen),
		Counts:   findingsDocument{Open: c.Open, Ignored: c.Ignored, Fixed: c.Fixed, New: c.New, NewlyFixed: c.NewlyFixed},
		Findings: entries,
		Warnings: append([]string{}, warnings...),
	})
}

// newFindingEntryDocument converts f, an open finding, into its --json entry: it has no fixed_at.
func newFindingEntryDocument(f store.Finding) findingEntryDocument {
	items := make([]findingItemDocument, len(f.Items))
	for i, item := range f.Items {
		items[i] = newFindingItemDocument(item)
	}
	return findingEntryDocument{
		ID: f.ID, Type: string(f.Type), Status: string(finding.StatusOpen),
		FirstFoundAt: jsonTimestamp(f.FirstFoundAt),
		Fix:          f.Type.Fix().Sentence, Items: items,
	}
}

// newFindingItemDocument converts item into its --json entry; category, transactions and splits stay null until a type carries them.
func newFindingItemDocument(item store.FindingItem) findingItemDocument {
	return findingItemDocument{
		TransactionID: item.TransactionID, SplitID: item.SplitID, PayeeID: item.PayeeID, CategoryID: item.CategoryID,
		Date: item.Date.Format(jsonDateLayout), AccountID: item.AccountID, Account: item.Account, Currency: item.Currency,
		Payee: jsonNullString(item.Payee), Amount: jsonMoney(item.Amount),
		OtherAccount: item.OtherAccount, OtherAccountID: item.OtherAccountID,
	}
}
