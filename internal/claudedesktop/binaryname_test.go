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

const outsideTempRoot = "/var/quarry-test-temp"

func Test_install_refuses_a_binary_whose_last_element_is_not_exactly_quarry(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, exe, base string }{
		{name: "another name", exe: "/opt/homebrew/bin/qry", base: "qry"},
		{name: "upper case", exe: "/opt/homebrew/bin/Quarry", base: "Quarry"},
		{name: "an extension", exe: "/opt/homebrew/bin/quarry.sh", base: "quarry.sh"},
		{name: "a prefix", exe: "/opt/homebrew/bin/xquarry", base: "xquarry"},
		{name: "a trailing slash", exe: "/opt/homebrew/bin/quarry/", base: ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home, folder := desktopFolder(t)
			writeConfig(t, folder, `{"globalShortcut":"Cmd+Space"}`, 0o644)
			before := snapshot(t, folder)

			_, err := installWithPath(t, home, c.exe, outsideTempRoot, nil)

			nameErr, ok := errors.AsType[*claudedesktop.BinaryNameError](err)
			require.True(t, ok, "got %v", err)
			assert.Equal(t, c.base, nameErr.Base)
			assert.Equal(t, before, snapshot(t, folder))
		})
	}
}

func Test_install_names_the_binary_in_the_wrong_name_refusal(t *testing.T) {
	t.Parallel()
	home, _ := desktopFolder(t)

	_, err := installWithPath(t, home, "/opt/homebrew/bin/qry", outsideTempRoot, nil)

	require.EqualError(t, err, `the quarry binary is named "qry"`)
}

func Test_install_reports_a_temporary_build_before_a_wrong_binary_name(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)

	_, err := installWithPath(t, home, outsideTempRoot+"/work/qry", outsideTempRoot, nil)

	_, temp := errors.AsType[*claudedesktop.TempBuildError](err)
	assert.True(t, temp, "got %v", err)
	assert.Empty(t, snapshot(t, folder))
}

func Test_install_refuses_a_wrong_binary_name_before_reading_a_config_it_would_refuse(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	require.NoError(t, os.Symlink(filepath.Join(t.TempDir(), "elsewhere.json"), filepath.Join(folder, configName)))
	before := snapshot(t, folder)

	_, err := installWithPath(t, home, "/opt/homebrew/bin/qry", outsideTempRoot, nil)

	_, ok := errors.AsType[*claudedesktop.BinaryNameError](err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, before, snapshot(t, folder))
}

func Test_install_refuses_a_renamed_binary_when_the_quarry_on_PATH_is_another_file(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	renamed := filepath.Join(t.TempDir(), "qry")
	require.NoError(t, os.WriteFile(renamed, []byte("#!/bin/sh\n"), 0o755)) //nolint:gosec // an executable stand-in
	onPath := writeQuarry(t, t.TempDir())

	_, err := installWithPath(t, home, renamed, outsideTempRoot, lookPathReturning(onPath))

	_, ok := errors.AsType[*claudedesktop.BinaryNameError](err)
	require.True(t, ok, "got %v", err)
	assert.Empty(t, snapshot(t, folder))
}

func Test_install_writes_the_quarry_link_on_PATH_when_the_running_binary_is_renamed(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	renamed := filepath.Join(t.TempDir(), "qry")
	require.NoError(t, os.WriteFile(renamed, []byte("#!/bin/sh\n"), 0o755)) //nolint:gosec // an executable stand-in
	pathLink := filepath.Join(t.TempDir(), "quarry")
	require.NoError(t, os.Symlink(renamed, pathLink))

	res, err := installWithPath(t, home, renamed, outsideTempRoot, lookPathReturning(pathLink))

	require.NoError(t, err)
	assert.Equal(t, pathLink, res.Command)
	assert.JSONEq(t, quarryEntryConfig(`{"command":"`+pathLink+`","args":["mcp"]}`), readConfig(t, folder))
}

func Test_uninstall_removes_the_entry_install_wrote(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	_, err := install(t, home, &fakeExecutable{path: quarryBinary})
	require.NoError(t, err)

	res, err := uninstall(t, home)

	require.NoError(t, err)
	assert.True(t, res.Removed)
	assert.JSONEq(t, `{"mcpServers":{}}`, readConfig(t, folder))
}

func Test_uninstall_removes_the_entry_install_repointed(t *testing.T) {
	t.Parallel()
	home, folder := desktopFolder(t)
	_, err := install(t, home, &fakeExecutable{path: "/old/bin/quarry"})
	require.NoError(t, err)
	updated, err := install(t, home, &fakeExecutable{path: quarryBinary})
	require.NoError(t, err)
	require.Equal(t, claudedesktop.Updated, updated.Outcome)

	res, err := uninstall(t, home)

	require.NoError(t, err)
	assert.True(t, res.Removed)
	assert.JSONEq(t, `{"mcpServers":{}}`, readConfig(t, folder))
}
