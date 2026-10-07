package mcp_test

import (
	"testing"

	"github.com/koblas/quarry/internal/mcp"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_a_successful_call_writes_nothing_to_stderr(t *testing.T) {
	h := newHarness(t, &fakeStore{result: rowsOf(1)}, nil)

	result := h.query(t, map[string]any{"sql": "SELECT 1"})

	require.False(t, result.IsError, textOf(t, result))
	assert.Empty(t, h.stderr.String())
}

func Test_a_result_is_the_document_as_compact_text_and_as_structured_content(t *testing.T) {
	h := newHarness(t, &fakeStore{result: rowsOf(1)}, nil)

	result := h.query(t, map[string]any{"sql": "SELECT 1"})

	const want = `{"columns":[{"name":"n","type":"BIGINT"}],"rows":[[0]],"row_count":1,"limit":500,"truncated":false,"warnings":[]}`
	assert.Equal(t, want, textOf(t, result)) //nolint:testifylint // the key order is the contract and JSONEq ignores it
	assert.JSONEq(t, want, jsonOf(t, result.StructuredContent))
}

func Test_a_refused_call_is_one_text_block_and_one_stderr_line(t *testing.T) {
	h := newHarness(t, &fakeStore{}, errFactoryBroke)

	result := h.query(t, map[string]any{"sql": "SELECT 1"})

	assert.True(t, result.IsError)
	assert.Equal(t, "the report factory broke", textOf(t, result))
	assert.Nil(t, result.StructuredContent)
	assert.Equal(t, logPrefixQuery+failedLogLine+"\n", h.stderr.String())
}

func Test_a_call_the_schema_refuses_logs_only_that_its_arguments_were_refused(t *testing.T) {
	cases := []struct {
		name      string
		arguments map[string]any
	}{
		{name: "sql missing", arguments: map[string]any{"limit": 5}},
		{name: "sql of the wrong type", arguments: map[string]any{"sql": 42}},
		{name: "limit out of range, sql in the call", arguments: map[string]any{"sql": "SELECT 'Chequing' AS payee", "limit": 0}},
		{name: "unknown property, sql in the call", arguments: map[string]any{"sql": "SELECT 'Chequing' AS payee", "extra": "Chequing"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, &fakeStore{}, nil)

			result := h.query(t, c.arguments)

			assert.True(t, result.IsError)
			assert.Equal(t, logPrefixQuery+argsRefusedLog+"\n", h.stderr.String())
			assert.Empty(t, h.store.asked)
		})
	}
}

func Test_a_tool_answers_a_report_factory_failure_with_the_generic_log_line(t *testing.T) {
	cases := []struct{ tool, logPrefix string }{
		{"anomalies", anomaliesLogPrefix},
		{"cash_flow", cashFlowLogPrefix},
		{"data_quality", dqLogPrefix},
		{"holdings", holdingsLogPrefix},
		{"monthly_summary", summaryLogPrefix},
		{"net_worth", netWorthLogPrefix},
		{"recurring_charges", recurringChargesLogPrefix},
		{"search_transactions", searchLogPrefix},
		{"spending", spendingLogPrefix},
	}

	for _, c := range cases {
		t.Run(c.tool, func(t *testing.T) {
			h := newHarness(t, &fakeStore{}, errFactoryBroke, withDefaultConfig(), atSeptember29())

			result := callTool(t, h.session, c.tool, map[string]any{})

			assert.True(t, result.IsError)
			assert.Equal(t, errFactoryBroke.Error(), textOf(t, result))
			assert.Equal(t, c.logPrefix+failedLogLine+"\n", h.stderr.String())
		})
	}
}

func Test_a_tool_sends_a_store_refusal_verbatim_to_the_client_and_stderr(t *testing.T) {
	cases := []struct{ tool, logPrefix string }{
		{"anomalies", anomaliesLogPrefix},
		{"cash_flow", cashFlowLogPrefix},
		{"holdings", holdingsLogPrefix},
		{"monthly_summary", summaryLogPrefix},
		{"net_worth", netWorthLogPrefix},
		{"recurring_charges", recurringChargesLogPrefix},
		{"spending", spendingLogPrefix},
	}

	for _, c := range cases {
		t.Run(c.tool, func(t *testing.T) {
			h := newHarness(t, &fakeStore{err: &store.OpenError{Fault: store.OpenFaultMissing, Path: testStorePath}}, nil, withDefaultConfig(), atSeptember29())

			result := callTool(t, h.session, c.tool, map[string]any{})

			assert.True(t, result.IsError)
			assert.Equal(t, missingStoreLine, textOf(t, result))
			assert.Equal(t, c.logPrefix+missingStoreLine+"\n", h.stderr.String())
		})
	}
}

func Test_a_tool_answers_a_plain_store_fault_with_the_generic_log_line(t *testing.T) {
	cases := []struct{ tool, logPrefix string }{
		{"anomalies", anomaliesLogPrefix},
		{"cash_flow", cashFlowLogPrefix},
		{"holdings", holdingsLogPrefix},
		{"monthly_summary", summaryLogPrefix},
		{"net_worth", netWorthLogPrefix},
		{"recurring_charges", recurringChargesLogPrefix},
		{"spending", spendingLogPrefix},
	}

	for _, c := range cases {
		t.Run(c.tool, func(t *testing.T) {
			h := newHarness(t, &fakeStore{err: errDiskOnFire}, nil, withDefaultConfig(), atSeptember29())

			result := callTool(t, h.session, c.tool, map[string]any{})

			assert.True(t, result.IsError)
			assert.Equal(t, errDiskOnFire.Error(), textOf(t, result))
			assert.Equal(t, c.logPrefix+failedLogLine+"\n", h.stderr.String())
		})
	}
}

func Test_a_tool_refuses_an_unreadable_config_before_building_the_report(t *testing.T) {
	cases := []struct{ tool, logPrefix, configLog string }{
		{"acb", acbLogPrefix, acbConfigLog},
		{"anomalies", anomaliesLogPrefix, anomaliesConfigLog},
		{"cash_flow", cashFlowLogPrefix, cashFlowConfigLog},
		{"holdings", holdingsLogPrefix, holdingsConfigLog},
		{"monthly_summary", summaryLogPrefix, summaryConfigLog},
		{"net_worth", netWorthLogPrefix, netWorthConfigLog},
		{"recurring_charges", recurringChargesLogPrefix, recurringChargesConfigLog},
		{"spending", spendingLogPrefix, spendingConfigLog},
	}

	for _, c := range cases {
		t.Run(c.tool, func(t *testing.T) {
			stub := &configStub{err: errBadConfig}
			h := newHarness(t, &fakeStore{}, errFactoryBroke, mcp.WithConfig(stub.load), atSeptember29())

			result := callTool(t, h.session, c.tool, map[string]any{})

			assert.True(t, result.IsError)
			assert.Equal(t, errBadConfig.Error(), textOf(t, result))
			assert.Zero(t, h.built)
			assert.Equal(t, c.logPrefix+c.configLog+"\n", h.stderr.String())
		})
	}
}

func Test_a_tool_does_not_read_the_config_when_the_call_names_a_currency(t *testing.T) {
	type currencyDoc struct {
		Currency string `json:"currency"`
	}
	tools := []string{"anomalies", "cash_flow", "holdings", "net_worth", "recurring_charges", "spending"}

	for _, tool := range tools {
		t.Run(tool, func(t *testing.T) {
			stub := &configStub{err: errBadConfig}
			h := newHarness(t, &fakeStore{}, nil, mcp.WithConfig(stub.load), atSeptember29())

			doc := decodeDoc[currencyDoc](t, callTool(t, h.session, tool, map[string]any{"currency": "USD"}))

			assert.Equal(t, "USD", doc.Currency)
			assert.Empty(t, stub.commands)
		})
	}
}

func Test_a_tool_reads_the_config_as_the_mcp_command_when_the_call_names_no_currency(t *testing.T) {
	tools := []string{"anomalies", "cash_flow", "recurring_charges", "spending"}

	for _, tool := range tools {
		t.Run(tool, func(t *testing.T) {
			stub := &configStub{}
			h := newHarness(t, &fakeStore{}, nil, mcp.WithConfig(stub.load), atSeptember29())

			require.False(t, callTool(t, h.session, tool, map[string]any{}).IsError)

			assert.Equal(t, []string{"mcp"}, stub.commands)
		})
	}
}
