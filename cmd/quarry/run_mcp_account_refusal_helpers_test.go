package main

import (
	"context"
	"testing"

	"github.com/koblas/quarry/internal/store"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// refuseAccountKeepingItsNameOffStderr calls tool with one unreadable accounts entry per case and asserts the
// client text names the argument while stderr carries only the class line behind logPrefix.
func refuseAccountKeepingItsNameOffStderr(t *testing.T, tool, logPrefix string) {
	t.Helper()
	cases := []struct {
		name       string
		arg        string
		want       string
		wantStderr string
		absent     []string
	}{
		{
			name: "no account has the name", arg: "Nope",
			want:       `no account named "Nope"; call describe_schema to list the accounts`,
			wantStderr: logPrefix + unknownAccountLog + "\n",
			absent:     []string{"Nope"},
		},
		{
			name: "the name is empty", arg: "",
			want:       `no account named ""; call describe_schema to list the accounts`,
			wantStderr: logPrefix + unknownAccountLog + "\n",
			absent:     []string{`""`},
		},
		{
			name: "two accounts share the name", arg: "Visa",
			want:       `2 accounts are named "Visa"; pass one of their ids instead: acct-812, acct-977`,
			wantStderr: logPrefix + ambiguousAccountLog + "\n",
			absent:     []string{"Visa", "acct-812", "acct-977"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			replaceStore(t, home, spendRows([]store.Account{
				chequingAccount("acct-chq", 1),
				{ID: "acct-977", SourceID: 2, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true},
				{ID: "acct-812", SourceID: 3, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true},
			}))
			ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
			defer cancel()
			peer := startClockedMCP(ctx, t)

			result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{
				Name: tool, Arguments: map[string]any{"accounts": []string{c.arg}},
			})
			require.NoError(t, err)
			require.NoError(t, peer.session.Close())
			peer.waitForExit(ctx, t)

			assert.True(t, result.IsError)
			assert.Equal(t, c.want, textOf(result))
			assert.Equal(t, c.wantStderr, peer.stderr.String())
			for _, text := range c.absent {
				assert.NotContains(t, peer.stderr.String(), text)
			}
		})
	}
}
