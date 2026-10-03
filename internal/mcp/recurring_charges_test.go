package mcp_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/mcp"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	recurringChargesLogPrefix   = "quarry: mcp: recurring_charges: "
	recurringChargesConfigLog   = "cannot read quarry's config file; run quarry recurring to see why"
	recurringChargesFutureSince = "since 2099 is after today; recurring_charges lists charges up to today only, so pass an earlier since"
)

// recurringChargesToday is the instant the window tests fix, so the default since is 2026-01-01.
var recurringChargesToday = time.Date(2026, time.September, 30, 12, 0, 0, 0, time.UTC)

// decodeRecurring is result's one text block decoded as the recurring_charges document.
func decodeRecurring(t *testing.T, result *sdk.CallToolResult) document.Recurring {
	t.Helper()
	require.False(t, result.IsError, textOf(t, result))
	var doc document.Recurring
	require.NoError(t, json.Unmarshal([]byte(textOf(t, result)), &doc))
	return doc
}

func Test_recurring_charges_reads_today_once_at_the_start_of_every_call(t *testing.T) {
	fake := &fakeStore{}
	clock := &steppingClock{times: []time.Time{
		time.Date(2026, time.September, 29, 23, 59, 0, 0, time.UTC),
		time.Date(2026, time.September, 30, 0, 1, 0, 0, time.UTC),
	}}
	h := newHarness(t, fake, nil, mcp.WithClock(clock.now))

	first := decodeRecurring(t, h.recurringCharges(t, map[string]any{}))
	second := decodeRecurring(t, h.recurringCharges(t, map[string]any{}))

	assert.Equal(t, "2026-09-29", first.Until)
	assert.Equal(t, "2026-09-30", second.Until)
	assert.Equal(t, 2, clock.reads)
	assert.Equal(t, []time.Time{
		time.Date(2026, time.September, 29, 0, 0, 0, 0, time.UTC),
		time.Date(2026, time.September, 30, 0, 0, 0, 0, time.UTC),
	}, fake.through)
}

func Test_recurring_charges_does_not_refuse_a_future_until_and_still_reads_charges_through_today(t *testing.T) {
	fake := &fakeStore{}
	h := newHarness(t, fake, nil, mcp.WithClock(func() time.Time { return recurringChargesToday }))

	doc := decodeRecurring(t, h.recurringCharges(t, map[string]any{"until": "2099"}))

	assert.Equal(t, "2099-12-31", doc.Until)
	assert.Equal(t, []time.Time{time.Date(2026, time.September, 30, 0, 0, 0, 0, time.UTC)}, fake.through)
}

func Test_recurring_charges_refuses_a_future_since_in_its_own_words_before_the_config_or_the_store(t *testing.T) {
	stub := &configStub{}
	h := newHarness(t, &fakeStore{}, nil, mcp.WithConfig(stub.load))

	result := h.recurringCharges(t, map[string]any{"since": "2099"})

	assert.True(t, result.IsError)
	assert.Equal(t, recurringChargesFutureSince, textOf(t, result))
	assert.Empty(t, stub.commands)
	assert.Zero(t, h.built)
	assert.Equal(t, recurringChargesLogPrefix+windowRefusedLine+"\n", h.stderr.String())
}

func Test_recurring_charges_refuses_a_window_it_cannot_read_with_the_class_line_before_the_config_or_the_store(t *testing.T) {
	cases := []struct {
		name      string
		arguments map[string]any
		want      string
	}{
		{"a since that is not a date", map[string]any{"since": "last spring"}, `since "last spring" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`},
		{"an empty since", map[string]any{"since": ""}, `since "" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`},
		{"a since after until", map[string]any{"since": "2025", "until": "2024"}, "since 2025 is after until 2024"},
		{"an until before the default since", map[string]any{"until": "2025-03"}, "until 2025-03 is before the default since 2026-01-01; pass since too"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stub := &configStub{}
			h := newHarness(t, &fakeStore{}, nil, mcp.WithConfig(stub.load), mcp.WithClock(func() time.Time { return recurringChargesToday }))

			result := h.recurringCharges(t, c.arguments)

			assert.True(t, result.IsError)
			assert.Equal(t, c.want, textOf(t, result))
			assert.Empty(t, stub.commands)
			assert.Zero(t, h.built)
			assert.Equal(t, recurringChargesLogPrefix+windowRefusedLine+"\n", h.stderr.String())
		})
	}
}

func Test_recurring_charges_refuses_an_unreadable_config_before_building_the_report(t *testing.T) {
	stub := &configStub{err: errBadConfig}
	h := newHarness(t, &fakeStore{}, errFactoryBroke, mcp.WithConfig(stub.load))

	result := h.recurringCharges(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, errBadConfig.Error(), textOf(t, result))
	assert.Zero(t, h.built)
	assert.Equal(t, recurringChargesLogPrefix+recurringChargesConfigLog+"\n", h.stderr.String())
}

func Test_recurring_charges_does_not_read_the_config_when_the_call_names_a_currency(t *testing.T) {
	stub := &configStub{err: errBadConfig}
	h := newHarness(t, &fakeStore{}, nil, mcp.WithConfig(stub.load))

	doc := decodeRecurring(t, h.recurringCharges(t, map[string]any{"currency": "USD"}))

	assert.Equal(t, "USD", doc.Currency)
	assert.Empty(t, stub.commands)
}

func Test_recurring_charges_reads_the_config_as_the_mcp_command_when_the_call_names_no_currency(t *testing.T) {
	stub := &configStub{}
	h := newHarness(t, &fakeStore{}, nil, mcp.WithConfig(stub.load))

	decodeRecurring(t, h.recurringCharges(t, map[string]any{}))

	assert.Equal(t, []string{"mcp"}, stub.commands)
}

func Test_recurring_charges_answers_a_report_factory_failure_with_the_generic_log_line(t *testing.T) {
	h := newHarness(t, &fakeStore{}, errFactoryBroke)

	result := h.recurringCharges(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, errFactoryBroke.Error(), textOf(t, result))
	assert.Equal(t, recurringChargesLogPrefix+failedLogLine+"\n", h.stderr.String())
}

func Test_recurring_charges_sends_a_store_refusal_verbatim_to_the_client_and_stderr(t *testing.T) {
	h := newHarness(t, &fakeStore{err: &store.OpenError{Fault: store.OpenFaultMissing, Path: testStorePath}}, nil)

	result := h.recurringCharges(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, missingStoreLine, textOf(t, result))
	assert.Equal(t, recurringChargesLogPrefix+missingStoreLine+"\n", h.stderr.String())
}

func Test_recurring_charges_answers_a_plain_store_fault_with_the_generic_log_line(t *testing.T) {
	h := newHarness(t, &fakeStore{err: errDiskOnFire}, nil)

	result := h.recurringCharges(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, errDiskOnFire.Error(), textOf(t, result))
	assert.Equal(t, recurringChargesLogPrefix+failedLogLine+"\n", h.stderr.String())
}

func Test_recurring_charges_refuses_arguments_the_schema_rejects_without_reading_the_config_or_the_store(t *testing.T) {
	cases := map[string]map[string]any{
		"a null since":          {"since": nil},
		"a lower-case currency": {"currency": "cad"},
		"an unknown property":   {"order": "desc"},
		"a grouping":            {"by": "month"},
		"a non-list accounts":   {"accounts": "Chequing"},
	}

	for name, arguments := range cases {
		t.Run(name, func(t *testing.T) {
			stub := &configStub{}
			h := newHarness(t, &fakeStore{}, nil, mcp.WithConfig(stub.load))

			result := h.recurringCharges(t, arguments)

			assert.True(t, result.IsError)
			assert.Empty(t, stub.commands)
			assert.Zero(t, h.built)
			assert.Equal(t, recurringChargesLogPrefix+argsRefusedLog+"\n", h.stderr.String())
		})
	}
}

func Test_recurring_charges_cuts_series_to_the_cap_and_ends_the_warnings_with_the_line(t *testing.T) {
	const (
		seriesCount = 501
		amount      = 1000
	)
	stub := &configStub{cfg: config.Config{Currency: money.CAD, WarningsAbsolute: []string{configUnknownKeyWarning}}}
	charges := monthlyCharges(seriesCount, amount)
	charges.FirstRate = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	inUSD(charges.Rows, payeeNamed(seriesCount-1))
	h := newHarness(t, &fakeStore{charges: charges}, nil, mcp.WithConfig(stub.load), mcp.WithClock(func() time.Time { return recurringChargesToday }))

	doc := decodeRecurring(t, h.recurringCharges(t, map[string]any{}))

	require.Len(t, doc.Series, 500)
	require.Len(t, doc.Totals, 2)
	require.Len(t, doc.Warnings, 3)
	assert.Equal(t, configUnknownKeyWarning, doc.Warnings[0])
	assert.Equal(t, "1 series with a charge dated before 2026-03-01, the first exchange rate in the store, is listed in USD, not converted to CAD", doc.Warnings[1])
	assert.Equal(t, "recurring_charges lists the first 500 series of 501; totals count every series; pass a shorter period or fewer accounts", doc.Warnings[2])
}

// monthlyCharges is n payees, each charged amount on the 15th of July, August and September 2026.
func monthlyCharges(n int, amount int64) store.Charges {
	rows := make([]store.Charge, 0, n*3)
	for _, month := range []time.Month{time.July, time.August, time.September} {
		for i := range n {
			name, id := payeeNamed(i), "payee-"+payeeNamed(i)
			rows = append(rows, store.Charge{
				TransactionID: "tx-" + name + month.String(),
				SourceID:      int64(len(rows) + 1),
				Date:          time.Date(2026, month, 15, 0, 0, 0, 0, time.UTC),
				Account:       store.Account{ID: "acct", Name: "Chequing", Currency: "CAD"},
				PayeeID:       &id,
				Payee:         &name,
				Currency:      "CAD",
				Amount:        amount,
				ExpenseSplits: 1,
			})
		}
	}
	return store.Charges{Rows: rows}
}
