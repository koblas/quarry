package importer

import (
	"maps"
	"slices"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

func Test_investment_action_map_covers_every_store_action(t *testing.T) {
	mapped := slices.Sorted(maps.Values(investmentActions))

	assert.Equal(t, store.Actions(), mapped)
}
