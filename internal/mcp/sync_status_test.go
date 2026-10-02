package mcp_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/mcp"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	duplicateID = "duplicate:txn-1+txn-2"
	fixedID     = "duplicate:txn-3+txn-4"
	logPrefix   = "quarry: mcp: sync_status: "
)

// configStub is a config loader that answers cfg, or err when set, and records the commands it was asked for.
type configStub struct {
	cfg      config.Config
	err      error
	commands []string
}

func (c *configStub) load(command string) (config.Config, error) {
	c.commands = append(c.commands, command)
	return c.cfg, c.err
}

// statusFixture is a store status holding one open duplicate and one fixed one.
func statusFixture() store.Status {
	fixedAt := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	return store.Status{
		Path:    testStorePath,
		BuiltAt: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC),
		Findings: []store.Finding{
			{ID: duplicateID, Type: finding.Duplicate},
			{ID: fixedID, Type: finding.Duplicate, FixedAt: &fixedAt},
		},
	}
}

// decodeStatus is result's one text block decoded as the status document.
func decodeStatus(t *testing.T, result *sdk.CallToolResult) document.Status {
	t.Helper()
	var doc document.Status
	require.NoError(t, json.Unmarshal([]byte(textOf(t, result)), &doc))
	return doc
}

func Test_sync_status_returns_the_status_document_of_what_the_store_holds_as_compact_json(t *testing.T) {
	st := statusFixture()
	stub := &configStub{}
	h := newHarness(t, &fakeStore{status: st}, nil, mcp.WithConfig(stub.load))
	want, err := json.Marshal(document.NewStatus(st, document.FindingsTally{Counts: report.CountFindings(st, nil), IgnoreKnown: true}, nil))
	require.NoError(t, err)

	result := h.syncStatus(t)

	require.False(t, result.IsError, textOf(t, result))
	assert.Equal(t, string(want), textOf(t, result))
	assert.JSONEq(t, string(want), jsonOf(t, result.StructuredContent))
	assert.Equal(t, 1, decodeStatus(t, result).Findings.Open)
}

func Test_sync_status_reports_no_dates_for_a_store_without_transactions(t *testing.T) {
	h := newHarness(t, &fakeStore{status: store.Status{Path: testStorePath}}, nil, mcp.WithConfig((&configStub{}).load))

	result := h.syncStatus(t)

	require.False(t, result.IsError, textOf(t, result))
	doc := decodeStatus(t, result)
	assert.Nil(t, doc.Dates.First)
	assert.Nil(t, doc.Dates.Last)
}

func Test_sync_status_answers_a_store_fault_as_isError_with_its_text_and_one_stderr_line_without_reading_the_config(t *testing.T) {
	stub := &configStub{}
	h := newHarness(t, &fakeStore{err: errDiskOnFire}, nil, mcp.WithConfig(stub.load))

	result := h.syncStatus(t)

	assert.True(t, result.IsError)
	assert.Equal(t, errDiskOnFire.Error(), textOf(t, result))
	assert.Equal(t, logPrefix+errDiskOnFire.Error()+"\n", h.stderr.String())
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

	doc := decodeStatus(t, result)
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
	doc := decodeStatus(t, result)
	assert.Nil(t, doc.Findings.Ignored)
	assert.Equal(t, 1, doc.Findings.Open)
	assert.Equal(t, []string{document.CannotTellIgnored(config.ProblemAbsolute(refusal))}, doc.Warnings)
	assert.Contains(t, doc.Warnings[0], path)
}

func Test_sync_status_answers_a_loader_failure_that_is_not_a_config_refusal_with_the_same_warning(t *testing.T) {
	stub := &configStub{err: errNoHome}
	h := newHarness(t, &fakeStore{status: statusFixture()}, nil, mcp.WithConfig(stub.load))

	result := h.syncStatus(t)

	require.False(t, result.IsError, textOf(t, result))
	doc := decodeStatus(t, result)
	assert.Nil(t, doc.Findings.Ignored)
	assert.Equal(t, []string{document.CannotTellIgnored(errNoHome.Error())}, doc.Warnings)
}

func Test_sync_status_leaves_warnings_empty_for_a_config_with_unknown_keys(t *testing.T) {
	stub := &configStub{cfg: config.Config{
		Warnings:         []string{"~/config.toml: unknown key x"},
		WarningsAbsolute: []string{"/home/dave/config.toml: unknown key x"},
	}}
	h := newHarness(t, &fakeStore{status: statusFixture()}, nil, mcp.WithConfig(stub.load))

	result := h.syncStatus(t)

	assert.Equal(t, []string{}, decodeStatus(t, result).Warnings)
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
