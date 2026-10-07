package cli_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/platform/lockfile"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const pruneHeldLine = "another quarry sync or quarry snapshots prune is running, so this prune deleted nothing; " +
	"run the command again once that one finishes"

var errNoLocks = errors.New("no locks available")

// eventLog records, in order, what the lock and the delete seam were asked to do.
type eventLog struct{ events []string }

func (l *eventLog) add(event string) { l.events = append(l.events, event) }

// logLocker logs acquire and release into log; Acquire fails with err when it is set.
type logLocker struct {
	log *eventLog
	err error
}

func (l logLocker) Acquire(context.Context) (func(), error) {
	l.log.add("acquire")
	if l.err != nil {
		return nil, l.err
	}
	return func() { l.log.add("release") }, nil
}

// pruneRun is what one snapshots prune command run through Execute produced.
type pruneRun struct {
	err    error
	stdout string
	log    *eventLog
}

// runPrune runs `quarry <args>` over a snapshots folder holding three snapshots, with locker
// wired into the Server and every delete logged but not performed. snapshotDir "" means that folder.
func runPrune(t *testing.T, locker logLocker, loadConfig cli.ConfigLoader, snapshotDir string, args ...string) pruneRun {
	t.Helper()
	home := t.TempDir()
	if snapshotDir == "" {
		snapshotDir = filepath.Join(home, "snapshots")
		require.NoError(t, os.Mkdir(snapshotDir, 0o700))
		for _, id := range []string{"20260101T000000Z", "20260201T000000Z", "20260301T000000Z"} {
			require.NoError(t, os.WriteFile(filepath.Join(snapshotDir, id+".sqlite"), []byte("snapshot"), 0o600))
		}
	}
	if loadConfig == nil {
		loadConfig = func(string) (config.Config, error) { return config.Config{Keep: 1}, nil }
	}
	var stdout, stderr bytes.Buffer
	env := cli.Env{
		Stdout:     &stdout,
		Stderr:     &stderr,
		LoadConfig: loadConfig,
		NewSnapshots: func(context.Context, string) (*snapshot.Server, error) {
			return snapshot.NewServer(
				snapshot.WithHome(home),
				snapshot.WithSnapshotDir(snapshotDir),
				snapshot.WithLocker(locker),
				snapshot.WithRemove(func(string) error { locker.log.add("remove"); return nil }),
			), nil
		},
	}

	err := cli.Execute(t.Context(), args, env)

	return pruneRun{err: err, stdout: stdout.String(), log: locker.log}
}

func Test_prune_takes_the_lock_before_it_deletes(t *testing.T) {
	locker := logLocker{log: &eventLog{}}

	got := runPrune(t, locker, nil, "", "snapshots", "prune")

	require.NoError(t, got.err)
	assert.Equal(t, []string{"acquire", "remove", "remove", "release"}, got.log.events)
}

func Test_prune_refused_by_a_held_lock_deletes_nothing_and_prints_nothing(t *testing.T) {
	cells := []struct {
		name string
		args []string
	}{
		{name: "text", args: []string{"snapshots", "prune"}},
		{name: "json", args: []string{"--json", "snapshots", "prune"}},
	}

	for _, c := range cells {
		t.Run(c.name, func(t *testing.T) {
			locker := logLocker{log: &eventLog{}, err: &lockfile.Error{Kind: lockfile.KindHeld}}

			got := runPrune(t, locker, nil, "", c.args...)

			refusal, ok := errors.AsType[snapshot.RefusalError](got.err)
			require.True(t, ok, "want a RefusalError, got %v", got.err)
			assert.Equal(t, pruneHeldLine, refusal.Error())
			assert.Empty(t, got.stdout)
			assert.Equal(t, []string{"acquire"}, got.log.events)
		})
	}
}

func Test_prune_returns_a_lock_failure_that_is_not_a_held_lock_with_nothing_on_stdout(t *testing.T) {
	locker := logLocker{log: &eventLog{}, err: errNoLocks}

	got := runPrune(t, locker, nil, "", "snapshots", "prune")

	require.ErrorIs(t, got.err, errNoLocks)
	assert.Empty(t, got.stdout)
	assert.Equal(t, []string{"acquire"}, got.log.events)
}

func Test_prune_dry_run_takes_no_lock(t *testing.T) {
	locker := logLocker{log: &eventLog{}}

	got := runPrune(t, locker, nil, "", "snapshots", "prune", "--dry-run")

	require.NoError(t, got.err)
	assert.NotEmpty(t, got.stdout)
	assert.Empty(t, got.log.events)
}

func Test_prune_releases_the_lock_when_it_returns(t *testing.T) {
	locker := logLocker{log: &eventLog{}}

	got := runPrune(t, locker, nil, "", "snapshots", "prune")

	require.NoError(t, got.err)
	assert.Equal(t, 1, countOf(got.log.events, "release"))
}

func Test_prune_releases_the_lock_when_it_refuses_after_taking_it(t *testing.T) {
	notAFolder := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(notAFolder, []byte("not a folder"), 0o600))
	locker := logLocker{log: &eventLog{}}

	got := runPrune(t, locker, nil, notAFolder, "snapshots", "prune")

	require.Error(t, got.err)
	assert.Equal(t, []string{"acquire", "release"}, got.log.events)
}

func Test_prune_refused_before_the_lock_never_takes_it(t *testing.T) {
	cells := []struct {
		name       string
		loadConfig cli.ConfigLoader
		args       []string
	}{
		{name: "a keep below one", args: []string{"snapshots", "prune", "--keep", "0"}},
		{
			name:       "an unreadable config",
			loadConfig: func(string) (config.Config, error) { return config.Config{}, errNoConfig },
			args:       []string{"snapshots", "prune"},
		},
	}

	for _, c := range cells {
		t.Run(c.name, func(t *testing.T) {
			locker := logLocker{log: &eventLog{}}

			got := runPrune(t, locker, c.loadConfig, "", c.args...)

			require.Error(t, got.err)
			assert.Empty(t, got.log.events)
		})
	}
}

var errNoConfig = errors.New("config unreadable")

func countOf(events []string, event string) int {
	n := 0
	for _, e := range events {
		if e == event {
			n++
		}
	}
	return n
}
