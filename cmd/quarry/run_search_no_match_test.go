package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_search_with_text_that_matches_nothing_prints_an_empty_result_and_the_no_match_warning(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, searchStore())
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"search", "--json", "zzz"}, spendEnv(&stdout, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	doc := decodeSearchJSON(t, stdout.String())
	assert.Equal(t, []searchTransactionJSON{}, doc.Transactions)
	assert.Equal(t, 0, doc.Matched)
	assert.Equal(t, "quarry: warning: no transactions match the search; the store's transactions run 2026-01-20 to 2026-04-02\n", stderr.String())
}
