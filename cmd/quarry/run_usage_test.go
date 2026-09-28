// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_sync_help_names_both_documents_folders(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--help"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode)
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), "Without --quicken, quarry looks for .quicken files in ~/Documents and in\n"+
		"~/Library/Application Support/Quicken/Documents, and uses the one it finds\n"+
		"if there is exactly one.")
	assert.Contains(t, stdout.String(),
		"path to the .quicken file to snapshot (default: the only one in ~/Documents or Quicken's Documents folder)")
}

func Test_run_refuses_an_empty_or_whitespace_quicken_flag_as_a_usage_error_even_with_a_bundle_in_documents(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "empty", args: []string{"sync", "--quicken="}},
		{name: "all whitespace", args: []string{"sync", "--quicken=   "}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), c.args, &stdout, &stderr)

			assert.Equal(t, 2, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, "quarry: flag needs an argument: --quicken; Run 'quarry sync --help' for usage.\n", stderr.String())
			_, statErr := os.Stat(filepath.Join(home, "Library", "Application Support", "quarry", "snapshots"))
			assert.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}
}

func Test_run_rejects_usage_errors(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{
			name:       "positional argument",
			args:       []string{"sync", "~/x.quicken"},
			wantStderr: "quarry: sync takes no arguments; pass the file with --quicken <path>\n",
		},
		{
			name:       "unknown flag",
			args:       []string{"sync", "--bogus"},
			wantStderr: "quarry: unknown flag: --bogus; Run 'quarry sync --help' for usage.\n",
		},
		{
			name:       "flag missing its value",
			args:       []string{"sync", "--quicken"},
			wantStderr: "quarry: flag needs an argument: --quicken; Run 'quarry sync --help' for usage.\n",
		},
		{
			name:       "unknown command",
			args:       []string{"frob"},
			wantStderr: "quarry: unknown command \"frob\" for \"quarry\"; Run 'quarry sync --help' for usage.\n",
		},
		{
			name:       "near miss of a known command",
			args:       []string{"synk"},
			wantStderr: "quarry: unknown command \"synk\" for \"quarry\"; Run 'quarry sync --help' for usage.\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), c.args, &stdout, &stderr)

			require.Equal(t, 2, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
			assert.NotContains(t, stderr.String(), "Did you mean")
		})
	}
}

// Home resolution happens only inside sync's RunE, on a valid sync
// invocation with HOME unset.
func Test_run_reports_exit_1_when_home_directory_cannot_be_resolved(t *testing.T) {
	t.Setenv("HOME", "")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync"}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry sync again\n",
		stderr.String())
}

func Test_run_help_and_usage_errors_do_not_need_home(t *testing.T) {
	t.Setenv("HOME", "")

	t.Run("root help", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		exitCode := run(context.Background(), []string{"--help"}, &stdout, &stderr)

		assert.Equal(t, 0, exitCode)
		assert.NotEmpty(t, stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("sync help", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		exitCode := run(context.Background(), []string{"sync", "--help"}, &stdout, &stderr)

		assert.Equal(t, 0, exitCode)
		assert.NotEmpty(t, stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("unknown command is still a usage error, not the home-directory refusal", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		exitCode := run(context.Background(), []string{"frob"}, &stdout, &stderr)

		assert.Equal(t, 2, exitCode)
		assert.Empty(t, stdout.String())
		assert.Equal(t, "quarry: unknown command \"frob\" for \"quarry\"; Run 'quarry sync --help' for usage.\n", stderr.String())
	})
}

func Test_run_reports_exit_1_when_the_context_is_already_cancelled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout, stderr bytes.Buffer

	exitCode := run(ctx, []string{"sync", "--quicken", filepath.Join(home, "Any.quicken")}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: sync interrupted; nothing was kept; run quarry sync again\n", stderr.String())
}
