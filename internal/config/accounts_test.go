package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	registeredMust    = "accounts.registered must be a list of account ids in quotes, such as [\"acct-12\"], got "
	registeredOnly    = "accounts.registered must hold only account ids in quotes, got "
	nonRegisteredMust = "accounts.non-registered must be a list of account ids in quotes, such as [\"acct-12\"], got "
	nonRegisteredOnly = "accounts.non-registered must hold only account ids in quotes, got "
	accountsMust      = "accounts must be a table, such as accounts.registered = [\"acct-12\"], got "
)

func Test_load_reads_both_account_lists_in_file_order_keeping_spelling_and_duplicates(t *testing.T) {
	_, cfg, err := load(t, "[accounts]\nregistered = [\"acct-3\", \"acct-1\", \"acct-3\", \"Acct-9\"]\nnon-registered = [\"acct-2\", \"x\", \"acct-2\"]\n")

	require.NoError(t, err)
	assert.Equal(t, []string{"acct-3", "acct-1", "acct-3", "Acct-9"}, cfg.Registered)
	assert.Equal(t, []string{"acct-2", "x", "acct-2"}, cfg.NonRegistered)
	assert.Empty(t, cfg.Warnings)
}

func Test_load_leaves_the_account_lists_nil_when_unset(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{name: "empty file", content: ""},
		{name: "accounts table without the keys", content: "[accounts]\n"},
		{name: "other keys only", content: "snapshots.keep = 2\n"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, cfg, err := load(t, c.content)

			require.NoError(t, err)
			assert.Nil(t, cfg.Registered)
			assert.Nil(t, cfg.NonRegistered)
			assert.Empty(t, cfg.Warnings)
		})
	}
}

func Test_load_reads_the_account_lists_from_dotted_keys_and_an_inline_table(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{name: "dotted keys", content: "accounts.registered = [\"a\"]\naccounts.non-registered = [\"b\"]\n"},
		{name: "inline table", content: "accounts = { registered = [\"a\"], non-registered = [\"b\"] }\n"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, cfg, err := load(t, c.content)

			require.NoError(t, err)
			assert.Equal(t, []string{"a"}, cfg.Registered)
			assert.Equal(t, []string{"b"}, cfg.NonRegistered)
		})
	}
}

func Test_load_keeps_text_that_is_not_an_account_id_as_written(t *testing.T) {
	_, cfg, err := load(t, "accounts.registered = [\"checking\", \"\"]\n")

	require.NoError(t, err)
	assert.Equal(t, []string{"checking", ""}, cfg.Registered)
}

func Test_load_warns_of_a_misspelled_account_key_in_both_warning_lists(t *testing.T) {
	_, cfg, err := load(t, "[accounts]\nregistred = [\"a\"]\n")

	require.NoError(t, err)
	assert.Equal(t, []string{shownPath + ": unknown key accounts.registred; quarry ignores it"}, cfg.Warnings)
	assert.Equal(t, []string{cfg.Path + ": unknown key accounts.registred; quarry ignores it"}, cfg.WarningsAbsolute)
}

func Test_load_refuses_an_account_list_that_is_not_a_list(t *testing.T) {
	cases := []struct {
		name    string
		content string
		must    string
		got     string
	}{
		{name: "registered string", content: "accounts.registered = \"a\"\n", must: registeredMust, got: `"a"`},
		{name: "registered integer", content: "[accounts]\nregistered = 12\n", must: registeredMust, got: "12"},
		{name: "registered table header", content: "[accounts.registered]\nx = 1\n", must: registeredMust, got: "a table"},
		{name: "non-registered string", content: "accounts.non-registered = \"a\"\n", must: nonRegisteredMust, got: `"a"`},
		{name: "non-registered boolean", content: "[accounts]\nnon-registered = true\n", must: nonRegisteredMust, got: "true"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := refusal(t, c.content)

			assert.Equal(t, shownPath+": "+c.must+c.got+fixLine, got)
		})
	}
}

func Test_load_refuses_an_account_list_item_that_is_not_a_string(t *testing.T) {
	cases := []struct {
		name    string
		content string
		only    string
		got     string
	}{
		{name: "registered integer as the first item", content: "accounts.registered = [12]\n", only: registeredOnly, got: "12 as item 1"},
		{name: "registered boolean as the third item", content: "accounts.registered = [\"a\", \"b\", true]\n", only: registeredOnly, got: "true as item 3"},
		{name: "non-registered integer as the second item", content: "accounts.non-registered = [\"a\", 7]\n", only: nonRegisteredOnly, got: "7 as item 2"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := refusal(t, c.content)

			assert.Equal(t, shownPath+": "+c.only+c.got+fixLine, got)
		})
	}
}

func Test_load_refuses_accounts_as_a_plain_value(t *testing.T) {
	got := refusal(t, "accounts = 5\n")

	assert.Equal(t, shownPath+": "+accountsMust+"5"+fixLine, got)
}

func Test_load_masks_an_account_number_in_a_refusal_of_an_account_list(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{name: "registered plain integer", content: "accounts.registered = 12345678\n", want: registeredMust + "****5678"},
		{name: "registered quoted string", content: "accounts.registered = \"12345678\"\n", want: registeredMust + `"****5678"`},
		{name: "non-registered plain integer", content: "accounts.non-registered = 12345678\n", want: nonRegisteredMust + "****5678"},
		{name: "non-registered quoted string", content: "accounts.non-registered = \"12345678\"\n", want: nonRegisteredMust + `"****5678"`},
		{name: "registered item that is a number", content: "accounts.registered = [12345678]\n", want: registeredOnly + "****5678 as item 1"},
		{name: "non-registered item that is a number", content: "accounts.non-registered = [12345678]\n", want: nonRegisteredOnly + "****5678 as item 1"},
		{name: "second item that is a longer number", content: "accounts.registered = [\"a\", 123456789012]\n", want: registeredOnly + "********9012 as item 2"},
		{name: "accounts written as a number", content: "accounts = 12345678\n", want: accountsMust + "****5678"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := refusal(t, c.content)

			assert.Equal(t, shownPath+": "+c.want+fixLine, got)
		})
	}
}

func Test_load_leaves_text_with_at_most_four_digits_in_an_account_refusal_as_written(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{name: "a name with a year", content: "accounts = \"RBC 2019 TFSA\"\n", want: accountsMust + `"RBC 2019 TFSA"`},
		{name: "the item number is not counted", content: "accounts.registered = [\"a\", 1234]\n", want: registeredOnly + "1234 as item 2"},
		{name: "an id of the form acct-digits", content: "accounts.registered = [\"a\", 12345678, \"acct-12345678\"]\n", want: registeredOnly + "****5678 as item 2"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := refusal(t, c.content)

			assert.Equal(t, shownPath+": "+c.want+fixLine, got)
		})
	}
}

func Test_load_masks_an_account_number_named_by_an_unknown_key_in_both_warning_lists(t *testing.T) {
	cases := []struct {
		name    string
		content string
		key     string
	}{
		{name: "a number as the key", content: "[accounts]\n12345678 = 1\n", key: "accounts.****5678"},
		{name: "an acct id as the key", content: "[accounts]\nacct-12345678 = 1\n", key: "accounts.acct-12345678"},
		{name: "a quoted name holding a number", content: "[accounts]\n\"RBC 12345678\" = 1\n", key: `accounts."RBC ****5678"`},
		{name: "a number outside accounts", content: "x12345678 = 1\n", key: "x12345678"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, cfg, err := load(t, c.content)

			require.NoError(t, err)
			assert.Equal(t, []string{shownPath + ": unknown key " + c.key + "; quarry ignores it"}, cfg.Warnings)
			assert.Equal(t, []string{cfg.Path + ": unknown key " + c.key + "; quarry ignores it"}, cfg.WarningsAbsolute)
		})
	}
}

func Test_load_refuses_a_bad_registered_list_before_a_bad_non_registered_one(t *testing.T) {
	got := refusal(t, "accounts.non-registered = 1\naccounts.registered = 2\n")

	assert.Equal(t, shownPath+": "+registeredMust+"2"+fixLine, got)
}

const inBothMust = "an account must be in only one of accounts.registered and accounts.non-registered, got "

func Test_load_refuses_an_id_listed_in_both_account_lists(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{name: "an acct id", content: "accounts.registered = [\"acct-3\"]\naccounts.non-registered = [\"acct-3\"]\n", want: `"acct-3" in both`},
		{name: "a number is masked", content: "accounts.registered = [\"12345678\"]\naccounts.non-registered = [\"12345678\"]\n", want: `"****5678" in both`},
		{name: "two shared ids name the first in registered order", content: "accounts.registered = [\"b\", \"a\"]\naccounts.non-registered = [\"a\", \"b\"]\n", want: `"b" in both`},
		{name: "an empty id", content: "accounts.registered = [\"\"]\naccounts.non-registered = [\"\"]\n", want: `"" in both`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := refusal(t, c.content)

			assert.Equal(t, shownPath+": "+inBothMust+c.want+fixLine, got)
		})
	}
}

func Test_load_refuses_an_id_listed_in_both_account_lists_in_either_list_order(t *testing.T) {
	got := refusal(t, "accounts.non-registered = [\"acct-3\"]\naccounts.registered = [\"acct-3\"]\n")

	assert.Equal(t, shownPath+": "+inBothMust+`"acct-3" in both`+fixLine, got)
}

func Test_load_loads_account_lists_that_share_no_id_exactly(t *testing.T) {
	cases := []struct {
		name          string
		content       string
		registered    []string
		nonRegistered []string
	}{
		{name: "the same id twice in one list", content: "accounts.registered = [\"acct-3\", \"acct-3\"]\n", registered: []string{"acct-3", "acct-3"}},
		{name: "ids that differ in letter case", content: "accounts.registered = [\"Acct-3\"]\naccounts.non-registered = [\"acct-3\"]\n", registered: []string{"Acct-3"}, nonRegistered: []string{"acct-3"}},
		{name: "only the non-registered list set", content: "accounts.non-registered = [\"acct-3\"]\n", nonRegistered: []string{"acct-3"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, cfg, err := load(t, c.content)

			require.NoError(t, err)
			assert.Equal(t, c.registered, cfg.Registered)
			assert.Equal(t, c.nonRegistered, cfg.NonRegistered)
		})
	}
}

func Test_load_refuses_a_bad_account_list_before_an_id_listed_in_both(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{name: "bad registered item", content: "accounts.registered = [\"a\", 1]\naccounts.non-registered = [\"a\"]\n", want: registeredOnly + "1 as item 2"},
		{name: "bad non-registered item", content: "accounts.registered = [\"a\"]\naccounts.non-registered = [\"a\", 1]\n", want: nonRegisteredOnly + "1 as item 2"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := refusal(t, c.content)

			assert.Equal(t, shownPath+": "+c.want+fixLine, got)
		})
	}
}
