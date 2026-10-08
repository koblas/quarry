package claudedesktop_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/claudedesktop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errNoQuarryOnPath = errors.New("quarry not found in PATH")

// writeQuarry creates an executable file named quarry in dir and returns its path.
func writeQuarry(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "quarry")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755)) //nolint:gosec // an executable stand-in
	return path
}

// installWithPath runs Install with the running binary at exe, PATH resolving quarry through
// lookPath, and temp as the temporary directory root.
func installWithPath(t *testing.T, home, exe, temp string, lookPath claudedesktop.LookPath) (claudedesktop.Result, error) {
	t.Helper()
	srv := claudedesktop.NewServer(
		claudedesktop.WithHome(home),
		claudedesktop.WithExecutable((&fakeExecutable{path: exe}).executable),
		claudedesktop.WithLookPath(lookPath),
		claudedesktop.WithTempDir(temp),
	)
	return srv.Install(t.Context())
}

func lookPathReturning(path string) claudedesktop.LookPath {
	return func(string) (string, error) { return path, nil }
}

func Test_install_writes_the_quarry_path_that_claude_desktop_will_start(t *testing.T) {
	t.Parallel()

	t.Run("the PATH link to the running binary is written, with no PATH warning", func(t *testing.T) {
		t.Parallel()
		home, folder := desktopFolder(t)
		cellar := writeQuarry(t, t.TempDir())
		pathLink := filepath.Join(t.TempDir(), "quarry")
		require.NoError(t, os.Symlink(cellar, pathLink))

		res, err := installWithPath(t, home, cellar, t.TempDir(), lookPathReturning(pathLink))

		require.NoError(t, err)
		assert.Equal(t, claudedesktop.Result{Folder: folder, Config: filepath.Join(folder, configName), Command: pathLink}, res)
		assert.JSONEq(t, quarryEntryConfig(`{"command":"`+pathLink+`","args":["mcp"]}`), readConfig(t, folder))
	})

	t.Run("the running binary is written and the other PATH file is reported when they differ", func(t *testing.T) {
		t.Parallel()
		home, folder := desktopFolder(t)
		running := writeQuarry(t, t.TempDir())
		onPath := writeQuarry(t, t.TempDir())

		res, err := installWithPath(t, home, running, t.TempDir(), lookPathReturning(onPath))

		require.NoError(t, err)
		assert.Equal(t, claudedesktop.Result{Folder: folder, Config: filepath.Join(folder, configName), Command: running, PathQuarry: onPath}, res)
		assert.JSONEq(t, quarryEntryConfig(`{"command":"`+running+`","args":["mcp"]}`), readConfig(t, folder))
	})

	t.Run("the running binary is written with no PATH warning when PATH holds no quarry", func(t *testing.T) {
		t.Parallel()
		home, folder := desktopFolder(t)
		running := writeQuarry(t, t.TempDir())
		lookPath := func(string) (string, error) { return "", errNoQuarryOnPath }

		res, err := installWithPath(t, home, running, t.TempDir(), lookPath)

		require.NoError(t, err)
		assert.Equal(t, claudedesktop.Result{Folder: folder, Config: filepath.Join(folder, configName), Command: running}, res)
		assert.JSONEq(t, quarryEntryConfig(`{"command":"`+running+`","args":["mcp"]}`), readConfig(t, folder))
	})

	t.Run("a binary under the temporary directory is refused and nothing is written", func(t *testing.T) {
		t.Parallel()
		home, folder := desktopFolder(t)
		temp := t.TempDir()
		built := writeQuarry(t, temp)

		_, err := installWithPath(t, home, built, temp, lookPathReturning(""))

		tempErr, ok := errors.AsType[*claudedesktop.TempBuildError](err)
		require.True(t, ok, "got %v", err)
		assert.Equal(t, built, tempErr.Path)
		assert.Empty(t, snapshot(t, folder))
	})
}

func Test_install_writes_the_path_link_when_it_is_a_hard_link_to_the_running_binary(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	running := writeQuarry(t, t.TempDir())
	hardLink := filepath.Join(t.TempDir(), "quarry")
	require.NoError(t, os.Link(running, hardLink))

	res, err := installWithPath(t, home, running, t.TempDir(), lookPathReturning(hardLink))

	require.NoError(t, err)
	assert.Equal(t, claudedesktop.Result{Folder: folder, Config: filepath.Join(folder, configName), Command: hardLink}, res)
}

func Test_install_writes_the_path_quarry_when_the_running_binary_is_a_link_to_it(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	onPath := writeQuarry(t, t.TempDir())
	running := filepath.Join(t.TempDir(), "quarry")
	require.NoError(t, os.Symlink(onPath, running))

	res, err := installWithPath(t, home, running, t.TempDir(), lookPathReturning(onPath))

	require.NoError(t, err)
	assert.Equal(t, claudedesktop.Result{Folder: folder, Config: filepath.Join(folder, configName), Command: onPath}, res)
}

func Test_install_writes_the_running_binary_without_a_path_warning_when_the_path_quarry_cannot_be_used(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		lookup func(t *testing.T) claudedesktop.LookPath
	}{
		{name: "PATH names a link to nothing", lookup: func(t *testing.T) claudedesktop.LookPath {
			t.Helper()
			dangling := filepath.Join(t.TempDir(), "quarry")
			require.NoError(t, os.Symlink(filepath.Join(t.TempDir(), "gone"), dangling))
			return lookPathReturning(dangling)
		}},
		// doc.go exists in the test's working directory, so only the absolute-path rule rejects it.
		{name: "PATH names a relative path", lookup: func(*testing.T) claudedesktop.LookPath { return lookPathReturning("doc.go") }},
		{name: "no lookup is configured", lookup: func(*testing.T) claudedesktop.LookPath { return nil }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home, folder := desktopFolder(t)
			running := writeQuarry(t, t.TempDir())

			res, err := installWithPath(t, home, running, t.TempDir(), c.lookup(t))

			require.NoError(t, err)
			assert.Equal(t, claudedesktop.Result{Folder: folder, Config: filepath.Join(folder, configName), Command: running}, res)
		})
	}
}

func Test_install_writes_the_running_binary_and_reports_the_path_quarry_when_the_running_binary_cannot_be_checked(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	running := filepath.Join(t.TempDir(), "quarry")
	onPath := writeQuarry(t, t.TempDir())

	res, err := installWithPath(t, home, running, t.TempDir(), lookPathReturning(onPath))

	require.NoError(t, err)
	assert.Equal(t, claudedesktop.Result{Folder: folder, Config: filepath.Join(folder, configName), Command: running, PathQuarry: onPath}, res)
}

func Test_install_writes_the_running_binary_without_a_path_warning_when_neither_binary_can_be_checked(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	running := filepath.Join(t.TempDir(), "quarry")
	onPath := filepath.Join(t.TempDir(), "quarry")

	res, err := installWithPath(t, home, running, t.TempDir(), lookPathReturning(onPath))

	require.NoError(t, err)
	assert.Equal(t, claudedesktop.Result{Folder: folder, Config: filepath.Join(folder, configName), Command: running}, res)
}

func Test_install_reports_the_path_quarry_for_an_entry_it_repoints(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	writeConfig(t, folder, quarryEntryConfig(`{"command":"/old/quarry","args":["mcp"]}`), 0o600)
	running := writeQuarry(t, t.TempDir())
	onPath := writeQuarry(t, t.TempDir())

	res, err := installWithPath(t, home, running, t.TempDir(), lookPathReturning(onPath))

	require.NoError(t, err)
	assert.Equal(t, claudedesktop.Result{
		Folder: folder, Config: filepath.Join(folder, configName), Outcome: claudedesktop.Updated,
		Command: running, Previous: "/old/quarry", PathQuarry: onPath,
	}, res)
}

func Test_install_reports_the_path_quarry_for_an_entry_that_already_starts_the_running_binary(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	running := writeQuarry(t, t.TempDir())
	onPath := writeQuarry(t, t.TempDir())
	writeConfig(t, folder, quarryEntryConfig(`{"command":"`+running+`","args":["mcp"]}`), 0o600)

	res, err := installWithPath(t, home, running, t.TempDir(), lookPathReturning(onPath))

	require.NoError(t, err)
	assert.Equal(t, claudedesktop.Result{
		Folder: folder, Config: filepath.Join(folder, configName), Outcome: claudedesktop.Unchanged,
		Command: running, PathQuarry: onPath,
	}, res)
}

func Test_install_names_the_path_link_in_the_symbolic_link_refusal(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	config := filepath.Join(folder, configName)
	require.NoError(t, os.Symlink(filepath.Join(t.TempDir(), "elsewhere.json"), config))
	running := writeQuarry(t, t.TempDir())
	pathLink := filepath.Join(t.TempDir(), "quarry")
	require.NoError(t, os.Symlink(running, pathLink))

	_, err := installWithPath(t, home, running, t.TempDir(), lookPathReturning(pathLink))

	link, ok := errors.AsType[*claudedesktop.SymlinkError](err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, pathLink, link.Command)
}

func Test_install_does_not_look_for_quarry_when_the_desktop_folder_is_missing(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	calls := 0
	lookPath := func(string) (string, error) { calls++; return "", errNoQuarryOnPath }
	exe := &fakeExecutable{path: quarryBinary}
	srv := claudedesktop.NewServer(claudedesktop.WithHome(home), claudedesktop.WithExecutable(exe.executable), claudedesktop.WithLookPath(lookPath))

	res, err := srv.Install(t.Context())

	require.NoError(t, err)
	assert.True(t, res.Skipped)
	assert.Zero(t, calls)
}

func Test_install_refuses_a_temporary_build_and_names_it(t *testing.T) {
	t.Parallel()
	const root = "/var/quarry-test-temp"
	cases := []struct{ name, temp, exe string }{
		{name: "the binary is under the temp root", temp: root, exe: root + "/work/quarry"},
		{name: "the root has a trailing slash", temp: root + "/", exe: root + "/work/quarry"},
		{name: "the root is not clean", temp: root + "/./a/..", exe: root + "/work/quarry"},
		{name: "go-build is a middle element", temp: root, exe: "/opt/go-build/exe/quarry"},
		{name: "an element only starts with go-build", temp: root, exe: "/opt/go-build-1/quarry"},
		{name: "an element only starts with two dots", temp: root, exe: root + "/..cache/quarry"},
		{name: "go-build is the binary's own name", temp: root, exe: "/opt/go-build"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home, folder := desktopFolder(t)

			_, err := installWithPath(t, home, c.exe, c.temp, nil)

			tempErr, ok := errors.AsType[*claudedesktop.TempBuildError](err)
			require.True(t, ok, "got %v", err)
			assert.Equal(t, c.exe, tempErr.Path)
			require.EqualError(t, err, c.exe+" is a temporary build")
			assert.Empty(t, snapshot(t, folder))
		})
	}
}

func Test_install_accepts_a_binary_outside_the_temporary_directory(t *testing.T) {
	t.Parallel()
	const root = "/var/quarry-test-temp"
	cases := []struct{ name, temp, exe string }{
		{name: "a sibling directory shares the root's prefix", temp: root, exe: root + "x/quarry"},
		{name: "the binary is the root itself", temp: "/var/quarry", exe: "/var/quarry"},
		{name: "an element ends with go-build", temp: root, exe: "/opt/ago-build/quarry"},
		{name: "the root is empty", temp: "", exe: root + "/work/quarry"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home, _ := desktopFolder(t)

			res, err := installWithPath(t, home, c.exe, c.temp, nil)

			require.NoError(t, err)
			assert.Equal(t, c.exe, res.Command)
		})
	}
}

func Test_install_refuses_a_path_quarry_under_the_temporary_directory(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	temp := t.TempDir()
	running := writeQuarry(t, t.TempDir())
	pathLink := filepath.Join(temp, "quarry")
	require.NoError(t, os.Symlink(running, pathLink))

	_, err := installWithPath(t, home, running, temp, lookPathReturning(pathLink))

	tempErr, ok := errors.AsType[*claudedesktop.TempBuildError](err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, pathLink, tempErr.Path)
	assert.Empty(t, snapshot(t, folder))
}

func Test_install_refuses_a_temporary_build_before_reading_a_config_it_would_refuse(t *testing.T) {
	t.Parallel()
	const built = "/var/quarry-test-temp/quarry"
	cases := []struct {
		name  string
		setup func(t *testing.T, config string)
	}{
		{name: "the config is not valid JSON", setup: func(t *testing.T, config string) {
			t.Helper()
			require.NoError(t, os.WriteFile(config, []byte("{"), 0o600))
		}},
		{name: "the config is a symbolic link", setup: func(t *testing.T, config string) {
			t.Helper()
			require.NoError(t, os.Symlink(filepath.Join(t.TempDir(), "elsewhere.json"), config))
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home, folder := desktopFolder(t)
			c.setup(t, filepath.Join(folder, configName))
			before := snapshot(t, folder)

			_, err := installWithPath(t, home, built, "/var/quarry-test-temp", nil)

			_, ok := errors.AsType[*claudedesktop.TempBuildError](err)
			require.True(t, ok, "got %v", err)
			assert.Equal(t, before, snapshot(t, folder))
		})
	}
}

func Test_install_refuses_a_temporary_build_even_when_a_different_quarry_is_on_PATH(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	temp := t.TempDir()
	built := writeQuarry(t, temp)
	onPath := writeQuarry(t, t.TempDir())

	_, err := installWithPath(t, home, built, temp, lookPathReturning(onPath))

	tempErr, ok := errors.AsType[*claudedesktop.TempBuildError](err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, built, tempErr.Path)
	assert.Empty(t, snapshot(t, folder))
}

func Test_install_refuses_a_temporary_build_before_reading_a_config_it_cannot_read(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	config := writeConfig(t, folder, "{}", 0o000)
	t.Cleanup(func() { _ = os.Chmod(config, 0o600) })

	_, err := installWithPath(t, home, "/var/quarry-test-temp/quarry", "/var/quarry-test-temp", nil)

	require.NoError(t, os.Chmod(config, 0o600))
	_, ok := errors.AsType[*claudedesktop.TempBuildError](err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, []string{"claude_desktop_config.json -rw------- {}"}, snapshot(t, folder))
}
