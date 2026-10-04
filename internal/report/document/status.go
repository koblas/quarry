package document

import (
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
)

// Status is status's --json document and the sync_status tool's structured result.
type Status struct {
	Store     StatusStore     `json:"store"`
	Snapshot  StatusSnapshot  `json:"snapshot"`
	Dates     StatusDates     `json:"dates"`
	Balances  StatusBalances  `json:"balances"`
	Splits    StatusSplits    `json:"splits"`
	Shares    StatusShares    `json:"shares"`
	Transfers StatusTransfers `json:"transfers"`
	Findings  StatusFindings  `json:"findings"`
	Rates     StatusRates     `json:"rates"`
	Warnings  []string        `json:"warnings"`
}

// StatusStore is the "store" object: where the store lives, how it was built, and its row counts.
type StatusStore struct {
	Path          string `json:"path"`
	FormatVersion int    `json:"format_version"`
	QuarryVersion string `json:"quarry_version"`
	BuiltAt       string `json:"built_at"`
	Rows          Rows   `json:"rows"`
}

// StatusSnapshot is the "snapshot" object; TakenAt and Source are null when the manifest did not record them.
type StatusSnapshot struct {
	ID      string  `json:"id"`
	Path    string  `json:"path"`
	TakenAt *string `json:"taken_at"`
	Source  *string `json:"source"`
	SHA256  string  `json:"sha256"`
}

// StatusDates is the "dates" object: the first and last transaction days, null when the store holds no transactions.
type StatusDates struct {
	First *string `json:"first"`
	Last  *string `json:"last"`
}

// StatusBalances is the "balances" object: counts only.
type StatusBalances struct {
	Checked            int `json:"checked"`
	NeverReconciled    int `json:"never_reconciled"`
	InvestmentAccounts int `json:"investment_accounts"`
}

// StatusSplits is the "splits" object: how many transactions sync checked against their splits.
type StatusSplits struct {
	Checked int `json:"checked"`
}

// StatusShares is the "shares" object: how many holdings sync checked against Quicken's share counts.
type StatusShares struct {
	Checked int `json:"checked"`
}

// StatusTransfers is the "transfers" object: counts only.
type StatusTransfers struct {
	Paired        int `json:"paired"`
	CrossCurrency int `json:"cross_currency"`
	OneSided      int `json:"one_sided"`
}

// StatusFindings is the "findings" object; Ignored is null when findings.ignore could not be read.
type StatusFindings struct {
	Open       int  `json:"open"`
	Ignored    *int `json:"ignored"`
	Fixed      int  `json:"fixed"`
	New        int  `json:"new"`
	NewlyFixed int  `json:"newly_fixed"`
}

// StatusRates is the "rates" object: the first and last dates in fx_rates and the reason the
// last sync's fetch fell short; each is null when there is none.
type StatusRates struct {
	First      *string `json:"first"`
	Last       *string `json:"last"`
	FetchError *string `json:"fetch_error"`
}

// FindingsTally is the findings tally status reports; IgnoreKnown is false when findings.ignore
// could not be read, so every ignored finding is counted open and "ignored" cannot be said.
type FindingsTally struct {
	Counts      finding.Counts
	IgnoreKnown bool
}

// StatusIgnore applies status's ignore policy: with no problem it returns ignore and no warnings;
// with a problem reading the config it returns a nil list and the one cannot-tell warning naming
// problem. Status never refuses over the config.
func StatusIgnore(ignore []string, problem string) ([]string, []string) {
	if problem == "" {
		return ignore, nil
	}
	return nil, []string{CannotTellIgnored(problem)}
}

// CannotTellIgnored is the warning for a config that cannot be read, naming problem.
func CannotTellIgnored(problem string) string {
	return "cannot tell which findings you ignored: " + problem + "; findings you ignored are counted as open"
}

// NewStatus converts st into the status document. Paths stay absolute, times are UTC RFC 3339,
// dates are calendar days as stored, and warnings is never null.
func NewStatus(st store.Status, findings FindingsTally, warnings []string) Status {
	run := st.Run
	return Status{
		Store: StatusStore{
			Path:          st.Path,
			FormatVersion: st.FormatVersion,
			QuarryVersion: st.QuarryVersion,
			BuiltAt:       timestamp(st.BuiltAt),
			Rows:          NewRows(run.Counts),
		},
		Snapshot: StatusSnapshot{
			ID:      report.SnapshotID(run.Snapshot.Path),
			Path:    run.Snapshot.Path,
			TakenAt: nullTimestamp(run.Snapshot.TakenAt),
			Source:  NullString(run.Snapshot.Source),
			SHA256:  run.Snapshot.SHA256,
		},
		Dates: StatusDates{First: nullDate(st.FirstDate), Last: nullDate(st.LastDate)},
		Balances: StatusBalances{
			Checked:            run.BalancesChecked,
			NeverReconciled:    run.BalancesNeverReconciled,
			InvestmentAccounts: run.InvestmentAccounts,
		},
		Splits: StatusSplits{Checked: run.Counts.Transactions},
		Shares: StatusShares{Checked: run.SharesChecked},
		Transfers: StatusTransfers{
			Paired:        run.TransfersPaired,
			CrossCurrency: run.TransfersCrossCurrency,
			OneSided:      run.TransfersOneSided,
		},
		Findings: newStatusFindings(findings),
		Rates: StatusRates{
			First: nullDate(st.Rates.First), Last: nullDate(st.Rates.Last), FetchError: NullString(st.Rates.FetchError),
		},
		Warnings: append([]string{}, warnings...),
	}
}

// newStatusFindings converts f into the "findings" object.
func newStatusFindings(f FindingsTally) StatusFindings {
	c := f.Counts
	doc := StatusFindings{Open: c.Open, Fixed: c.Fixed, New: c.New, NewlyFixed: c.NewlyFixed}
	if f.IgnoreKnown {
		doc.Ignored = &c.Ignored
	}
	return doc
}

// timestamp formats t as RFC 3339 in UTC.
func timestamp(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// nullTimestamp is timestamp, or nil for the zero time.
func nullTimestamp(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := timestamp(t)
	return &s
}

// nullDate formats the calendar day d as a date, or nil for the zero time.
func nullDate(d time.Time) *string {
	if d.IsZero() {
		return nil
	}
	s := d.Format(DateLayout)
	return &s
}
