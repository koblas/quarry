package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
)

// statusDocument is status's --json stdout shape.
type statusDocument struct {
	Store       statusStoreDocument     `json:"store"`
	Snapshot    statusSnapshotDocument  `json:"snapshot"`
	Dates       statusDatesDocument     `json:"dates"`
	Balances    statusBalancesDocument  `json:"balances"`
	Splits      statusSplitsDocument    `json:"splits"`
	Transfers   statusTransfersDocument `json:"transfers"`
	NotImported notImportedDocument     `json:"not_imported"`
	Warnings    []string                `json:"warnings"`
}

// statusStoreDocument is the --json "store" object: where the store lives,
// how it was built, and its row counts.
type statusStoreDocument struct {
	Path          string       `json:"path"`
	FormatVersion int          `json:"format_version"`
	QuarryVersion string       `json:"quarry_version"`
	BuiltAt       string       `json:"built_at"`
	Rows          rowsDocument `json:"rows"`
}

// statusSnapshotDocument is the --json "snapshot" object; TakenAt and Source
// are null when the manifest did not record them.
type statusSnapshotDocument struct {
	ID      string  `json:"id"`
	Path    string  `json:"path"`
	TakenAt *string `json:"taken_at"`
	Source  *string `json:"source"`
	SHA256  string  `json:"sha256"`
}

// statusDatesDocument is the --json "dates" object: the first and last
// transaction days, null when the store holds no transactions.
type statusDatesDocument struct {
	First *string `json:"first"`
	Last  *string `json:"last"`
}

// statusBalancesDocument is the --json "balances" object: counts only.
type statusBalancesDocument struct {
	Checked            int `json:"checked"`
	NeverReconciled    int `json:"never_reconciled"`
	InvestmentAccounts int `json:"investment_accounts"`
}

// statusSplitsDocument is the --json "splits" object: how many transactions
// sync checked against their splits.
type statusSplitsDocument struct {
	Checked int `json:"checked"`
}

// statusTransfersDocument is the --json "transfers" object: counts only.
type statusTransfersDocument struct {
	Paired        int `json:"paired"`
	CrossCurrency int `json:"cross_currency"`
	OneSided      int `json:"one_sided"`
}

// renderStatusJSON renders st as status's --json document, encoded like
// sync's: 2-space indent, trailing newline.
func renderStatusJSON(st store.Status) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(newStatusDocument(st)); err != nil {
		// unreachable: every statusDocument field is a string, int, pointer to string or slice of strings; none can fail JSON encoding.
		return nil, fmt.Errorf("encode status: %w", err)
	}
	return buf.Bytes(), nil
}

// newStatusDocument converts st into the --json shape. Paths stay absolute,
// times are UTC RFC 3339, and dates are calendar days as stored.
func newStatusDocument(st store.Status) statusDocument {
	run := st.Run
	return statusDocument{
		Store: statusStoreDocument{
			Path:          st.Path,
			FormatVersion: st.FormatVersion,
			QuarryVersion: st.QuarryVersion,
			BuiltAt:       jsonTimestamp(st.BuiltAt),
			Rows:          newRowsDocument(run.Counts),
		},
		Snapshot: statusSnapshotDocument{
			ID:      report.SnapshotID(run.Snapshot.Path),
			Path:    run.Snapshot.Path,
			TakenAt: jsonNullTimestamp(run.Snapshot.TakenAt),
			Source:  jsonNullString(run.Snapshot.Source),
			SHA256:  run.Snapshot.SHA256,
		},
		Dates: statusDatesDocument{First: jsonNullDate(st.FirstDate), Last: jsonNullDate(st.LastDate)},
		Balances: statusBalancesDocument{
			Checked:            run.BalancesChecked,
			NeverReconciled:    run.BalancesNeverReconciled,
			InvestmentAccounts: run.InvestmentAccounts,
		},
		Splits: statusSplitsDocument{Checked: run.Counts.Transactions},
		Transfers: statusTransfersDocument{
			Paired:        run.TransfersPaired,
			CrossCurrency: run.TransfersCrossCurrency,
			OneSided:      run.TransfersOneSided,
		},
		NotImported: notImportedDocument{InvestmentTransactions: run.InvestmentTransactionsNotImported},
		Warnings:    []string{},
	}
}

// jsonTimestamp formats t as RFC 3339 in UTC.
func jsonTimestamp(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// jsonNullTimestamp is jsonTimestamp, or nil for the zero time.
func jsonNullTimestamp(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := jsonTimestamp(t)
	return &s
}

// jsonNullDate formats the calendar day d as a date, or nil for the zero time.
func jsonNullDate(d time.Time) *string {
	if d.IsZero() {
		return nil
	}
	s := d.Format(jsonDateLayout)
	return &s
}
