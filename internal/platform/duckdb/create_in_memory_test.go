package duckdb_test

import (
	"context"
	"testing"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_create_in_memory_gives_a_database_that_stores_and_reads_back_rows(t *testing.T) {
	t.Parallel()
	db, err := duckdb.CreateInMemory(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(t.Context(), "CREATE TABLE t (id INTEGER)")
	require.NoError(t, err)
	require.NoError(t, db.AppendRows(t.Context(), "t", [][]any{{int32(7)}, {int32(8)}}))

	var got []int32
	err = db.QueryRows(t.Context(), "SELECT id FROM t ORDER BY id", nil, func(scan func(dest ...any) error) error {
		var id int32
		if err := scan(&id); err != nil {
			return err
		}
		got = append(got, id)
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, []int32{7, 8}, got)
}

func Test_create_in_memory_gives_each_call_a_database_of_its_own(t *testing.T) {
	t.Parallel()
	first, err := duckdb.CreateInMemory(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = first.Close() })
	second, err := duckdb.CreateInMemory(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = second.Close() })
	_, err = first.Exec(t.Context(), "CREATE TABLE only_in_first (id INTEGER)")
	require.NoError(t, err)

	_, err = second.Exec(t.Context(), "INSERT INTO only_in_first VALUES (1)")

	require.Error(t, err)
}

func Test_create_in_memory_never_fetches_an_extension_or_reaches_outside(t *testing.T) {
	t.Parallel()
	db, err := duckdb.CreateInMemory(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	cases := []struct {
		setting string
		want    string
	}{
		{setting: "autoload_known_extensions", want: "false"},
		{setting: "autoinstall_known_extensions", want: "false"},
		{setting: "enable_external_access", want: "false"},
	}

	for _, c := range cases {
		t.Run(c.setting, func(t *testing.T) {
			t.Parallel()

			table, err := db.QueryTable(t.Context(), "SELECT current_setting('"+c.setting+"')::VARCHAR", 0)

			require.NoError(t, err)
			require.Len(t, table.Rows, 1)
			assert.Equal(t, c.want, table.Rows[0][0].Text)
		})
	}
}

func Test_create_in_memory_fails_when_the_context_is_already_cancelled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := duckdb.CreateInMemory(ctx)

	require.ErrorIs(t, err, context.Canceled)
}
