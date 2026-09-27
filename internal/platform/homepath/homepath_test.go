package homepath_test

import (
	"testing"

	"github.com/koblas/quarry/internal/platform/homepath"
	"github.com/stretchr/testify/assert"
)

func Test_Abbreviate_replaces_the_home_prefix_with_a_tilde(t *testing.T) {
	got := homepath.Abbreviate("/Users/dave", "/Users/dave/Documents/Home.quicken")

	assert.Equal(t, "~/Documents/Home.quicken", got)
}

func Test_Abbreviate_abbreviates_home_itself_to_a_bare_tilde(t *testing.T) {
	got := homepath.Abbreviate("/Users/dave", "/Users/dave")

	assert.Equal(t, "~", got)
}

func Test_Abbreviate_does_not_abbreviate_a_path_that_only_shares_a_prefix_with_home(t *testing.T) {
	got := homepath.Abbreviate("/Users/dave", "/Users/davex/Documents/Home.quicken")

	assert.Equal(t, "/Users/davex/Documents/Home.quicken", got)
}

func Test_Expand_expands_a_leading_tilde_path(t *testing.T) {
	got := homepath.Expand("/Users/dave", "~/Documents/Home.quicken")

	assert.Equal(t, "/Users/dave/Documents/Home.quicken", got)
}

func Test_Expand_leaves_an_absolute_path_unchanged(t *testing.T) {
	got := homepath.Expand("/Users/dave", "/Documents/Home.quicken")

	assert.Equal(t, "/Documents/Home.quicken", got)
}

func Test_Expand_leaves_a_relative_non_tilde_path_unchanged(t *testing.T) {
	got := homepath.Expand("/Users/dave", "Documents/Home.quicken")

	assert.Equal(t, "Documents/Home.quicken", got)
}

func Test_Expand_leaves_a_bare_tilde_unchanged(t *testing.T) {
	got := homepath.Expand("/Users/dave", "~")

	assert.Equal(t, "~", got)
}
