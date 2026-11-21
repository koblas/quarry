// White-box: referenceEntities is the unexported pinned map; the test holds it equal to the numbers the
// embedded reference's own names carry.
package v9

import (
	"regexp"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_reference_entity_map_keys_equal_the_numbers_in_reference_names(t *testing.T) {
	reference, err := Reference(t.Context())
	require.NoError(t, err)
	numberOf := regexp.MustCompile(`^Z(?:_(\d+)[A-Z]|(\d+)_)`)
	found := map[int64]bool{}
	note := func(name string) {
		parts := numberOf.FindStringSubmatch(name)
		if parts == nil {
			return
		}
		digits := parts[1] + parts[2]
		n, err := strconv.ParseInt(digits, 10, 64)
		require.NoError(t, err)
		found[n] = true
	}
	for table, columns := range reference {
		note(table)
		for _, column := range columns {
			note(column)
		}
	}
	want := map[int64]bool{}
	for n := range referenceEntities {
		want[n] = true
	}

	require.NotEmpty(t, found)
	assert.Equal(t, found, want)
}
