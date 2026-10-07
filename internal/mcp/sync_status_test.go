package mcp_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/mcp"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	duplicateID = "duplicate:txn-1+txn-2"
	fixedID     = "duplicate:txn-3+txn-4"
	logPrefix   = "quarry: mcp: sync_status: "
)

func Test_sync_status_returns_the_status_document_of_what_the_store_holds_as_compact_json(t *testing.T) {
	st := statusFixture()
	stub := &configStub{}
	h := newHarness(t, &fakeStore{status: st}, nil, mcp.WithConfig(stub.load))
	want, err := json.Marshal(document.NewStatus(st, document.FindingsTally{Counts: report.CountFindings(st, nil, report.Classification{}), IgnoreKnown: true}, nil))
	require.NoError(t, err)

	result := h.syncStatus(t)

	require.False(t, result.IsError, textOf(t, result))
	assert.Equal(t, string(want), textOf(t, result))
	assert.JSONEq(t, string(want), jsonOf(t, result.StructuredContent))
	assert.Equal(t, 1, decodeDoc[document.Status](t, result).Findings.Open)
}

func Test_sync_status_reports_no_dates_for_a_store_without_transactions(t *testing.T) {
	h := newHarness(t, &fakeStore{status: store.Status{Path: testStorePath}}, nil, withDefaultConfig())

	result := h.syncStatus(t)

	require.False(t, result.IsError, textOf(t, result))
	doc := decodeDoc[document.Status](t, result)
	assert.Nil(t, doc.Dates.First)
	assert.Nil(t, doc.Dates.Last)
}

func Test_sync_status_answers_a_store_fault_as_isError_with_its_text_and_one_stderr_line_without_reading_the_config(t *testing.T) {
	stub := &configStub{}
	h := newHarness(t, &fakeStore{err: errDiskOnFire}, nil, mcp.WithConfig(stub.load))

	result := h.syncStatus(t)

	assert.True(t, result.IsError)
	assert.Equal(t, errDiskOnFire.Error(), textOf(t, result))
	assert.Equal(t, logPrefix+failedLogLine+"\n", h.stderr.String())
	assert.Empty(t, stub.commands)
}

func Test_sync_status_answers_a_report_factory_failure_as_isError_with_its_text(t *testing.T) {
	stub := &configStub{}
	h := newHarness(t, &fakeStore{}, errFactoryBroke, mcp.WithConfig(stub.load))

	result := h.syncStatus(t)

	assert.True(t, result.IsError)
	assert.Equal(t, errFactoryBroke.Error(), textOf(t, result))
	assert.Empty(t, stub.commands)
}

func Test_sync_status_moves_a_finding_from_open_to_ignored_when_the_config_ignores_it(t *testing.T) {
	stub := &configStub{cfg: config.Config{Ignore: []string{duplicateID, fixedID}}}
	h := newHarness(t, &fakeStore{status: statusFixture()}, nil, mcp.WithConfig(stub.load))

	result := h.syncStatus(t)

	doc := decodeDoc[document.Status](t, result)
	require.NotNil(t, doc.Findings.Ignored)
	assert.Equal(t, 1, *doc.Findings.Ignored)
	assert.Equal(t, 0, doc.Findings.Open)
	assert.Equal(t, 1, doc.Findings.Fixed)
}

func Test_sync_status_says_it_cannot_tell_what_is_ignored_when_the_config_is_refused(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("[snapshots]\nkeep = 0\n"), 0o600))
	_, refusal := config.Load(home, path)
	require.Error(t, refusal)
	stub := &configStub{err: refusal}
	h := newHarness(t, &fakeStore{status: statusFixture()}, nil, mcp.WithConfig(stub.load))

	result := h.syncStatus(t)

	require.False(t, result.IsError, textOf(t, result))
	doc := decodeDoc[document.Status](t, result)
	assert.Nil(t, doc.Findings.Ignored)
	assert.Equal(t, 1, doc.Findings.Open)
	assert.Equal(t, []string{document.CannotTellChoices(config.ProblemAbsolute(refusal))}, doc.Warnings)
	assert.Contains(t, doc.Warnings[0], path)
}

func Test_sync_status_answers_a_loader_failure_that_is_not_a_config_refusal_with_the_same_warning(t *testing.T) {
	stub := &configStub{err: errNoHome}
	h := newHarness(t, &fakeStore{status: statusFixture()}, nil, mcp.WithConfig(stub.load))

	result := h.syncStatus(t)

	require.False(t, result.IsError, textOf(t, result))
	doc := decodeDoc[document.Status](t, result)
	assert.Nil(t, doc.Findings.Ignored)
	assert.Equal(t, []string{document.CannotTellChoices(errNoHome.Error())}, doc.Warnings)
}

func Test_sync_status_leaves_warnings_empty_for_a_config_with_unknown_keys(t *testing.T) {
	stub := &configStub{cfg: config.Config{
		Warnings:         []string{"~/config.toml: unknown key x"},
		WarningsAbsolute: []string{"/home/dave/config.toml: unknown key x"},
	}}
	h := newHarness(t, &fakeStore{status: statusFixture()}, nil, mcp.WithConfig(stub.load))

	result := h.syncStatus(t)

	assert.Equal(t, []string{}, decodeDoc[document.Status](t, result).Warnings)
}

func Test_sync_status_loads_the_config_and_reads_the_store_on_every_call(t *testing.T) {
	stub := &configStub{}
	h := newHarness(t, &fakeStore{status: statusFixture()}, nil, mcp.WithConfig(stub.load))

	h.syncStatus(t)
	h.syncStatus(t)

	assert.Equal(t, []string{"mcp", "mcp"}, stub.commands)
	assert.Equal(t, 2, h.store.statusReads)
	assert.Equal(t, []string{"mcp", "mcp"}, h.commands)
}

// statusWithBrokerage is statusFixture plus one brokerage account the config may classify.
func statusWithBrokerage() store.Status {
	st := statusFixture()
	st.Accounts = []store.Account{{ID: "acct-1", Name: "TFSA", Type: store.AccountTypeBrokerage, Currency: "CAD"}}
	return st
}

func Test_sync_status_counts_an_account_the_config_does_not_classify_as_open(t *testing.T) {
	stub := &configStub{}
	h := newHarness(t, &fakeStore{status: statusWithBrokerage()}, nil, mcp.WithConfig(stub.load))

	result := h.syncStatus(t)

	assert.Equal(t, 2, decodeDoc[document.Status](t, result).Findings.Open)
}

func Test_sync_status_leaves_an_account_the_config_classifies_out_of_the_open_count(t *testing.T) {
	stub := &configStub{cfg: config.Config{NonRegistered: []string{"acct-1"}}}
	h := newHarness(t, &fakeStore{status: statusWithBrokerage()}, nil, mcp.WithConfig(stub.load))

	result := h.syncStatus(t)

	assert.Equal(t, 1, decodeDoc[document.Status](t, result).Findings.Open)
}

func Test_sync_status_counts_an_ignored_unclassified_account_as_ignored(t *testing.T) {
	stub := &configStub{cfg: config.Config{Ignore: []string{"unclassified-account:acct-1"}}}
	h := newHarness(t, &fakeStore{status: statusWithBrokerage()}, nil, mcp.WithConfig(stub.load))

	doc := decodeDoc[document.Status](t, h.syncStatus(t))

	require.NotNil(t, doc.Findings.Ignored)
	assert.Equal(t, 1, *doc.Findings.Ignored)
	assert.Equal(t, 1, doc.Findings.Open)
}

func Test_sync_status_counts_every_investment_account_open_when_the_config_is_refused(t *testing.T) {
	stub := &configStub{err: errNoHome}
	h := newHarness(t, &fakeStore{status: statusWithBrokerage()}, nil, mcp.WithConfig(stub.load))

	doc := decodeDoc[document.Status](t, h.syncStatus(t))

	assert.Equal(t, 2, doc.Findings.Open)
	assert.Equal(t, []string{document.CannotTellChoices(errNoHome.Error())}, doc.Warnings)
}
