package snapshot_test

import (
	"testing"

	"github.com/koblas/quarry/internal/platform/sqlite"
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

func Test_sync_and_import_verifies_a_file_whose_entities_were_renumbered(t *testing.T) {
	bundle := renumberedBundle(t)
	db, err := sqlite.OpenReadOnly(t.Context(), bundle.DataPath)
	require.NoError(t, err)
	schema, err := db.Schema(t.Context())
	require.NoError(t, err)
	require.NoError(t, db.Close())
	require.Contains(t, schema, "Z_16USERTAGS")
	oldBuilder := v9fixture.NewBuilder()
	oldBuilder.Account(v9fixture.AccountRow{Name: "Checking", Active: true})
	importer := builtStore()
	srv := newImportServer(t, t.TempDir(), importer, snapshot.WithEntityResolver(v9.ResolveName))
	oldSrv := newImportServer(t, t.TempDir(), builtStore(), snapshot.WithEntityResolver(v9.ResolveName))

	outcome, err := srv.SyncAndImport(t.Context(), bundle.Dir)
	oldOutcome, oldErr := oldSrv.SyncAndImport(t.Context(), oldBuilder.WriteBundle(t, t.TempDir()).Dir)

	require.NoError(t, err)
	require.NoError(t, oldErr)
	info := outcome.Manifest.Schema
	assert.True(t, info.Verified)
	assert.Empty(t, info.MissingTables)
	assert.Empty(t, info.MissingColumns)
	assert.Empty(t, outcome.Manifest.Warnings)
	assert.Len(t, importer.calls, 1)
	assert.NotEqual(t, info.ReferenceFingerprint, info.Fingerprint)
	assert.Equal(t, oldOutcome.Manifest.Schema.ReferenceFingerprint, info.ReferenceFingerprint)
}
