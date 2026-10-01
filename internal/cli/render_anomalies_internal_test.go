// White-box: renderAnomalies and anomaliesFooter are unexported layout rules whose column widths,
// cells and footer forms are best driven directly.
package cli

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

const anomaliesCaption = "Unusually large charges 2026-01-01 to 2026-03-09 in all accounts\n\n"

const anomaliesHeaderLine = "Date  Account  Payee  Category  Amount  Usual  Times  Compared with\n"

// anomalyOf is a payee-baseline anomaly of the given payee, category and splits: 412.00 against a
// usual 96.05, 4.3 times, from 38 earlier charges, on 2026-03-02 in Chequing (CAD).
func anomalyOf(payee *string, category *store.ChargeCategory, splits int) report.Anomaly {
	return report.Anomaly{
		Date:    time.Date(2026, time.March, 2, 0, 0, 0, 0, time.UTC),
		Account: store.Account{ID: "acct-1", Name: "Chequing", Currency: "CAD", Active: true},
		Payee:   payee, Currency: "CAD", Amount: 41200, Category: category, ExpenseSplits: splits,
		Baseline: report.BaselinePayee, Usual: 9605, Earlier: 38, TimesTenths: 43,
	}
}

func listed(anomalies ...report.Anomaly) report.Anomalies {
	return report.Anomalies{Window: spendingWindow(), Listed: anomalies, Checked: len(anomalies)}
}

func Test_renderAnomalies_pads_each_column_and_trims_the_trailing_spaces_of_a_row(t *testing.T) {
	longer := anomalyOf(new("Bell Canada"), &store.ChargeCategory{Path: "Utilities:Phone"}, 1)
	longer.Date = time.Date(2026, time.March, 5, 0, 0, 0, 0, time.UTC)
	longer.Amount, longer.Usual, longer.TimesTenths, longer.Earlier = 184210, 21040, 88, 212
	shorter := anomalyOf(new("Rogers"), &store.ChargeCategory{Path: "Home"}, 1)

	got := renderAnomalies(listed(longer, shorter))

	want := anomaliesCaption +
		"Date        Account         Payee        Category           Amount   Usual  Times  Compared with\n" +
		"2026-03-05  Chequing (CAD)  Bell Canada  Utilities:Phone  1,842.10  210.40   8.8x  payee, 212 earlier\n" +
		"2026-03-02  Chequing (CAD)  Rogers       Home               412.00   96.05   4.3x  payee, 38 earlier\n" +
		"\n" +
		"2 charges checked\n"
	assert.Equal(t, want, got)
}

func Test_renderAnomalies_shows_a_charge_without_a_category_as_uncategorized(t *testing.T) {
	got := renderAnomalies(listed(anomalyOf(new("Rogers"), nil, 1)))

	assert.Equal(t, anomaliesCaption+
		"Date        Account         Payee   Category         Amount  Usual  Times  Compared with\n"+
		"2026-03-02  Chequing (CAD)  Rogers  (uncategorized)  412.00  96.05   4.3x  payee, 38 earlier\n"+
		"\n1 charge checked\n", got)
}

func Test_renderAnomalies_shows_a_charge_of_several_splits_as_split_though_they_share_a_category(t *testing.T) {
	got := renderAnomalies(listed(anomalyOf(new("Rogers"), &store.ChargeCategory{Path: "Home"}, 2)))

	assert.Equal(t, anomaliesCaption+
		"Date        Account         Payee   Category  Amount  Usual  Times  Compared with\n"+
		"2026-03-02  Chequing (CAD)  Rogers  (split)   412.00  96.05   4.3x  payee, 38 earlier\n"+
		"\n1 charge checked\n", got)
}

func Test_renderAnomalies_shows_a_charge_without_a_payee_as_no_payee(t *testing.T) {
	got := renderAnomalies(listed(anomalyOf(nil, &store.ChargeCategory{Path: "Home"}, 1)))

	assert.Equal(t, anomaliesCaption+
		"Date        Account         Payee       Category  Amount  Usual  Times  Compared with\n"+
		"2026-03-02  Chequing (CAD)  (no payee)  Home      412.00  96.05   4.3x  payee, 38 earlier\n"+
		"\n1 charge checked\n", got)
}

func Test_renderAnomalies_escapes_line_breaks_in_the_payee_account_and_category(t *testing.T) {
	an := anomalyOf(new("Foo\nBar"), &store.ChargeCategory{Path: "Home\r:Fix"}, 1)
	an.Account.Name = "Chq\tOne"

	got := renderAnomalies(listed(an))

	assert.Equal(t, anomaliesCaption+
		"Date        Account         Payee     Category    Amount  Usual  Times  Compared with\n"+
		`2026-03-02  Chq\tOne (CAD)  Foo\nBar  Home\r:Fix  412.00  96.05   4.3x  payee, 38 earlier`+"\n"+
		"\n1 charge checked\n", got)
}

func Test_renderAnomalies_groups_thousands_in_the_earlier_count(t *testing.T) {
	an := anomalyOf(new("Rogers"), nil, 1)
	an.Earlier = 1204

	got := renderAnomalies(listed(an))

	assert.Contains(t, got, "payee, 1,204 earlier\n")
}

func Test_renderAnomalies_without_a_listing_prints_the_caption_header_and_footer(t *testing.T) {
	got := renderAnomalies(report.Anomalies{Window: spendingWindow()})

	assert.Equal(t, anomaliesCaption+anomaliesHeaderLine+"\n0 charges checked\n", got)
}

func Test_anomaliesFooter_counts_the_charges_and_those_too_new_to_judge(t *testing.T) {
	cases := []struct {
		name               string
		checked, notJudged int
		want               string
	}{
		{name: "none checked", want: "0 charges checked"},
		{name: "one checked", checked: 1, want: "1 charge checked"},
		{name: "none left unjudged omits the clause", checked: 12, want: "12 charges checked"},
		{name: "one left unjudged", checked: 12, notJudged: 1, want: "12 charges checked; 1 had too little history to judge"},
		{name: "thousands grouped in both counts", checked: 1204, notJudged: 1087, want: "1,204 charges checked; 1,087 had too little history to judge"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, anomaliesFooter(c.checked, c.notJudged))
		})
	}
}

func Test_renderAnomalies_ends_with_the_footer_of_its_counts(t *testing.T) {
	a := report.Anomalies{Window: spendingWindow(), Checked: 1204, NotJudged: 87}

	got := renderAnomalies(a)

	assert.Equal(t, anomaliesCaption+anomaliesHeaderLine+"\n1,204 charges checked; 87 had too little history to judge\n", got)
}
