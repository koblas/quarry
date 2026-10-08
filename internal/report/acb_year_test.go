package report_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func yearSecurity(id string, events ...report.ACBEvent) report.ACBSecurity {
	return report.ACBSecurity{Security: store.Security{ID: id, Name: id}, Events: events}
}

func yearBuy(year int) report.ACBEvent {
	return report.ACBEvent{Date: day(year, time.January, 5), Action: store.ActionBuy}
}

func yearSale(security string, year int) report.ACBSale {
	return report.ACBSale{SecurityID: security, Date: day(year, time.June, 1), Proceeds: 100}
}

func yearExcess(year int) report.ACBEvent {
	return report.ACBEvent{Date: day(year, time.March, 1), Action: report.ACBActionReturnOfCapital, Realized: true, Gain: 40}
}

func Test_in_year_keeps_that_years_sales_and_the_securities_that_sold_in_it(t *testing.T) {
	sales2025 := report.ACBYear{Year: 2025, Sales: []report.ACBSale{yearSale("sec-b", 2025)}, Proceeds: 100}
	a := report.ACB{
		Year: 2025,
		Years: []report.ACBYear{
			{Year: 2024, Sales: []report.ACBSale{yearSale("sec-a", 2024)}, Proceeds: 100},
			sales2025,
		},
		Securities: []report.ACBSecurity{yearSecurity("sec-a"), yearSecurity("sec-b"), yearSecurity("sec-c")},
	}

	got := a.InYear()

	assert.Equal(t, []report.ACBYear{sales2025}, got.Years)
	assert.Equal(t, []string{"sec-b"}, securityIDs(got))
}

func Test_in_year_lists_securities_in_the_reports_order(t *testing.T) {
	a := report.ACB{
		Year: 2025,
		Years: []report.ACBYear{{Year: 2025, Sales: []report.ACBSale{
			yearSale("sec-z", 2025), yearSale("sec-a", 2025), yearSale("sec-m", 2025),
		}}},
		Securities: []report.ACBSecurity{yearSecurity("sec-a"), yearSecurity("sec-m"), yearSecurity("sec-z")},
	}

	assert.Equal(t, []string{"sec-a", "sec-m", "sec-z"}, securityIDs(a.InYear()))
}

func Test_in_year_is_one_empty_year_when_the_report_has_none_inside_its_span(t *testing.T) {
	a := report.ACB{
		Year: 2023,
		Years: []report.ACBYear{
			{Year: 2022, Sales: []report.ACBSale{yearSale("sec-a", 2022)}},
			{Year: 2024, Sales: []report.ACBSale{yearSale("sec-a", 2024)}},
		},
		Securities: []report.ACBSecurity{yearSecurity("sec-a")},
	}

	got := a.InYear()

	assert.Equal(t, []report.ACBYear{{Year: 2023}}, got.Years)
	assert.Empty(t, got.Securities)
}

func Test_in_year_lists_a_security_whose_only_event_that_year_is_a_return_of_capital_above_its_acb(t *testing.T) {
	a := report.ACB{
		Year:       2024,
		Years:      []report.ACBYear{{Year: 2024, ReturnOfCapitalGain: 40}},
		Securities: []report.ACBSecurity{yearSecurity("sec-roc", yearBuy(2023), yearExcess(2024)), yearSecurity("sec-idle", yearBuy(2023))},
	}

	got := a.InYear()

	assert.Equal(t, []string{"sec-roc"}, securityIDs(got))
	assert.Equal(t, []report.ACBYear{{Year: 2024, ReturnOfCapitalGain: 40}}, got.Years)
}

func Test_in_year_drops_a_security_whose_return_of_capital_above_its_acb_was_another_year(t *testing.T) {
	a := report.ACB{
		Year:       2025,
		Years:      []report.ACBYear{{Year: 2024, ReturnOfCapitalGain: 40}},
		Securities: []report.ACBSecurity{yearSecurity("sec-roc", yearBuy(2023), yearExcess(2024))},
	}

	assert.Empty(t, a.InYear().Securities)
}

func Test_in_year_ignores_a_return_of_capital_that_stayed_within_the_acb(t *testing.T) {
	within := report.ACBEvent{Date: day(2024, time.March, 1), Action: report.ACBActionReturnOfCapital}
	a := report.ACB{Year: 2024, Securities: []report.ACBSecurity{yearSecurity("sec-roc", yearBuy(2023), within)}}

	assert.Empty(t, a.InYear().Securities)
}

func Test_in_year_leaves_out_a_security_quarry_could_not_value_though_it_has_an_excess_event(t *testing.T) {
	unvalued := yearSecurity("sec-usd", yearBuy(2023), yearExcess(2024))
	unvalued.NoRate = &report.ACBNoRate{Date: day(2023, time.January, 5), Currency: "USD"}
	a := report.ACB{Year: 2024, Securities: []report.ACBSecurity{unvalued}}

	got := a.InYear()

	assert.Empty(t, got.Securities)
	assert.Equal(t, []report.ACBYear{{Year: 2024}}, got.Years)
}

func Test_in_year_keeps_a_listed_securitys_events_from_every_year(t *testing.T) {
	events := []report.ACBEvent{yearBuy(2022), yearBuy(2023), yearBuy(2025)}
	a := report.ACB{
		Year:       2025,
		Years:      []report.ACBYear{{Year: 2025, Sales: []report.ACBSale{yearSale("sec-a", 2025)}}},
		Securities: []report.ACBSecurity{yearSecurity("sec-a", events...)},
	}

	assert.Equal(t, events, a.InYear().Securities[0].Events)
}

func Test_in_year_keeps_the_rest_of_the_report(t *testing.T) {
	a := report.ACB{
		Year: 2025, AsOf: day(2026, time.March, 12), FirstRate: day(2024, time.January, 2),
		AdjustmentIssues: []report.ACBAdjustmentIssue{{Item: 2}},
	}

	got := a.InYear()

	assert.Equal(t, a.AsOf, got.AsOf)
	assert.Equal(t, a.FirstRate, got.FirstRate)
	assert.Equal(t, a.AdjustmentIssues, got.AdjustmentIssues)
	assert.Equal(t, 2025, got.Year)
}

func Test_in_year_is_the_report_itself_when_it_names_no_year(t *testing.T) {
	a := report.ACB{
		Years:      []report.ACBYear{{Year: 2024, Sales: []report.ACBSale{yearSale("sec-a", 2024)}}},
		Securities: []report.ACBSecurity{yearSecurity("sec-a"), yearSecurity("sec-b")},
	}

	assert.Equal(t, a, a.InYear())
}

func cutSale(security string, year int, proceeds, gain int64) report.ACBSale {
	return report.ACBSale{SecurityID: security, Date: day(year, time.June, 1), Proceeds: proceeds, Outlays: 1, ACBRemoved: proceeds - gain - 1, Gain: gain}
}

func Test_acb_cut_keeps_only_the_named_securities_sales_and_totals(t *testing.T) {
	a := report.ACB{
		Selected:    true,
		SelectedIDs: []string{"sec-b"},
		Years: []report.ACBYear{{
			Year: 2024, Proceeds: 300, Outlays: 2, ACBRemoved: 247, Gain: 51,
			Sales: []report.ACBSale{cutSale("sec-a", 2024, 100, 10), cutSale("sec-b", 2024, 200, 41)},
		}},
		Securities: []report.ACBSecurity{yearSecurity("sec-a"), yearSecurity("sec-b")},
	}

	got := a.Cut()

	assert.Equal(t, []string{"sec-b"}, securityIDs(got))
	assert.Equal(t, []report.ACBYear{{
		Year: 2024, Proceeds: 200, Outlays: 1, ACBRemoved: 158, Gain: 41, Sales: []report.ACBSale{cutSale("sec-b", 2024, 200, 41)},
	}}, got.Years)
}

func Test_acb_cut_without_a_selection_is_the_year_cut_alone(t *testing.T) {
	a := report.ACB{
		Year:       2025,
		Years:      []report.ACBYear{{Year: 2024, Sales: []report.ACBSale{yearSale("sec-a", 2024)}}, {Year: 2025, Sales: []report.ACBSale{yearSale("sec-b", 2025)}}},
		Securities: []report.ACBSecurity{yearSecurity("sec-a"), yearSecurity("sec-b")},
	}

	assert.Equal(t, a.InYear(), a.Cut())
}

func Test_acb_cut_without_a_selection_or_year_is_the_report_itself(t *testing.T) {
	a := report.ACB{
		Years:      []report.ACBYear{{Year: 2024, Sales: []report.ACBSale{yearSale("sec-a", 2024)}}},
		Securities: []report.ACBSecurity{yearSecurity("sec-a"), yearSecurity("sec-b")},
	}

	assert.Equal(t, a, a.Cut())
}

func Test_acb_cut_drops_a_year_that_has_no_named_sale(t *testing.T) {
	a := report.ACB{
		Selected:    true,
		SelectedIDs: []string{"sec-b"},
		Years: []report.ACBYear{
			{Year: 2023, Sales: []report.ACBSale{yearSale("sec-a", 2023)}, Proceeds: 100},
			{Year: 2024, Sales: []report.ACBSale{yearSale("sec-b", 2024)}, Proceeds: 100},
		},
		Securities: []report.ACBSecurity{yearSecurity("sec-a"), yearSecurity("sec-b")},
	}

	got := a.Cut()

	require.Len(t, got.Years, 1)
	assert.Equal(t, 2024, got.Years[0].Year)
}

func Test_acb_cut_sums_the_return_of_capital_above_acb_of_the_named_securities_only(t *testing.T) {
	a := report.ACB{
		Selected:    true,
		SelectedIDs: []string{"sec-roc"},
		Years:       []report.ACBYear{{Year: 2024, ReturnOfCapitalGain: 80}},
		Securities: []report.ACBSecurity{
			yearSecurity("sec-roc", yearBuy(2023), yearExcess(2024)),
			yearSecurity("sec-other", yearBuy(2023), yearExcess(2024)),
		},
	}

	got := a.Cut()

	assert.Equal(t, []report.ACBYear{{Year: 2024, ReturnOfCapitalGain: 40}}, got.Years)
}

func Test_acb_cut_leaves_out_the_return_of_capital_above_acb_of_a_named_security_quarry_could_not_value(t *testing.T) {
	unvalued := yearSecurity("sec-usd", yearBuy(2023), yearExcess(2024))
	unvalued.NoRate = &report.ACBNoRate{Date: day(2023, time.January, 5), Currency: "USD"}
	a := report.ACB{Selected: true, SelectedIDs: []string{"sec-usd"}, Securities: []report.ACBSecurity{unvalued}}

	got := a.Cut()

	assert.Empty(t, got.Years)
}

func Test_acb_cut_applies_the_year_to_the_named_securities(t *testing.T) {
	a := report.ACB{
		Year:        2025,
		Selected:    true,
		SelectedIDs: []string{"sec-b"},
		Years: []report.ACBYear{
			{Year: 2025, Sales: []report.ACBSale{cutSale("sec-a", 2025, 100, 10), cutSale("sec-b", 2025, 200, 41)}, Proceeds: 300, Outlays: 2, ACBRemoved: 247, Gain: 51},
		},
		Securities: []report.ACBSecurity{yearSecurity("sec-a"), yearSecurity("sec-b")},
	}

	got := a.Cut()

	assert.Equal(t, []string{"sec-b"}, securityIDs(got))
	require.Len(t, got.Years, 1)
	assert.Equal(t, int64(200), got.Years[0].Proceeds)
}

func Test_acb_cut_is_an_empty_year_when_the_named_securities_have_no_sale_in_it(t *testing.T) {
	a := report.ACB{
		Year:        2025,
		Selected:    true,
		SelectedIDs: []string{"sec-b"},
		Years:       []report.ACBYear{{Year: 2025, Sales: []report.ACBSale{yearSale("sec-a", 2025)}, Proceeds: 100}},
		Securities:  []report.ACBSecurity{yearSecurity("sec-a"), yearSecurity("sec-b")},
	}

	got := a.Cut()

	assert.Equal(t, []report.ACBYear{{Year: 2025}}, got.Years)
	assert.Empty(t, got.Securities)
}

func Test_acb_carries_the_requested_year_without_cutting_the_walk(t *testing.T) {
	buy := acbTx(t, 1, "acct-1", "sec-1", "2023-03-01", store.ActionBuy, "CAD", 10*acbMillion, -100_000)
	sell := acbTx(t, 2, "acct-1", "sec-1", "2024-03-01", store.ActionSell, "CAD", -4*acbMillion, 50_000)
	srv := report.NewServer(report.WithStore(fakeStore{history: store.InvestmentHistory{
		Accounts: acbAccounts(), Securities: []store.Security{acbSecurity("sec-1", "XEQT", "CAD")},
		Transactions: []store.InvestmentTransaction{buy, sell},
	}}))

	got, err := srv.ACB(t.Context(), report.ACBRequest{Classification: acbClassification(), Today: acbToday, Year: 2023})

	require.NoError(t, err)
	assert.Equal(t, 2023, got.Year)
	require.Len(t, got.Years, 1)
	assert.Equal(t, 2024, got.Years[0].Year)
}

func Test_no_pool_events_is_true_only_when_the_report_has_no_security(t *testing.T) {
	cases := []struct {
		name string
		acb  report.ACB
		want bool
	}{
		{name: "no securities", acb: report.ACB{}, want: true},
		{name: "a security that only bought", acb: report.ACB{Securities: []report.ACBSecurity{yearSecurity("sec-a", yearBuy(2024))}}, want: false},
		{name: "a security whose every trade was left out of the years", acb: report.ACB{
			Securities: []report.ACBSecurity{{Security: store.Security{ID: "sec-a"}, NoRate: &report.ACBNoRate{Currency: "USD"}}},
		}, want: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, c.acb.NoPoolEvents())
		})
	}
}

func Test_sale_years_is_the_first_and_last_year_with_a_counted_sale(t *testing.T) {
	a := report.ACB{Years: []report.ACBYear{
		{Year: 2020, ReturnOfCapitalGain: 40},
		{Year: 2022, Sales: []report.ACBSale{yearSale("sec-a", 2022)}},
		{Year: 2023, ReturnOfCapitalGain: 40},
		{Year: 2025, Sales: []report.ACBSale{yearSale("sec-a", 2025)}},
		{Year: 2026, ReturnOfCapitalGain: 40},
	}}

	first, last, ok := a.SaleYears()

	assert.True(t, ok)
	assert.Equal(t, 2022, first)
	assert.Equal(t, 2025, last)
}

func Test_sale_years_is_one_year_when_only_one_has_a_sale(t *testing.T) {
	a := report.ACB{Years: []report.ACBYear{{Year: 2024, Sales: []report.ACBSale{yearSale("sec-a", 2024)}}}}

	first, last, ok := a.SaleYears()

	assert.True(t, ok)
	assert.Equal(t, 2024, first)
	assert.Equal(t, 2024, last)
}

func Test_sale_years_is_none_when_no_year_has_a_sale(t *testing.T) {
	a := report.ACB{Years: []report.ACBYear{{Year: 2024, ReturnOfCapitalGain: 40}}}

	_, _, ok := a.SaleYears()

	assert.False(t, ok)
}

func Test_year_is_empty_only_without_a_counted_sale_or_a_return_of_capital_above_the_acb(t *testing.T) {
	years := []report.ACBYear{
		{Year: 2022, Sales: []report.ACBSale{yearSale("sec-a", 2022)}},
		{Year: 2023, ReturnOfCapitalGain: 40},
	}
	cases := []struct {
		name string
		year int
		want bool
	}{
		{name: "a year with a sale", year: 2022, want: false},
		{name: "a year with only a return of capital above the ACB", year: 2023, want: false},
		{name: "a year the report has no entry for", year: 2024, want: true},
		{name: "no year asked", year: 0, want: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, report.ACB{Year: c.year, Years: years}.YearIsEmpty())
		})
	}
}

func Test_parse_acb_year_reads_four_digits_up_to_this_year(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  int
	}{
		{name: "the first year", value: "0001", want: 1},
		{name: "an ordinary year", value: "2024", want: 2024},
		{name: "this year", value: "2026", want: 2026},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := report.ParseACBYear(c.value, windowNow)

			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}

func Test_parse_acb_year_refuses_text_that_is_not_four_digits_naming_a_year(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{name: "year zero", value: "0000"},
		{name: "a plus sign", value: "+2024"},
		{name: "a plus sign within four characters", value: "+202"},
		{name: "a minus sign", value: "-202"},
		{name: "five digits", value: "20245"},
		{name: "letters", value: "abcd"},
		{name: "two digits", value: "24"},
		{name: "a month", value: "2024-03"},
		{name: "empty", value: ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := report.ParseACBYear(c.value, windowNow)

			require.Equal(t, report.ACBYearError{Kind: report.ACBYearNotAYear, Value: c.value}, err)
		})
	}
}

func Test_parse_acb_year_refuses_the_year_after_this_one(t *testing.T) {
	_, err := report.ParseACBYear("2027", windowNow)

	require.Equal(t, report.ACBYearError{Kind: report.ACBYearAfterThisYear, Value: "2027"}, err)
}

func Test_parse_acb_year_reads_this_year_in_the_clocks_own_zone(t *testing.T) {
	// 2027-01-01 00:30 at UTC+14 is still 2026-12-31 in UTC.
	now := time.Date(2027, time.January, 1, 0, 30, 0, 0, time.FixedZone("UTC+14", 14*60*60))

	got, err := report.ParseACBYear("2027", now)

	require.NoError(t, err)
	assert.Equal(t, 2027, got)
}

func Test_acb_year_error_words_each_refusal_for_the_command_line(t *testing.T) {
	notAYear := report.ACBYearError{Kind: report.ACBYearNotAYear, Value: "2024-03"}
	afterThisYear := report.ACBYearError{Kind: report.ACBYearAfterThisYear, Value: "2027"}

	assert.Equal(t, `--year "2024-03" is not a year; use YYYY, such as 2024`, notAYear.Error())
	assert.Equal(t, "--year 2027 is after this year; pass this year or an earlier one", afterThisYear.Error())
}
