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

func Test_load_refuses_a_bad_registered_list_before_a_bad_non_registered_one(t *testing.T) {
	got := refusal(t, "accounts.non-registered = 1\naccounts.registered = 2\n")

	assert.Equal(t, shownPath+": "+registeredMust+"2"+fixLine, got)
}
