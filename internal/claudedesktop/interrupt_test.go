package claudedesktop_test

import (
	"context"
	"testing"

	"github.com/koblas/quarry/internal/claudedesktop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	ourEntry     = `{"command":"/opt/homebrew/bin/quarry","args":["mcp"]}`
	foreignEntry = `{"command":"/usr/bin/other","args":[]}`
	otherServers = `{"globalShortcut":"Cmd+Shift+Space"}`
)

// cancelledContext returns a context that has already ended with context.Canceled.
func cancelledContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	return ctx
}

func installUnder(ctx context.Context, t *testing.T, home string) (claudedesktop.Result, error) {
	t.Helper()
	srv := claudedesktop.NewServer(claudedesktop.WithHome(home), claudedesktop.WithExecutable((&fakeExecutable{path: quarryBinary}).executable))
	return srv.Install(ctx)
}

func uninstallUnder(ctx context.Context, t *testing.T, home string) (claudedesktop.UninstallResult, error) {
	t.Helper()
	return claudedesktop.NewServer(claudedesktop.WithHome(home)).Uninstall(ctx)
}

func Test_install_stops_before_writing_when_the_context_is_cancelled(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		setup func(t *testing.T, folder string)
	}{
		{name: "no config to create", setup: func(*testing.T, string) {}},
		{name: "a config to back up and replace", setup: func(t *testing.T, folder string) {
			t.Helper()
			writeConfig(t, folder, otherServers, 0o644)
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home, folder := desktopFolder(t)
			c.setup(t, folder)
			before := snapshot(t, folder)

			_, err := installUnder(cancelledContext(t), t, home)

			require.ErrorIs(t, err, claudedesktop.ErrInterrupted)
			require.ErrorIs(t, err, context.Canceled)
			assert.Equal(t, before, snapshot(t, folder))
		})
	}
}

func Test_install_writes_the_entry_when_the_context_is_live(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	writeConfig(t, folder, otherServers, 0o644)
	before := snapshot(t, folder)

	_, err := installUnder(t.Context(), t, home)

	require.NoError(t, err)
	assert.NotEqual(t, before, snapshot(t, folder))
}

func Test_install_with_a_cancelled_context_leaves_an_entry_of_ours_unchanged_without_an_error(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	writeConfig(t, folder, quarryEntryConfig(ourEntry), 0o644)
	before := snapshot(t, folder)

	res, err := installUnder(cancelledContext(t), t, home)

	require.NoError(t, err)
	assert.Equal(t, claudedesktop.Unchanged, res.Outcome)
	assert.Equal(t, before, snapshot(t, folder))
}

func Test_install_with_a_cancelled_context_still_refuses_a_foreign_entry(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	writeConfig(t, folder, quarryEntryConfig(foreignEntry), 0o644)

	_, err := installUnder(cancelledContext(t), t, home)

	var foreign *claudedesktop.ForeignEntryError
	require.ErrorAs(t, err, &foreign)
	assert.NotErrorIs(t, err, claudedesktop.ErrInterrupted)
}

func Test_uninstall_stops_before_removing_when_the_context_is_cancelled(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	writeConfig(t, folder, quarryEntryConfig(ourEntry), 0o644)
	before := snapshot(t, folder)

	res, err := uninstallUnder(cancelledContext(t), t, home)

	require.ErrorIs(t, err, claudedesktop.ErrInterrupted)
	require.ErrorIs(t, err, context.Canceled)
	assert.False(t, res.Removed)
	assert.Equal(t, before, snapshot(t, folder))
}

func Test_uninstall_removes_the_entry_when_the_context_is_live(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	writeConfig(t, folder, quarryEntryConfig(ourEntry), 0o644)

	res, err := uninstallUnder(t.Context(), t, home)

	require.NoError(t, err)
	assert.True(t, res.Removed)
}

func Test_uninstall_with_a_cancelled_context_finds_nothing_to_remove_without_an_error(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	writeConfig(t, folder, otherServers, 0o644)
	before := snapshot(t, folder)

	res, err := uninstallUnder(cancelledContext(t), t, home)

	require.NoError(t, err)
	assert.False(t, res.Removed)
	assert.Equal(t, before, snapshot(t, folder))
}

func Test_uninstall_with_a_cancelled_context_still_refuses_a_foreign_entry(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	writeConfig(t, folder, quarryEntryConfig(foreignEntry), 0o644)

	_, err := uninstallUnder(cancelledContext(t), t, home)

	var foreign *claudedesktop.ForeignEntryError
	require.ErrorAs(t, err, &foreign)
	assert.NotErrorIs(t, err, claudedesktop.ErrInterrupted)
}
