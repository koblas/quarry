package mcp_test

import (
	"encoding/json"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/mcp"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	holdingsLogPrefix = "quarry: mcp: holdings: "
	holdingsConfigLog = "cannot read quarry's config file; run quarry holdings to see why"
	asOfRefusedLine   = "refused the call's as_of; details went to the client only"
)

// decodeHoldings is result's one text block decoded as the holdings document.
func decodeHoldings(t *testing.T, result *sdk.CallToolResult) document.Holdings {
	t.Helper()
	require.False(t, result.IsError, textOf(t, result))
	var doc document.Holdings
	require.NoError(t, json.Unmarshal([]byte(textOf(t, result)), &doc))
	return doc
}

// cadHoldings is n priced CAD holdings, each worth 1.00, security ids sec-000 upward.
func cadHoldings(n int) store.Holdings {
	rows := make([]store.Holding, n)
	for i := range rows {
		price, currency := int64(1_000_000), "CAD"
		rows[i] = store.Holding{
			AccountID: "acct-1", SecurityID: fmt.Sprintf("sec-%03d", i), Account: "Brokerage",
			Currency: &currency, Shares: 1_000_000, Price: &price,
			Value: big.NewInt(100), ValueCAD: big.NewInt(100),
		}
	}
	return store.Holdings{Holdings: rows}
}

// listed is how many holdings doc lists; a count keeps a failure from printing every row.
func listed(doc document.Holdings) int { return len(doc.Holdings) }

func Test_holdings_lists_the_first_500_and_totals_every_holding_with_a_cut_note(t *testing.T) {
	h := newHarness(t, &fakeStore{held: cadHoldings(501)}, nil)

	doc := decodeHoldings(t, h.holdings(t, map[string]any{"currency": "CAD"}))

	require.Equal(t, 500, listed(doc))
	assert.Equal(t, "sec-000", doc.Holdings[0].SecurityID)
	assert.Equal(t, "sec-499", doc.Holdings[499].SecurityID)
	assert.Equal(t, []document.HoldingsTotal{{Currency: "CAD", Value: "501.00"}}, doc.Totals)
	assert.Equal(t, []string{"holdings lists the first 500 holdings of 501; query the v_holdings table for the rest"}, doc.Warnings)
}

func Test_holdings_lists_exactly_500_with_no_cut_note(t *testing.T) {
	h := newHarness(t, &fakeStore{held: cadHoldings(500)}, nil)

	doc := decodeHoldings(t, h.holdings(t, map[string]any{"currency": "CAD"}))

	assert.Equal(t, 500, listed(doc))
	assert.Equal(t, []document.HoldingsTotal{{Currency: "CAD", Value: "500.00"}}, doc.Totals)
	assert.Empty(t, doc.Warnings)
}

func Test_holdings_puts_the_cut_note_after_the_config_warnings_and_the_holdings_warnings(t *testing.T) {
	held := cadHoldings(501)
	held.Holdings[0].Price, held.Holdings[0].Value, held.Holdings[0].ValueCAD = nil, nil, nil
	stub := &configStub{cfg: config.Config{WarningsAbsolute: []string{configUnknownKeyWarning}}}
	h := newHarness(t, &fakeStore{held: held}, nil, mcp.WithConfig(stub.load))

	doc := decodeHoldings(t, h.holdings(t, map[string]any{}))

	require.Len(t, doc.Warnings, 3)
	assert.Equal(t, configUnknownKeyWarning, doc.Warnings[0])
	assert.Contains(t, doc.Warnings[1], "has no price")
	assert.Equal(t, "holdings lists the first 500 holdings of 501; query the v_holdings table for the rest", doc.Warnings[2])
}

func Test_holdings_reads_today_once_at_the_start_of_every_call(t *testing.T) {
	clock := &steppingClock{times: []time.Time{
		time.Date(2026, time.September, 29, 23, 59, 0, 0, time.UTC),
		time.Date(2026, time.September, 30, 0, 1, 0, 0, time.UTC),
	}}
	h := newHarness(t, &fakeStore{}, nil, mcp.WithClock(clock.now))

	first := decodeHoldings(t, h.holdings(t, map[string]any{}))
	second := decodeHoldings(t, h.holdings(t, map[string]any{}))

	assert.Equal(t, "2026-09-29", first.AsOf)
	assert.Equal(t, "2026-09-30", second.AsOf)
	assert.Equal(t, 2, clock.reads)
}

func Test_holdings_asks_the_store_for_the_day_as_of_names_and_for_today_when_it_is_absent(t *testing.T) {
	clock := func() time.Time { return time.Date(2026, time.September, 29, 12, 0, 0, 0, time.UTC) }
	fake := &fakeStore{}
	h := newHarness(t, fake, nil, mcp.WithClock(clock))

	decodeHoldings(t, h.holdings(t, map[string]any{"as_of": "2026-03"}))
	decodeHoldings(t, h.holdings(t, map[string]any{}))

	require.Len(t, fake.heldAsked, 2)
	assert.Equal(t, time.Date(2026, time.March, 31, 0, 0, 0, 0, time.UTC), fake.heldAsked[0].AsOf)
	assert.Equal(t, time.Date(2026, time.September, 29, 0, 0, 0, 0, time.UTC), fake.heldAsked[1].AsOf)
}

func Test_holdings_with_absent_null_or_empty_arguments_values_today(t *testing.T) {
	for name, args := range map[string]any{
		"omitted": nil,
		"null":    json.RawMessage("null"),
		"empty":   map[string]any{},
	} {
		t.Run(name, func(t *testing.T) {
			clock := func() time.Time { return time.Date(2026, time.September, 29, 12, 0, 0, 0, time.UTC) }
			h := newHarness(t, &fakeStore{}, nil, mcp.WithClock(clock))

			doc := decodeHoldings(t, h.holdings(t, args))

			assert.Equal(t, "2026-09-29", doc.AsOf)
		})
	}
}

func Test_holdings_refuses_an_as_of_it_cannot_use_with_the_class_line_before_the_config_or_the_store(t *testing.T) {
	cases := []struct {
		name string
		asOf string
		want string
	}{
		{name: "not a date", asOf: "2024-13", want: `as_of "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`},
		{name: "empty", asOf: "", want: `as_of "" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`},
		{name: "after today", asOf: "2099", want: "as_of 2099 is after today; holdings are valued up to today only, so pass an earlier as_of"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stub := &configStub{}
			h := newHarness(t, &fakeStore{}, nil, mcp.WithConfig(stub.load))

			result := h.holdings(t, map[string]any{"as_of": c.asOf})

			assert.True(t, result.IsError)
			assert.Equal(t, c.want, textOf(t, result))
			assert.Empty(t, stub.commands)
			assert.Zero(t, h.built)
			assert.Equal(t, holdingsLogPrefix+asOfRefusedLine+"\n", h.stderr.String())
		})
	}
}

func Test_holdings_refuses_an_unreadable_config_before_building_the_report(t *testing.T) {
	stub := &configStub{err: errBadConfig}
	h := newHarness(t, &fakeStore{}, errFactoryBroke, mcp.WithConfig(stub.load))

	result := h.holdings(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, errBadConfig.Error(), textOf(t, result))
	assert.Zero(t, h.built)
	assert.Equal(t, holdingsLogPrefix+holdingsConfigLog+"\n", h.stderr.String())
}

func Test_holdings_does_not_read_the_config_when_the_call_names_a_currency(t *testing.T) {
	stub := &configStub{err: errBadConfig}
	h := newHarness(t, &fakeStore{}, nil, mcp.WithConfig(stub.load))

	doc := decodeHoldings(t, h.holdings(t, map[string]any{"currency": "USD"}))

	assert.Equal(t, "USD", doc.Currency)
	assert.Empty(t, stub.commands)
}

func Test_holdings_reads_the_config_as_the_mcp_command_when_the_call_names_no_currency(t *testing.T) {
	stub := &configStub{cfg: config.Config{WarningsAbsolute: []string{configUnknownKeyWarning}}}
	h := newHarness(t, &fakeStore{}, nil, mcp.WithConfig(stub.load))

	doc := decodeHoldings(t, h.holdings(t, map[string]any{}))

	assert.Equal(t, []string{"mcp"}, stub.commands)
	require.NotEmpty(t, doc.Warnings)
	assert.Equal(t, configUnknownKeyWarning, doc.Warnings[0])
}

func Test_holdings_answers_a_report_factory_failure_with_the_generic_log_line(t *testing.T) {
	h := newHarness(t, &fakeStore{}, errFactoryBroke)

	result := h.holdings(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, errFactoryBroke.Error(), textOf(t, result))
	assert.Equal(t, holdingsLogPrefix+failedLogLine+"\n", h.stderr.String())
}

func Test_holdings_sends_a_store_refusal_verbatim_to_the_client_and_stderr(t *testing.T) {
	h := newHarness(t, &fakeStore{err: &store.OpenError{Fault: store.OpenFaultMissing, Path: testStorePath}}, nil)

	result := h.holdings(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, missingStoreLine, textOf(t, result))
	assert.Equal(t, holdingsLogPrefix+missingStoreLine+"\n", h.stderr.String())
}

func Test_holdings_answers_a_plain_store_fault_with_the_generic_log_line(t *testing.T) {
	h := newHarness(t, &fakeStore{err: errDiskOnFire}, nil)

	result := h.holdings(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, errDiskOnFire.Error(), textOf(t, result))
	assert.Equal(t, holdingsLogPrefix+failedLogLine+"\n", h.stderr.String())
}

func Test_holdings_refuses_arguments_the_schema_rejects_without_reading_the_config_or_the_store(t *testing.T) {
	cases := map[string]map[string]any{
		"a null as_of":          {"as_of": nil},
		"a lower-case currency": {"currency": "cad"},
		"an unknown property":   {"since": "2026"},
		"a non-list accounts":   {"accounts": "Brokerage"},
	}

	for name, arguments := range cases {
		t.Run(name, func(t *testing.T) {
			stub := &configStub{}
			h := newHarness(t, &fakeStore{}, nil, mcp.WithConfig(stub.load))

			result := h.holdings(t, arguments)

			assert.True(t, result.IsError)
			assert.Empty(t, stub.commands)
			assert.Zero(t, h.built)
			assert.Equal(t, holdingsLogPrefix+argsRefusedLog+"\n", h.stderr.String())
		})
	}
}
