package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_run_mcp_search_transactions_returns_the_search_json_document(t *testing.T) {
	cases := []struct {
		name      string
		cliArgs   []string
		arguments map[string]any
	}{
		{name: "nothing given", cliArgs: []string{"search"}, arguments: map[string]any{}},
		{
			name: "all eight given",
			cliArgs: []string{
				"search", "bakery", "--since", "2026-02", "--until", "2026-03", "--account", "Chequing",
				"--category", "food:groceries", "--min", "5", "--max", "20", "--limit", "10",
			},
			arguments: map[string]any{
				"text": "bakery", "since": "2026-02", "until": "2026-03", "accounts": []string{"Chequing"},
				"category": "food:groceries", "min": "5", "max": "20", "limit": 10,
			},
		},
		{name: "no match", cliArgs: []string{"search", "zzz"}, arguments: map[string]any{"text": "zzz"}},
		{
			name: "no match with accounts named", cliArgs: []string{"search", "zzz", "--account", "Chequing"},
			arguments: map[string]any{"text": "zzz", "accounts": []string{"Chequing"}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{
				store: replaceSearchStore, cliArgs: c.cliArgs, tool: "search_transactions", arguments: c.arguments,
			})

			assert.Equal(t, got.cliBody, got.toolBody)
			assert.Equal(t, got.cliWarnings, got.toolWarnings)
		})
	}
}

// replaceSearchStore seeds home with searchStore, in the shape runBothSurfaces' store hook takes.
func replaceSearchStore(t *testing.T, home string) {
	t.Helper()
	replaceStore(t, home, searchStore())
}
