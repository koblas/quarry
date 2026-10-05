package config_test

import (
	"testing"

	"github.com/koblas/quarry/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	adjListMust   = "acb.adjustment must be a list of tables, each under its own [[acb.adjustment]] line, got "
	adjAcbMust    = "acb must be a table, such as [[acb.adjustment]], got "
	adjItemOne    = "acb.adjustment item 1"
	adjAmountMust = " must be an amount in CAD above 0 with at most two decimals, such as 12.34, got "
	needsSecurity = `acb.adjustment item 1 needs security, such as security = "sec-41"`
	needsDate     = "acb.adjustment item 1 needs date, such as date = 2024-12-31"
	needsAmount   = "acb.adjustment item 1 needs return-of-capital or reinvested-distribution, such as return-of-capital = 12.34"
	goodItem      = "[[acb.adjustment]]\nsecurity = \"sec-41\"\ndate = 2024-12-31\nreturn-of-capital = 12.34\n"
)

func Test_load_refuses_acb_adjustment_that_is_not_a_list_of_tables_under_double_brackets(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{name: "inline array of tables", content: "acb.adjustment = [{ security = \"sec-41\" }]\n", want: adjListMust + `[{ security = "sec-41" }]`},
		{name: "empty inline array", content: "acb.adjustment = []\n", want: adjListMust + "[]"},
		{name: "scalar", content: "acb.adjustment = 5\n", want: adjListMust + "5"},
		{name: "single-bracket table", content: "[acb.adjustment]\nsecurity = \"sec-41\"\n", want: adjListMust + "a table"},
		{name: "dotted keys", content: "acb.adjustment.security = \"sec-41\"\n", want: adjListMust + "a table"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := load(t, c.content)

			require.EqualError(t, err, shownPath+": "+c.want+fixLine)
		})
	}
}

func Test_load_refuses_acb_that_is_not_a_table(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{name: "number", content: "acb = 5\n", want: adjAcbMust + "5"},
		{name: "list of tables", content: "[[acb]]\nx = 1\n", want: adjAcbMust + "a list of tables"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := load(t, c.content)

			require.EqualError(t, err, shownPath+": "+c.want+fixLine)
		})
	}
}

func Test_load_refuses_an_item_that_lacks_a_key_it_needs(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{name: "no security", content: "[[acb.adjustment]]\ndate = 2024-12-31\nreturn-of-capital = 12.34\n", want: needsSecurity},
		{name: "no date", content: "[[acb.adjustment]]\nsecurity = \"sec-41\"\nreturn-of-capital = 12.34\n", want: needsDate},
		{name: "no amount", content: "[[acb.adjustment]]\nsecurity = \"sec-41\"\ndate = 2024-12-31\n", want: needsAmount},
		{name: "empty item", content: "[[acb.adjustment]]\n", want: needsSecurity},
		{name: "key spelled in another letter case", content: "[[acb.adjustment]]\nsecurity = \"sec-41\"\nDate = 2024-12-31\nreturn-of-capital = 12.34\n", want: needsDate},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := load(t, c.content)

			require.EqualError(t, err, shownPath+": "+c.want+fixLine)
		})
	}
}

func Test_load_numbers_a_bad_item_by_its_place_in_the_file(t *testing.T) {
	_, _, err := load(t, goodItem+"\n[[acb.adjustment]]\nsecurity = \"sec-41\"\nreturn-of-capital = 12.34\n")

	require.EqualError(t, err, shownPath+": acb.adjustment item 2 needs date, such as date = 2024-12-31"+fixLine)
}

func Test_load_refuses_the_first_bad_item_when_a_later_one_is_bad_too(t *testing.T) {
	_, _, err := load(t, "[[acb.adjustment]]\ndate = 2024-12-31\nreturn-of-capital = 12.34\n\n[[acb.adjustment]]\nsecurity = \"sec-41\"\nreturn-of-capital = 12.34\n")

	require.EqualError(t, err, shownPath+": "+`acb.adjustment item 1 needs security, such as security = "sec-41"`+fixLine)
}

func Test_load_reports_a_missing_key_before_a_wrong_type_in_the_same_item(t *testing.T) {
	_, _, err := load(t, "[[acb.adjustment]]\nsecurity = 41\nreturn-of-capital = 12.34\n")

	require.EqualError(t, err, shownPath+": acb.adjustment item 1 needs date, such as date = 2024-12-31"+fixLine)
}

func Test_load_refuses_a_security_that_is_not_a_non_empty_string(t *testing.T) {
	cases := []struct {
		name     string
		security string
	}{
		{name: "number", security: "41"},
		{name: "empty string", security: `""`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := load(t, "[[acb.adjustment]]\nsecurity = "+c.security+"\ndate = 2024-12-31\nreturn-of-capital = 12.34\n")

			require.EqualError(t, err, shownPath+": "+adjItemOne+`: security must be a security id in quotes, such as "sec-41", got `+c.security+fixLine)
		})
	}
}

func Test_load_refuses_a_date_that_is_not_a_local_date(t *testing.T) {
	cases := []struct {
		name string
		date string
	}{
		{name: "quoted string", date: `"2024-12-31"`},
		{name: "date with a time and zone", date: "2024-12-31T10:00:00Z"},
		{name: "date with a time and no zone", date: "2024-12-31T10:00:00"},
		{name: "integer", date: "20241231"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := load(t, "[[acb.adjustment]]\nsecurity = \"sec-41\"\ndate = "+c.date+"\nreturn-of-capital = 12.34\n")

			require.EqualError(t, err, shownPath+": "+adjItemOne+": date must be a date such as 2024-12-31, got "+c.date+fixLine)
		})
	}
}

func Test_load_refuses_a_calendar_date_that_does_not_exist_as_unreadable_toml(t *testing.T) {
	_, _, err := load(t, "[[acb.adjustment]]\nsecurity = \"sec-41\"\ndate = 2024-02-30\nreturn-of-capital = 12.34\n")

	require.ErrorContains(t, err, "cannot read "+shownPath+": line 3: ")
}

func Test_load_refuses_an_adjustment_amount_that_is_not_above_zero_with_two_decimals(t *testing.T) {
	cases := []struct {
		name   string
		amount string
	}{
		{name: "zero", amount: "0"},
		{name: "zero with decimals", amount: "0.00"},
		{name: "negative", amount: "-5"},
		{name: "three decimals", amount: "12.345"},
		{name: "three decimals ending in zero", amount: "12.340"},
		{name: "quoted", amount: `"12.34"`},
		{name: "exponent", amount: "1e2"},
		{name: "underscore", amount: "1_000.5"},
		{name: "too large for an int64 of cents", amount: "92233720368547758.08"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := load(t, "[[acb.adjustment]]\nsecurity = \"sec-41\"\ndate = 2024-12-31\nreturn-of-capital = "+c.amount+"\n")

			require.EqualError(t, err, shownPath+": "+adjItemOne+": return-of-capital"+adjAmountMust+c.amount+fixLine)
		})
	}
}

func Test_load_accepts_an_amount_of_one_cent_and_a_whole_amount(t *testing.T) {
	_, cfg, err := load(t, "[[acb.adjustment]]\nsecurity = \"sec-41\"\ndate = 2024-12-31\nreturn-of-capital = 0.01\nreinvested-distribution = 12\n")

	require.NoError(t, err)
	require.Len(t, cfg.Adjustments, 1)
	assert.Equal(t, int64(1), cfg.Adjustments[0].ReturnOfCapital)
	assert.Equal(t, int64(1200), cfg.Adjustments[0].ReinvestedDistribution)
}

func Test_load_names_reinvested_distribution_when_that_amount_is_bad(t *testing.T) {
	_, _, err := load(t, "[[acb.adjustment]]\nsecurity = \"sec-41\"\ndate = 2024-12-31\nreinvested-distribution = 0\n")

	require.EqualError(t, err, shownPath+": "+adjItemOne+": reinvested-distribution"+adjAmountMust+"0"+fixLine)
}

func Test_load_reports_return_of_capital_before_reinvested_distribution_when_both_are_bad(t *testing.T) {
	_, _, err := load(t, "[[acb.adjustment]]\nsecurity = \"sec-41\"\ndate = 2024-12-31\nreinvested-distribution = 0\nreturn-of-capital = -1\n")

	require.EqualError(t, err, shownPath+": "+adjItemOne+": return-of-capital"+adjAmountMust+"-1"+fixLine)
}

func Test_load_shows_an_amount_written_as_a_table_by_kind(t *testing.T) {
	_, _, err := load(t, "[[acb.adjustment]]\nsecurity = \"sec-41\"\ndate = 2024-12-31\nreturn-of-capital.x = 1\n")

	require.EqualError(t, err, shownPath+": "+adjItemOne+": return-of-capital"+adjAmountMust+"a table"+fixLine)
}

func Test_load_shows_an_amount_without_its_trailing_comment(t *testing.T) {
	_, _, err := load(t, "[[acb.adjustment]]\nsecurity = \"sec-41\"\ndate = 2024-12-31\nreturn-of-capital = 12.345   # T3 box 42\n")

	require.EqualError(t, err, shownPath+": "+adjItemOne+": return-of-capital"+adjAmountMust+"12.345"+fixLine)
}

func Test_load_leaves_adjustments_nil_when_the_file_has_none(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{name: "empty file", content: ""},
		{name: "other keys only", content: "snapshots.keep = 2\n"},
		{name: "empty acb table", content: "[acb]\n"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, cfg, err := load(t, c.content)

			require.NoError(t, err)
			assert.Nil(t, cfg.Adjustments)
		})
	}
}

func Test_ProblemAbsolute_names_the_config_file_by_its_absolute_path_in_an_adjustment_refusal(t *testing.T) {
	_, path, err := loadPath(t, "[[acb.adjustment]]\ndate = 2024-12-31\n")

	assert.Equal(t, path+`: acb.adjustment item 1 needs security, such as security = "sec-41"`, config.ProblemAbsolute(err))
}
