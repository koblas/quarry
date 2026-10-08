package mcp_test

import (
	"fmt"
	"testing"

	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/mcp"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	spendingLogPrefix = "quarry: mcp: spending: "
	spendingConfigLog = "cannot read quarry's config file; run quarry spend to see why"
	windowRefusedLine = "refused the call's since or until; details went to the client only"
	missingStoreLine  = "no store at ~/Library/Application Support/quarry/quarry.duckdb yet; run quarry sync to build it"
)

func Test_spending_reads_today_once_at_the_start_of_every_call(t *testing.T) {
	clock := newMidnightClock()
	h := newHarness(t, &fakeStore{}, nil, mcp.WithClock(clock.now))

	first := decodeDoc[document.Spending](t, h.spending(t, map[string]any{}))
	second := decodeDoc[document.Spending](t, h.spending(t, map[string]any{}))

	assert.Equal(t, "2026-09-29", first.Until)
	assert.Equal(t, "2026-09-30", second.Until)
	assert.Equal(t, 2, clock.reads)
}

func Test_spending_refuses_a_window_it_cannot_read_with_the_class_line_before_the_config_or_the_store(t *testing.T) {
	stub := &configStub{}
	h := newHarness(t, &fakeStore{}, nil, mcp.WithConfig(stub.load))

	result := h.spending(t, map[string]any{"since": "last spring"})

	assert.True(t, result.IsError)
	assert.Empty(t, stub.commands)
	assert.Equal(t, `since "last spring" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`, textOf(t, result))
	assert.Zero(t, h.built)
	assert.Equal(t, spendingLogPrefix+windowRefusedLine+"\n", h.stderr.String())
}

func Test_spending_refuses_arguments_the_schema_rejects_without_reading_the_config_or_the_store(t *testing.T) {
	cases := map[string]map[string]any{
		"a null since":          {"since": nil},
		"a lower-case currency": {"currency": "cad"},
		"an unknown property":   {"order": "desc"},
		"an unknown grouping":   {"by": "weekday"},
		"a non-list accounts":   {"accounts": "Chequing"},
	}

	for name, arguments := range cases {
		t.Run(name, func(t *testing.T) {
			stub := &configStub{}
			h := newHarness(t, &fakeStore{}, nil, mcp.WithConfig(stub.load))

			result := h.spending(t, arguments)

			assert.True(t, result.IsError)
			assert.Empty(t, stub.commands)
			assert.Zero(t, h.built)
			assert.Equal(t, spendingLogPrefix+argsRefusedLog+"\n", h.stderr.String())
		})
	}
}

// configUnknownKeyWarning is a config warning the cap tests seed to show it stays ahead of the cap line.
const configUnknownKeyWarning = "/home/dave/config.toml: unknown key x"

func Test_spending_cuts_rows_to_the_cap_keeps_every_total_and_ends_the_warnings_with_the_cap_line(t *testing.T) {
	rows := make([]store.SpendingRow, 501)
	for i := range rows {
		key := fmt.Sprintf("Payee %03d", i)
		rows[i] = store.SpendingRow{Key: &key, Currency: "CAD", Spent: 100}
	}
	totals := []store.SpendingTotal{{Currency: "CAD", Spent: 50100}, {Currency: "USD", Spent: 7}}
	stub := &configStub{cfg: config.Config{WarningsAbsolute: []string{configUnknownKeyWarning}}}
	h := newHarness(t, &fakeStore{spent: store.Spending{Rows: rows, Totals: totals, Unconverted: store.Unconverted{Transactions: 1}}}, nil, mcp.WithConfig(stub.load))

	doc := decodeDoc[document.Spending](t, h.spending(t, map[string]any{"by": "payee"}))

	assert.Len(t, doc.Rows, 500)
	assert.Len(t, doc.Totals, 2)
	require.Len(t, doc.Warnings, 3)
	assert.Equal(t, configUnknownKeyWarning, doc.Warnings[0])
	assert.Equal(t, "spending lists the first 500 rows of 501; totals count every row; pass a shorter period or fewer accounts, or query v_spending for the rest", doc.Warnings[2])
}
