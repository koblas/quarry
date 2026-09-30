package config_test

import (
	"errors"
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
