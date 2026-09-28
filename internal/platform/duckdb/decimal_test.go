package duckdb_test

import (
	"testing"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_decimal_refuses_an_unscaled_magnitude_of_10_to_the_width(t *testing.T) {
	_, err := duckdb.Decimal(1_000_000_000_000_000_000, 18, 2)

	require.Error(t, err)
}

func Test_decimal_accepts_an_unscaled_magnitude_one_below_10_to_the_width(t *testing.T) {
	value, err := duckdb.Decimal(999_999_999_999_999_999, 18, 2)

	require.NoError(t, err)
	assert.NotNil(t, value)
}

func Test_decimal_refuses_a_negative_unscaled_magnitude_of_10_to_the_width(t *testing.T) {
	_, err := duckdb.Decimal(-1_000_000_000_000_000_000, 18, 2)

	require.Error(t, err)
}
