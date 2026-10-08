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

// quarryOnlyConfig is the config Install writes when nothing else is in it.
const quarryOnlyConfig = "{\n" +
	"  \"mcpServers\": {\n" +
	"    \"quarry\": {\n" +
	"      \"command\": \"/opt/homebrew/bin/quarry\",\n" +
	"      \"args\": [\n" +
	"        \"mcp\"\n" +
	"      ]\n" +
	"    }\n" +
	"  }\n" +
	"}\n"

// writeConfig puts body in the folder's config at exactly mode and returns its path.
func writeConfig(t *testing.T, folder, body string, mode fs.FileMode) string {
	t.Helper()
	config := filepath.Join(folder, configName)
	require.NoError(t, os.WriteFile(config, []byte(body), 0o600))
	require.NoError(t, os.Chmod(config, mode))
	return config
}

func readConfig(t *testing.T, folder string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(folder, configName))
	require.NoError(t, err)
	return string(body)
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
	entries, err := os.ReadDir(folder)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
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

func Test_install_refuses_a_config_path_that_is_not_a_regular_file(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		setup func(config string) error
	}{
		{name: "a folder", setup: func(config string) error { return os.Mkdir(config, 0o755) }},
		{name: "a dangling symlink", setup: func(config string) error { return os.Symlink(filepath.Join(filepath.Dir(config), "nowhere"), config) }},
		{name: "a symlink to a regular file", setup: func(config string) error {
			target := filepath.Join(filepath.Dir(config), "real.json")
			if err := os.WriteFile(target, []byte(`{}`), 0o600); err != nil {
				return err
			}
			return os.Symlink(target, config)
		}},
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

func Test_install_keeps_other_values_byte_for_byte_in_value(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	writeConfig(t, folder, `{"theme":"dark","mcpServers":{"other":{"command":"x","env":{"K":"<>&"},"big":12345678901234567890}}}`, 0o600)

	_, err := install(t, home, &fakeExecutable{path: quarryBinary})

	require.NoError(t, err)
	want := "{\n" +
		"  \"mcpServers\": {\n" +
		"    \"other\": {\n" +
		"      \"command\": \"x\",\n" +
		"      \"env\": {\n" +
		"        \"K\": \"<>&\"\n" +
		"      },\n" +
		"      \"big\": 12345678901234567890\n" +
		"    },\n" +
		"    \"quarry\": {\n" +
		"      \"command\": \"/opt/homebrew/bin/quarry\",\n" +
		"      \"args\": [\n" +
		"        \"mcp\"\n" +
		"      ]\n" +
		"    }\n" +
		"  },\n" +
		"  \"theme\": \"dark\"\n" +
		"}\n"
	assert.Equal(t, want, readConfig(t, folder))
}

func Test_install_merges_into_an_empty_or_whitespace_config(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, body, want string }{
		{name: "an empty file", body: "", want: quarryOnlyConfig},
		{name: "only whitespace", body: " \n\t\n", want: quarryOnlyConfig},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home, folder := desktopFolder(t)
			writeConfig(t, folder, c.body, 0o600)

			_, err := install(t, home, &fakeExecutable{path: quarryBinary})

			require.NoError(t, err)
			assert.Equal(t, c.want, readConfig(t, folder))
		})
	}
}

func Test_install_merges_into_a_config_whose_mcpservers_is_null_or_absent(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, body, want string }{
		{name: "an empty object", body: `{}`, want: quarryOnlyConfig},
		{name: "mcpServers null", body: `{"mcpServers": null}`, want: quarryOnlyConfig},
		{name: "mcpServers absent", body: `{"theme": "dark"}`, want: "{\n" +
			"  \"mcpServers\": {\n" +
			"    \"quarry\": {\n" +
			"      \"command\": \"/opt/homebrew/bin/quarry\",\n" +
			"      \"args\": [\n" +
			"        \"mcp\"\n" +
			"      ]\n" +
			"    }\n" +
			"  },\n" +
			"  \"theme\": \"dark\"\n" +
			"}\n"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home, folder := desktopFolder(t)
			writeConfig(t, folder, c.body, 0o600)

			_, err := install(t, home, &fakeExecutable{path: quarryBinary})

			require.NoError(t, err)
			assert.Equal(t, c.want, readConfig(t, folder))
		})
	}
}

func Test_install_never_overwrites_a_present_quarry_entry(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, body string }{
		{name: "our own entry", body: `{"mcpServers":{"quarry":{"command":"/opt/homebrew/bin/quarry","args":["mcp"]}}}`},
		{name: "a foreign entry", body: `{"mcpServers":{"quarry":{"command":"/usr/bin/other","args":[]}}}`},
		{name: "a null entry", body: `{"mcpServers":{"quarry":null}}`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home, folder := desktopFolder(t)
			writeConfig(t, folder, c.body, 0o644)
			before := snapshot(t, folder)

			_, err := install(t, home, &fakeExecutable{path: quarryBinary})

			require.ErrorIs(t, err, claudedesktop.ErrEntryPresent)
			assert.Equal(t, before, snapshot(t, folder))
		})
	}
}

func Test_install_writes_nothing_when_the_config_is_not_a_json_object_it_can_merge_into(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, body string }{
		{name: "invalid JSON", body: `{"mcpServers": `},
		{name: "a top-level array", body: `[]`},
		{name: "a top-level null", body: `null`},
		{name: "a top-level string", body: `"servers"`},
		{name: "mcpServers an array", body: `{"mcpServers": []}`},
		{name: "mcpServers a string", body: `{"mcpServers": "none"}`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home, folder := desktopFolder(t)
			writeConfig(t, folder, c.body, 0o644)
			before := snapshot(t, folder)

			_, err := install(t, home, &fakeExecutable{path: quarryBinary})

			require.Error(t, err)
			require.NotErrorIs(t, err, fs.ErrExist)
			require.NotErrorIs(t, err, claudedesktop.ErrEntryPresent)
			assert.Equal(t, before, snapshot(t, folder))
		})
	}
}

func Test_install_fails_without_writing_when_the_config_cannot_be_read(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	config := writeConfig(t, folder, `{}`, 0o644)
	before := snapshot(t, folder)
	require.NoError(t, os.Chmod(config, 0o000))
	t.Cleanup(func() { _ = os.Chmod(config, 0o644) })

	_, err := install(t, home, &fakeExecutable{path: quarryBinary})

	require.ErrorIs(t, err, fs.ErrPermission)
	require.NoError(t, os.Chmod(config, 0o644))
	assert.Equal(t, before, snapshot(t, folder))
}

func Test_install_keeps_the_mode_of_the_config_it_replaces(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		mode fs.FileMode
	}{
		{name: "a 0644 config", mode: 0o644},
		{name: "a 0600 config", mode: 0o600},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home, folder := desktopFolder(t)
			config := writeConfig(t, folder, `{}`, c.mode)

			_, err := install(t, home, &fakeExecutable{path: quarryBinary})

			require.NoError(t, err)
			info, err := os.Stat(config)
			require.NoError(t, err)
			assert.Equal(t, c.mode, info.Mode().Perm())
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

const (
	backupSuffix = ".before-quarry"

	// userImmutableFlag is the BSD UF_IMMUTABLE file flag, which package syscall does not define.
	userImmutableFlag = 0x2
)

func Test_install_leaves_the_config_unchanged_when_the_backup_cannot_be_saved(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	config := writeConfig(t, folder, `{"globalShortcut": "Cmd+Shift+Space"}`, 0o644)
	backup := config + backupSuffix
	require.NoError(t, os.MkdirAll(filepath.Join(backup, "kept"), 0o755))
	before := snapshot(t, folder)

	_, err := install(t, home, &fakeExecutable{path: quarryBinary})

	var backupErr *claudedesktop.BackupError
	require.ErrorAs(t, err, &backupErr)
	assert.Equal(t, backup, backupErr.Path)
	require.ErrorContains(t, err, backup)
	require.ErrorIs(t, err, fs.ErrExist)
	assert.Equal(t, before, snapshot(t, folder))
}

func Test_install_reports_a_failed_config_write_and_leaves_no_temp(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	const original = `{"globalShortcut": "Cmd+Shift+Space"}`
	config := writeConfig(t, folder, original, 0o644)
	require.NoError(t, syscall.Chflags(config, userImmutableFlag))
	t.Cleanup(func() { _ = syscall.Chflags(config, 0) })

	_, err := install(t, home, &fakeExecutable{path: quarryBinary})

	var writeErr *claudedesktop.WriteError
	require.ErrorAs(t, err, &writeErr)
	assert.Equal(t, config, writeErr.Path)
	require.ErrorContains(t, err, config)
	require.ErrorIs(t, err, fs.ErrPermission)
	assert.JSONEq(t, original, readConfig(t, folder))
	backup, readErr := os.ReadFile(config + backupSuffix)
	require.NoError(t, readErr)
	assert.JSONEq(t, original, string(backup))
	assert.Equal(t, []string{configName, configName + backupSuffix}, entryNames(t, folder))
}

func Test_install_replaces_a_symlink_at_the_backup_name_without_following_it(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	const original = `{"globalShortcut": "Cmd+Shift+Space"}`
	config := writeConfig(t, folder, original, 0o644)
	target := filepath.Join(folder, "precious.txt")
	require.NoError(t, os.WriteFile(target, []byte("precious"), 0o600))
	require.NoError(t, os.Symlink(target, config+backupSuffix))

	_, err := install(t, home, &fakeExecutable{path: quarryBinary})

	require.NoError(t, err)
	kept, readErr := os.ReadFile(target)
	require.NoError(t, readErr)
	assert.Equal(t, "precious", string(kept))
	info, statErr := os.Lstat(config + backupSuffix)
	require.NoError(t, statErr)
	assert.True(t, info.Mode().IsRegular())
	assert.Equal(t, fs.FileMode(0o600), info.Mode().Perm())
	backup, readErr := os.ReadFile(config + backupSuffix)
	require.NoError(t, readErr)
	assert.JSONEq(t, original, string(backup))
}

func Test_install_overwrites_the_backup_of_an_earlier_write(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	config := writeConfig(t, folder, `{"second": true}`, 0o644)
	require.NoError(t, os.WriteFile(config+backupSuffix, []byte(`{"first": true}`), 0o600))

	_, err := install(t, home, &fakeExecutable{path: quarryBinary})

	require.NoError(t, err)
	backup, readErr := os.ReadFile(config + backupSuffix)
	require.NoError(t, readErr)
	assert.JSONEq(t, `{"second": true}`, string(backup))
}

func Test_install_writes_a_backup_of_an_empty_config(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	config := writeConfig(t, folder, "", 0o644)

	_, err := install(t, home, &fakeExecutable{path: quarryBinary})

	require.NoError(t, err)
	info, statErr := os.Stat(config + backupSuffix)
	require.NoError(t, statErr)
	assert.Zero(t, info.Size())
	assert.Equal(t, fs.FileMode(0o600), info.Mode().Perm())
}

func entryNames(t *testing.T, folder string) []string {
	t.Helper()
	entries, err := os.ReadDir(folder)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}
