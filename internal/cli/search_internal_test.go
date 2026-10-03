// White-box: searchCutNote's wording and number grouping are unexported copy, pinned directly.
package cli

import (
	"testing"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

func Test_searchCutNote_names_the_listed_and_matching_counts_grouped_by_thousands(t *testing.T) {
	s := report.Search{Rows: make([]store.SearchRow, 500), Matched: 1234}

	assert.Equal(t, "showing the newest 500 of 1,234 matching transactions; pass --limit 0 to list every one", searchCutNote(s))
}
