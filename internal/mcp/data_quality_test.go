package mcp_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/mcp"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	dqLogPrefix = "quarry: mcp: data_quality: "
	dqConfigLog = "cannot read quarry's config file; run quarry findings to see why"
	dqConfig    = testHome + "/Library/Application Support/quarry/config.toml"
)

var errBadConfig = errors.New(dqConfig + ": snapshots.keep must be at least 1; fix the file and run the command again")

// uncategorized is n open uncategorized findings, ids p-0000.., each with one item, so they list in id order.
func uncategorized(n int) []store.Finding {
	findings := make([]store.Finding, n)
	for i := range findings {
		findings[i] = store.Finding{
			ID: fmt.Sprintf("uncategorized:p-%04d", i), Type: finding.Uncategorized,
			Items: []store.FindingItem{{Payee: "Payee"}},
		}
	}
	return findings
}

// withItems is f holding n items.
func withItems(f store.Finding, n int) store.Finding {
	f.Items = make([]store.FindingItem, n)
	return f
}

// listOf is a fake store holding findings.
func listOf(findings ...store.Finding) *fakeStore {
	return &fakeStore{findings: store.FindingList{Findings: findings}}
}

// findingIDs is the ids of doc's findings, in listed order.
func findingIDs(doc document.FindingsList) []string {
	ids := make([]string, len(doc.Findings))
	for i, f := range doc.Findings {
		ids[i] = f.ID
	}
	return ids
}

// itemsOf is the number of items of doc's finding id.
func itemsOf(t *testing.T, doc document.FindingsList, id string) int {
	t.Helper()
	for _, f := range doc.Findings {
		if f.ID == id {
			return len(f.Items)
		}
	}
	require.FailNow(t, "finding not listed", id)
	return 0
}

func Test_data_quality_returns_the_findings_list_document_of_what_the_store_holds_as_compact_json(t *testing.T) {
	st := listOf(uncategorized(2)...)
	stub := &configStub{}
	h := newHarness(t, st, nil, mcp.WithConfig(stub.load))
	listing, err := report.NewServer(report.WithStore(st), report.WithHome(testHome)).Findings(t.Context(), report.FindingsRequest{})
	require.NoError(t, err)
	want, err := json.Marshal(document.NewFindingsList(listing, finding.StatusOpen, "", nil))
	require.NoError(t, err)

	result := h.dataQuality(t, map[string]any{})

	require.False(t, result.IsError, textOf(t, result))
	assert.Equal(t, string(want), textOf(t, result))
	assert.JSONEq(t, string(want), jsonOf(t, result.StructuredContent))
	doc := decodeDoc[document.FindingsList](t, result)
	assert.Equal(t, "open", doc.Status)
	assert.Nil(t, doc.Type)
	assert.Equal(t, []string{"uncategorized:p-0000", "uncategorized:p-0001"}, findingIDs(doc))
}

func Test_data_quality_lists_a_finding_the_config_ignores_under_ignored_not_open(t *testing.T) {
	stub := &configStub{cfg: config.Config{Ignore: []string{duplicateID}}}
	h := newHarness(t, listOf(store.Finding{ID: duplicateID, Type: finding.Duplicate}), nil, mcp.WithConfig(stub.load))

	open := decodeDoc[document.FindingsList](t, h.dataQuality(t, map[string]any{}))
	ignored := decodeDoc[document.FindingsList](t, h.dataQuality(t, map[string]any{"status": "ignored"}))

	assert.Empty(t, open.Findings)
	assert.Equal(t, 0, open.Counts.Open)
	assert.Equal(t, []string{duplicateID}, findingIDs(ignored))
	assert.Equal(t, "ignored", ignored.Findings[0].Status)
	assert.Equal(t, 1, ignored.Counts.Ignored)
}

func Test_data_quality_lists_a_fixed_finding_under_fixed_with_no_items(t *testing.T) {
	fixedAt := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	h := newHarness(t, listOf(store.Finding{ID: fixedID, Type: finding.Duplicate, FixedAt: &fixedAt}), nil, withDefaultConfig())

	doc := decodeDoc[document.FindingsList](t, h.dataQuality(t, map[string]any{"status": "fixed"}))

	assert.Equal(t, []string{fixedID}, findingIDs(doc))
	assert.Empty(t, doc.Findings[0].Items)
	assert.Equal(t, []string{}, doc.Warnings)
}

func Test_data_quality_filters_by_type_and_says_which_type_in_the_document(t *testing.T) {
	st := listOf(store.Finding{ID: duplicateID, Type: finding.Duplicate}, uncategorized(1)[0])
	h := newHarness(t, st, nil, withDefaultConfig())

	doc := decodeDoc[document.FindingsList](t, h.dataQuality(t, map[string]any{"type": "duplicate"}))

	require.NotNil(t, doc.Type)
	assert.Equal(t, "duplicate", *doc.Type)
	assert.Equal(t, []string{duplicateID}, findingIDs(doc))
	assert.Equal(t, 1, doc.Counts.Open)
}

func Test_data_quality_lists_nothing_and_warns_of_nothing_for_a_store_without_findings(t *testing.T) {
	h := newHarness(t, listOf(), nil, withDefaultConfig())

	doc := decodeDoc[document.FindingsList](t, h.dataQuality(t, map[string]any{}))

	assert.Equal(t, []document.FindingEntry{}, doc.Findings)
	assert.Equal(t, []string{}, doc.Warnings)
}

func Test_data_quality_sends_the_configs_unknown_key_lines_with_absolute_paths(t *testing.T) {
	stub := &configStub{cfg: config.Config{
		Warnings:         []string{"~/config.toml: unknown key x"},
		WarningsAbsolute: []string{"/home/dave/config.toml: unknown key x", "/home/dave/config.toml: unknown key y"},
	}}
	h := newHarness(t, listOf(), nil, mcp.WithConfig(stub.load))

	doc := decodeDoc[document.FindingsList](t, h.dataQuality(t, map[string]any{}))

	assert.Equal(t, stub.cfg.WarningsAbsolute, doc.Warnings)
}

func Test_data_quality_warns_with_the_absolute_config_path_of_an_ignored_id_naming_no_finding(t *testing.T) {
	stub := &configStub{cfg: config.Config{Path: dqConfig, Ignore: []string{"duplicate:gone"}}}
	h := newHarness(t, listOf(), nil, mcp.WithConfig(stub.load))

	doc := decodeDoc[document.FindingsList](t, h.dataQuality(t, map[string]any{}))

	assert.Equal(t, document.UnmatchedIgnoreWarnings(dqConfig, []string{"duplicate:gone"}), doc.Warnings)
	assert.Contains(t, doc.Warnings[0], testHome+"/Library")
}

func Test_data_quality_refuses_a_bad_config_before_touching_the_store(t *testing.T) {
	refusal := errBadConfig
	stub := &configStub{err: refusal}
	h := newHarness(t, listOf(uncategorized(1)...), nil, mcp.WithConfig(stub.load))

	result := h.dataQuality(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, refusal.Error(), textOf(t, result))
	assert.Equal(t, dqLogPrefix+dqConfigLog+"\n", h.stderr.String())
	assert.Zero(t, h.built)
	assert.Zero(t, h.store.findingsReads)
}

func Test_data_quality_refuses_a_loader_failure_that_is_not_a_config_refusal_the_same_way(t *testing.T) {
	stub := &configStub{err: errNoHome}
	h := newHarness(t, listOf(), nil, mcp.WithConfig(stub.load))

	result := h.dataQuality(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, errNoHome.Error(), textOf(t, result))
	assert.Equal(t, dqLogPrefix+dqConfigLog+"\n", h.stderr.String())
	assert.Zero(t, h.built)
}

func Test_data_quality_answers_a_report_factory_failure_as_isError_with_its_text(t *testing.T) {
	h := newHarness(t, listOf(), errFactoryBroke, withDefaultConfig())

	result := h.dataQuality(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, errFactoryBroke.Error(), textOf(t, result))
	assert.Equal(t, dqLogPrefix+failedLogLine+"\n", h.stderr.String())
}

func Test_data_quality_answers_a_store_fault_as_isError_with_its_text(t *testing.T) {
	h := newHarness(t, &fakeStore{err: errDiskOnFire}, nil, withDefaultConfig())

	result := h.dataQuality(t, map[string]any{})

	assert.True(t, result.IsError)
	assert.Equal(t, errDiskOnFire.Error(), textOf(t, result))
	assert.Equal(t, dqLogPrefix+failedLogLine+"\n", h.stderr.String())
	assert.Nil(t, result.StructuredContent)
}

func Test_data_quality_loads_the_config_and_reads_the_store_on_every_call(t *testing.T) {
	stub := &configStub{}
	h := newHarness(t, listOf(store.Finding{ID: duplicateID, Type: finding.Duplicate}), nil, mcp.WithConfig(stub.load))

	before := decodeDoc[document.FindingsList](t, h.dataQuality(t, map[string]any{}))
	stub.cfg = config.Config{Ignore: []string{duplicateID}}
	after := decodeDoc[document.FindingsList](t, h.dataQuality(t, map[string]any{}))

	assert.Equal(t, []string{duplicateID}, findingIDs(before))
	assert.Empty(t, after.Findings)
	assert.Equal(t, []string{"mcp", "mcp"}, stub.commands)
	assert.Equal(t, 2, h.store.findingsReads)
	assert.Equal(t, []string{"mcp", "mcp"}, h.commands)
}

func Test_data_quality_lists_the_first_limit_findings_and_warns_of_the_total_only_when_it_cuts(t *testing.T) {
	cases := []struct {
		name         string
		findings     int
		limit        int
		wantListed   int
		wantWarnings []string
	}{
		{name: "exactly limit findings", findings: 3, limit: 3, wantListed: 3, wantWarnings: []string{}},
		{
			name: "one more than limit", findings: 4, limit: 3, wantListed: 3,
			wantWarnings: []string{"listed the first 3 of 4 open findings; pass type to narrow the list, or a larger limit (at most 500)"},
		},
		{
			name: "limit 1", findings: 2, limit: 1, wantListed: 1,
			wantWarnings: []string{"listed the first 1 of 2 open findings; pass type to narrow the list, or a larger limit (at most 500)"},
		},
		{
			name: "limit 500 over 501 findings", findings: 501, limit: 500, wantListed: 500,
			wantWarnings: []string{"listed the first 500 of 501 open findings; pass type to narrow the list"},
		},
		{name: "limit 500 at 500 findings", findings: 500, limit: 500, wantListed: 500, wantWarnings: []string{}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, listOf(uncategorized(c.findings)...), nil, withDefaultConfig())

			doc := decodeDoc[document.FindingsList](t, h.dataQuality(t, map[string]any{"limit": c.limit}))

			assert.Len(t, doc.Findings, c.wantListed)
			assert.Equal(t, c.wantWarnings, doc.Warnings)
			assert.Equal(t, uncategorizedIDs(c.wantListed), findingIDs(doc))
		})
	}
}

// uncategorizedIDs is the ids of uncategorized(n)'s first n findings, in listed order.
func uncategorizedIDs(n int) []string {
	ids := make([]string, n)
	for i, f := range uncategorized(n) {
		ids[i] = f.ID
	}
	return ids
}

func Test_data_quality_lists_the_first_50_findings_when_limit_is_omitted(t *testing.T) {
	h := newHarness(t, listOf(uncategorized(60)...), nil, withDefaultConfig())

	doc := decodeDoc[document.FindingsList](t, h.dataQuality(t, map[string]any{}))

	assert.Equal(t, uncategorizedIDs(50), findingIDs(doc))
	assert.Equal(t, []string{"listed the first 50 of 60 open findings; pass type to narrow the list, or a larger limit (at most 500)"}, doc.Warnings)
}

func Test_data_quality_cuts_across_groups_in_the_order_findings_lists_them(t *testing.T) {
	st := listOf(
		withItems(store.Finding{ID: "uncategorized:few", Type: finding.Uncategorized}, 1),
		withItems(store.Finding{ID: "uncategorized:many", Type: finding.Uncategorized}, 3),
		store.Finding{ID: "duplicate:b", Type: finding.Duplicate},
		store.Finding{ID: "duplicate:a", Type: finding.Duplicate},
	)
	h := newHarness(t, st, nil, withDefaultConfig())

	doc := decodeDoc[document.FindingsList](t, h.dataQuality(t, map[string]any{"limit": 3}))

	assert.Equal(t, []string{"duplicate:a", "duplicate:b", "uncategorized:many"}, findingIDs(doc))
}

func Test_data_quality_words_the_findings_cap_for_each_status(t *testing.T) {
	fixedAt := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	st := listOf(
		store.Finding{ID: "duplicate:a", Type: finding.Duplicate},
		store.Finding{ID: "duplicate:b", Type: finding.Duplicate},
		store.Finding{ID: "duplicate:c", Type: finding.Duplicate, FixedAt: &fixedAt},
		store.Finding{ID: "duplicate:d", Type: finding.Duplicate, FixedAt: &fixedAt},
	)
	stub := &configStub{cfg: config.Config{Ignore: []string{"duplicate:a", "duplicate:b"}}}
	const tail = "; pass type to narrow the list, or a larger limit (at most 500)"
	cases := []struct {
		status string
		want   string
	}{
		{status: "ignored", want: "listed the first 1 of 2 ignored findings" + tail},
		{status: "fixed", want: "listed the first 1 of 2 fixed findings" + tail},
		{status: "all", want: "listed the first 1 of 4 findings" + tail},
	}

	for _, c := range cases {
		t.Run(c.status, func(t *testing.T) {
			h := newHarness(t, st, nil, mcp.WithConfig(stub.load))

			doc := decodeDoc[document.FindingsList](t, h.dataQuality(t, map[string]any{"status": c.status, "limit": 1}))

			assert.Equal(t, []string{c.want}, doc.Warnings)
		})
	}
}

func Test_data_quality_words_the_findings_cap_tail_by_limit_and_type(t *testing.T) {
	cases := []struct {
		name  string
		args  map[string]any
		total int
		want  string
	}{
		{
			name: "limit under 500, no type", args: map[string]any{"limit": 2}, total: 3,
			want: "listed the first 2 of 3 open findings; pass type to narrow the list, or a larger limit (at most 500)",
		},
		{
			name: "limit under 500, type given", args: map[string]any{"limit": 2, "type": "uncategorized"}, total: 3,
			want: "listed the first 2 of 3 open findings; pass a larger limit (at most 500)",
		},
		{
			name: "limit 500, no type", args: map[string]any{"limit": 500}, total: 501,
			want: "listed the first 500 of 501 open findings; pass type to narrow the list",
		},
		{
			name: "limit 500, type given", args: map[string]any{"limit": 500, "type": "uncategorized"}, total: 501,
			want: "listed the first 500 of 501 open findings",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, listOf(uncategorized(c.total)...), nil, withDefaultConfig())

			doc := decodeDoc[document.FindingsList](t, h.dataQuality(t, c.args))

			assert.Equal(t, []string{c.want}, doc.Warnings)
			assert.NotContains(t, doc.Warnings[0], "uncategorized")
		})
	}
}

func Test_data_quality_groups_the_totals_in_the_findings_cap_line(t *testing.T) {
	h := newHarness(t, listOf(uncategorized(1234)...), nil, withDefaultConfig())

	doc := decodeDoc[document.FindingsList](t, h.dataQuality(t, map[string]any{"limit": 500}))

	assert.Equal(t, []string{"listed the first 500 of 1,234 open findings; pass type to narrow the list"}, doc.Warnings)
}

func Test_data_quality_never_trims_counts(t *testing.T) {
	fixedAt := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	findings := append(uncategorized(5), store.Finding{ID: duplicateID, Type: finding.Duplicate}, store.Finding{ID: fixedID, Type: finding.Duplicate, FixedAt: &fixedAt})
	h := newHarness(t, listOf(findings...), nil, withDefaultConfig())

	all := decodeDoc[document.FindingsList](t, h.dataQuality(t, map[string]any{"limit": 2, "status": "all"}))
	typed := decodeDoc[document.FindingsList](t, h.dataQuality(t, map[string]any{"limit": 2, "type": "uncategorized"}))

	assert.Len(t, all.Findings, 2)
	assert.Equal(t, document.FindingCounts{Open: 6, Fixed: 1}, all.Counts)
	assert.Len(t, typed.Findings, 2)
	assert.Equal(t, document.FindingCounts{Open: 5}, typed.Counts)
}

func Test_data_quality_refuses_a_limit_outside_1_to_500_and_accepts_the_bounds(t *testing.T) {
	h := newHarness(t, listOf(uncategorized(2)...), nil, withDefaultConfig())

	accepted := []int{1, 500}
	refused := []int{0, 501}

	for _, limit := range accepted {
		decodeDoc[document.FindingsList](t, h.dataQuality(t, map[string]any{"limit": limit}))
	}
	for _, limit := range refused {
		result := h.dataQuality(t, map[string]any{"limit": limit})
		assert.True(t, result.IsError, "limit %d", limit)
	}
}

func Test_data_quality_refuses_a_status_or_type_outside_its_enum_and_an_unknown_parameter(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
	}{
		{name: "status", args: map[string]any{"status": "pending"}},
		{name: "type", args: map[string]any{"type": "orphan"}},
		{name: "unknown parameter", args: map[string]any{"page": 2}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, listOf(uncategorized(1)...), nil, withDefaultConfig())

			result := h.dataQuality(t, c.args)

			assert.True(t, result.IsError)
			assert.Zero(t, h.store.findingsReads)
		})
	}
}

func Test_data_quality_lists_the_first_25_items_of_a_finding_and_warns_only_over_the_cap(t *testing.T) {
	cases := []struct {
		name  string
		items int
		want  []string
	}{
		{name: "exactly 25 items", items: 25, want: []string{}},
		{
			name: "26 items", items: 26,
			want: []string{"finding uncategorized:heavy lists the first 25 of 26 items; query finding_items WHERE finding_id = 'uncategorized:heavy' for the rest"},
		},
		{
			name: "1,234 items", items: 1234,
			want: []string{"finding uncategorized:heavy lists the first 25 of 1,234 items; query finding_items WHERE finding_id = 'uncategorized:heavy' for the rest"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			heavy := withItems(store.Finding{ID: "uncategorized:heavy", Type: finding.Uncategorized}, c.items)
			h := newHarness(t, listOf(heavy), nil, withDefaultConfig())

			doc := decodeDoc[document.FindingsList](t, h.dataQuality(t, map[string]any{}))

			assert.Equal(t, min(c.items, 25), itemsOf(t, doc, "uncategorized:heavy"))
			assert.Equal(t, c.want, doc.Warnings)
		})
	}
}

func Test_data_quality_warns_for_each_over_cap_finding_in_listed_order(t *testing.T) {
	st := listOf(
		withItems(store.Finding{ID: "uncategorized:b", Type: finding.Uncategorized}, 27),
		withItems(store.Finding{ID: "uncategorized:a", Type: finding.Uncategorized}, 30),
		withItems(store.Finding{ID: "uncategorized:c", Type: finding.Uncategorized}, 2),
	)
	h := newHarness(t, st, nil, withDefaultConfig())

	doc := decodeDoc[document.FindingsList](t, h.dataQuality(t, map[string]any{}))

	assert.Equal(t, []string{
		"finding uncategorized:a lists the first 25 of 30 items; query finding_items WHERE finding_id = 'uncategorized:a' for the rest",
		"finding uncategorized:b lists the first 25 of 27 items; query finding_items WHERE finding_id = 'uncategorized:b' for the rest",
	}, doc.Warnings)
	assert.Equal(t, 2, itemsOf(t, doc, "uncategorized:c"))
}

func Test_data_quality_gives_a_finding_cut_by_the_findings_cap_no_items_warning(t *testing.T) {
	st := listOf(
		withItems(store.Finding{ID: "uncategorized:kept", Type: finding.Uncategorized}, 400),
		withItems(store.Finding{ID: "uncategorized:dropped", Type: finding.Uncategorized}, 300),
	)
	h := newHarness(t, st, nil, withDefaultConfig())

	doc := decodeDoc[document.FindingsList](t, h.dataQuality(t, map[string]any{"limit": 1}))

	assert.Equal(t, []string{"uncategorized:kept"}, findingIDs(doc))
	assert.Equal(t, []string{
		"listed the first 1 of 2 open findings; pass type to narrow the list, or a larger limit (at most 500)",
		"finding uncategorized:kept lists the first 25 of 400 items; query finding_items WHERE finding_id = 'uncategorized:kept' for the rest",
	}, doc.Warnings)
}

func Test_data_quality_leaves_the_stores_items_whole_so_a_second_call_warns_again(t *testing.T) {
	heavy := withItems(store.Finding{ID: "uncategorized:heavy", Type: finding.Uncategorized}, 26)
	h := newHarness(t, listOf(heavy), nil, withDefaultConfig())

	first := decodeDoc[document.FindingsList](t, h.dataQuality(t, map[string]any{}))
	second := decodeDoc[document.FindingsList](t, h.dataQuality(t, map[string]any{}))

	assert.Len(t, first.Warnings, 1)
	assert.Equal(t, first.Warnings, second.Warnings)
	assert.Len(t, h.store.findings.Findings[0].Items, 26)
}

func Test_data_quality_orders_its_warnings(t *testing.T) {
	stub := &configStub{cfg: config.Config{
		Path:             dqConfig,
		Ignore:           []string{"duplicate:gone"},
		Registered:       []string{"acct-99"},
		WarningsAbsolute: []string{"/home/dave/config.toml: unknown key x"},
	}}
	st := listOf(
		withItems(store.Finding{ID: "uncategorized:b", Type: finding.Uncategorized}, 26),
		withItems(store.Finding{ID: "uncategorized:a", Type: finding.Uncategorized}, 27),
		withItems(store.Finding{ID: "uncategorized:c", Type: finding.Uncategorized}, 2),
	)
	h := newHarness(t, st, nil, mcp.WithConfig(stub.load))

	doc := decodeDoc[document.FindingsList](t, h.dataQuality(t, map[string]any{"limit": 2}))

	assert.Equal(t, []string{
		"/home/dave/config.toml: unknown key x",
		document.UnmatchedIgnoreWarnings(dqConfig, []string{"duplicate:gone"})[0],
		document.UnmatchedAccountWarnings(dqConfig, report.UnmatchedAccounts{Registered: []string{"acct-99"}})[0],
		"listed the first 2 of 3 open findings; pass type to narrow the list, or a larger limit (at most 500)",
		"finding uncategorized:a lists the first 25 of 27 items; query finding_items WHERE finding_id = 'uncategorized:a' for the rest",
		"finding uncategorized:b lists the first 25 of 26 items; query finding_items WHERE finding_id = 'uncategorized:b' for the rest",
	}, doc.Warnings)
}
