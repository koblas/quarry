package mcp_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/mcp"
	"github.com/koblas/quarry/internal/report/document"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// steppingClock answers each read with the next of its times, then the last one again, and counts the reads.
type steppingClock struct {
	times []time.Time
	reads int
}

func (c *steppingClock) now() time.Time {
	at := c.times[min(c.reads, len(c.times)-1)]
	c.reads++
	return at
}

// decodeSpending is result's one text block decoded as the spending document.
func decodeSpending(t *testing.T, result *sdk.CallToolResult) document.Spending {
	t.Helper()
	require.False(t, result.IsError, textOf(t, result))
	var doc document.Spending
	require.NoError(t, json.Unmarshal([]byte(textOf(t, result)), &doc))
	return doc
}

func Test_spending_reads_today_once_at_the_start_of_every_call(t *testing.T) {
	clock := &steppingClock{times: []time.Time{
		time.Date(2026, time.September, 29, 23, 59, 0, 0, time.UTC),
		time.Date(2026, time.September, 30, 0, 1, 0, 0, time.UTC),
	}}
	h := newHarness(t, &fakeStore{}, nil, mcp.WithClock(clock.now))

	first := decodeSpending(t, h.spending(t, map[string]any{}))
	second := decodeSpending(t, h.spending(t, map[string]any{}))

	assert.Equal(t, "2026-09-29", first.Until)
	assert.Equal(t, "2026-09-30", second.Until)
	assert.Equal(t, 2, clock.reads)
}
