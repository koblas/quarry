package snapshot_test

import (
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_manifest_encodes_empty_lists_as_arrays(t *testing.T) {
	m := snapshot.Manifest{
		Schema: snapshot.SchemaInfo{
			MissingTables: []string{}, MissingColumns: []snapshot.ColumnRef{},
			UnexpectedTables: []string{}, UnexpectedColumns: []snapshot.ColumnRef{},
		},
		Warnings: []string{},
	}

	got, err := m.Encode()

	require.NoError(t, err)
	body := string(got)
	assert.Contains(t, body, `"missing_tables": []`)
	assert.Contains(t, body, `"missing_columns": []`)
	assert.Contains(t, body, `"unexpected_tables": []`)
	assert.Contains(t, body, `"unexpected_columns": []`)
	assert.Contains(t, body, `"warnings": []`)
}

func Test_manifest_encodes_with_two_space_indent_and_a_trailing_newline(t *testing.T) {
	m := snapshot.Manifest{Warnings: []string{}}

	got, err := m.Encode()

	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(string(got), "}\n"))
	assert.Contains(t, string(got), "{\n  \"snapshot\"")
}

func Test_manifest_keys_appear_in_snapshot_schema_warnings_order(t *testing.T) {
	m := snapshot.Manifest{Warnings: []string{}}

	got, err := m.Encode()

	require.NoError(t, err)
	body := string(got)
	snapshotIdx := strings.Index(body, `"snapshot"`)
	schemaIdx := strings.Index(body, `"schema"`)
	warningsIdx := strings.Index(body, `"warnings"`)
	assert.True(t, snapshotIdx < schemaIdx && schemaIdx < warningsIdx)
}
