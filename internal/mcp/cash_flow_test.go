package mcp_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/mcp"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

const (
	cashFlowLogPrefix = "quarry: mcp: cash_flow: "
	cashFlowConfigLog = "cannot read quarry's config file; run quarry cashflow to see why"
)

func Test_cash_flow_reads_today_once_at_the_start_of_every_call(t *testing.T) {
	clock := &steppingClock{times: []time.Time{
		time.Date(2026, time.September, 29, 23, 59, 0, 0, time.UTC),
		time.Date(2026, time.September, 30, 0, 1, 0, 0, time.UTC),
	}}
	h := newHarness(t, &fakeStore{}, nil, mcp.WithClock(clock.now))

	first := decodeCashFlow(t, h.cashFlow(t, map[string]any{}))
	second := decodeCashFlow(t, h.cashFlow(t, map[string]any{}))

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

func Test_cash_flow_refuses_an_unreadable_config_before_building_the_report(t *testing.T) {
	stub := &configStub{err: errBadConfig}
	h := newHarness(t, &fakeStore{}, errFactoryBroke, mcp.WithConfig(stub.load))

	result := h.cashFlow(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, errBadConfig.Error(), textOf(t, result))
	assert.Zero(t, h.built)
	assert.Equal(t, cashFlowLogPrefix+cashFlowConfigLog+"\n", h.stderr.String())
}

func Test_cash_flow_does_not_read_the_config_when_the_call_names_a_currency(t *testing.T) {
	stub := &configStub{err: errBadConfig}
	h := newHarness(t, &fakeStore{}, nil, mcp.WithConfig(stub.load))

	doc := decodeCashFlow(t, h.cashFlow(t, map[string]any{"currency": "USD"}))

	assert.Equal(t, "USD", doc.Currency)
	assert.Empty(t, stub.commands)
}

func Test_cash_flow_reads_the_config_as_the_mcp_command_when_the_call_names_no_currency(t *testing.T) {
	stub := &configStub{}
	h := newHarness(t, &fakeStore{}, nil, mcp.WithConfig(stub.load))

	decodeCashFlow(t, h.cashFlow(t, map[string]any{}))

	assert.Equal(t, []string{"mcp"}, stub.commands)
}

func Test_cash_flow_groups_by_year_when_the_call_asks_for_it(t *testing.T) {
	h := newHarness(t, &fakeStore{flow: store.CashFlow{}}, nil)

	doc := decodeCashFlow(t, h.cashFlow(t, map[string]any{"by": "year"}))

	assert.Equal(t, "year", doc.By)
}

func Test_cash_flow_answers_a_report_factory_failure_with_the_generic_log_line(t *testing.T) {
	h := newHarness(t, &fakeStore{}, errFactoryBroke)

	result := h.cashFlow(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, errFactoryBroke.Error(), textOf(t, result))
	assert.Equal(t, cashFlowLogPrefix+failedLogLine+"\n", h.stderr.String())
}

func Test_cash_flow_sends_a_store_refusal_verbatim_to_the_client_and_stderr(t *testing.T) {
	h := newHarness(t, &fakeStore{err: &store.OpenError{Fault: store.OpenFaultMissing, Path: testStorePath}}, nil)

	result := h.cashFlow(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, missingStoreLine, textOf(t, result))
	assert.Equal(t, cashFlowLogPrefix+missingStoreLine+"\n", h.stderr.String())
}

func Test_cash_flow_answers_a_plain_store_fault_with_the_generic_log_line(t *testing.T) {
	h := newHarness(t, &fakeStore{err: errDiskOnFire}, nil)

	result := h.cashFlow(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, errDiskOnFire.Error(), textOf(t, result))
	assert.Equal(t, cashFlowLogPrefix+failedLogLine+"\n", h.stderr.String())
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
