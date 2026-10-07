package mcp_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/mcp"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	anomaliesLogPrefix   = "quarry: mcp: anomalies: "
	anomaliesConfigLog   = "cannot read quarry's config file; run quarry anomalies to see why"
	anomaliesFutureSince = "since 2099 is after today; anomalies lists charges up to today only, so pass an earlier since"
)

func Test_anomalies_reads_today_once_at_the_start_of_every_call(t *testing.T) {
	fake := &fakeStore{}
	clock := newMidnightClock()
	h := newHarness(t, fake, nil, mcp.WithClock(clock.now))

	first := decodeDoc[document.Anomalies](t, h.anomalies(t, map[string]any{}))
	second := decodeDoc[document.Anomalies](t, h.anomalies(t, map[string]any{}))

	assert.Equal(t, "2026-09-29", first.Until)
	assert.Equal(t, "2026-09-30", second.Until)
	assert.Equal(t, 2, clock.reads)
	assert.Equal(t, []time.Time{
		time.Date(2026, time.September, 29, 0, 0, 0, 0, time.UTC),
		time.Date(2026, time.September, 30, 0, 0, 0, 0, time.UTC),
	}, fake.through)
}

func Test_anomalies_does_not_refuse_a_future_until_and_still_reads_charges_through_today(t *testing.T) {
	fake := &fakeStore{}
	h := newHarness(t, fake, nil, atInstant(windowToday))

	doc := decodeDoc[document.Anomalies](t, h.anomalies(t, map[string]any{"until": "2099"}))

	assert.Equal(t, "2099-12-31", doc.Until)
	assert.Equal(t, []time.Time{time.Date(2026, time.September, 30, 0, 0, 0, 0, time.UTC)}, fake.through)
}

func Test_anomalies_refuses_a_future_since_in_its_own_words_before_the_config_or_the_store(t *testing.T) {
	stub := &configStub{}
	h := newHarness(t, &fakeStore{}, nil, mcp.WithConfig(stub.load))

	result := h.anomalies(t, map[string]any{"since": "2099"})

	assert.True(t, result.IsError)
	assert.Equal(t, anomaliesFutureSince, textOf(t, result))
	assert.Empty(t, stub.commands)
	assert.Zero(t, h.built)
	assert.Equal(t, anomaliesLogPrefix+windowRefusedLine+"\n", h.stderr.String())
}

func Test_anomalies_refuses_a_window_it_cannot_read_with_the_class_line_before_the_config_or_the_store(t *testing.T) {
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
			h := newHarness(t, &fakeStore{}, nil, mcp.WithConfig(stub.load), atInstant(windowToday))

			result := h.anomalies(t, c.arguments)

			assert.True(t, result.IsError)
			assert.Equal(t, c.want, textOf(t, result))
			assert.Empty(t, stub.commands)
			assert.Zero(t, h.built)
			assert.Equal(t, anomaliesLogPrefix+windowRefusedLine+"\n", h.stderr.String())
		})
	}
}

func Test_anomalies_refuses_an_unreadable_config_before_building_the_report(t *testing.T) {
	stub := &configStub{err: errBadConfig}
	h := newHarness(t, &fakeStore{}, errFactoryBroke, mcp.WithConfig(stub.load))

	result := h.anomalies(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, errBadConfig.Error(), textOf(t, result))
	assert.Zero(t, h.built)
	assert.Equal(t, anomaliesLogPrefix+anomaliesConfigLog+"\n", h.stderr.String())
}

func Test_anomalies_does_not_read_the_config_when_the_call_names_a_currency(t *testing.T) {
	stub := &configStub{err: errBadConfig}
	h := newHarness(t, &fakeStore{}, nil, mcp.WithConfig(stub.load))

	doc := decodeDoc[document.Anomalies](t, h.anomalies(t, map[string]any{"currency": "USD"}))

	assert.Equal(t, "USD", doc.Currency)
	assert.Empty(t, stub.commands)
}

func Test_anomalies_reads_the_config_as_the_mcp_command_when_the_call_names_no_currency(t *testing.T) {
	stub := &configStub{}
	h := newHarness(t, &fakeStore{}, nil, mcp.WithConfig(stub.load))

	decodeDoc[document.Anomalies](t, h.anomalies(t, map[string]any{}))

	assert.Equal(t, []string{"mcp"}, stub.commands)
}

func Test_anomalies_answers_a_report_factory_failure_with_the_generic_log_line(t *testing.T) {
	h := newHarness(t, &fakeStore{}, errFactoryBroke)

	result := h.anomalies(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, errFactoryBroke.Error(), textOf(t, result))
	assert.Equal(t, anomaliesLogPrefix+failedLogLine+"\n", h.stderr.String())
}

func Test_anomalies_sends_a_store_refusal_verbatim_to_the_client_and_stderr(t *testing.T) {
	h := newHarness(t, &fakeStore{err: &store.OpenError{Fault: store.OpenFaultMissing, Path: testStorePath}}, nil)

	result := h.anomalies(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, missingStoreLine, textOf(t, result))
	assert.Equal(t, anomaliesLogPrefix+missingStoreLine+"\n", h.stderr.String())
}

func Test_anomalies_answers_a_plain_store_fault_with_the_generic_log_line(t *testing.T) {
	h := newHarness(t, &fakeStore{err: errDiskOnFire}, nil)

	result := h.anomalies(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, errDiskOnFire.Error(), textOf(t, result))
	assert.Equal(t, anomaliesLogPrefix+failedLogLine+"\n", h.stderr.String())
}

func Test_anomalies_refuses_arguments_the_schema_rejects_without_reading_the_config_or_the_store(t *testing.T) {
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

			result := h.anomalies(t, arguments)

			assert.True(t, result.IsError)
			assert.Empty(t, stub.commands)
			assert.Zero(t, h.built)
			assert.Equal(t, anomaliesLogPrefix+argsRefusedLog+"\n", h.stderr.String())
		})
	}
}

func Test_anomalies_cuts_charges_to_the_cap_and_ends_the_warnings_with_the_line(t *testing.T) {
	const payees = 501
	stub := &configStub{cfg: config.Config{Currency: money.CAD, WarningsAbsolute: []string{configUnknownKeyWarning}}}
	charges := unusualCharges(payees)
	charges.FirstRate = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	inUSD(charges.Rows, payeeNamed(payees-1))
	h := newHarness(t, &fakeStore{charges: charges}, nil, mcp.WithConfig(stub.load), atInstant(windowToday))

	doc := decodeDoc[document.Anomalies](t, h.anomalies(t, map[string]any{}))

	assert.Len(t, doc.Anomalies, 500)
	assert.Equal(t, payees*(report.AnomalyPayeeMinHistory+1), doc.Checked)
	assert.Equal(t, payees*report.AnomalyPayeeMinHistory, doc.NotJudged)
	require.Len(t, doc.Warnings, 3)
	assert.Equal(t, configUnknownKeyWarning, doc.Warnings[0])
	assert.Equal(t, "1 charge dated before 2026-03-01, the first exchange rate in the store, is listed in USD, not converted to CAD", doc.Warnings[1])
	assert.Equal(t, "anomalies lists the first 500 charges of 501; pass a shorter period or fewer accounts", doc.Warnings[2])
}

// unusualCharges is n payees, each with report.AnomalyPayeeMinHistory usual charges and then one the payee's
// baseline makes unusual, all in June 2026.
func unusualCharges(n int) store.Charges { return unusualChargesIn(time.June, 0, n) }

// unusualChargesIn is unusualCharges in month of 2026, for the payees named payeeNamed(first) on.
func unusualChargesIn(month time.Month, first, n int) store.Charges {
	var rows []store.Charge
	add := func(i int, day int, amount int64) {
		name := payeeNamed(first + i)
		id := "payee-" + name
		rows = append(rows, store.Charge{
			TransactionID: "tx-" + name + strconv.Itoa(day),
			SourceID:      int64(len(rows) + 1),
			Date:          time.Date(2026, month, day, 0, 0, 0, 0, time.UTC),
			Account:       store.Account{ID: "acct", Name: "Chequing", Currency: "CAD"},
			PayeeID:       &id,
			Payee:         &name,
			Currency:      "CAD",
			Amount:        amount,
			ExpenseSplits: 1,
		})
	}
	for day := 1; day <= report.AnomalyPayeeMinHistory; day++ {
		for i := range n {
			add(i, day, report.AnomalyMinAmount)
		}
	}
	for i := range n {
		add(i, report.AnomalyPayeeMinHistory+1, report.AnomalyPayeeMultiplier*report.AnomalyMinAmount+1)
	}
	return store.Charges{Rows: rows}
}
