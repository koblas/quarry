// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_status_json_describes_the_store_sync_built(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundle := writeStatusFixtureBundle(t, home)
	syncBundle(t, bundle)
	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	snapshotPath := onlyFileWithSuffix(t, snapshotsDir, ".sqlite")
	raw, err := os.ReadFile(onlyFileWithSuffix(t, snapshotsDir, ".json"))
	require.NoError(t, err)
	var manifest struct {
		Snapshot struct {
			TakenAt string `json:"taken_at"`
			SHA256  string `json:"sha256"`
		} `json:"snapshot"`
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"status", "--json"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	var unfixed struct {
		Store struct {
			BuiltAt string `json:"built_at"`
		} `json:"store"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &unfixed))
	builtAt, err := time.Parse(time.RFC3339, unfixed.Store.BuiltAt)
	require.NoError(t, err)
	assert.Equal(t, time.UTC, builtAt.Location())
	assert.Equal(t, unfixed.Store.BuiltAt, builtAt.UTC().Format(time.RFC3339))
	want := fmt.Sprintf(`{
  "store": {
    "path": %q,
    "format_version": %d,
    "quarry_version": %q,
    "built_at": %q,
    "rows": {
      "accounts": 3,
      "categories": 0,
      "payees": 0,
      "tags": 0,
      "transactions": 4,
      "splits": 4,
      "split_tags": 0,
      "transfers": 2
    }
  },
  "snapshot": {
    "id": %q,
    "path": %q,
    "taken_at": %q,
    "source": %q,
    "sha256": %q
  },
  "dates": {
    "first": "2026-01-05",
    "last": "2026-03-20"
  },
  "balances": {
    "checked": 1,
    "never_reconciled": 1,
    "investment_accounts": 1
  },
  "splits": {
    "checked": 4
  },
  "transfers": {
    "paired": 1,
    "cross_currency": 1,
    "one_sided": 1
  },
  "not_imported": {
    "investment_transactions": 0
  },
  "warnings": []
}
`,
		storePathUnder(home), duckstore.FormatVersion, "(devel)", unfixed.Store.BuiltAt,
		snapshotID(snapshotPath), snapshotPath, manifest.Snapshot.TakenAt, bundle.Dir, manifest.Snapshot.SHA256)
	assert.Equal(t, want, stdout.String())
}

func Test_run_status_json_reports_the_latest_build_when_import_runs_holds_several(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundle := writeStatusFixtureBundle(t, home)
	syncBundle(t, bundle)
	snapshotsDir := filepath.Join(storeDirUnder(home), "snapshots")
	earlierPath := onlyFileWithSuffix(t, snapshotsDir, ".sqlite")
	laterPath := filepath.Join(snapshotsDir, "later-build.sqlite")
	var before bytes.Buffer
	require.Equal(t, 0, run(context.Background(), []string{"status", "--json"}, &before, &bytes.Buffer{}))
	editStore(t, home, "INSERT INTO import_runs SELECT * REPLACE (2 AS id) FROM import_runs") //nolint:unqueryvet // a copy of the row is the point
	editStore(t, home, "UPDATE import_runs SET snapshot_path = '"+laterPath+"' WHERE id = 2")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"status", "--json"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	wantJSON := strings.ReplaceAll(before.String(), earlierPath, laterPath)
	wantJSON = strings.ReplaceAll(wantJSON, snapshotID(earlierPath), "later-build")
	assert.Equal(t, wantJSON, stdout.String()) //nolint:testifylint // the bytes are the contract
}
