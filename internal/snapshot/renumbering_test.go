package snapshot_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/platform/sqlite"
	"github.com/koblas/quarry/internal/platform/sqlschema"
	v9 "github.com/koblas/quarry/internal/quicken/v9"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// renumberedBundle is a file in which Quicken inserted an entity at Z_ENT 2, shifting every later number by one.
func renumberedBundle(t *testing.T) v9fixture.Bundle {
	t.Helper()
	b := v9fixture.NewBuilder().WithInsertedEntity("AccountGroup", 2)
	b.Account(v9fixture.AccountRow{Name: "Checking", Active: true})
	return b.WriteBundle(t, t.TempDir())
}

// oldNumberingReferenceFingerprint is the reference fingerprint of a sync of a file that keeps the original entity numbering.
func oldNumberingReferenceFingerprint(t *testing.T) string {
	t.Helper()
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Checking", Active: true})
	srv := newImportServer(t, t.TempDir(), builtStore(), snapshot.WithEntityResolver(v9.ResolveName))

	outcome, err := srv.SyncAndImport(t.Context(), b.WriteBundle(t, t.TempDir()).Dir)

	require.NoError(t, err)
	return outcome.Manifest.Schema.ReferenceFingerprint
}

func Test_sync_and_import_verifies_a_file_whose_entities_were_renumbered(t *testing.T) {
	bundle := renumberedBundle(t)
	db, err := sqlite.OpenReadOnly(t.Context(), bundle.DataPath)
	require.NoError(t, err)
	schema, err := db.Schema(t.Context())
	require.NoError(t, err)
	require.NoError(t, db.Close())
	require.Contains(t, schema, "Z_16USERTAGS")
	oldFingerprint := oldNumberingReferenceFingerprint(t)
	importer := builtStore()
	srv := newImportServer(t, t.TempDir(), importer, snapshot.WithEntityResolver(v9.ResolveName))

	outcome, err := srv.SyncAndImport(t.Context(), bundle.Dir)

	require.NoError(t, err)
	info := outcome.Manifest.Schema
	assert.True(t, info.Verified)
	assert.Empty(t, info.MissingTables)
	assert.Empty(t, info.MissingColumns)
	assert.Empty(t, outcome.Manifest.Warnings)
	assert.Len(t, importer.calls, 1)
	assert.NotEqual(t, info.ReferenceFingerprint, info.Fingerprint)
	assert.Equal(t, oldFingerprint, info.ReferenceFingerprint)
}

const (
	accountDDL = "CREATE TABLE ZACCOUNT (Z_PK INTEGER PRIMARY KEY, ZNAME TEXT)"
	accountRow = "INSERT INTO ZACCOUNT (ZNAME) VALUES ('Checking')"
)

// syncCopyAgainst syncs a SQLite file holding one account plus ddl, comparing it with reference through resolve.
func syncCopyAgainst(t *testing.T, reference sqlschema.Schema, resolve snapshot.EntityResolver, ddl ...string) (snapshot.SchemaInfo, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "snap.sqlite")
	sqliteFileWith(t, path, append([]string{accountDDL, accountRow}, ddl...)...)
	home := t.TempDir()
	srv := snapshot.NewServer(
		snapshot.WithReference(v9.ReferenceLabel, reference),
		snapshot.WithEntityResolver(resolve),
		snapshot.WithSource(&fakeSource{}),
		snapshot.WithDestination(&fixedPathDestination{snapshotPath: path}),
		snapshot.WithHome(home),
	)

	manifest, err := srv.Sync(t.Context(), filepath.Join(home, "Documents", "Home.quicken"))
	return manifest.Schema, err
}

// renamer is an EntityResolver that renames the names in renames and reports every name in unresolved as unresolved.
func renamer(renames map[string]string, unresolved ...string) snapshot.EntityResolver {
	return func(name string, _ map[string][]int64) (string, bool) {
		if to, ok := renames[name]; ok {
			return to, true
		}
		return name, !slices.Contains(unresolved, name)
	}
}

func Test_sync_reads_a_file_without_z_primarykey_as_having_no_entity_numbers(t *testing.T) {
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Checking", Active: true})
	bundle := b.WriteBundle(t, t.TempDir())
	sqliteFileWith(t, bundle.DataPath, "DROP TABLE Z_PRIMARYKEY")
	srv := newImportServer(t, t.TempDir(), builtStore(), snapshot.WithEntityResolver(v9.ResolveName))

	outcome, err := srv.SyncAndImport(t.Context(), bundle.Dir)

	var mismatch snapshot.MismatchError
	require.ErrorAs(t, err, &mismatch)
	assert.Equal(t, []string{"Z_PRIMARYKEY"}, outcome.Manifest.Schema.MissingTables)
	assert.Empty(t, outcome.Manifest.Schema.MissingColumns)
}

func Test_sync_reads_the_entity_numbers_of_a_z_primarykey_row_with_no_name(t *testing.T) {
	bundle := renumberedBundle(t)
	sqliteFileWith(t, bundle.DataPath, "INSERT INTO Z_PRIMARYKEY (Z_ENT, Z_NAME) VALUES (200, NULL)")
	srv := newImportServer(t, t.TempDir(), builtStore(), snapshot.WithEntityResolver(v9.ResolveName))

	outcome, err := srv.SyncAndImport(t.Context(), bundle.Dir)

	require.NoError(t, err)
	assert.True(t, outcome.Manifest.Schema.Verified)
}

func Test_sync_keeps_every_z_ent_listed_for_a_duplicated_z_name(t *testing.T) {
	bundle := renumberedBundle(t)
	sqliteFileWith(t, bundle.DataPath, "INSERT INTO Z_PRIMARYKEY (Z_ENT, Z_NAME) VALUES (200, 'UserTag')")
	srv := newImportServer(t, t.TempDir(), builtStore(), snapshot.WithEntityResolver(v9.ResolveName))

	outcome, err := srv.SyncAndImport(t.Context(), bundle.Dir)

	require.ErrorAs(t, err, new(snapshot.MismatchError))
	info := outcome.Manifest.Schema
	assert.Empty(t, info.MissingTables)
	assert.Equal(t, []snapshot.ColumnRef{
		{Table: "Z_15USERTAGS", Column: "Z_76USERTAGS"},
		{Table: "Z_55USERTAGS", Column: "Z_76USERTAGS1"},
	}, info.MissingColumns)
	assert.Contains(t, info.UnexpectedColumns, snapshot.ColumnRef{Table: "Z_16USERTAGS", Column: "Z_77USERTAGS"})
}

func Test_sync_refuses_and_discards_a_snapshot_whose_z_primarykey_cannot_be_read(t *testing.T) {
	cases := []struct {
		name string
		ddl  []string
		want string
	}{
		{name: "no Z_NAME column", ddl: []string{"CREATE TABLE Z_PRIMARYKEY (Z_ENT INTEGER PRIMARY KEY)"}, want: "no such column: Z_NAME"},
		{name: "Z_ENT that is not a number", ddl: []string{
			"CREATE TABLE Z_PRIMARYKEY (Z_ENT TEXT, Z_NAME TEXT)", "INSERT INTO Z_PRIMARYKEY VALUES ('abc', 'Account')",
		}, want: "invalid syntax"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			b := v9fixture.NewBuilder()
			b.Account(v9fixture.AccountRow{Name: "Checking", Active: true})
			bundle := b.WriteBundle(t, home)
			sqliteFileWith(t, bundle.DataPath, append([]string{"DROP TABLE Z_PRIMARYKEY"}, c.ddl...)...)
			importer := builtStore()
			srv := newImportServer(t, home, importer, snapshot.WithEntityResolver(v9.ResolveName))

			_, err := srv.Sync(t.Context(), bundle.Dir)

			text := refusalText(t, err)
			assert.True(t, strings.HasPrefix(text, "cannot read the snapshot of ~/Home.quicken: "), text)
			assert.Contains(t, text, c.want)
			assert.True(t, strings.HasSuffix(text, "; nothing was kept; run quarry sync again"), text)
			assertSnapshotsDirEmpty(t, filepath.Join(home, "snapshots"))
		})
	}
}

func Test_import_from_refuses_a_snapshot_whose_z_primarykey_cannot_be_read(t *testing.T) {
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	writeSnapshotPair(t, filepath.Join(home, "snapshots"), idOldest,
		accountDDL, accountRow, "CREATE TABLE Z_PRIMARYKEY (Z_ENT INTEGER PRIMARY KEY)")

	_, err := srv.ImportFrom(t.Context(), idOldest)

	require.EqualError(t, err, "cannot read ~/snapshots/"+idOldest+".sqlite: no such column: Z_NAME; take a new snapshot with quarry sync")
	assert.Empty(t, fake.calls)
}

func Test_sync_requires_the_columns_of_every_reference_table_that_resolves_onto_one_name(t *testing.T) {
	reference := sqlschema.Schema{"ZA": {"ZX"}, "ZB": {"ZY"}}
	resolve := renamer(map[string]string{"ZA": "ZC", "ZB": "ZC"})

	info, err := syncCopyAgainst(t, reference, resolve, "CREATE TABLE ZC (ZOTHER TEXT)")

	require.ErrorAs(t, err, new(snapshot.MismatchError))
	assert.Equal(t, []snapshot.ColumnRef{{Table: "ZC", Column: "ZX"}, {Table: "ZC", Column: "ZY"}}, info.MissingColumns)
	assert.Equal(t, 2, info.ReferenceTables)
}

func Test_sync_names_a_missing_column_once_when_two_reference_columns_of_a_table_resolve_onto_it(t *testing.T) {
	reference := sqlschema.Schema{"ZT": {"ZX", "ZY"}}
	resolve := renamer(map[string]string{"ZX": "ZC", "ZY": "ZC"})

	info, err := syncCopyAgainst(t, reference, resolve, "CREATE TABLE ZT (ZOTHER TEXT)")

	require.ErrorAs(t, err, new(snapshot.MismatchError))
	assert.Equal(t, []snapshot.ColumnRef{{Table: "ZT", Column: "ZC"}}, info.MissingColumns)
}

func Test_sync_shows_the_first_reference_column_that_resolved_onto_a_missing_column(t *testing.T) {
	reference := sqlschema.Schema{"ZT": {"ZX", "ZY"}}
	resolve := func(name string, _ map[string][]int64) (string, bool) {
		if name == "ZT" {
			return name, true
		}
		return "ZC", name == "ZX"
	}

	info, err := syncCopyAgainst(t, reference, resolve, "CREATE TABLE ZT (ZOTHER TEXT)")

	require.ErrorAs(t, err, new(snapshot.MismatchError))
	assert.Equal(t, []snapshot.ColumnRef{{Table: "ZT", Column: "ZC"}}, info.MissingColumns)
}

func Test_sync_shows_a_missing_column_under_the_file_numbering_when_its_table_and_column_resolved(t *testing.T) {
	reference := sqlschema.Schema{"ZT": {"ZC"}}
	resolve := renamer(map[string]string{"ZT": "ZT5", "ZC": "ZC5"})

	info, err := syncCopyAgainst(t, reference, resolve, "CREATE TABLE ZT5 (ZOTHER TEXT)")

	require.ErrorAs(t, err, new(snapshot.MismatchError))
	assert.Equal(t, []snapshot.ColumnRef{{Table: "ZT5", Column: "ZC5"}}, info.MissingColumns)
}

func Test_sync_shows_a_missing_column_as_the_reference_writes_it_when_its_column_did_not_resolve(t *testing.T) {
	reference := sqlschema.Schema{"ZM": {"ZC1", "ZU"}}
	resolve := renamer(map[string]string{"ZM": "ZM9"}, "ZU")

	info, err := syncCopyAgainst(t, reference, resolve, "CREATE TABLE ZM9 (ZOTHER TEXT)")

	require.ErrorAs(t, err, new(snapshot.MismatchError))
	assert.Equal(t, []snapshot.ColumnRef{{Table: "ZM", Column: "ZU"}, {Table: "ZM9", Column: "ZC1"}}, info.MissingColumns)
	assert.Equal(t, []snapshot.ColumnRef{{Table: "ZM9", Column: "ZOTHER"}}, info.UnexpectedColumns)
}

func Test_sync_shows_a_missing_column_as_the_reference_writes_it_when_its_table_did_not_resolve(t *testing.T) {
	reference := sqlschema.Schema{"ZN": {"ZC"}}
	resolve := renamer(map[string]string{"ZC": "ZC5"}, "ZN")

	info, err := syncCopyAgainst(t, reference, resolve, "CREATE TABLE ZN (ZOTHER TEXT)")

	require.ErrorAs(t, err, new(snapshot.MismatchError))
	assert.Equal(t, []snapshot.ColumnRef{{Table: "ZN", Column: "ZC"}}, info.MissingColumns)
}
