package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// execOn runs each statement against the SQLite file at path through a connection of its own.
func execOn(t *testing.T, path string, statements ...string) {
	t.Helper()
	conn, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	for _, statement := range statements {
		_, err := conn.ExecContext(t.Context(), statement)
		require.NoError(t, err, statement)
	}
}

// schemaOfSnapshot re-compares the one snapshot under home with `sync --from --json` and returns the
// document's schema object, whatever the exit code.
func schemaOfSnapshot(t *testing.T, home string) snapshot.SchemaInfo {
	t.Helper()
	art := readSyncArtifacts(t, home)

	_, stdout, _ := runCapture(context.Background(), []string{"sync", "--from", snapshotID(art.snapshotPath), "--json"})

	var doc struct {
		Schema snapshot.SchemaInfo `json:"schema"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	return doc.Schema
}

// refusedSchemaBlock is the stdout of a sync the schema check refused: the snapshot head, the Schema line and the diff rows.
func refusedSchemaBlock(t *testing.T, home, bundleDir, schemaLine, rows string) string {
	t.Helper()
	art := readSyncArtifacts(t, home)
	return art.head(t, home, bundleDir, "1 account") + fmt.Sprintf("%-10s%s\n", "Schema", schemaLine) + rows
}

// schemaRefusal is the stderr line of a sync the schema check refused, naming what is missing, such as "1 table".
func schemaRefusal(t *testing.T, home, missing string) string {
	t.Helper()
	art := readSyncArtifacts(t, home)
	return "quarry: schema check failed: Home.quicken is missing " + missing + " that the schema reference expects; " +
		"the snapshot is kept at " + abbreviated(t, art.snapshotPath, home) +
		" and the diff is in its .json manifest; quarry cannot import this file until its schema reference is updated\n"
}

// Needs the whole map in Z_PRIMARYKEY: OpenBundle seeds only three entities.
func Test_run_verifies_a_file_with_the_original_entity_numbering(t *testing.T) {
	home := newHome(t)
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Checking", Type: "CHECKING", Currency: "CAD", Active: true})
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir})

	require.Equal(t, 0, exitCode)
	assert.Empty(t, stderr.String())
	art := readSyncArtifacts(t, home)
	want := art.head(t, home, bundle.Dir, "1 account") + fmt.Sprintf(
		"%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n%-10s%s\n",
		"Schema", referenceMatch,
		"Store", abbreviated(t, art.storePath, home),
		"Rows", "0 transactions, 0 splits, 0 transfers, 0 payees, 0 categories, 0 tags; 0 investment transactions, 0 securities, 0 prices",
		"Balances", "no accounts to check; 1 never reconciled",
		"Splits", "no transactions to check",
		"Shares", "no holdings to check",
		"Transfers", "none",
		"Findings", "none open",
		"Rates", fakeRatesText,
	)
	assert.Equal(t, want, stdout.String())
	schema := schemaOfSnapshot(t, home)
	assert.Equal(t, referenceLabel, schema.Reference)
	assert.True(t, schema.Verified)
	assert.Empty(t, schema.MissingTables)
	assert.Empty(t, schema.MissingColumns)
	assert.Empty(t, schema.UnexpectedTables)
	assert.Empty(t, schema.UnexpectedColumns)
}

// Drops the column before the sync: SQLite refuses to drop an indexed one, which the Z79_ column is not.
func Test_run_names_a_missing_column_of_a_renumbered_file_under_its_own_numbering(t *testing.T) {
	home := newHome(t)
	b := v9fixture.NewBuilder().WithInsertedEntity("AccountGroup", 2)
	b.Account(v9fixture.AccountRow{Name: "Checking", Active: true})
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	execOn(t, bundle.DataPath, "ALTER TABLE ZATTACHMENT DROP COLUMN Z79_TRANSACTION")

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir})

	require.Equal(t, 1, exitCode)
	assert.Equal(t, refusedSchemaBlock(t, home, bundle.Dir,
		"DIFFERS from reference "+referenceLabel+": 1 column missing",
		"  - column  ZATTACHMENT.Z79_TRANSACTION\n"), stdout.String())
	assert.Equal(t, schemaRefusal(t, home, "1 column"), stderr.String())
	schema := schemaOfSnapshot(t, home)
	assert.False(t, schema.Verified)
	assert.Empty(t, schema.MissingTables)
	assert.Equal(t, []snapshot.ColumnRef{{Table: "ZATTACHMENT", Column: "Z79_TRANSACTION"}}, schema.MissingColumns)
	assert.Empty(t, schema.UnexpectedTables)
	assert.Empty(t, schema.UnexpectedColumns)
}

func Test_run_refuses_a_join_table_whose_number_belongs_to_another_entity(t *testing.T) {
	home := newHome(t)
	b := v9fixture.NewBuilder().WithEntity("CashFlowTransactionEntry", 16)
	b.Account(v9fixture.AccountRow{Name: "Checking", Active: true})
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir})

	require.Equal(t, 1, exitCode)
	assert.Equal(t, refusedSchemaBlock(t, home, bundle.Dir,
		"DIFFERS from reference "+referenceLabel+": 1 table missing, 1 table not in reference",
		"  - table   Z_16USERTAGS\n  + table   Z_15USERTAGS\n"), stdout.String())
	assert.Equal(t, schemaRefusal(t, home, "1 table"), stderr.String())
	schema := schemaOfSnapshot(t, home)
	assert.False(t, schema.Verified)
	assert.Equal(t, []string{"Z_16USERTAGS"}, schema.MissingTables)
	assert.Empty(t, schema.MissingColumns)
	assert.Equal(t, []string{"Z_15USERTAGS"}, schema.UnexpectedTables)
	assert.Empty(t, schema.UnexpectedColumns)
}

func Test_run_names_items_of_an_entity_absent_from_z_primarykey_as_the_reference_writes_them(t *testing.T) {
	home := newHome(t)
	b := v9fixture.NewBuilder().WithInsertedEntity("AccountGroup", 2).WithoutEntity("UserTag")
	b.Account(v9fixture.AccountRow{Name: "Checking", Active: true})
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir})

	require.Equal(t, 1, exitCode)
	assert.Equal(t, refusedSchemaBlock(t, home, bundle.Dir,
		"DIFFERS from reference "+referenceLabel+": 2 columns missing, 2 columns not in reference",
		"  - column  Z_15USERTAGS.Z_76USERTAGS\n  - column  Z_55USERTAGS.Z_76USERTAGS1\n"+
			"  + column  Z_16USERTAGS.Z_77USERTAGS\n  + column  Z_56USERTAGS.Z_77USERTAGS1\n"), stdout.String())
	assert.Equal(t, schemaRefusal(t, home, "2 columns"), stderr.String())
	schema := schemaOfSnapshot(t, home)
	assert.False(t, schema.Verified)
	assert.Empty(t, schema.MissingTables)
	assert.Equal(t, []snapshot.ColumnRef{
		{Table: "Z_15USERTAGS", Column: "Z_76USERTAGS"},
		{Table: "Z_55USERTAGS", Column: "Z_76USERTAGS1"},
	}, schema.MissingColumns)
	assert.Empty(t, schema.UnexpectedTables)
	assert.Equal(t, []snapshot.ColumnRef{
		{Table: "Z_16USERTAGS", Column: "Z_77USERTAGS"},
		{Table: "Z_56USERTAGS", Column: "Z_77USERTAGS1"},
	}, schema.UnexpectedColumns)
}

// renumberedTaggedSplit writes a bundle under home whose entities Quicken renumbered, holding one split linked to
// one tag, and returns the bundle, the split's Z_PK and the tag's.
func renumberedTaggedSplit(t *testing.T, home string) (v9fixture.Bundle, int64, int64) {
	t.Helper()
	b := v9fixture.NewBuilder().WithInsertedEntity("AccountGroup", 2)
	accountPK := b.Account(v9fixture.AccountRow{Name: "Checking", Type: "CHECKING", Currency: "CAD", Active: true})
	categoryPK := b.Category(v9fixture.TagRow{Name: "Food", Type: new(int64(1))})
	tagPK := b.UserTag(v9fixture.TagRow{Name: "Reimbursable"})
	posted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	txnPK := b.Transaction(v9fixture.TransactionRow{Account: accountPK, Amount: "7.00", PostedDate: &posted})
	splitPK := b.Entry(v9fixture.EntryRow{Parent: txnPK, Amount: "7.00", CategoryTag: categoryPK})
	b.LinkUserTag(splitPK, tagPK)
	return b.WriteBundle(t, filepath.Join(home, "Documents")), splitPK, tagPK
}

func Test_run_imports_the_split_tags_of_a_file_whose_entities_were_renumbered(t *testing.T) {
	home := newHome(t)
	bundle, splitPK, tagPK := renumberedTaggedSplit(t, home)

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--quicken", bundle.Dir})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), fmt.Sprintf("%-10s%s\n", "Schema", referenceMatch))
	assert.Equal(t, map[string]string{
		fmt.Sprintf("split-%d", splitPK): fmt.Sprintf("tag-%d", tagPK),
	}, stringMap(t, openStoreReadOnly(t, home), "SELECT split_id, tag_id FROM split_tags"))
}

func Test_run_sync_from_imports_a_snapshot_whose_manifest_recorded_a_refusal_under_the_old_label(t *testing.T) {
	home := newHome(t)
	bundle, splitPK, tagPK := renumberedTaggedSplit(t, home)
	syncBundle(t, bundle)
	art := readSyncArtifacts(t, home)
	raw, err := os.ReadFile(art.manifestPath)
	require.NoError(t, err)
	var manifest snapshot.Manifest
	require.NoError(t, json.Unmarshal(raw, &manifest))
	manifest.Schema = snapshot.SchemaInfo{
		Reference: "hardkoded/quicken-skills@752107b+quarry.1", MissingTables: []string{"Z_16USERTAGS"},
		UnexpectedTables: []string{"Z_15USERTAGS"},
	}
	refused, err := manifest.Encode()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(art.manifestPath, refused, 0o600))
	require.NoError(t, os.Remove(art.storePath))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sync", "--from", snapshotID(art.snapshotPath)})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), fmt.Sprintf("%-10s%s\n", "Schema", referenceMatch))
	assert.Equal(t, map[string]string{
		fmt.Sprintf("split-%d", splitPK): fmt.Sprintf("tag-%d", tagPK),
	}, stringMap(t, openStoreReadOnly(t, home), "SELECT split_id, tag_id FROM split_tags"))
}
