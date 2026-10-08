package cli_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	desktopRemovedLine    = "Removed the quarry MCP server from Claude Desktop.\n"
	desktopQuitUnloadLine = "Quit and reopen Claude Desktop to unload it.\n"

	desktopAbsentLine = "The quarry MCP server is not in Claude Desktop.\n"

	desktopNoHomeUninstallLine = "quarry: claude uninstall: cannot find your home directory ($HOME is not set), " +
		"so quarry cannot look for Claude Desktop; set HOME, then run quarry claude uninstall again\n"

	quarryEntry = `"quarry": {"command": "` + desktopQuarryBinary + `", "args": ["mcp"]}`

	codeUninstalledLines  = pluginUninstalledLine + marketplaceRemovedLine + uninstallRestartLine
	desktopRetryUninstall = ", then run quarry claude uninstall again\n"
)

// uninstallDesktop runs quarry claude uninstall over a Code with the plugin and marketplace installed,
// at home and with no Executable, writing to stdout and stderr.
func uninstallDesktop(t *testing.T, tool *toolCalls, home string, stdout, stderr io.Writer) error {
	t.Helper()
	return cli.Execute(t.Context(), []string{"claude", "uninstall"}, cli.Env{
		Stdout:  stdout,
		Stderr:  stderr,
		RunTool: tool.run,
		Home:    home,
	})
}

// folderState maps each entry of folder to its mode, size and modification time.
func folderState(t *testing.T, folder string) map[string]string {
	t.Helper()
	state := map[string]string{}
	for _, name := range entryNames(t, folder) {
		info, err := os.Lstat(filepath.Join(folder, name))
		require.NoError(t, err)
		state[name] = info.Mode().String() + " " + strconv.FormatInt(info.Size(), 10) + " " + info.ModTime().Format(time.RFC3339Nano)
	}
	return state
}

func Test_claude_uninstall_removes_quarry_from_claude_desktop_after_claude_code(t *testing.T) {
	cases := []struct {
		name     string
		existing string
		want     string
	}{
		{
			name: "beside other servers",
			existing: `{
  "globalShortcut": "Cmd+Shift+Space",
  "bigNumber": 12345678901234567890,
  "mcpServers": {
    "github": {"command": "npx", "args": ["-y", "server-github"], "env": {"ID": 12345678901234567890}},
    ` + quarryEntry + `
  }
}
`,
			want: `{
  "globalShortcut": "Cmd+Shift+Space",
  "bigNumber": 12345678901234567890,
  "mcpServers": {
    "github": {"command": "npx", "args": ["-y", "server-github"], "env": {"ID": 12345678901234567890}}
  }
}
`,
		},
		{
			name:     "the only server",
			existing: `{"mcpServers": {` + quarryEntry + `}}`,
			want:     `{"mcpServers": {}}`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home, folder := desktopHome(t)
			config := filepath.Join(folder, desktopConfigName)
			writeDesktopConfig(t, config, c.existing)
			tool := (&toolCalls{}).lists(ourMarketplace, userPluginOn)

			stdout, stderr, err := runClaudeAt(t, tool, nil, home, "claude", "uninstall")

			require.NoError(t, err)
			assert.Equal(t, pluginUninstalledLine+marketplaceRemovedLine+uninstallRestartLine+
				desktopRemovedLine+desktopQuitUnloadLine, stdout)
			assert.Empty(t, stderr)
			written, err := os.ReadFile(config)
			require.NoError(t, err)
			assert.Equal(t, decodeConfig(t, []byte(c.want)), decodeConfig(t, written))
			backup, err := os.ReadFile(config + ".before-quarry")
			require.NoError(t, err)
			assert.Equal(t, c.existing, string(backup))
		})
	}
}

func Test_claude_uninstall_leaves_claude_desktop_alone_when_its_config_has_no_quarry_entry(t *testing.T) {
	cases := []struct {
		name string
		seed func(t *testing.T, config string)
	}{
		{name: "missing", seed: func(*testing.T, string) {}},
		{name: "empty", seed: func(t *testing.T, config string) { t.Helper(); writeDesktopConfig(t, config, "") }},
		{name: "whitespace only", seed: func(t *testing.T, config string) { t.Helper(); writeDesktopConfig(t, config, " \n\t\n") }},
		{name: "null", seed: func(t *testing.T, config string) { t.Helper(); writeDesktopConfig(t, config, "null") }},
		{name: "an array", seed: func(t *testing.T, config string) { t.Helper(); writeDesktopConfig(t, config, `[{"mcpServers": {}}]`) }},
		{name: "an object with no mcpServers", seed: func(t *testing.T, config string) {
			t.Helper()
			writeDesktopConfig(t, config, `{"globalShortcut": "Cmd+Shift+Space"}`)
		}},
		{name: "mcpServers null", seed: func(t *testing.T, config string) {
			t.Helper()
			writeDesktopConfig(t, config, `{"mcpServers": null}`)
		}},
		{name: "mcpServers an array", seed: func(t *testing.T, config string) {
			t.Helper()
			writeDesktopConfig(t, config, `{"mcpServers": []}`)
		}},
		{name: "mcpServers without quarry", seed: func(t *testing.T, config string) {
			t.Helper()
			writeDesktopConfig(t, config, `{"mcpServers": {"github": {"command": "npx"}}}`)
		}},
		{name: "a folder", seed: func(t *testing.T, config string) {
			t.Helper()
			require.NoError(t, os.Mkdir(config, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(config, "inner"), []byte("kept"), 0o600))
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home, folder := desktopHome(t)
			c.seed(t, filepath.Join(folder, desktopConfigName))
			before := folderState(t, folder)
			tool := (&toolCalls{}).lists(ourMarketplace, userPluginOn)

			stdout, stderr, err := runClaudeAt(t, tool, nil, home, "claude", "uninstall")

			require.NoError(t, err)
			assert.Equal(t, codeUninstalledLines+desktopAbsentLine, stdout)
			assert.Empty(t, stderr)
			assert.Equal(t, before, folderState(t, folder))
		})
	}
}

func Test_claude_uninstall_reports_a_missing_home_after_the_claude_code_lines(t *testing.T) {
	var out, errOut bytes.Buffer

	err := uninstallDesktop(t, (&toolCalls{}).lists(ourMarketplace, userPluginOn), "", &out, &errOut)

	require.ErrorIs(t, err, cli.ReportedError{})
	assert.Equal(t, codeUninstalledLines, out.String())
	assert.Equal(t, desktopNoHomeUninstallLine, errOut.String())
}

func Test_claude_uninstall_prints_the_claude_code_hint_before_the_claude_desktop_lines(t *testing.T) {
	home, _ := desktopHome(t)
	tool := (&toolCalls{}).lists(ourMarketplace, "["+projectPluginUnder(home)+"]")
	var both bytes.Buffer

	err := uninstallDesktop(t, tool, home, &both, &both)

	require.NoError(t, err)
	assert.Equal(t, pluginAbsentLine+marketplaceKeptLine+
		`quarry: claude uninstall: the quarry plugin is still installed for project "~/repos/foo"; `+
		"to remove it, run claude plugin uninstall --scope project quarry@quarry in that directory\n"+
		desktopAbsentLine, both.String())
}

func Test_claude_uninstall_returns_the_error_of_a_failed_claude_desktop_line_write(t *testing.T) {
	home, _ := desktopHome(t)
	var errOut bytes.Buffer

	err := uninstallDesktop(t, (&toolCalls{}).lists(ourMarketplace, userPluginOn), home, &failsAfterFirstWrite{err: errPipeClosed}, &errOut)

	require.ErrorIs(t, err, errPipeClosed)
	require.NotErrorIs(t, err, cli.ReportedError{})
}

func Test_claude_uninstall_refuses_a_desktop_config_it_cannot_judge(t *testing.T) {
	const (
		oursConfig  = `{"mcpServers": {` + quarryEntry + `}}`
		linkTarget  = "elsewhere.json"
		linkedBytes = `{"mcpServers": {}}`
	)
	cases := []struct {
		name        string
		seed        func(t *testing.T, folder string)
		wantLine    string
		wantEntries []string
	}{
		{
			name: "a foreign quarry entry",
			seed: func(t *testing.T, folder string) {
				t.Helper()
				writeDesktopConfig(t, filepath.Join(folder, desktopConfigName), `{"mcpServers": {"quarry": {"command": "/usr/local/bin/other-tool", "args": ["serve"]}}}`)
			},
			wantLine: "quarry: claude uninstall: Claude Desktop has an MCP server named \"quarry\" that does not run quarry mcp, " +
				"so quarry leaves it alone; to remove it, delete it from " + desktopConfigShown + " yourself\n",
			wantEntries: []string{desktopConfigName},
		},
		{
			name: "a symbolic link",
			seed: func(t *testing.T, folder string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(folder, linkTarget), []byte(linkedBytes), 0o600))
				require.NoError(t, os.Symlink(linkTarget, filepath.Join(folder, desktopConfigName)))
			},
			wantLine: "quarry: claude uninstall: " + desktopConfigShown + " is a symbolic link, so quarry leaves it alone; " +
				"if it has a \"quarry\" entry under mcpServers, remove it yourself\n",
			wantEntries: []string{desktopConfigName, linkTarget},
		},
		{
			name: "not valid JSON",
			seed: func(t *testing.T, folder string) {
				t.Helper()
				writeDesktopConfig(t, filepath.Join(folder, desktopConfigName), `{"globalShortcut": "x", "mcpServers": {`)
			},
			wantLine: "quarry: claude uninstall: cannot read " + desktopConfigShown +
				": it is not valid JSON (unexpected end of JSON input, at byte 39); fix it so Claude Desktop can read it too" +
				", then run quarry claude uninstall again\n",
			wantEntries: []string{desktopConfigName},
		},
		{
			name: "unreadable",
			seed: func(t *testing.T, folder string) {
				t.Helper()
				config := filepath.Join(folder, desktopConfigName)
				writeDesktopConfig(t, config, oursConfig)
				require.NoError(t, os.Chmod(config, 0o000))
				t.Cleanup(func() { _ = os.Chmod(config, 0o644) })
			},
			wantLine: "quarry: claude uninstall: cannot read " + desktopConfigShown +
				" (permission denied); check its permissions" + desktopRetryUninstall,
			wantEntries: []string{desktopConfigName},
		},
		{
			name: "ours, but the backup cannot be saved",
			seed: func(t *testing.T, folder string) {
				t.Helper()
				writeDesktopConfig(t, filepath.Join(folder, desktopConfigName), oursConfig)
				require.NoError(t, os.MkdirAll(filepath.Join(folder, desktopBackupName, "kept"), 0o755))
			},
			wantLine: "quarry: claude uninstall: cannot save " + desktopBackupShown + " (file exists), so " + desktopConfigShown +
				" is unchanged; check the permissions of " + desktopFolderShown + desktopRetryUninstall,
			wantEntries: []string{desktopConfigName, desktopBackupName},
		},
		{
			name: "ours, but the write fails",
			seed: func(t *testing.T, folder string) {
				t.Helper()
				config := filepath.Join(folder, desktopConfigName)
				writeDesktopConfig(t, config, oursConfig)
				require.NoError(t, syscall.Chflags(config, userImmutableFlag))
				t.Cleanup(func() { _ = syscall.Chflags(config, 0) })
			},
			wantLine: "quarry: claude uninstall: cannot write " + desktopConfigShown + " (operation not permitted), so it is unchanged; " +
				"check the permissions of " + desktopFolderShown + desktopRetryUninstall,
			wantEntries: []string{desktopConfigName, desktopBackupName},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home, folder := desktopHome(t)
			c.seed(t, folder)
			before := desktopRefusalSnapshot(t, folder)
			tool := (&toolCalls{}).lists(ourMarketplace, userPluginOn)

			stdout, stderr, err := runClaudeAt(t, tool, nil, home, "claude", "uninstall")

			require.ErrorIs(t, err, cli.ReportedError{})
			assert.Equal(t, codeUninstalledLines, stdout)
			assert.Equal(t, c.wantLine, stderr)
			after := desktopRefusalSnapshot(t, folder)
			assert.Equal(t, c.wantEntries, after.entries)
			assert.Equal(t, before.mode, after.mode)
			assert.Equal(t, before.size, after.size)
			assert.Equal(t, before.modTime, after.modTime)
		})
	}
}

func Test_claude_uninstall_skips_claude_desktop_when_its_path_is_a_file(t *testing.T) {
	tool := (&toolCalls{}).lists(ourMarketplace, userPluginOn)
	var stdout, stderr bytes.Buffer

	err := uninstallDesktop(t, tool, homeWithClaudeAsFile(t), &stdout, &stderr)

	require.NoError(t, err)
	assert.Equal(t, codeUninstalledLines+desktopNotAFolderLine, stdout.String())
	assert.Empty(t, stderr.String())
}
