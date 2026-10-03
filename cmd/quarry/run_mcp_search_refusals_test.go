package main

import (
	"context"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	argumentsRefusedLog = "refused the call's arguments; details went to the client only"
	textRefusedLog      = "refused the call's text; details went to the client only"
	amountRefusedLog    = "refused the call's min or max; details went to the client only"
	unknownCategoryLog  = "refused the call's category: it names no category; details went to the client only"
)

// callSearchRefused calls search_transactions over replaceSearchStore's store and returns the refused result and
// everything the server wrote to stderr.
func callSearchRefused(t *testing.T, arguments map[string]any) (*sdk.CallToolResult, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceSearchStore(t, home)
	ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
	defer cancel()
	peer := startClockedMCP(ctx, t)

	result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{Name: "search_transactions", Arguments: arguments})
	require.NoError(t, err)
	require.NoError(t, peer.session.Close())
	peer.waitForExit(ctx, t)

	require.True(t, result.IsError, textOf(result))
	return result, peer.stderr.String()
}

func Test_run_mcp_search_transactions_refuses_bad_input_without_the_callers_values_on_stderr(t *testing.T) {
	t.Run("handler refusals", func(t *testing.T) {
		cases := []struct {
			name       string
			arguments  map[string]any
			wantText   string
			wantStderr string
			absent     string
		}{
			{
				name: "blank text", arguments: map[string]any{"text": "  "},
				wantText:   "text is blank; leave it out to search by date, account, category or amount alone",
				wantStderr: searchLogPrefix + textRefusedLog + "\n",
				absent:     "  ",
			},
			{
				name: "min with a sign", arguments: map[string]any{"min": "-12"},
				wantText:   `min "-12" is not an amount; use digits with up to 2 decimals and no sign, such as "25" or "19.99"`,
				wantStderr: searchLogPrefix + amountRefusedLog + "\n",
				absent:     "-12",
			},
			{
				name: "min above max", arguments: map[string]any{"min": "50", "max": "20"},
				wantText:   "min 50 is more than max 20",
				wantStderr: searchLogPrefix + amountRefusedLog + "\n",
				absent:     "50",
			},
			{
				name: "since not a date", arguments: map[string]any{"since": "2024-13"},
				wantText:   `since "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`,
				wantStderr: searchLogPrefix + windowRefusedLog + "\n",
				absent:     "2024-13",
			},
			{
				name: "account names none", arguments: map[string]any{"accounts": []string{"Nope"}},
				wantText:   `no account named "Nope"; call describe_schema to list the accounts`,
				wantStderr: searchLogPrefix + unknownAccountLog + "\n",
				absent:     "Nope",
			},
			{
				name: "category names none", arguments: map[string]any{"category": "Fod"},
				wantText:   `no category named "Fod"; call describe_schema to list the categories`,
				wantStderr: searchLogPrefix + unknownCategoryLog + "\n",
				absent:     "Fod",
			},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				result, stderr := callSearchRefused(t, c.arguments)

				assert.Equal(t, c.wantText, textOf(result))
				assert.Equal(t, c.wantStderr, stderr)
				assert.NotContains(t, stderr, c.absent)
			})
		}
	})

	t.Run("schema refusals", func(t *testing.T) {
		cases := []struct {
			name      string
			arguments map[string]any
			wantField string
		}{
			{name: "min as a JSON number", arguments: map[string]any{"min": 12}, wantField: "min"},
			{name: "limit above 500", arguments: map[string]any{"limit": 501}, wantField: "limit"},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				result, stderr := callSearchRefused(t, c.arguments)

				assert.Contains(t, textOf(result), c.wantField)
				assert.Equal(t, searchLogPrefix+argumentsRefusedLog+"\n", stderr)
			})
		}
	})
}
