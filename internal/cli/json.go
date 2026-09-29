package cli

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store"
)

// jsonDateLayout is the --json document's date format for every date field.
const jsonDateLayout = "2006-01-02"

// resultDocument is sync's --json stdout shape: the manifest, the store
// result (nil before an import is attempted), and every warning.
type resultDocument struct {
	Snapshot snapshot.Info       `json:"snapshot"`
	Schema   snapshot.SchemaInfo `json:"schema"`
	Store    *storeDocument      `json:"store"`
	Warnings []string            `json:"warnings"`
}

// storeDocument is the --json "store" object.
type storeDocument struct {
	Path        string              `json:"path"`
	Built       bool                `json:"built"`
	Rows        rowsDocument        `json:"rows"`
	Balances    balancesDocument    `json:"balances"`
	Splits      splitsDocument      `json:"splits"`
	Transfers   transfersDocument   `json:"transfers"`
	NotImported notImportedDocument `json:"not_imported"`
}

// rowsDocument is the --json "store.rows" object: one count per table.
type rowsDocument struct {
	Accounts     int `json:"accounts"`
	Categories   int `json:"categories"`
	Payees       int `json:"payees"`
	Tags         int `json:"tags"`
	Transactions int `json:"transactions"`
	Splits       int `json:"splits"`
	SplitTags    int `json:"split_tags"`
	Transfers    int `json:"transfers"`
}

// balancesDocument is the --json "store.balances" object.
type balancesDocument struct {
	Checked            int                       `json:"checked"`
	Mismatched         []balanceMismatchDocument `json:"mismatched"`
	NeverReconciled    []accountDocument         `json:"never_reconciled"`
	InvestmentAccounts int                       `json:"investment_accounts"`
}

// accountDocument names one account in "never_reconciled".
type accountDocument struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Currency string `json:"currency"`
	Closed   bool   `json:"closed"`
	Active   bool   `json:"active"`
}

// balanceMismatchDocument is one entry of "store.balances.mismatched".
type balanceMismatchDocument struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Currency      string `json:"currency"`
	Closed        bool   `json:"closed"`
	Active        bool   `json:"active"`
	StatementDate string `json:"statement_date"`
	Quarry        string `json:"quarry"`
	Quicken       string `json:"quicken"`
	Difference    string `json:"difference"`
}

// splitsDocument is the --json "store.splits" object.
type splitsDocument struct {
	Checked    int                     `json:"checked"`
	Mismatched []splitMismatchDocument `json:"mismatched"`
}

// splitMismatchDocument is one entry of "store.splits.mismatched".
type splitMismatchDocument struct {
	ID          string  `json:"id"`
	Date        string  `json:"date"`
	Account     string  `json:"account"`
	Currency    string  `json:"currency"`
	Payee       *string `json:"payee"`
	Amount      string  `json:"amount"`
	SplitsTotal string  `json:"splits_total"`
}

// transfersDocument is the --json "store.transfers" object.
type transfersDocument struct {
	Paired        int                `json:"paired"`
	CrossCurrency int                `json:"cross_currency"`
	OneSided      []oneSidedDocument `json:"one_sided"`
}

// oneSidedDocument is one entry of "store.transfers.one_sided".
type oneSidedDocument struct {
	ID             string  `json:"id"`
	Date           string  `json:"date"`
	Account        string  `json:"account"`
	Currency       string  `json:"currency"`
	Payee          *string `json:"payee"`
	Amount         string  `json:"amount"`
	OtherAccount   *string `json:"other_account"`
	OtherAccountID *string `json:"other_account_id"`
}

// notImportedDocument is the --json "store.not_imported" object.
type notImportedDocument struct {
	InvestmentTransactions int `json:"investment_transactions"`
}

// renderJSON renders outcome as sync's --json document: 2-space indented
// JSON with a trailing newline, matching Manifest.Encode's formatting.
func renderJSON(outcome snapshot.Outcome) ([]byte, error) {
	doc := resultDocument{
		Snapshot: outcome.Manifest.Snapshot,
		Schema:   outcome.Manifest.Schema,
		Store:    newStoreDocument(outcome.Store),
		Warnings: outcome.Warnings(),
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		// unreachable: every resultDocument field is a string, bool, int, pointer or slice of those; none can fail JSON encoding.
		return nil, fmt.Errorf("encode result: %w", err)
	}
	return buf.Bytes(), nil
}

// newStoreDocument converts result into the --json store document, nil
// when the import was never attempted (a schema mismatch).
func newStoreDocument(result *store.Result) *storeDocument {
	if result == nil {
		return nil
	}
	return &storeDocument{
		Path:        result.Path,
		Built:       result.Built,
		Rows:        newRowsDocument(result.Counts),
		Balances:    newBalancesDocument(result.Validation.Balances),
		Splits:      newSplitsDocument(result.Validation.Splits),
		Transfers:   newTransfersDocument(result.Validation.Transfers),
		NotImported: notImportedDocument{InvestmentTransactions: result.NotImported.InvestmentTransactions},
	}
}

// newRowsDocument converts c's table counts into the --json shape.
func newRowsDocument(c store.Counts) rowsDocument {
	return rowsDocument{
		Accounts: c.Accounts, Categories: c.Categories, Payees: c.Payees, Tags: c.Tags,
		Transactions: c.Transactions, Splits: c.Splits, SplitTags: c.SplitTags, Transfers: c.Transfers,
	}
}

// newBalancesDocument converts bc into the --json shape; its lists are
// always non-nil, even when empty.
func newBalancesDocument(bc store.BalanceCheck) balancesDocument {
	return balancesDocument{
		Checked:            bc.Checked,
		Mismatched:         newBalanceMismatchDocuments(bc.Mismatched),
		NeverReconciled:    newAccountDocuments(bc.NeverReconciled),
		InvestmentAccounts: bc.InvestmentAccounts,
	}
}

// newAccountDocuments converts accounts into the --json "never_reconciled" shape.
func newAccountDocuments(accounts []store.Account) []accountDocument {
	out := make([]accountDocument, len(accounts))
	for i, a := range accounts {
		out[i] = accountDocument{ID: a.ID, Name: a.Name, Currency: a.Currency, Closed: a.Closed, Active: a.Active}
	}
	return out
}

// newBalanceMismatchDocuments converts mismatches into the --json shape.
func newBalanceMismatchDocuments(mismatches []store.BalanceMismatch) []balanceMismatchDocument {
	out := make([]balanceMismatchDocument, len(mismatches))
	for i, m := range mismatches {
		out[i] = balanceMismatchDocument{
			ID: m.ID, Name: m.Name, Currency: m.Currency, Closed: m.Closed, Active: m.Active,
			StatementDate: m.StatementDate.Format(jsonDateLayout),
			Quarry:        jsonMoney(m.Quarry),
			Quicken:       jsonMoney(m.Quicken),
			Difference:    jsonMoney(m.Difference),
		}
	}
	return out
}

// newSplitsDocument converts sc into the --json shape; Mismatched is
// always non-nil, even when empty.
func newSplitsDocument(sc store.SplitCheck) splitsDocument {
	return splitsDocument{Checked: sc.Checked, Mismatched: newSplitMismatchDocuments(sc.Mismatched)}
}

// newSplitMismatchDocuments converts mismatches into the --json shape.
func newSplitMismatchDocuments(mismatches []store.SplitMismatch) []splitMismatchDocument {
	out := make([]splitMismatchDocument, len(mismatches))
	for i, m := range mismatches {
		out[i] = splitMismatchDocument{
			ID: m.ID, Date: m.Date.Format(jsonDateLayout), Account: m.Account, Currency: m.Currency,
			Payee: jsonPayee(m.Payee), Amount: jsonMoney(m.Amount), SplitsTotal: jsonMoney(m.SplitsTotal),
		}
	}
	return out
}

// newTransfersDocument converts tc into the --json shape; OneSided is
// always non-nil, even when empty.
func newTransfersDocument(tc store.TransferCheck) transfersDocument {
	return transfersDocument{Paired: tc.Paired, CrossCurrency: tc.CrossCurrency, OneSided: newOneSidedDocuments(tc.OneSided)}
}

// newOneSidedDocuments converts legs into the --json shape.
func newOneSidedDocuments(legs []store.OneSidedTransfer) []oneSidedDocument {
	out := make([]oneSidedDocument, len(legs))
	for i, leg := range legs {
		out[i] = oneSidedDocument{
			ID: leg.ID, Date: leg.Date.Format(jsonDateLayout), Account: leg.Account, Currency: leg.Currency,
			Payee: jsonPayee(leg.Payee), Amount: jsonMoney(leg.Amount),
			OtherAccount: leg.OtherAccount, OtherAccountID: leg.OtherAccountID,
		}
	}
	return out
}

// jsonPayee returns nil for an absent payee (""), else a pointer to name.
func jsonPayee(name string) *string {
	if name == "" {
		return nil
	}
	return &name
}

// jsonMoney renders cents as a 2-decimal amount with a leading "-" for a
// negative value and no thousands grouping.
func jsonMoney(cents int64) string {
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
