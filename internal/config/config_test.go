package config_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	shownPath = "~/Library/Application Support/quarry/config.toml"
	fixLine   = "; fix the file and run the command again"
)

// newHome is a fresh home and the config file's path under it, its folder made.
func newHome(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	path := filepath.Join(home, "Library", "Application Support", "quarry", "config.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))

	return home, path
}

// load writes content as the config file under a fresh home and loads it.
func load(t *testing.T, content string) (string, config.Config, error) {
	t.Helper()
	home, path := newHome(t)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	cfg, err := config.Load(home, path)

	return home, cfg, err
}

// skipIfReadable skips the test when path can be read despite its mode, as it can by root.
func skipIfReadable(t *testing.T, path string) {
	t.Helper()
	if _, err := os.ReadFile(path); err == nil {
		t.Skip("running with permission to read a mode 000 file")
	}
}

// refusal loads content and returns the refusal text, requiring an empty Config with it.
func refusal(t *testing.T, content string) string {
	t.Helper()
	_, cfg, err := load(t, content)
	require.Error(t, err)
	assert.Zero(t, cfg)

	return err.Error()
}

func Test_load_returns_defaults_for_a_missing_file(t *testing.T) {
	home, path := newHome(t)

	cfg, err := config.Load(home, path)

	require.NoError(t, err)
	assert.Equal(t, config.Config{Path: path, Keep: config.DefaultKeep}, cfg)
}

func Test_load_returns_defaults_for_an_empty_file(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{name: "empty file", content: ""},
		{name: "only a comment", content: "# nothing set\n"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home, path := newHome(t)
			require.NoError(t, os.WriteFile(path, []byte(c.content), 0o600))

			cfg, err := config.Load(home, path)

			require.NoError(t, err)
			assert.Equal(t, config.Config{Path: path, Keep: config.DefaultKeep}, cfg)
		})
	}
}

func Test_load_refuses_a_directory_in_place_of_the_file(t *testing.T) {
	home, path := newHome(t)
	require.NoError(t, os.Mkdir(path, 0o700))

	_, err := config.Load(home, path)

	require.EqualError(t, err, "cannot read "+shownPath+": is a directory"+fixLine)
	assert.ErrorAs(t, err, new(*fs.PathError))
}

func Test_load_refuses_a_config_folder_that_is_a_file(t *testing.T) {
	home := t.TempDir()
	folder := filepath.Join(home, "Library", "Application Support", "quarry")
	require.NoError(t, os.MkdirAll(filepath.Dir(folder), 0o700))
	require.NoError(t, os.WriteFile(folder, nil, 0o600))

	_, err := config.Load(home, filepath.Join(folder, "config.toml"))

	require.EqualError(t, err, "cannot read "+shownPath+": not a directory"+fixLine)
}

func Test_load_refuses_a_file_it_has_no_permission_to_read(t *testing.T) {
	home, path := newHome(t)
	require.NoError(t, os.WriteFile(path, []byte("[snapshots]\n"), 0o000))
	skipIfReadable(t, path)

	_, err := config.Load(home, path)

	require.EqualError(t, err, "cannot read "+shownPath+": permission denied"+fixLine)
}

func Test_load_refuses_malformed_toml_naming_the_line(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{name: "unclosed table header on line 1", content: "[snapshots\nkeep = 24\n", want: "line 1: expected ']' to close table name"},
		{name: "value missing on line 3", content: "# c\na = 1\nb = \n", want: "line 3: unexpected character U+000A at start of value"},
		{name: "duplicate key", content: "a = 1\na = 2\n", want: "line 2: key a is already defined"},
		{name: "invalid UTF-8", content: "a = \"\xff\"\n", want: "line 1: invalid UTF-8 character in basic string"},
		{name: "byte order mark", content: "\ufeffa = 1\n", want: "line 1: invalid character at start of key: U+00EF 'ï'"},
		{name: "integer too large", content: "[snapshots]\nkeep = 99999999999999999999\n", want: "line 2: decimal number is too large to fit in a 64-bit signed integer"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := refusal(t, c.content)

			assert.Equal(t, "cannot read "+shownPath+": "+c.want+fixLine, got)
		})
	}
}

func Test_load_reads_a_key_from_a_table_or_a_dotted_key(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{name: "table header", content: "[snapshots]\nkeep = 24\n"},
		{name: "dotted key", content: "snapshots.keep = 24\n"},
		{name: "inline table", content: "snapshots = { keep = 24 }\n"},
		{name: "hexadecimal", content: "snapshots.keep = 0x18\n"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, cfg, err := load(t, c.content)

			require.NoError(t, err)
			assert.Equal(t, 24, cfg.Keep)
		})
	}
}

func Test_load_sets_the_config_path_and_leaves_quicken_path_empty_when_unset(t *testing.T) {
	home, path := newHome(t)
	require.NoError(t, os.WriteFile(path, []byte("[snapshots]\nkeep = 3\n"), 0o600))

	cfg, err := config.Load(home, path)

	require.NoError(t, err)
	assert.Equal(t, config.Config{Path: path, Keep: 3}, cfg)
}

func Test_load_accepts_a_snapshots_keep_of_one(t *testing.T) {
	_, cfg, err := load(t, "snapshots.keep = 1\n")

	require.NoError(t, err)
	assert.Equal(t, 1, cfg.Keep)
}

func Test_load_refuses_a_snapshots_keep_below_one_or_not_an_integer(t *testing.T) {
	cases := []struct {
		name    string
		content string
		got     string
	}{
		{name: "zero", content: "snapshots.keep = 0\n", got: "0"},
		{name: "negative", content: "snapshots.keep = -1\n", got: "-1"},
		{name: "fraction", content: "snapshots.keep = 2.5\n", got: "2.5"},
		{name: "float with a whole value", content: "snapshots.keep = 12.0\n", got: "12.0"},
		{name: "string keeps its quotes", content: "snapshots.keep = \"twelve\"\n", got: `"twelve"`},
		{name: "literal string keeps its quotes", content: "snapshots.keep = 'twelve'\n", got: "'twelve'"},
		{name: "boolean", content: "snapshots.keep = true\n", got: "true"},
		{name: "date", content: "snapshots.keep = 2026-01-01\n", got: "2026-01-01"},
		{name: "trailing comment is not part of the value", content: "[snapshots]\nkeep = 0 # none\n", got: "0"},
		{name: "inline table right of the equal sign", content: "snapshots.keep = { a = 1 }\n", got: "{ a = 1 }"},
		{name: "key inside an inline snapshots table", content: "snapshots = { keep = 0 }\n", got: "0"},
		{name: "the exact key, not one that differs in letter case, before it", content: "[snapshots]\nkeep = \"x\"\nKeep = 9\n", got: `"x"`},
		{name: "the exact key, not one that differs in letter case, after it", content: "[snapshots]\nKeep = 9\nkeep = \"x\"\n", got: `"x"`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := refusal(t, c.content)

			assert.Equal(t, shownPath+": snapshots.keep must be a whole number of 1 or more, got "+c.got+fixLine, got)
		})
	}
}

func Test_load_refuses_a_quicken_path_that_is_not_a_string(t *testing.T) {
	cases := []struct {
		name    string
		content string
		got     string
	}{
		{name: "integer", content: "quicken.path = 12\n", got: "12"},
		{name: "boolean", content: "[quicken]\npath = true\n", got: "true"},
		{name: "array", content: "quicken.path = [\"~/a.quicken\"]\n", got: `["~/a.quicken"]`},
		{name: "inline table", content: "quicken.path = { x = 1 }\n", got: "{ x = 1 }"},
		{name: "inline table under a header", content: "[quicken]\npath = { a = \"b\" }\n", got: `{ a = "b" }`},
		{name: "table header", content: "[quicken.path]\nx = 1\n", got: "a table"},
		{name: "dotted keys", content: "quicken.path.x = 1\n", got: "a table"},
		{name: "the exact key, not one that differs in letter case", content: "[quicken]\npath = 12\nPATH = \"~/ok.quicken\"\n", got: "12"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := refusal(t, c.content)

			assert.Equal(t, shownPath+": quicken.path must be a path in quotes, got "+c.got+fixLine, got)
		})
	}
}

func Test_load_refuses_a_relative_or_empty_quicken_path(t *testing.T) {
	cases := []struct {
		name    string
		content string
		got     string
	}{
		{name: "bare file name", content: "quicken.path = \"Home.quicken\"\n", got: `"Home.quicken"`},
		{name: "empty", content: "quicken.path = \"\"\n", got: `""`},
		{name: "home alone", content: "quicken.path = \"~\"\n", got: `"~"`},
		{name: "another user's home", content: "quicken.path = \"~user/x\"\n", got: `"~user/x"`},
		{name: "literal string keeps its quotes", content: "quicken.path = 'Home.quicken'\n", got: "'Home.quicken'"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := refusal(t, c.content)

			assert.Equal(t, shownPath+": quicken.path must be a full path or start with ~/, got "+c.got+fixLine, got)
		})
	}
}

func Test_load_expands_a_home_relative_quicken_path_only(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    func(home string) string
	}{
		{name: "home relative", content: "quicken.path = \"~/Books/Home.quicken\"\n", want: func(home string) string { return home + "/Books/Home.quicken" }},
		{name: "absolute", content: "quicken.path = \"/Users/x/Home.quicken\"\n", want: func(string) string { return "/Users/x/Home.quicken" }},
		{name: "trailing slash is kept", content: "quicken.path = \"~/Books/Home.quicken/\"\n", want: func(home string) string { return home + "/Books/Home.quicken/" }},
		{name: "dots are not cleaned", content: "quicken.path = \"~/Books/../Home.quicken\"\n", want: func(home string) string { return home + "/Books/../Home.quicken" }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home, cfg, err := load(t, c.content)

			require.NoError(t, err)
			assert.Equal(t, c.want(home), cfg.QuickenPath)
		})
	}
}

func Test_load_warns_about_unknown_keys_in_file_order(t *testing.T) {
	content := "zeta = 1\nsnapshot.keep = 3\n[foo]\nx = 1\n[foo.bar]\ny = 2\n[snapshots]\nkeep = 2\nother = 3\n"

	_, cfg, err := load(t, content)

	require.NoError(t, err)
	assert.Equal(t, []string{
		shownPath + ": unknown key zeta; quarry ignores it",
		shownPath + ": unknown key snapshot.keep; quarry ignores it",
		shownPath + ": unknown key foo; quarry ignores it",
		shownPath + ": unknown key snapshots.other; quarry ignores it",
	}, cfg.Warnings)
}

func Test_load_warns_about_a_key_that_differs_from_a_known_one_only_in_letter_case(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{name: "table name", content: "[Snapshots]\nKeep = 50\n", want: "Snapshots"},
		{name: "key name", content: "[snapshots]\nKeep = 50\n", want: "snapshots.Keep"},
		{name: "dotted key", content: "SNAPSHOTS.KEEP = 50\n", want: "SNAPSHOTS.KEEP"},
		{name: "quicken table with a value that would be refused", content: "[Quicken]\nPath = 12\n", want: "Quicken"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, cfg, err := load(t, c.content)

			require.NoError(t, err)
			assert.Equal(t, []string{shownPath + ": unknown key " + c.want + "; quarry ignores it"}, cfg.Warnings)
		})
	}
}

func Test_load_warns_about_an_unknown_key_inside_an_inline_table(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{name: "inside a known table", content: "snapshots = { keep = 2, other = 3 }\n", want: "snapshots.other"},
		{name: "inside an unknown table, named once", content: "foo = { a = 1, b = 2 }\n", want: "foo"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, cfg, err := load(t, c.content)

			require.NoError(t, err)
			assert.Equal(t, []string{shownPath + ": unknown key " + c.want + "; quarry ignores it"}, cfg.Warnings)
		})
	}
}

func Test_load_ignores_the_values_of_keys_that_differ_from_known_ones_only_in_letter_case(t *testing.T) {
	_, cfg, err := load(t, "[Snapshots]\nKeep = 50\n[Quicken]\nPath = \"/x/Home.quicken\"\n")

	require.NoError(t, err)
	assert.Equal(t, config.DefaultKeep, cfg.Keep)
	assert.Empty(t, cfg.QuickenPath)
}

func Test_load_keeps_the_known_values_beside_unknown_keys(t *testing.T) {
	_, cfg, err := load(t, "extra = 1\nsnapshots.keep = 2\nquicken.path = \"/x/Home.quicken\"\n")

	require.NoError(t, err)
	assert.Equal(t, 2, cfg.Keep)
	assert.Equal(t, "/x/Home.quicken", cfg.QuickenPath)
}

func Test_load_has_no_warnings_when_every_key_is_known(t *testing.T) {
	_, cfg, err := load(t, "snapshots.keep = 2\n")

	require.NoError(t, err)
	assert.Empty(t, cfg.Warnings)
}

func Test_load_checks_snapshots_keep_before_quicken_path(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{name: "quicken first in the file", content: "quicken.path = 12\nsnapshots.keep = 0\n"},
		{name: "quicken is a plain value", content: "quicken = \"x\"\nsnapshots.keep = 0\n"},
		{name: "unknown key beside both", content: "extra = 1\nquicken.path = 12\nsnapshots.keep = 0\n"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := refusal(t, c.content)

			assert.Equal(t, shownPath+": snapshots.keep must be a whole number of 1 or more, got 0"+fixLine, got)
		})
	}
}

func Test_load_never_reports_a_shape_mismatch_as_malformed(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{
			name: "snapshots is an integer", content: "snapshots = 3\n",
			want: shownPath + ": snapshots must be a table, such as snapshots.keep = 12, got 3",
		},
		{
			name: "quicken is a string", content: "quicken = \"x\"\n",
			want: shownPath + ": quicken must be a table, such as quicken.path = \"~/Documents/Home.quicken\", got \"x\"",
		},
		{
			name: "snapshots is a multi-line array", content: "snapshots = [1,\n  2]\n",
			want: shownPath + ": snapshots must be a table, such as snapshots.keep = 12, got [1, 2]",
		},
		{
			name: "quicken is an inline array of strings", content: "quicken = [\"a\", \"b\"]\n",
			want: shownPath + ": quicken must be a table, such as quicken.path = \"~/Documents/Home.quicken\", got [\"a\", \"b\"]",
		},
		{
			name: "snapshots.keep is a table header", content: "[snapshots.keep]\nx = 1\n",
			want: shownPath + ": snapshots.keep must be a whole number of 1 or more, got a table",
		},
		{
			name: "snapshots.keep is a multi-line array", content: "snapshots.keep = [ 1,\n   2 ]\n",
			want: shownPath + ": snapshots.keep must be a whole number of 1 or more, got [ 1, 2 ]",
		},
		{
			name: "quicken.path is a table header", content: "[quicken.path]\nx = 1\n",
			want: shownPath + ": quicken.path must be a path in quotes, got a table",
		},
		{
			name: "quicken.path is a multi-line array", content: "quicken.path = [\n  \"a\",\n  \"b\"\n]\n",
			want: shownPath + ": quicken.path must be a path in quotes, got [ \"a\", \"b\" ]",
		},
		{
			name: "snapshots is a list of tables", content: "[[snapshots]]\n",
			want: shownPath + ": snapshots must be a table, such as snapshots.keep = 12, got a list of tables",
		},
		{
			name: "quicken is a list of tables", content: "[[quicken]]\nx = 1\n",
			want: shownPath + ": quicken must be a table, such as quicken.path = \"~/Documents/Home.quicken\", got a list of tables",
		},
		{
			name: "snapshots.keep is a list of tables", content: "[[snapshots.keep]]\nx = 1\n",
			want: shownPath + ": snapshots.keep must be a whole number of 1 or more, got a list of tables",
		},
		{
			name: "quicken.path is a list of tables", content: "[[quicken.path]]\nx = 1\n",
			want: shownPath + ": quicken.path must be a path in quotes, got a list of tables",
		},
		{
			name: "a quoted list-of-tables header is still a list of tables", content: "[[\"snapshots\"]]\n",
			want: shownPath + ": snapshots must be a table, such as snapshots.keep = 12, got a list of tables",
		},
		{
			name: "snapshots is an integer beside an unrelated list of tables", content: "snapshots = 3\n[[foo]]\nx = 1\n",
			want: shownPath + ": snapshots must be a table, such as snapshots.keep = 12, got 3",
		},
		{
			name: "snapshots.keep is a multi-line array beside an unrelated list of tables", content: "snapshots.keep = [1,\n 2]\n[[foo]]\nx = 1\n",
			want: shownPath + ": snapshots.keep must be a whole number of 1 or more, got [1, 2]",
		},
		{
			name: "snapshots is an inline array of inline tables", content: "snapshots = [{a = 1}]\n",
			want: shownPath + ": snapshots must be a table, such as snapshots.keep = 12, got [{a = 1}]",
		},
		{
			name: "snapshots.keep is an inline array of inline tables", content: "snapshots.keep = [{a = 1},\n  {b = 2}]\n",
			want: shownPath + ": snapshots.keep must be a whole number of 1 or more, got [{a = 1}, {b = 2}]",
		},
		{
			name: "quicken.path is an inline array of inline tables", content: "[quicken]\npath = [{a = 1}]\n",
			want: shownPath + ": quicken.path must be a path in quotes, got [{a = 1}]",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := refusal(t, c.content)

			assert.Equal(t, c.want+fixLine, got)
		})
	}
}

func Test_load_checks_a_plain_snapshots_value_before_quicken_path(t *testing.T) {
	got := refusal(t, "quicken.path = 12\nsnapshots = 3\n")

	assert.Equal(t, shownPath+": snapshots must be a table, such as snapshots.keep = 12, got 3"+fixLine, got)
}
