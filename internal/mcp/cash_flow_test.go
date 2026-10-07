package mcp_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/mcp"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	cashFlowLogPrefix = "quarry: mcp: cash_flow: "
	cashFlowConfigLog = "cannot read quarry's config file; run quarry cashflow to see why"
)

func Test_cash_flow_reads_today_once_at_the_start_of_every_call(t *testing.T) {
	clock := newMidnightClock()
	h := newHarness(t, &fakeStore{}, nil, mcp.WithClock(clock.now))

	first := decodeDoc[document.CashFlow](t, h.cashFlow(t, map[string]any{}))
	second := decodeDoc[document.CashFlow](t, h.cashFlow(t, map[string]any{}))

	assert.Equal(t, "2026-09-29", first.Until)
	assert.Equal(t, "2026-09-30", second.Until)
	assert.Equal(t, 2, clock.reads)
}

func Test_cash_flow_refuses_a_window_it_cannot_read_with_the_class_line_before_the_config_or_the_store(t *testing.T) {
	stub := &configStub{}
	h := newHarness(t, &fakeStore{}, nil, mcp.WithConfig(stub.load))

	result := h.cashFlow(t, map[string]any{"since": "last spring"})

	assert.True(t, result.IsError)
	assert.Empty(t, stub.commands)
	assert.Equal(t, `since "last spring" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`, textOf(t, result))
	assert.Zero(t, h.built)
	assert.Equal(t, cashFlowLogPrefix+windowRefusedLine+"\n", h.stderr.String())
}

func Test_cash_flow_refuses_a_future_since_without_until_before_the_config_or_the_store(t *testing.T) {
	stub := &configStub{}
	h := newHarness(t, &fakeStore{}, nil, mcp.WithConfig(stub.load))

	result := h.cashFlow(t, map[string]any{"since": "2099"})

	assert.True(t, result.IsError)
	assert.Empty(t, stub.commands)
	assert.Equal(t, "since 2099 is after today; pass until to include future-dated transactions", textOf(t, result))
	assert.Zero(t, h.built)
	assert.Equal(t, cashFlowLogPrefix+windowRefusedLine+"\n", h.stderr.String())
}

func Test_cash_flow_groups_by_year_when_the_call_asks_for_it(t *testing.T) {
	h := newHarness(t, &fakeStore{flow: store.CashFlow{}}, nil)

	doc := decodeDoc[document.CashFlow](t, h.cashFlow(t, map[string]any{"by": "year"}))

	assert.Equal(t, "year", doc.By)
}

func Test_cash_flow_refuses_arguments_the_schema_rejects_without_reading_the_config_or_the_store(t *testing.T) {
	cases := map[string]map[string]any{
		"a null since":          {"since": nil},
		"a lower-case currency": {"currency": "cad"},
		"an unknown property":   {"order": "desc"},
		"a week grouping":       {"by": "week"},
		"a weekday grouping":    {"by": "weekday"},
		"a spending grouping":   {"by": "category"},
		"a non-list accounts":   {"accounts": "Chequing"},
	}

	for name, arguments := range cases {
		t.Run(name, func(t *testing.T) {
			stub := &configStub{}
			h := newHarness(t, &fakeStore{}, nil, mcp.WithConfig(stub.load))

			result := h.cashFlow(t, arguments)

			assert.True(t, result.IsError)
			assert.Empty(t, stub.commands)
			assert.Zero(t, h.built)
			assert.Equal(t, cashFlowLogPrefix+argsRefusedLog+"\n", h.stderr.String())
		})
	}
}

func Test_cash_flow_puts_the_cap_line_after_every_other_warning(t *testing.T) {
	totals := []store.CashFlowTotal{{Currency: "CAD", Spent: 50100}}
	stub := &configStub{cfg: config.Config{Currency: money.CAD, WarningsAbsolute: []string{configUnknownKeyWarning}}}
	unconverted := store.Unconverted{Transactions: 1, FirstRate: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)}
	h := newHarness(t, &fakeStore{flow: store.CashFlow{Totals: totals, Unconverted: unconverted}}, nil, mcp.WithConfig(stub.load))

	doc := decodeDoc[document.CashFlow](t, h.cashFlow(t, map[string]any{"since": "1985-01", "until": "2026-09"}))

	require.Len(t, doc.Periods, 500)
	assert.Equal(t, "1985-01", doc.Periods[0].Period)
	assert.Equal(t, "2026-08", doc.Periods[499].Period)
	assert.Len(t, doc.Totals, 1)
	require.Len(t, doc.Warnings, 3)
	assert.Equal(t, configUnknownKeyWarning, doc.Warnings[0])
	assert.Equal(t, "1 transaction dated before 2026-03-01, the first exchange rate in the store, is listed in USD, not converted to CAD", doc.Warnings[1])
	assert.Equal(t, "cash_flow lists the first 500 periods of 501; totals count every period; pass a later since, or by year", doc.Warnings[2])
}
