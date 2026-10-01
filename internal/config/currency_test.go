package config_test

import (
	"testing"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_load_reads_reporting_currency_in_any_letter_case(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    money.Currency
	}{
		{name: "usd lower case", content: "reporting.currency = \"usd\"\n", want: money.USD},
		{name: "USD upper case", content: "reporting.currency = \"USD\"\n", want: money.USD},
		{name: "native lower case", content: "reporting.currency = \"native\"\n", want: money.Native},
		{name: "native upper case", content: "reporting.currency = \"NATIVE\"\n", want: money.Native},
		{name: "mixed case cad", content: "reporting.currency = \"Cad\"\n", want: money.CAD},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, cfg, err := load(t, c.content)

			require.NoError(t, err)
			assert.Equal(t, c.want, cfg.Currency)
		})
	}
}

func Test_load_reads_reporting_currency_from_a_table_or_a_dotted_key(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{name: "table header", content: "[reporting]\ncurrency = \"usd\"\n"},
		{name: "dotted key", content: "reporting.currency = \"usd\"\n"},
		{name: "inline table", content: "reporting = { currency = \"usd\" }\n"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, cfg, err := load(t, c.content)

			require.NoError(t, err)
			assert.Equal(t, money.USD, cfg.Currency)
		})
	}
}

func Test_load_defaults_reporting_currency_to_cad_when_the_key_is_unset(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{name: "no reporting table", content: "[snapshots]\nkeep = 3\n"},
		{name: "empty reporting table", content: "[reporting]\n"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, cfg, err := load(t, c.content)

			require.NoError(t, err)
			assert.Equal(t, money.CAD, cfg.Currency)
		})
	}
}

func Test_load_refuses_a_reporting_currency_it_cannot_read_showing_it_as_written(t *testing.T) {
	cases := []struct {
		name    string
		content string
		got     string
	}{
		{name: "another currency", content: "reporting.currency = \"EUR\"\n", got: `"EUR"`},
		{name: "another currency in lower case", content: "reporting.currency = \"eur\"\n", got: `"eur"`},
		{name: "empty string", content: "reporting.currency = \"\"\n", got: `""`},
		{name: "leading space", content: "reporting.currency = \" CAD\"\n", got: `" CAD"`},
		{name: "trailing space", content: "reporting.currency = \"CAD \"\n", got: `"CAD "`},
		{name: "truncated native", content: "reporting.currency = \"nativ\"\n", got: `"nativ"`},
		{name: "integer", content: "reporting.currency = 12\n", got: "12"},
		{name: "boolean", content: "reporting.currency = true\n", got: "true"},
		{name: "table header", content: "[reporting.currency]\nx = 1\n", got: "a table"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := refusal(t, c.content)

			assert.Equal(t, shownPath+": reporting.currency must be CAD, USD or native, got "+c.got+fixLine, got)
		})
	}
}

func Test_load_refuses_reporting_written_as_a_plain_value(t *testing.T) {
	cases := []struct {
		name    string
		content string
		got     string
	}{
		{name: "string", content: "reporting = \"CAD\"\n", got: `"CAD"`},
		{name: "integer", content: "reporting = 1\n", got: "1"},
		{name: "list of tables", content: "[[reporting]]\ncurrency = \"CAD\"\n", got: "a list of tables"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := refusal(t, c.content)

			want := shownPath + `: reporting must be a table, such as reporting.currency = "CAD", got ` + c.got + fixLine
			assert.Equal(t, want, got)
		})
	}
}

func Test_load_checks_findings_ignore_before_reporting_currency(t *testing.T) {
	got := refusal(t, "reporting.currency = \"EUR\"\nfindings.ignore = 12\n")

	assert.Contains(t, got, "findings.ignore must be a list of finding ids in quotes")
}

func Test_load_warns_about_an_unknown_key_in_the_reporting_table(t *testing.T) {
	_, cfg, err := load(t, "[reporting]\ncurrency = \"usd\"\ncolour = 1\n")

	require.NoError(t, err)
	assert.Equal(t, []string{shownPath + ": unknown key reporting.colour; quarry ignores it"}, cfg.Warnings)
	assert.Equal(t, money.USD, cfg.Currency)
}

func Test_load_ignores_a_reporting_currency_that_differs_from_the_key_only_in_letter_case(t *testing.T) {
	_, cfg, err := load(t, "[Reporting]\nCurrency = \"usd\"\n")

	require.NoError(t, err)
	assert.Equal(t, money.CAD, cfg.Currency)
	assert.Equal(t, []string{shownPath + ": unknown key Reporting; quarry ignores it"}, cfg.Warnings)
}

func Test_load_has_no_warnings_when_reporting_currency_is_set(t *testing.T) {
	_, cfg, err := load(t, "[reporting]\ncurrency = \"native\"\n")

	require.NoError(t, err)
	assert.Nil(t, cfg.Warnings)
}
