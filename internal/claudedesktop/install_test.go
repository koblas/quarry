package claudedesktop_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/koblas/quarry/internal/claudedesktop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	quarryBinary = "/opt/homebrew/bin/quarry"
	configName   = "claude_desktop_config.json"
)

var errNoExecutable = errors.New("no executable path")

// fakeExecutable records its calls and reports a fixed path or error.
type fakeExecutable struct {
	path  string
	err   error
	calls int
}

func (f *fakeExecutable) executable() (string, error) {
	f.calls++
	return f.path, f.err
}

// desktopFolder creates a home holding Claude Desktop's folder and returns both paths.
func desktopFolder(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	folder := filepath.Join(home, "Library", "Application Support", "Claude")
	require.NoError(t, os.MkdirAll(folder, 0o755))
	return home, folder
}

func install(t *testing.T, home string, exe *fakeExecutable) (claudedesktop.Result, error) {
	t.Helper()
	srv := claudedesktop.NewServer(claudedesktop.WithHome(home), claudedesktop.WithExecutable(exe.executable))
	return srv.Install(t.Context())
}

// snapshot lists a folder's entries with their type, mode and contents or link target.
func snapshot(t *testing.T, folder string) []string {
	t.Helper()
	entries, err := os.ReadDir(folder)
	require.NoError(t, err)
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		path := filepath.Join(folder, e.Name())
		info, err := os.Lstat(path)
		require.NoError(t, err)
		line := fmt.Sprintf("%s %s", e.Name(), info.Mode())
		switch {
		case info.Mode().IsRegular():
			body, err := os.ReadFile(path)
			require.NoError(t, err)
			line += " " + string(body)
		case info.Mode()&fs.ModeSymlink != 0:
			target, err := os.Readlink(path)
			require.NoError(t, err)
			line += " -> " + target
		}
		out = append(out, line)
	}
	return out
}

func Test_install_creates_the_config_with_the_quarry_entry_when_the_folder_holds_none(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	exe := &fakeExecutable{path: quarryBinary}

	res, err := install(t, home, exe)

	require.NoError(t, err)
	config := filepath.Join(folder, configName)
	assert.Equal(t, claudedesktop.Result{Folder: folder, Config: config, Command: quarryBinary}, res)
	info, err := os.Stat(config)
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o600), info.Mode().Perm())
}

func Test_install_writes_two_space_indented_json_with_a_trailing_newline(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)

	_, err := install(t, home, &fakeExecutable{path: quarryBinary})

	require.NoError(t, err)
	written, err := os.ReadFile(filepath.Join(folder, configName))
	require.NoError(t, err)
	want := "{\n" +
		"  \"mcpServers\": {\n" +
		"    \"quarry\": {\n" +
		"      \"command\": \"/opt/homebrew/bin/quarry\",\n" +
		"      \"args\": [\n" +
		"        \"mcp\"\n" +
		"      ]\n" +
		"    }\n" +
		"  }\n" +
		"}\n"
	assert.Equal(t, want, string(written))
}

func Test_install_keeps_html_characters_in_the_path_literal(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)

	_, err := install(t, home, &fakeExecutable{path: "/opt/a<b>&c/quarry"})

	require.NoError(t, err)
	written, err := os.ReadFile(filepath.Join(folder, configName))
	require.NoError(t, err)
	assert.Contains(t, string(written), `"command": "/opt/a<b>&c/quarry"`)
}

func Test_install_skips_a_missing_desktop_folder_without_asking_for_the_executable(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	exe := &fakeExecutable{path: quarryBinary}

	res, err := install(t, home, exe)

	require.NoError(t, err)
	folder := filepath.Join(home, "Library", "Application Support", "Claude")
	assert.Equal(t, claudedesktop.Result{Folder: folder, Config: filepath.Join(folder, configName), Skipped: true}, res)
	assert.Zero(t, exe.calls)
	assert.Empty(t, snapshot(t, home))
}

func Test_install_refuses_a_config_path_that_already_holds_anything(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		setup func(config string) error
	}{
		{name: "an empty file", setup: func(config string) error { return os.WriteFile(config, nil, 0o600) }},
		{name: "a JSON object", setup: func(config string) error { return os.WriteFile(config, []byte(`{"mcpServers": {}}`), 0o600) }},
		{name: "a folder", setup: func(config string) error { return os.Mkdir(config, 0o755) }},
		{name: "a dangling symlink", setup: func(config string) error { return os.Symlink(filepath.Join(filepath.Dir(config), "nowhere"), config) }},
		{name: "a FIFO", setup: func(config string) error { return syscall.Mkfifo(config, 0o600) }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home, folder := desktopFolder(t)
			require.NoError(t, c.setup(filepath.Join(folder, configName)))
			before := snapshot(t, folder)
			exe := &fakeExecutable{path: quarryBinary}

			_, err := install(t, home, exe)

			require.ErrorIs(t, err, fs.ErrExist)
			assert.Equal(t, before, snapshot(t, folder))
			assert.Equal(t, 1, exe.calls)
		})
	}
}

func Test_install_refuses_an_empty_home_without_writing_under_the_working_directory(t *testing.T) {
	relative := filepath.Join("Library", "Application Support", "Claude")
	cwd := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(cwd, relative), 0o755))
	t.Chdir(cwd)
	exe := &fakeExecutable{path: quarryBinary}

	_, err := install(t, "", exe)

	require.ErrorIs(t, err, claudedesktop.ErrNoHome)
	assert.Empty(t, snapshot(t, filepath.Join(cwd, relative)))
	assert.Zero(t, exe.calls)
}

func Test_install_fails_without_writing_when_the_folder_cannot_be_checked(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	parent := filepath.Dir(folder)
	require.NoError(t, os.Chmod(parent, 0o000))
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })

	_, err := install(t, home, &fakeExecutable{path: quarryBinary})

	require.ErrorIs(t, err, fs.ErrPermission)
	require.NoError(t, os.Chmod(parent, 0o755))
	assert.Empty(t, snapshot(t, folder))
}

func Test_install_fails_without_writing_when_the_config_cannot_be_checked(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	support := filepath.Join(home, "Library", "Application Support")
	require.NoError(t, os.MkdirAll(support, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(support, "Claude"), []byte("not a folder"), 0o600))

	_, err := install(t, home, &fakeExecutable{path: quarryBinary})

	require.ErrorIs(t, err, syscall.ENOTDIR)
	assert.Equal(t, []string{"Claude -rw------- not a folder"}, snapshot(t, support))
}

func Test_install_fails_without_writing_when_the_executable_path_is_unknown(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)

	_, err := install(t, home, &fakeExecutable{err: errNoExecutable})

	require.ErrorIs(t, err, errNoExecutable)
	assert.Empty(t, snapshot(t, folder))
}

func Test_install_fails_without_leaving_a_file_when_the_folder_is_read_only(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	require.NoError(t, os.Chmod(folder, 0o500))
	t.Cleanup(func() { _ = os.Chmod(folder, 0o755) })

	_, err := install(t, home, &fakeExecutable{path: quarryBinary})

	require.ErrorIs(t, err, fs.ErrPermission)
	require.NoError(t, os.Chmod(folder, 0o755))
	assert.Empty(t, snapshot(t, folder))
}
