package mcp_test

import (
	"encoding/json"
	"testing"

	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	searchLogPrefix   = "quarry: mcp: search_transactions: "
	textRefusedLine   = "refused the call's text; details went to the client only"
	amountRefusedLine = "refused the call's min or max; details went to the client only"
	narrowWording     = "narrow the search with text, since, until, accounts, category, min or max"
	higherWording     = "pass a higher limit, up to 500, or "
)

// decodeSearch is result's one text block decoded as the search document.
func decodeSearch(t *testing.T, result *sdk.CallToolResult) document.Search {
	t.Helper()
	require.False(t, result.IsError, textOf(t, result))
	var doc document.Search
	require.NoError(t, json.Unmarshal([]byte(textOf(t, result)), &doc))
	return doc
}

// matchesOf is a search of total matches that holds the newest n of them.
func matchesOf(n, total int) store.Search {
	return store.Search{Rows: make([]store.SearchRow, n), Matched: total}
}

func Test_search_transactions_refuses_a_bad_window_in_the_tools_words(t *testing.T) {
	cases := []struct {
		name      string
		arguments map[string]any
		want      string
	}{
		{"a since that is not a date", map[string]any{"since": "last spring"}, `since "last spring" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`},
		{"an empty since", map[string]any{"since": ""}, `since "" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`},
		{"an empty until", map[string]any{"until": ""}, `until "" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`},
		{"a since after until", map[string]any{"since": "2025", "until": "2024"}, "since 2025 is after until 2024"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, &fakeStore{}, nil)

			result := h.searchTransactions(t, c.arguments)

			assert.True(t, result.IsError)
			assert.Equal(t, c.want, textOf(t, result))
			assert.Zero(t, h.built)
			assert.Equal(t, searchLogPrefix+windowRefusedLine+"\n", h.stderr.String())
		})
	}
}

func Test_search_transactions_refuses_blank_text_and_a_bad_amount_before_building_a_report(t *testing.T) {
	const amountHint = `is not an amount; use digits with up to 2 decimals and no sign, such as "25" or "19.99"`
	cases := []struct {
		name      string
		arguments map[string]any
		want      string
		wantClass string
	}{
		{
			"blank text",
			map[string]any{"text": "  "},
			"text is blank; leave it out to search by date, account, category or amount alone", textRefusedLine,
		},
		{
			"empty text",
			map[string]any{"text": ""},
			"text is blank; leave it out to search by date, account, category or amount alone", textRefusedLine,
		},
		{"a min that is not an amount", map[string]any{"min": "-12"}, `min "-12" ` + amountHint, amountRefusedLine},
		{"an empty max", map[string]any{"max": ""}, `max "" ` + amountHint, amountRefusedLine},
		{"a min above the max", map[string]any{"min": "50", "max": "20"}, "min 50 is more than max 20", amountRefusedLine},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := &fakeStore{}
			h := newHarness(t, fake, nil)

			result := h.searchTransactions(t, c.arguments)

			assert.True(t, result.IsError)
			assert.Equal(t, c.want, textOf(t, result))
			assert.Zero(t, h.built)
			assert.Empty(t, fake.searched)
			assert.Equal(t, searchLogPrefix+c.wantClass+"\n", h.stderr.String())
		})
	}
}

func Test_search_transactions_refuses_a_limit_the_schema_rejects_without_searching(t *testing.T) {
	cases := []struct {
		name  string
		limit any
	}{
		{"zero", 0},
		{"negative", -1},
		{"above the most the tool allows", 501},
		{"null", nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := &fakeStore{}
			h := newHarness(t, fake, nil)

			result := h.searchTransactions(t, map[string]any{"limit": c.limit})

			assert.True(t, result.IsError)
			assert.Zero(t, h.built)
			assert.Empty(t, fake.searched)
			assert.Equal(t, searchLogPrefix+argsRefusedLog+"\n", h.stderr.String())
		})
	}
}

func Test_search_transactions_answers_a_report_factory_failure_with_the_generic_log_line(t *testing.T) {
	h := newHarness(t, &fakeStore{}, errFactoryBroke)

	result := h.searchTransactions(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, errFactoryBroke.Error(), textOf(t, result))
	assert.Equal(t, searchLogPrefix+failedLogLine+"\n", h.stderr.String())
}

func Test_search_transactions_passes_its_arguments_to_the_store(t *testing.T) {
	fake := &fakeStore{}
	h := newHarness(t, fake, nil)

	decodeSearch(t, h.searchTransactions(t, map[string]any{"text": "Costco", "category": "", "limit": 20}))

	require.Len(t, fake.searched, 1)
	assert.Equal(t, "Costco", fake.searched[0].Text)
	assert.Equal(t, new(""), fake.searched[0].Category)
	assert.Equal(t, 20, fake.searched[0].Limit)
}

func Test_search_transactions_gives_the_store_no_category_when_the_call_names_none(t *testing.T) {
	fake := &fakeStore{}
	h := newHarness(t, fake, nil)

	decodeSearch(t, h.searchTransactions(t, map[string]any{}))

	require.Len(t, fake.searched, 1)
	assert.Nil(t, fake.searched[0].Category)
}

func Test_search_transactions_words_its_cut_line_by_the_limit_asked(t *testing.T) {
	cases := []struct {
		name  string
		limit int
		want  string
	}{
		{"at the most the tool allows", 500, "search_transactions lists the newest 500 of 1,234 matching transactions; " + narrowWording},
		{"just under the most", 499, "search_transactions lists the newest 499 of 1,234 matching transactions; " + higherWording + narrowWording},
		{"well under the most", 20, "search_transactions lists the newest 20 of 1,234 matching transactions; " + higherWording + narrowWording},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, &fakeStore{found: matchesOf(c.limit+1, 1234)}, nil)

			doc := decodeSearch(t, h.searchTransactions(t, map[string]any{"limit": c.limit}))

			assert.Equal(t, []string{c.want}, doc.Warnings)
			assert.True(t, doc.Truncated)
			assert.Empty(t, h.stderr.String())
		})
	}
}

func Test_search_transactions_adds_no_cut_line_when_the_limit_cut_nothing(t *testing.T) {
	h := newHarness(t, &fakeStore{found: matchesOf(20, 20)}, nil)

	doc := decodeSearch(t, h.searchTransactions(t, map[string]any{"limit": 20}))

	assert.Empty(t, doc.Warnings)
	assert.False(t, doc.Truncated)
}
