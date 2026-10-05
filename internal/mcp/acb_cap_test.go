package mcp_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/mcp"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// acbBuys is count one-share buys of security in acct-n, one a day from the first of 2025, ids numbered from 1.
func acbBuys(security string, count int) []store.InvestmentTransaction {
	buys := make([]store.InvestmentTransaction, count)
	for i := range buys {
		day := time.Date(2025, time.January, 1+i, 0, 0, 0, 0, time.UTC)
		buys[i] = acbBuy(fmt.Sprintf("inv-%s-%d", security, i+1), int64(i+1), "acct-n", security, day, 1_000_000, 100)
	}
	return buys
}

// acbManyEvents is acbHistory's accounts with Acme and Beta, bought acme and beta times.
func acbManyEvents(acme, beta int) store.InvestmentHistory {
	history := acbHistory()
	history.Transactions = append(acbBuys("sec-acme", acme), acbBuys("sec-beta", beta)...)
	return history
}

func Test_acb_caps_events_at_500_across_securities(t *testing.T) {
	const capNote = "acb lists the first 500 events of %s; pass security to narrow"
	cases := []struct {
		name         string
		acme, beta   int
		wantCounts   []int
		wantWarnings []string
	}{
		{name: "500 events are listed whole", acme: 300, beta: 200, wantCounts: []int{300, 200}, wantWarnings: []string{}},
		{
			name: "501 events lose the last one", acme: 300, beta: 201, wantCounts: []int{300, 200},
			wantWarnings: []string{fmt.Sprintf(capNote, "501")},
		},
		{
			name: "a security past the cap leaves the next with its header and no events", acme: 600, beta: 634, wantCounts: []int{500, 0},
			wantWarnings: []string{fmt.Sprintf(capNote, "1,234")},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, &fakeStore{history: acbManyEvents(c.acme, c.beta)}, nil, mcp.WithConfig((&configStub{cfg: acbConfig()}).load), atACBToday())

			doc := decodeACB(t, h.acb(t, map[string]any{}))

			require.Len(t, doc.Securities, 2)
			assert.Equal(t, c.wantCounts, []int{len(doc.Securities[0].Events), len(doc.Securities[1].Events)})
			assert.NotNil(t, doc.Securities[0].Events)
			assert.NotNil(t, doc.Securities[1].Events)
			assert.Equal(t, c.wantWarnings, doc.Warnings)
		})
	}
}

func Test_acb_keeps_the_header_of_a_security_whose_events_the_cap_cut_all(t *testing.T) {
	h := newHarness(t, &fakeStore{history: acbManyEvents(500, 1)}, nil, mcp.WithConfig((&configStub{cfg: acbConfig()}).load), atACBToday())

	doc := decodeACB(t, h.acb(t, map[string]any{}))

	assert.Equal(t, []string{"sec-acme", "sec-beta"}, securityIDs(doc))
	assert.Equal(t, []document.ACBEvent{}, doc.Securities[1].Events)
	assert.Equal(t, "Beta Fund", doc.Securities[1].Security)
	assert.Equal(t, "1.000000", doc.Securities[1].Shares)
}

func Test_acb_ends_its_warnings_with_the_cap_line(t *testing.T) {
	cfg := acbConfig()
	cfg.WarningsAbsolute = []string{configUnknownKeyWarning}
	h := newHarness(t, &fakeStore{history: acbManyEvents(300, 201)}, nil, mcp.WithConfig((&configStub{cfg: cfg}).load), atACBToday())

	doc := decodeACB(t, h.acb(t, map[string]any{}))

	assert.Equal(t, []string{configUnknownKeyWarning, "acb lists the first 500 events of 501; pass security to narrow"}, doc.Warnings)
}

func Test_acb_counts_only_the_securities_the_call_names_against_the_cap(t *testing.T) {
	h := newHarness(t, &fakeStore{history: acbManyEvents(300, 201)}, nil, mcp.WithConfig((&configStub{cfg: acbConfig()}).load), atACBToday())

	doc := decodeACB(t, h.acb(t, map[string]any{"security": []string{"sec-acme"}}))

	require.Len(t, doc.Securities, 1)
	assert.Len(t, doc.Securities[0].Events, 300)
	assert.Equal(t, []string{}, doc.Warnings)
}
