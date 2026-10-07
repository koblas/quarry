// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

// syncBundle runs sync on bundle under the test's HOME, failing t unless it succeeds.
func syncBundle(t *testing.T, bundle v9fixture.Bundle) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr), stderr.String())
}

// onlyFileWithSuffix fails the test unless exactly one entry in dir ends in
// suffix, returning its full path.
func onlyFileWithSuffix(t *testing.T, dir, suffix string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var found []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), suffix) {
			found = append(found, e.Name())
		}
	}
	require.Len(t, found, 1, "expected exactly one %s file in %s, found %v", suffix, dir, found)
	return filepath.Join(dir, found[0])
}

// abbreviated mirrors the CLI's ~-abbreviation so the expected string is
// built from the real path, not a re-derived one.
func abbreviated(t *testing.T, path, home string) string {
	t.Helper()
	require.True(t, strings.HasPrefix(path, home+string(filepath.Separator)))
	return "~" + strings.TrimPrefix(path, home)
}

// megabytes mirrors the CLI's decimal-MB rounding for a fixture small enough
// that thousands-grouping never applies.
func megabytes(bytes int64) string {
	tenths := (bytes*10 + 500000) / 1000000
	return fmt.Sprintf("%d.%d MB", tenths/10, tenths%10)
}

// snapshotID derives an <id> from a lowercase-extension fixture path: its
// basename with .sqlite removed. It does not fold case like the CLI.
func snapshotID(snapshotPath string) string {
	return strings.TrimSuffix(filepath.Base(snapshotPath), ".sqlite")
}

// skipAsRoot skips t under root, whom file modes do not stop.
func skipAsRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root ignores file modes")
	}
}

// replaceStore builds quarry's store under home straight from rows, skipping
// Quicken: view-level tests own every row the store holds.
func replaceStore(t *testing.T, home string, rows store.Rows) {
	t.Helper()
	_, err := duckstore.New(storeDirUnder(home)).Replace(context.Background(), rows)
	require.NoError(t, err)
}

// spendEnv is an env writing to stdout and stderr, with the clock at 2026-09-29 12:00 UTC.
func spendEnv(stdout, stderr *bytes.Buffer) cli.Env {
	return spendEnvAt(stdout, stderr, time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC))
}

// spendEnvAt is spendEnv with the clock at now.
func spendEnvAt(stdout, stderr *bytes.Buffer, now time.Time) cli.Env {
	e := testEnv(stdout, stderr)
	e.Now = func() time.Time { return now }
	return e
}

// chequingAccount is an active CAD chequing account named "Chequing".
func chequingAccount(id string, sourceID int64) store.Account {
	return store.Account{ID: id, SourceID: sourceID, Name: "Chequing", Type: "chequing", Currency: "CAD", Active: true}
}

// usdChequingAccount is an active USD chequing account named "US Chequing".
func usdChequingAccount(id string, sourceID int64) store.Account {
	return store.Account{ID: id, SourceID: sourceID, Name: "US Chequing", Type: "chequing", Currency: "USD", Active: true}
}

// utcMinus5 is a zone whose evening is already the next day in UTC.
var utcMinus5 = time.FixedZone("UTC-5", -5*60*60)

// spendSplit is one single-split transaction; "" category or payee means none,
// negative cents is money out, and tags are tag ids.
type spendSplit struct {
	id, account, category, payee, currency string
	day                                    time.Time
	cents                                  int64
	tags                                   []string
}

// spendRows is a store of accounts holding splits, inside one import run. It
// always holds the same reference data, which a split names by id:
//
//	cat-fuel "Auto:Fuel", cat-groceries "Food:Groceries"
//	payee-costco "Costco", payee-bakery "Bakery"
//	tag-vacation "Vacation", tag-alpha "alpha"
//
// Both categories are referenced, so no store of it has an unused-category finding.
func spendRows(accounts []store.Account, splits ...spendSplit) store.Rows {
	rows := store.Rows{
		Accounts: accounts,
		Payees: []store.Payee{
			{ID: "payee-costco", SourceID: 1, Name: "Costco"},
			{ID: "payee-bakery", SourceID: 2, Name: "Bakery"},
		},
		Tags: []store.Tag{
			{ID: "tag-vacation", SourceID: 1, Name: "Vacation"},
			{ID: "tag-alpha", SourceID: 2, Name: "alpha"},
		},
		Categories: []store.Category{
			{ID: "cat-fuel", SourceID: 1, Name: "Fuel", FullPath: "Auto:Fuel", Kind: "expense"},
			{ID: "cat-groceries", SourceID: 2, Name: "Groceries", FullPath: "Food:Groceries", Kind: "expense"},
		},
		ReferencedCategoryIDs: []string{"cat-fuel", "cat-groceries"},
		ImportRuns: []store.ImportRun{{
			ID: 1, StartedAt: time.Unix(0, 0).UTC(), FinishedAt: time.Unix(0, 0).UTC(),
			Snapshot: store.SnapshotRef{Path: "/snapshots/20260929T000000Z.sqlite", SHA256: "9f86", SchemaFingerprint: "sha256:abc"},
		}},
	}
	for _, s := range splits {
		var category *string
		if s.category != "" {
			category = new(s.category)
		}
		var payee *string
		if s.payee != "" {
			payee = new(s.payee)
		}
		rows.Transactions = append(rows.Transactions, store.Transaction{
			ID: "txn-" + s.id, SourceID: 1, AccountID: s.account, Date: s.day,
			Amount: s.cents, Currency: s.currency, Status: "uncleared", PayeeID: payee,
		})
		rows.Splits = append(rows.Splits, store.Split{
			ID: "split-" + s.id, SourceID: 1, TransactionID: "txn-" + s.id, CategoryID: category, Amount: s.cents,
		})
		for _, tag := range s.tags {
			rows.SplitTags = append(rows.SplitTags, store.SplitTag{SplitID: "split-" + s.id, TagID: tag})
		}
	}
	return rows
}

// chargeSplit is one split of a chargeTxn; "" category means none, negative cents is money out.
type chargeSplit struct {
	category string
	cents    int64
}

// chargeTxn is one transaction of any number of splits; "" payee means none, and the
// payee is stored under the id "payee-<name>".
type chargeTxn struct {
	id, account, payee, currency string
	day                          time.Time
	splits                       []chargeSplit
}

// chargeRows is spendRows' reference data (without its payees) plus txns, each given the
// next source id from 1 and a payee row per distinct name, so a transaction may hold several splits.
func chargeRows(accounts []store.Account, txns ...chargeTxn) store.Rows {
	rows := spendRows(accounts)
	rows.Payees = nil
	known := map[string]bool{}
	for i, tx := range txns {
		var payeeID *string
		if tx.payee != "" {
			id := "payee-" + tx.payee
			if !known[id] {
				known[id] = true
				rows.Payees = append(rows.Payees, store.Payee{ID: id, SourceID: int64(len(rows.Payees) + 1), Name: tx.payee})
			}
			payeeID = &id
		}
		var amount int64
		for j, sp := range tx.splits {
			var category *string
			if sp.category != "" {
				category = new(sp.category)
			}
			amount += sp.cents
			rows.Splits = append(rows.Splits, store.Split{
				ID: fmt.Sprintf("split-%s-%d", tx.id, j), SourceID: int64(j + 1), TransactionID: "txn-" + tx.id,
				CategoryID: category, Amount: sp.cents,
			})
		}
		rows.Transactions = append(rows.Transactions, store.Transaction{
			ID: "txn-" + tx.id, SourceID: int64(i + 1), AccountID: tx.account, Date: tx.day,
			Amount: amount, Currency: tx.currency, Status: "uncleared", PayeeID: payeeID,
		})
	}
	return rows
}

// day is the civil day y-m-d at UTC midnight, as the store dates transactions.
func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

// cashFlowRows is spendRows plus an income category, cat-salary.
func cashFlowRows(accounts []store.Account, splits ...spendSplit) store.Rows {
	rows := spendRows(accounts, splits...)
	rows.Categories = append(rows.Categories,
		store.Category{ID: "cat-salary", SourceID: 3, Name: "Salary", FullPath: "Income:Salary", Kind: "income"})
	return rows
}

const mcpTestDeadline = 30 * time.Second

// mcpPeer is a quarry mcp running in-process with an MCP client connected to it over pipes.
type mcpPeer struct {
	session        *sdk.ClientSession
	exit           <-chan int
	stdout, stderr *bytes.Buffer
}

// startMCP runs quarry mcp over pipes, lets tweak adjust its Env, and connects a client.
func startMCP(ctx context.Context, t *testing.T, tweak func(*cli.Env)) *mcpPeer {
	t.Helper()
	serverStdin, toServer := io.Pipe()
	serverStdout, fromServer := io.Pipe()
	peer := &mcpPeer{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	exit := make(chan int, 1)
	peer.exit = exit
	go func() {
		env := testEnv(io.MultiWriter(fromServer, peer.stdout), peer.stderr)
		env.Stdin = serverStdin
		tweak(&env)
		exit <- runWith(ctx, []string{"mcp"}, env)
		_ = fromServer.Close()
	}()

	client := sdk.NewClient(&sdk.Implementation{Name: "test-client", Version: "v0.0.0"}, nil)
	session, err := client.Connect(ctx, &sdk.IOTransport{Reader: serverStdout, Writer: toServer}, nil)
	require.NoError(t, err)
	peer.session = session
	return peer
}

// waitForExit returns quarry mcp's exit code, failing the test if it has not exited by ctx's deadline.
func (p *mcpPeer) waitForExit(ctx context.Context, t *testing.T) int {
	t.Helper()
	select {
	case code := <-p.exit:
		return code
	case <-ctx.Done():
		require.FailNow(t, "quarry mcp did not return after the client closed its session")
		return 0
	}
}

// textOf is the text of result's content, which must be a single TextContent.
func textOf(result *sdk.CallToolResult) string {
	if len(result.Content) != 1 {
		return fmt.Sprintf("<%d content blocks>", len(result.Content))
	}
	text, ok := result.Content[0].(*sdk.TextContent)
	if !ok {
		return "<not text>"
	}
	return text.Text
}

// dataQualityDocument is the findings list document, with findings left as decoded maps for comparison.
type dataQualityDocument struct {
	Status   string           `json:"status"`
	Type     *string          `json:"type"`
	Counts   map[string]any   `json:"counts"`
	Findings []map[string]any `json:"findings"`
	Warnings []string         `json:"warnings"`
}

func callDataQuality(ctx context.Context, t *testing.T, peer *mcpPeer, arguments map[string]any) *sdk.CallToolResult {
	t.Helper()
	result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{Name: "data_quality", Arguments: arguments})
	require.NoError(t, err)
	return result
}

// syncStatusDocument is the part of sync_status's result these tests read.
type syncStatusDocument struct {
	statusFindingsJSON

	Store struct {
		Rows struct {
			Accounts int `json:"accounts"`
		} `json:"rows"`
	} `json:"store"`
	Snapshot struct {
		ID string `json:"id"`
	} `json:"snapshot"`
}

// newStatusPeer sets a fresh HOME, lets seed build the store under it, and connects a client to quarry mcp.
func newStatusPeer(t *testing.T, seed func(t *testing.T, home string)) (context.Context, *mcpPeer) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	seed(t, home)
	return startStatusPeer(t)
}

// startStatusPeer connects a client to quarry mcp over the HOME the test already set.
func startStatusPeer(t *testing.T) (context.Context, *mcpPeer) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
	t.Cleanup(cancel)
	return ctx, startMCP(ctx, t, func(*cli.Env) {})
}

func callSyncStatus(ctx context.Context, t *testing.T, peer *mcpPeer) *sdk.CallToolResult {
	t.Helper()
	result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{Name: "sync_status", Arguments: map[string]any{}})
	require.NoError(t, err)
	return result
}

// readSyncStatus calls sync_status and decodes the document it returns.
func readSyncStatus(ctx context.Context, t *testing.T, peer *mcpPeer) syncStatusDocument {
	t.Helper()
	result := callSyncStatus(ctx, t, peer)
	require.False(t, result.IsError, textOf(result))
	var doc syncStatusDocument
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &doc))
	return doc
}

// acbInToolWords is the CLI warnings with each command a no-cost line names replaced by the tool call that does the same.
func acbInToolWords(cliWarnings []string) []string {
	mapped := make([]string, len(cliWarnings))
	for i, line := range cliWarnings {
		line = strings.ReplaceAll(line, "quarry findings --type shares-without-cost", "data_quality with type shares-without-cost")
		mapped[i] = strings.ReplaceAll(line, "quarry acb --security ", "acb with security ")
	}
	return mapped
}

// repoRoot is the repository root as seen from cmd/quarry, where go test runs.
const repoRoot = "../../"

// repoFile reads a file named relative to the repo root.
func repoFile(t *testing.T, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(repoRoot + rel)
	require.NoError(t, err)
	return string(raw)
}

// skillText is SKILL.md cut at the places the ruled copy is pinned: the
// frontmatter, the intro under the title, and each numbered section's body.
type skillText struct {
	frontmatter string
	title       string
	intro       string
	headings    []string
	bodies      map[string]string
}

func splitSkill(t *testing.T, raw string) skillText {
	t.Helper()
	rest, ok := strings.CutPrefix(raw, "---\n")
	require.True(t, ok, "SKILL.md must open with frontmatter")
	fm, body, ok := strings.Cut(rest, "\n---\n")
	require.True(t, ok, "SKILL.md frontmatter must be closed")

	skill := skillText{frontmatter: "---\n" + fm + "\n---", bodies: map[string]string{}}
	var current *[]string
	var title, intro []string
	sections := map[string]*[]string{}
	for line := range strings.SplitSeq(body, "\n") {
		switch {
		case strings.HasPrefix(line, "## "):
			skill.headings = append(skill.headings, line)
			lines := []string{}
			sections[line] = &lines
			current = &lines
		case strings.HasPrefix(line, "# ") && current == nil:
			title = append(title, line)
		case current == nil:
			intro = append(intro, line)
		default:
			*current = append(*current, line)
		}
	}
	require.Len(t, title, 1, "SKILL.md must have exactly one title line")
	skill.title = title[0]
	skill.intro = strings.Trim(strings.Join(intro, "\n"), "\n")
	for heading, lines := range sections {
		skill.bodies[heading] = strings.Trim(strings.Join(*lines, "\n"), "\n")
	}
	return skill
}

func (s skillText) description() string {
	for line := range strings.SplitSeq(s.frontmatter, "\n") {
		if value, ok := strings.CutPrefix(line, "description: "); ok {
			return value
		}
	}
	return ""
}

// newHome points HOME at a fresh temporary directory, as every test of a command that reads
// quarry's store or config needs, and returns it.
func newHome(tb testing.TB) string {
	tb.Helper()
	home := tb.TempDir()
	tb.Setenv("HOME", home)
	return home
}

// runCapture runs the command line over testEnv, returning its exit code and what it wrote to
// stdout and stderr.
func runCapture(ctx context.Context, args []string) (int, *bytes.Buffer, *bytes.Buffer) {
	var stdout, stderr bytes.Buffer
	return run(ctx, args, &stdout, &stderr), &stdout, &stderr
}

// runSpendCapture is runCapture over spendEnv's clock.
func runSpendCapture(ctx context.Context, args []string) (int, *bytes.Buffer, *bytes.Buffer) {
	var stdout, stderr bytes.Buffer
	return runWith(ctx, args, spendEnv(&stdout, &stderr)), &stdout, &stderr
}

// runSpendCaptureAt is runCapture over spendEnvAt's clock set to now.
func runSpendCaptureAt(ctx context.Context, args []string, now time.Time) (int, *bytes.Buffer, *bytes.Buffer) {
	var stdout, stderr bytes.Buffer
	return runWith(ctx, args, spendEnvAt(&stdout, &stderr, now)), &stdout, &stderr
}
