// White-box: renderAnomalies and anomaliesFooter are unexported layout rules whose column widths,
// cells and footer forms are best driven directly.
package cli

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

const anomaliesCaption = "Unusually large charges 2026-01-01 to 2026-03-09 in all accounts\n\n"

const anomaliesHeaderLine = "Date  Account  Payee  Category  Amount  Usual  Times  Compared with\n"

func Test_renderAnomalies_pads_each_column_and_trims_the_trailing_spaces_of_a_row(t *testing.T) {
	longer := anomalyOf(new("Bell Canada"), &store.ChargeCategory{Path: "Utilities:Phone"}, 1)
	longer.Date = utcDay(2026, time.March, 5)
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

func Test_renderAnomalies_measures_the_payee_column_in_characters_not_bytes(t *testing.T) {
	accented := anomalyOf(new("Société"), &store.ChargeCategory{Path: "Home"}, 1)
	plain := anomalyOf(new("Rogers"), &store.ChargeCategory{Path: "Home"}, 1)

	got := renderAnomalies(listed(accented, plain))

	want := anomaliesCaption +
		"Date        Account         Payee    Category  Amount  Usual  Times  Compared with\n" +
		"2026-03-02  Chequing (CAD)  Société  Home      412.00  96.05   4.3x  payee, 38 earlier\n" +
		"2026-03-02  Chequing (CAD)  Rogers   Home      412.00  96.05   4.3x  payee, 38 earlier\n" +
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

func Test_renderAnomalies_names_the_category_as_the_baseline_of_a_charge_judged_by_it(t *testing.T) {
	an := anomalyOf(nil, &store.ChargeCategory{Path: "Home"}, 1)
	an.Baseline, an.Earlier = report.BaselineCategory, 212

	got := renderAnomalies(listed(an))

	assert.Equal(t, anomaliesCaption+
		"Date        Account         Payee       Category  Amount  Usual  Times  Compared with\n"+
		"2026-03-02  Chequing (CAD)  (no payee)  Home      412.00  96.05   4.3x  category, 212 earlier\n"+
		"\n1 charge checked\n", got)
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

// usdAnomaly is anomalyOf in a USD account: 250.00 against a usual 40.00, listed as it was charged.
func usdAnomaly() report.Anomaly {
	an := anomalyOf(new("Hardware"), &store.ChargeCategory{Path: "Home"}, 1)
	an.Account = store.Account{ID: "acct-usd", Name: "US Chequing", Currency: "USD", Active: true}
	an.Currency, an.Amount, an.Usual, an.TimesTenths, an.Earlier = "USD", 125000, 4000, 313, 5
	return an
}

func Test_renderAnomalies_shows_the_converted_amount_and_usual_with_the_report_currency_in_the_caption(t *testing.T) {
	an := usdAnomaly()
	an.ListedCurrency, an.ListedAmount, an.ListedUsual = "CAD", 175000, 5600
	found := listed(an)
	found.Currency = money.CAD

	got := renderAnomalies(found)

	want := "Unusually large charges 2026-01-01 to 2026-03-09 in all accounts, amounts in CAD\n\n" +
		"Date        Account            Payee     Category    Amount  Usual  Times  Compared with\n" +
		"2026-03-02  US Chequing (USD)  Hardware  Home      1,750.00  56.00  31.3x  payee, 5 earlier\n" +
		"\n" +
		"1 charge checked\n"
	assert.Equal(t, want, got)
}

func Test_renderAnomalies_prefixes_both_cells_of_a_charge_left_in_its_own_currency(t *testing.T) {
	cases := []struct {
		name     string
		currency money.Currency
		native   string
		want     string
	}{
		{name: "a USD charge in a CAD report", currency: money.CAD, native: "USD", want: "USD 1,250.00  USD 40.00"},
		{name: "a CAD charge in a USD report", currency: money.USD, native: "CAD", want: "CAD 1,250.00  CAD 40.00"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			an := usdAnomaly()
			an.Currency = c.native
			found := listed(an)
			found.Currency = c.currency

			got := renderAnomalies(found)

			assert.Contains(t, got, c.want)
		})
	}
}

func Test_renderAnomalies_adds_no_prefix_for_a_charge_in_the_report_currency_or_in_native_mode(t *testing.T) {
	cases := []struct {
		name     string
		currency money.Currency
	}{
		{name: "native mode", currency: money.Native},
		{name: "a USD charge in a USD report", currency: money.USD},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			found := listed(usdAnomaly())
			found.Currency = c.currency

			got := renderAnomalies(found)

			assert.Contains(t, got, "1,250.00  40.00")
			assert.NotContains(t, got, "USD 1,250.00")
		})
	}
}
