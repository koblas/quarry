package config_test

import (
	"errors"
	"os"
	"testing"

	"github.com/koblas/quarry/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Problem_drops_the_instruction_to_fix_the_file(t *testing.T) {
	_, _, err := load(t, "[snapshots]\nkeep = 0\n")
	require.Error(t, err)

	got := config.Problem(err)

	assert.Equal(t, shownPath+": snapshots.keep must be a whole number of 1 or more, got 0", got)
}

var errNoHome = errors.New("cannot find your home directory")

func Test_Problem_returns_an_error_without_the_instruction_unchanged(t *testing.T) {
	assert.Equal(t, "cannot find your home directory", config.Problem(errNoHome))
}

func Test_ProblemAbsolute_names_the_file_by_its_absolute_path(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{"a bad value", "[snapshots]\nkeep = 0\n", ": snapshots.keep must be a whole number of 1 or more, got 0"},
		{"malformed TOML", "keep = \n", ": line 1: "},
		{"a bad reporting currency", "reporting.currency = \"EUR\"\n", ": reporting.currency must be CAD, USD or native, got \"EUR\""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home, path, err := loadPath(t, c.content)
			require.Error(t, err)

			got := config.ProblemAbsolute(err)

			assert.Contains(t, got, path+c.want)
			assert.NotContains(t, got, "~")
			assert.NotContains(t, got, fixLine)
			assert.Contains(t, config.Problem(err), "~"+path[len(home):]+c.want, "the abbreviated form is unchanged")
		})
	}
}

func Test_ProblemAbsolute_returns_an_error_without_a_config_path_unchanged(t *testing.T) {
	assert.Equal(t, "cannot find your home directory", config.ProblemAbsolute(errNoHome))
}

func Test_Load_warns_about_unknown_keys_by_the_absolute_path_beside_the_abbreviated_ones(t *testing.T) {
	home, path, err := loadPath(t, "zeta = 1\n[foo]\nx = 1\n")
	require.NoError(t, err)

	cfg, loadErr := config.Load(home, path)

	require.NoError(t, loadErr)
	assert.Equal(t, []string{
		path + ": unknown key zeta; quarry ignores it",
		path + ": unknown key foo; quarry ignores it",
	}, cfg.WarningsAbsolute)
	assert.Equal(t, []string{
		shownPath + ": unknown key zeta; quarry ignores it",
		shownPath + ": unknown key foo; quarry ignores it",
	}, cfg.Warnings)
}

func Test_Load_leaves_both_warning_lists_nil_without_unknown_keys(t *testing.T) {
	home, path, err := loadPath(t, "[snapshots]\nkeep = 3\n")
	require.NoError(t, err)

	cfg, loadErr := config.Load(home, path)

	require.NoError(t, loadErr)
	assert.Nil(t, cfg.Warnings)
	assert.Nil(t, cfg.WarningsAbsolute)
}

// loadPath writes content as the config file under a fresh home and returns the home and the file's path;
// err is the error loading it gives.
func loadPath(t *testing.T, content string) (string, string, error) {
	t.Helper()
	home, path := newHome(t)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	_, err := config.Load(home, path)

	return home, path, err
}

func Test_ProblemAbsolute_names_a_directory_in_place_of_the_file_by_its_absolute_path(t *testing.T) {
	home, path := newHome(t)
	require.NoError(t, os.Mkdir(path, 0o700))

	_, err := config.Load(home, path)

	require.Error(t, err)
	assert.Equal(t, "cannot read "+path+": is a directory", config.ProblemAbsolute(err))
}
