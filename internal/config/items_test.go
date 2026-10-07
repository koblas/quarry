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
		{
			name:    "ids that differ in letter case",
			content: "accounts.registered = [\"Acct-3\"]\naccounts.non-registered = [\"acct-3\"]\n", registered: []string{"Acct-3"}, nonRegistered: []string{"acct-3"},
		},
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

const (
	ignoreMust   = "findings.ignore must be a list of finding ids in quotes, such as [\"duplicate:txn-4410+txn-4412\"], got "
	ignoreOnly   = "findings.ignore must hold only finding ids in quotes, got "
	findingsMust = "findings must be a table, such as findings.ignore = [\"duplicate:txn-4410+txn-4412\"], got "
)

func Test_load_reads_findings_ignore_in_file_order_keeping_duplicates_and_empty_ids(t *testing.T) {
	_, cfg, err := load(t, "findings.ignore = [\"b\", \"a\", \"\", \"b\", \"duplicate:txn-1+txn-2\", 'lit:x']\n")

	require.NoError(t, err)
	assert.Equal(t, []string{"b", "a", "", "b", "duplicate:txn-1+txn-2", "lit:x"}, cfg.Ignore)
	assert.Empty(t, cfg.Warnings)
}

func Test_load_leaves_findings_ignore_empty_when_unset_or_an_empty_list(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{name: "unset", content: "snapshots.keep = 2\n"},
		{name: "findings table without the key", content: "[findings]\n"},
		{name: "empty list", content: "findings.ignore = []\n"},
		{name: "empty multi-line list", content: "findings.ignore = [\n  # none yet\n]\n"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, cfg, err := load(t, c.content)

			require.NoError(t, err)
			assert.Nil(t, cfg.Ignore)
			assert.Empty(t, cfg.Warnings)
		})
	}
}

func Test_load_reads_findings_ignore_in_its_written_forms(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    []string
	}{
		{name: "from_an_inline_table", content: "findings = { ignore = [\"a\"] }\n", want: []string{"a"}},
		{name: "spread_over_lines_with_comments_and_a_trailing_comma", content: "[findings]\nignore = [\n  \"a\", # first\n  # between\n  \"b\",\n]\n", want: []string{"a", "b"}},
		{
			name:    "items_holding_commas_brackets_quotes_and_hashes",
			content: "findings.ignore = [\"a,b\", 'c]d', \"e\\\"f,\", \"#g\", \"\"\"h,\ni\"\"\", '''j,'k''', \"\"\"l\\\"\"\"m\"\"\", \"\"\"n\"\"\"\"]\n",
			want:    []string{"a,b", "c]d", "e\"f,", "#g", "h,\ni", "j,'k", "l\"\"\"m", "n\""},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, cfg, err := load(t, c.content)

			require.NoError(t, err)
			assert.Equal(t, c.want, cfg.Ignore)
		})
	}
}

func Test_load_ignores_findings_ignore_spelled_with_another_letter_case(t *testing.T) {
	_, cfg, err := load(t, "[Findings]\nIgnore = [\"a\"]\n")

	require.NoError(t, err)
	assert.Nil(t, cfg.Ignore)
	assert.Equal(t, []string{shownPath + ": unknown key Findings; quarry ignores it"}, cfg.Warnings)
}

func Test_load_refuses_a_findings_ignore_that_is_not_a_list(t *testing.T) {
	cases := []struct {
		name    string
		content string
		got     string
	}{
		{name: "string keeps its quotes", content: "findings.ignore = \"duplicate:txn-4410+txn-4412\"\n", got: `"duplicate:txn-4410+txn-4412"`},
		{name: "literal string keeps its quotes", content: "findings.ignore = 'a'\n", got: "'a'"},
		{name: "integer", content: "[findings]\nignore = 12\n", got: "12"},
		{name: "boolean", content: "findings.ignore = true\n", got: "true"},
		{name: "float", content: "findings.ignore = 2.5\n", got: "2.5"},
		{name: "date", content: "findings.ignore = 2026-01-01\n", got: "2026-01-01"},
		{name: "inline table over lines is collapsed", content: "findings.ignore = { a = 1,\n   b = 2 }\n", got: "{ a = 1, b = 2 }"},
		{name: "table header", content: "[findings.ignore]\nx = 1\n", got: "a table"},
		{name: "dotted keys", content: "findings.ignore.x = 1\n", got: "a table"},
		{name: "list of tables header", content: "[[findings.ignore]]\nx = 1\n", got: "a list of tables"},
		{name: "string inside an inline table", content: "findings = { ignore = \"x\" }\n", got: `"x"`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := refusal(t, c.content)

			assert.Equal(t, shownPath+": "+ignoreMust+c.got+fixLine, got)
		})
	}
}

func Test_load_refuses_findings_as_a_plain_value(t *testing.T) {
	cases := []struct {
		name    string
		content string
		got     string
	}{
		{name: "integer", content: "findings = 3\n", got: "3"},
		{name: "string", content: "findings = \"x\"\n", got: `"x"`},
		{name: "multi-line array is collapsed", content: "findings = [1,\n  2]\n", got: "[1, 2]"},
		{name: "list of tables", content: "[[findings]]\nx = 1\n", got: "a list of tables"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := refusal(t, c.content)

			assert.Equal(t, shownPath+": "+findingsMust+c.got+fixLine, got)
		})
	}
}

func Test_load_leaves_a_findings_ignore_item_unmasked(t *testing.T) {
	got := refusal(t, "findings.ignore = [12345678]\n")

	assert.Equal(t, shownPath+": findings.ignore must hold only finding ids in quotes, got 12345678 as item 1"+fixLine, got)
}

func Test_load_refuses_a_findings_ignore_item_that_is_not_a_string(t *testing.T) {
	cases := []struct {
		name    string
		content string
		got     string
	}{
		{name: "integer as the third item", content: "findings.ignore = [\"a\", \"b\", 12]\n", got: "12 as item 3"},
		{name: "integer as the first item", content: "findings.ignore = [12]\n", got: "12 as item 1"},
		{name: "boolean", content: "findings.ignore = [\"a\", true]\n", got: "true as item 2"},
		{name: "float", content: "findings.ignore = [\"a\", 1.5]\n", got: "1.5 as item 2"},
		{name: "date", content: "findings.ignore = [\"a\", 2026-01-01]\n", got: "2026-01-01 as item 2"},
		{name: "nested list", content: "findings.ignore = [\"a\", [\"x\", 1]]\n", got: "[\"x\", 1] as item 2"},
		{name: "inline table over lines is collapsed", content: "findings.ignore = [\"a\", { a = 1,\n  b = \"x,y\" }]\n", got: "{ a = 1, b = \"x,y\" } as item 2"},
		{name: "first of two bad items", content: "findings.ignore = [\"a\", 1, 2]\n", got: "1 as item 2"},
		{name: "comment lines do not shift the index", content: "findings.ignore = [\n  # one\n  \"a\",\n  # two\n  \"b\", # three\n  7,\n]\n", got: "7 as item 3"},
		{name: "trailing comma", content: "findings.ignore = [\"a\", 9,]\n", got: "9 as item 2"},
		{name: "inline array of tables", content: "findings.ignore = [{ a = 1 }]\n", got: "{ a = 1 } as item 1"},
		{name: "comma inside a basic string", content: `findings.ignore = ["a,b", 12]` + "\n", got: "12 as item 2"},
		{name: "bracket inside a literal string", content: `findings.ignore = ['c]d', 12]` + "\n", got: "12 as item 2"},
		{name: "hash inside a basic string", content: `findings.ignore = ["#g", 12]` + "\n", got: "12 as item 2"},
		{name: "escaped quote inside a basic string", content: `findings.ignore = ["e\"f,", 12]` + "\n", got: "12 as item 2"},
		{name: "backslash ends a literal string", content: `findings.ignore = ['C:\', 12]` + "\n", got: "12 as item 2"},
		{name: "comma inside a multi-line basic string", content: "findings.ignore = [\"\"\"h,\ni\"\"\", 12]\n", got: "12 as item 2"},
		{name: "quote and comma inside a multi-line literal string", content: "findings.ignore = ['''j,'k''', 12]\n", got: "12 as item 2"},
		{name: "multi-line basic string ending in a quote", content: "findings.ignore = [\"\"\"n\"\"\"\", 12]\n", got: "12 as item 2"},
		{name: "escaped quotes inside a multi-line basic string", content: "findings.ignore = [\"\"\"l\\\"\"\"m\"\"\", 12]\n", got: "12 as item 2"},
		{name: "backslash ends a multi-line literal string", content: `findings.ignore = ['''C:\''', 12]` + "\n", got: "12 as item 2"},
		{name: "bracket inside a string in a nested list", content: `findings.ignore = ["a", ["]", 1], 12]` + "\n", got: `["]", 1] as item 2`},
		{name: "comma inside a nested list", content: `findings.ignore = ["a", [1, 2], 12]` + "\n", got: "[1, 2] as item 2"},
		{name: "brace inside a string in an inline table", content: `findings.ignore = ["a", {x = "}"}, 12]` + "\n", got: `{x = "}"} as item 2`},
		{name: "comma inside an inline table", content: `findings.ignore = ["a", {x = 1, y = 2}]` + "\n", got: "{x = 1, y = 2} as item 2"},
		{name: "comment after the closing bracket", content: `findings.ignore = ["a", 1] # ]` + "\n", got: "1 as item 2"},
		{name: "CRLF line endings", content: "findings.ignore = [\r\n \"a\",\r\n 12\r\n]\r\n", got: "12 as item 2"},
		{name: "list inside an inline table", content: `findings = { ignore = ["a", 1] }` + "\n", got: "1 as item 2"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := refusal(t, c.content)

			assert.Equal(t, shownPath+": "+ignoreOnly+c.got+fixLine, got)
		})
	}
}

// The bad item is a list, whose text differs from its decoded form, so a split that loses
// its place cannot pass by showing the decoded value.
func Test_load_spells_a_bad_item_as_written_after_a_string_holding_delimiters(t *testing.T) {
	cases := []struct {
		name  string
		first string
	}{
		{name: "comma inside a basic string", first: `"a,b"`},
		{name: "bracket inside a literal string", first: `'c]d'`},
		{name: "hash inside a basic string", first: `"#g"`},
		{name: "escaped quote inside a basic string", first: `"e\"f,"`},
		{name: "backslash ends a literal string", first: `'C:\'`},
		{name: "comma inside a multi-line basic string", first: "\"\"\"h,\ni\"\"\""},
		{name: "quote and comma inside a multi-line literal string", first: `'''j,'k'''`},
		{name: "multi-line basic string ending in a quote", first: `"""n""""`},
		{name: "escaped quotes inside a multi-line basic string", first: `"""l\"""m"""`},
		{name: "backslash ends a multi-line literal string", first: `'''C:\'''`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := refusal(t, "findings.ignore = ["+c.first+", [1, 2]]\n")

			assert.Equal(t, shownPath+": "+ignoreOnly+"[1, 2] as item 2"+fixLine, got)
		})
	}
}

func Test_load_checks_findings_ignore_after_quicken_path(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{name: "after snapshots.keep", content: "findings.ignore = 1\nsnapshots.keep = 0\n", want: "snapshots.keep must be a whole number of 1 or more, got 0"},
		{name: "after quicken.path", content: "findings.ignore = 1\nquicken.path = 12\n", want: "quicken.path must be a path in quotes, got 12"},
		{name: "findings plain value after quicken.path", content: "findings = 1\nquicken.path = 12\n", want: "quicken.path must be a path in quotes, got 12"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := refusal(t, c.content)

			assert.Equal(t, shownPath+": "+c.want+fixLine, got)
		})
	}
}

func Test_load_warns_about_an_unknown_key_under_findings(t *testing.T) {
	_, cfg, err := load(t, "zeta = 1\n[findings]\nignored = [\"a\"]\nignore = []\nother = 1\n")

	require.NoError(t, err)
	assert.Equal(t, []string{
		shownPath + ": unknown key zeta; quarry ignores it",
		shownPath + ": unknown key findings.ignored; quarry ignores it",
		shownPath + ": unknown key findings.other; quarry ignores it",
	}, cfg.Warnings)
}
