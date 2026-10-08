package claudedesktop_test

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/koblas/quarry/internal/claudedesktop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// uninstall runs Uninstall on a Server that has only a home, as the command builds it.
func uninstall(t *testing.T, home string) (claudedesktop.UninstallResult, error) {
	t.Helper()
	return claudedesktop.NewServer(claudedesktop.WithHome(home)).Uninstall(t.Context())
}

func Test_uninstall_refuses_a_quarry_entry_that_does_not_start_quarry_mcp(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, entry string }{
		{name: "entry null", entry: `null`},
		{name: "entry an array", entry: `["/usr/local/bin/quarry","mcp"]`},
		{name: "entry a string", entry: `"/usr/local/bin/quarry"`},
		{name: "entry a number", entry: `7`},
		{name: "entry a bool", entry: `true`},
		{name: "entry an empty object", entry: `{}`},
		{name: "args missing", entry: `{"command":"/usr/local/bin/quarry"}`},
		{name: "args null", entry: `{"command":"/usr/local/bin/quarry","args":null}`},
		{name: "args empty", entry: `{"command":"/usr/local/bin/quarry","args":[]}`},
		{name: "args with an extra argument", entry: `{"command":"/usr/local/bin/quarry","args":["mcp","x"]}`},
		{name: "args another word", entry: `{"command":"/usr/local/bin/quarry","args":["x"]}`},
		{name: "args in upper case", entry: `{"command":"/usr/local/bin/quarry","args":["MCP"]}`},
		{name: "args holding a number", entry: `{"command":"/usr/local/bin/quarry","args":["mcp",1]}`},
		{name: "args a string", entry: `{"command":"/usr/local/bin/quarry","args":"mcp"}`},
		{name: "command missing", entry: `{"args":["mcp"]}`},
		{name: "command null", entry: `{"command":null,"args":["mcp"]}`},
		{name: "command a number", entry: `{"command":7,"args":["mcp"]}`},
		{name: "command an array", entry: `{"command":["/usr/local/bin/quarry"],"args":["mcp"]}`},
		{name: "command an object", entry: `{"command":{},"args":["mcp"]}`},
		{name: "command empty", entry: `{"command":"","args":["mcp"]}`},
		{name: "command named quarry.sh", entry: `{"command":"/usr/local/bin/quarry.sh","args":["mcp"]}`},
		{name: "command named Quarry", entry: `{"command":"/usr/local/bin/Quarry","args":["mcp"]}`},
		{name: "command named QUARRY", entry: `{"command":"/usr/local/bin/QUARRY","args":["mcp"]}`},
		{name: "command named xquarry", entry: `{"command":"/usr/local/bin/xquarry","args":["mcp"]}`},
		{name: "command with a trailing slash", entry: `{"command":"/opt/x/quarry/","args":["mcp"]}`},
		{name: "command another program", entry: `{"command":"/usr/bin/other","args":[]}`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home, folder := desktopFolder(t)
			config := writeConfig(t, folder, quarryEntryConfig(c.entry), 0o644)
			before := snapshot(t, folder)

			_, err := uninstall(t, home)

			var foreign *claudedesktop.ForeignEntryError
			require.ErrorAs(t, err, &foreign)
			assert.Equal(t, config, foreign.Path)
			require.EqualError(t, err, config+": mcpServers.quarry is not an entry that starts quarry mcp")
			assert.Equal(t, before, snapshot(t, folder))
		})
	}
}

func Test_uninstall_refuses_a_quarry_entry_that_links_to_a_quarry_binary_under_another_name(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		link func(oldname, newname string) error
	}{
		{name: "symlink", link: os.Symlink},
		{name: "hard link", link: os.Link},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home, folder := desktopFolder(t)
			bin := t.TempDir()
			binary := filepath.Join(bin, "quarry")
			require.NoError(t, os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o600))
			alias := filepath.Join(bin, "qry")
			require.NoError(t, c.link(binary, alias))
			writeConfig(t, folder, quarryEntryConfig(`{"command":"`+alias+`","args":["mcp"]}`), 0o644)
			before := snapshot(t, folder)

			_, err := uninstall(t, home)

			var foreign *claudedesktop.ForeignEntryError
			require.ErrorAs(t, err, &foreign)
			assert.Equal(t, before, snapshot(t, folder))
		})
	}
}

func Test_uninstall_removes_an_entry_of_ours_at_any_path(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, command string }{
		{name: "a bare name", command: "quarry"},
		{name: "a relative path", command: "./quarry"},
		{name: "an installed path", command: "/usr/local/bin/quarry"},
		{name: "a path under the temporary directory", command: filepath.Join(os.TempDir(), "x", "quarry")},
		{name: "a go run build path", command: "/var/folders/ab/go-build123/b001/exe/quarry"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home, folder := desktopFolder(t)
			config := writeConfig(t, folder, quarryEntryConfig(`{"command":"`+c.command+`","args":["mcp"]}`), 0o644)

			res, err := uninstall(t, home)

			require.NoError(t, err)
			assert.Equal(t, claudedesktop.UninstallResult{Folder: folder, Config: config, Removed: true}, res)
			assert.JSONEq(t, `{"mcpServers":{}}`, readConfig(t, folder))
		})
	}
}

func Test_uninstall_keeps_an_empty_mcpservers_after_removing_the_only_entry(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	writeConfig(t, folder, `{"theme":"dark","mcpServers":{"quarry":{"command":"/usr/local/bin/quarry","args":["mcp"]}}}`, 0o644)

	_, err := uninstall(t, home)

	require.NoError(t, err)
	assert.JSONEq(t, `{"theme":"dark","mcpServers":{}}`, readConfig(t, folder))
}

func Test_uninstall_keeps_other_values_byte_for_byte_in_value(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	const original = `{"theme":"dark","mcpServers":{"other":{"command":"x","env":{"K":"<>&"},"big":12345678901234567890},` +
		`"quarry":{"command":"/usr/local/bin/quarry","args":["mcp"],"env":{"K":"v"}}}}`
	config := writeConfig(t, folder, original, 0o644)

	_, err := uninstall(t, home)

	require.NoError(t, err)
	want := "{\n" +
		"  \"mcpServers\": {\n" +
		"    \"other\": {\n" +
		"      \"command\": \"x\",\n" +
		"      \"env\": {\n" +
		"        \"K\": \"<>&\"\n" +
		"      },\n" +
		"      \"big\": 12345678901234567890\n" +
		"    }\n" +
		"  },\n" +
		"  \"theme\": \"dark\"\n" +
		"}\n"
	assert.Equal(t, want, readConfig(t, folder))
	backup, readErr := os.ReadFile(config + backupSuffix)
	require.NoError(t, readErr)
	assert.JSONEq(t, original, string(backup))
}

func Test_uninstall_changes_nothing_when_there_is_no_quarry_entry(t *testing.T) {
	t.Parallel()
	body := func(s string) func(string) error {
		return func(config string) error { return os.WriteFile(config, []byte(s), 0o600) }
	}
	cases := []struct {
		name  string
		setup func(config string) error
	}{
		{name: "no config file", setup: func(string) error { return nil }},
		{name: "an empty file", setup: body("")},
		{name: "only whitespace", setup: body(" \n\t\n")},
		{name: "top level null", setup: body(`null`)},
		{name: "top level an array", setup: body(`[]`)},
		{name: "top level a string", setup: body(`"servers"`)},
		{name: "top level a number", setup: body(`5`)},
		{name: "top level a boolean", setup: body(`true`)},
		{name: "mcpServers absent", setup: body(`{"theme":"dark"}`)},
		{name: "mcpServers null", setup: body(`{"mcpServers":null}`)},
		{name: "mcpServers an array", setup: body(`{"mcpServers":[]}`)},
		{name: "mcpServers a string", setup: body(`{"mcpServers":"none"}`)},
		{name: "mcpServers a number", setup: body(`{"mcpServers":7}`)},
		{name: "mcpServers a boolean", setup: body(`{"mcpServers":false}`)},
		{name: "mcpServers without quarry", setup: body(`{"mcpServers":{"other":{"command":"x"}}}`)},
		{name: "a folder at the config path", setup: func(config string) error { return os.Mkdir(config, 0o755) }},
		{name: "a FIFO at the config path", setup: func(config string) error { return syscall.Mkfifo(config, 0o600) }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home, folder := desktopFolder(t)
			require.NoError(t, c.setup(filepath.Join(folder, configName)))
			before := snapshot(t, folder)

			res, err := uninstall(t, home)

			require.NoError(t, err)
			assert.Equal(t, claudedesktop.UninstallResult{Folder: folder, Config: filepath.Join(folder, configName)}, res)
			assert.Equal(t, before, snapshot(t, folder))
		})
	}
}

func Test_uninstall_refuses_a_config_path_that_is_a_symlink(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		setup func(config string) error
	}{
		{name: "a dangling symlink", setup: func(config string) error { return os.Symlink(filepath.Join(filepath.Dir(config), "nowhere"), config) }},
		{name: "a symlink to a config holding our entry", setup: func(config string) error {
			target := filepath.Join(filepath.Dir(config), "real.json")
			if err := os.WriteFile(target, []byte(quarryEntryConfig(`{"command":"quarry","args":["mcp"]}`)), 0o600); err != nil {
				return err
			}
			return os.Symlink(target, config)
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home, folder := desktopFolder(t)
			config := filepath.Join(folder, configName)
			require.NoError(t, c.setup(config))
			before := snapshot(t, folder)

			_, err := uninstall(t, home)

			symlink, ok := errors.AsType[*claudedesktop.SymlinkError](err)
			require.True(t, ok, "got %v", err)
			assert.Equal(t, &claudedesktop.SymlinkError{Path: config}, symlink)
			assert.Equal(t, before, snapshot(t, folder))
		})
	}
}

func Test_uninstall_refuses_a_config_that_is_not_valid_json(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, body string }{
		{name: "truncated", body: `{"mcpServers": `},
		{name: "trailing data", body: `{} x`},
		{name: "a byte order mark", body: "\ufeff{}"},
		{name: "garbage", body: `not json`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home, folder := desktopFolder(t)
			config := writeConfig(t, folder, c.body, 0o644)
			before := snapshot(t, folder)

			_, err := uninstall(t, home)

			invalid, ok := errors.AsType[*claudedesktop.InvalidJSONError](err)
			require.True(t, ok, "got %v", err)
			assert.Equal(t, config, invalid.Path)
			require.Error(t, invalid.Err)
			assert.Equal(t, before, snapshot(t, folder))
		})
	}
}

func Test_uninstall_skips_a_missing_desktop_folder(t *testing.T) {
	t.Parallel()
	home := t.TempDir()

	res, err := uninstall(t, home)

	require.NoError(t, err)
	folder := filepath.Join(home, "Library", "Application Support", "Claude")
	assert.Equal(t, claudedesktop.UninstallResult{Folder: folder, Config: filepath.Join(folder, configName), Skipped: true}, res)
	assert.Empty(t, snapshot(t, home))
}

func Test_uninstall_refuses_an_empty_home_without_touching_the_working_directory(t *testing.T) {
	relative := filepath.Join("Library", "Application Support", "Claude")
	cwd := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(cwd, relative), 0o755))
	writeConfig(t, filepath.Join(cwd, relative), quarryEntryConfig(`{"command":"quarry","args":["mcp"]}`), 0o644)
	t.Chdir(cwd)
	before := snapshot(t, filepath.Join(cwd, relative))

	_, err := uninstall(t, "")

	require.ErrorIs(t, err, claudedesktop.ErrNoHome)
	assert.Equal(t, before, snapshot(t, filepath.Join(cwd, relative)))
}

func Test_uninstall_leaves_the_config_unchanged_when_the_backup_cannot_be_saved(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	config := writeConfig(t, folder, quarryEntryConfig(`{"command":"quarry","args":["mcp"]}`), 0o644)
	backup := config + backupSuffix
	require.NoError(t, os.MkdirAll(filepath.Join(backup, "kept"), 0o755))
	before := snapshot(t, folder)

	_, err := uninstall(t, home)

	var backupErr *claudedesktop.BackupError
	require.ErrorAs(t, err, &backupErr)
	assert.Equal(t, backup, backupErr.Path)
	assert.Equal(t, config, backupErr.Config)
	require.ErrorIs(t, err, os.ErrExist)
	assert.Equal(t, before, snapshot(t, folder))
}

func Test_uninstall_reports_a_failed_config_write_and_leaves_no_temp(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	original := quarryEntryConfig(`{"command":"quarry","args":["mcp"]}`)
	config := writeConfig(t, folder, original, 0o644)
	require.NoError(t, syscall.Chflags(config, userImmutableFlag))
	t.Cleanup(func() { _ = syscall.Chflags(config, 0) })

	_, err := uninstall(t, home)

	var writeErr *claudedesktop.WriteError
	require.ErrorAs(t, err, &writeErr)
	assert.Equal(t, config, writeErr.Path)
	require.ErrorIs(t, err, os.ErrPermission)
	assert.Equal(t, original, readConfig(t, folder))
	backup, readErr := os.ReadFile(config + backupSuffix)
	require.NoError(t, readErr)
	assert.Equal(t, []byte(original), backup)
	assert.Equal(t, []string{configName, configName + backupSuffix}, entryNames(t, folder))
}

func Test_uninstall_keeps_the_mode_of_the_config_it_replaces(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		mode os.FileMode
	}{
		{name: "a 0644 config", mode: 0o644},
		{name: "a 0640 config", mode: 0o640},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home, folder := desktopFolder(t)
			config := writeConfig(t, folder, quarryEntryConfig(`{"command":"quarry","args":["mcp"]}`), c.mode)

			_, err := uninstall(t, home)

			require.NoError(t, err)
			info, err := os.Stat(config)
			require.NoError(t, err)
			assert.Equal(t, c.mode, info.Mode().Perm())
		})
	}
}

func Test_uninstall_saves_the_original_in_a_private_backup_and_replaces_an_earlier_one(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	original := quarryEntryConfig(`{"command":"quarry","args":["mcp"]}`)
	config := writeConfig(t, folder, original, 0o644)
	require.NoError(t, os.WriteFile(config+backupSuffix, []byte(`{"first": true}`), 0o600))
	require.NoError(t, os.Chmod(config+backupSuffix, 0o644))

	_, err := uninstall(t, home)

	require.NoError(t, err)
	backup, readErr := os.ReadFile(config + backupSuffix)
	require.NoError(t, readErr)
	assert.Equal(t, []byte(original), backup)
	info, statErr := os.Stat(config + backupSuffix)
	require.NoError(t, statErr)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func Test_uninstall_replaces_a_symlink_at_the_backup_name_without_following_it(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	original := quarryEntryConfig(`{"command":"quarry","args":["mcp"]}`)
	config := writeConfig(t, folder, original, 0o644)
	target := filepath.Join(folder, "precious.txt")
	require.NoError(t, os.WriteFile(target, []byte("precious"), 0o600))
	require.NoError(t, os.Symlink(target, config+backupSuffix))

	_, err := uninstall(t, home)

	require.NoError(t, err)
	kept, readErr := os.ReadFile(target)
	require.NoError(t, readErr)
	assert.Equal(t, "precious", string(kept))
	info, statErr := os.Lstat(config + backupSuffix)
	require.NoError(t, statErr)
	assert.True(t, info.Mode().IsRegular())
	backup, readErr := os.ReadFile(config + backupSuffix)
	require.NoError(t, readErr)
	assert.Equal(t, []byte(original), backup)
}

func Test_uninstall_fails_without_writing_when_the_config_cannot_be_read(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	config := writeConfig(t, folder, quarryEntryConfig(`{"command":"quarry","args":["mcp"]}`), 0o644)
	before := snapshot(t, folder)
	require.NoError(t, os.Chmod(config, 0o000))
	t.Cleanup(func() { _ = os.Chmod(config, 0o644) })

	_, err := uninstall(t, home)

	require.ErrorIs(t, err, os.ErrPermission)
	readErr, ok := errors.AsType[*claudedesktop.ReadError](err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, config, readErr.Path)
	require.NoError(t, os.Chmod(config, 0o644))
	assert.Equal(t, before, snapshot(t, folder))
}

func Test_uninstall_fails_without_writing_when_the_config_cannot_be_checked(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	config := writeConfig(t, folder, quarryEntryConfig(`{"command":"quarry","args":["mcp"]}`), 0o644)
	before := snapshot(t, folder)
	require.NoError(t, os.Chmod(folder, 0o000))
	t.Cleanup(func() { _ = os.Chmod(folder, 0o755) })

	_, err := uninstall(t, home)

	require.NoError(t, os.Chmod(folder, 0o755))
	require.ErrorIs(t, err, os.ErrPermission)
	readErr, ok := errors.AsType[*claudedesktop.ReadError](err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, config, readErr.Path)
	assert.Equal(t, before, snapshot(t, folder))
}

func Test_uninstall_fails_without_writing_when_the_folder_cannot_be_checked(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	writeConfig(t, folder, quarryEntryConfig(`{"command":"quarry","args":["mcp"]}`), 0o644)
	before := snapshot(t, folder)
	parent := filepath.Dir(folder)
	require.NoError(t, os.Chmod(parent, 0o000))
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })

	_, err := uninstall(t, home)

	require.NoError(t, os.Chmod(parent, 0o755))
	require.ErrorIs(t, err, os.ErrPermission)
	require.ErrorContains(t, err, "check "+folder)
	_, classified := errors.AsType[*claudedesktop.ReadError](err)
	assert.False(t, classified)
	assert.Equal(t, before, snapshot(t, folder))
}
