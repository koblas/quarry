package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_search_prints_the_newest_500_of_501_matches_unless_limit_0(t *testing.T) {
	const cutLine = "quarry: warning: showing the newest 500 of 501 matching transactions; pass --limit 0 to list every one\n"
	cases := []struct {
		name          string
		args          []string
		wantRows      int
		wantOldest    string
		wantTruncated bool
		wantStderr    string
	}{
		{name: "default limit drops the oldest", args: []string{"search", "--json"}, wantRows: 500, wantOldest: "txn-n002", wantTruncated: true, wantStderr: cutLine},
		{name: "limit 0 lists every match", args: []string{"search", "--json", "--limit", "0"}, wantRows: 501, wantOldest: "txn-n001"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			replaceStore(t, home, searchRows([]store.Account{chequingAccount("acct-chq", 1)}, nil, manySearchTxns(501)...))
			var stdout, stderr bytes.Buffer

			exitCode := runWith(context.Background(), c.args, spendEnv(&stdout, &stderr))

			require.Equal(t, 0, exitCode, stderr.String())
			doc := decodeSearchJSON(t, stdout.String())
			require.Len(t, doc.Transactions, c.wantRows)
			assert.Equal(t, "txn-n501", doc.Transactions[0].TransactionID)
			assert.Equal(t, c.wantOldest, doc.Transactions[c.wantRows-1].TransactionID)
			assert.Equal(t, 501, doc.Matched)
			assert.Equal(t, c.wantTruncated, doc.Truncated)
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}
