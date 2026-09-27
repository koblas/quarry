package sqlschema_test

import (
	"testing"

	"github.com/koblas/quarry/internal/platform/sqlschema"
	"github.com/stretchr/testify/assert"
)

// Pinned against `printf 'ZA.X\nZA.Y\n' | shasum -a 256`, computed outside Go.
func Test_fingerprint_matches_the_pinned_vector_for_one_table(t *testing.T) {
	s := sqlschema.Schema{"ZA": {"X", "Y"}}

	got := sqlschema.Fingerprint(s)

	assert.Equal(t, "sha256:1f6f39a1e372c130d3e35a06bc1841bc957c4388a42508a269e4a669f54eb6a2", got)
}

func Test_fingerprint_is_unaffected_by_column_order(t *testing.T) {
	a := sqlschema.Fingerprint(sqlschema.Schema{"ZA": {"X", "Y"}})
	b := sqlschema.Fingerprint(sqlschema.Schema{"ZA": {"Y", "X"}})

	assert.Equal(t, a, b)
}

func Test_fingerprint_differs_for_different_schemas(t *testing.T) {
	a := sqlschema.Fingerprint(sqlschema.Schema{"ZA": {"X"}})
	b := sqlschema.Fingerprint(sqlschema.Schema{"ZB": {"X"}})

	assert.NotEqual(t, a, b)
}
