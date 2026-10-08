package cli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	desktopConfigName   = "claude_desktop_config.json"
	desktopQuarryBinary = "/opt/homebrew/bin/quarry"

	desktopAddedLine = "Added the quarry MCP server to Claude Desktop; it starts \"" + desktopQuarryBinary + "\".\n"
	desktopQuitLine  = "Quit and reopen Claude Desktop to load it.\n"
	desktopKeptLine  = "The quarry MCP server is already in Claude Desktop; it starts \"" + desktopQuarryBinary + "\".\n"

	desktopNoHomeLine = "quarry: claude install: cannot find your home directory ($HOME is not set), " +
		"so quarry cannot look for Claude Desktop; set HOME, then run quarry claude install again\n"
)

const (
	// userImmutableFlag is the BSD UF_IMMUTABLE file flag, which package syscall does not define.
	userImmutableFlag = 0x2

	desktopBackupName = desktopConfigName + ".before-quarry"

	desktopConfigShown = `"~/Library/Application Support/Claude/` + desktopConfigName + `"`
	desktopBackupShown = `"~/Library/Application Support/Claude/` + desktopBackupName + `"`
	desktopFolderShown = `"~/Library/Application Support/Claude"`

	desktopForeignLine = "quarry: claude install: Claude Desktop has an MCP server named \"quarry\" that does not run quarry mcp, " +
		"so quarry leaves it alone; rename or remove it in " + desktopConfigShown + ", then run quarry claude install again\n"

	// staleEntryTail is what follows an entry's command: args, env and a key quarry does not know.
	staleEntryTail = `"args": ["mcp"], "env": {"TOKEN": "t0k3n", "ID": 12345678901234567890}, "note": "a<b>&c"`

	staleEntryRest = `"globalShortcut": "Cmd+Shift+Space", "mcpServers": {"github": {"command": "npx", "args": ["-y", "server-github"]}`

	otherServersConfig = `{
  "globalShortcut": "Cmd+Shift+Space",
  "bigNumber": 12345678901234567890,
  "mcpServers": {
    "github": {
      "command": "npx",
      "args": ["-y", "server-github", "--note", "a<b>&c"],
      "env": {"GITHUB_TOKEN": "ghp_secret123", "ID": 12345678901234567890}
    }
  }
}
`
)

var errPipeClosed = errors.New("pipe closed")

// failsAfterFirstWrite is a writer whose first write succeeds and every later one fails with err.
type failsAfterFirstWrite struct {
	err   error
	wrote bool
}

func (w *failsAfterFirstWrite) Write(p []byte) (int, error) {
	if w.wrote {
		return 0, w.err
	}
	w.wrote = true
	return len(p), nil
}

// installDesktop runs quarry claude install over tool with home and the fixed quarry binary,
// writing to stdout and stderr.
func installDesktop(t *testing.T, tool *toolCalls, home string, stdout, stderr io.Writer) error {
	t.Helper()
	return installDesktopAs(t, tool, home, desktopQuarryBinary, stdout, stderr)
}

// installDesktopAs is installDesktop with the running quarry at exe.
func installDesktopAs(t *testing.T, tool *toolCalls, home, exe string, stdout, stderr io.Writer) error {
	t.Helper()
	return cli.Execute(t.Context(), []string{"claude", "install"}, cli.Env{
		Stdout:     stdout,
		Stderr:     stderr,
		RunTool:    tool.run,
		Home:       home,
		Executable: func() (string, error) { return exe, nil },
	})
}

// desktopHome returns a home directory holding an empty Claude Desktop folder, and that folder.
func desktopHome(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	folder := filepath.Join(home, "Library", "Application Support", "Claude")
	require.NoError(t, os.MkdirAll(folder, 0o755))
	return home, folder
}

func Test_claude_install_adds_quarry_to_claude_desktop_after_claude_code(t *testing.T) {
	home, folder := desktopHome(t)
	tool := &toolCalls{}
	var out, errOut bytes.Buffer

	err := cli.Execute(t.Context(), []string{"claude", "install"}, cli.Env{
		Stdout:     &out,
		Stderr:     &errOut,
		RunTool:    tool.run,
		Home:       home,
		Executable: func() (string, error) { return desktopQuarryBinary, nil },
	})

	require.NoError(t, err)
	assert.Equal(t, marketplaceAddedLine+pluginInstalledLine+installRestartLine+desktopAddedLine+desktopQuitLine, out.String())
	assert.Empty(t, errOut.String())
	config := filepath.Join(folder, desktopConfigName)
	info, err := os.Stat(config)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	written, err := os.ReadFile(config)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(written, &decoded))
	assert.Equal(t, map[string]any{
		"mcpServers": map[string]any{
			"quarry": map[string]any{"command": desktopQuarryBinary, "args": []any{"mcp"}},
		},
	}, decoded)
	listing, err := os.ReadDir(folder)
	require.NoError(t, err)
	require.Len(t, listing, 1)
	assert.Equal(t, desktopConfigName, listing[0].Name())
}

func Test_claude_install_prints_a_binary_under_home_as_tilde_and_writes_its_absolute_path(t *testing.T) {
	home, folder := desktopHome(t)
	binary := filepath.Join(home, "bin", "quarry")
	var out, errOut bytes.Buffer

	err := cli.Execute(t.Context(), []string{"claude", "install"}, cli.Env{
		Stdout:     &out,
		Stderr:     &errOut,
		RunTool:    (&toolCalls{}).run,
		Home:       home,
		Executable: func() (string, error) { return binary, nil },
	})

	require.NoError(t, err)
	assert.Equal(t, marketplaceAddedLine+pluginInstalledLine+installRestartLine+
		"Added the quarry MCP server to Claude Desktop; it starts \"~/bin/quarry\".\n"+desktopQuitLine, out.String())
	written, err := os.ReadFile(filepath.Join(folder, desktopConfigName))
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(written, &decoded))
	assert.Equal(t, map[string]any{
		"mcpServers": map[string]any{
			"quarry": map[string]any{"command": binary, "args": []any{"mcp"}},
		},
	}, decoded)
}

func Test_claude_install_skips_claude_desktop_when_its_folder_does_not_exist(t *testing.T) {
	home := t.TempDir()
	var out, errOut bytes.Buffer

	err := installDesktop(t, &toolCalls{}, home, &out, &errOut)

	require.NoError(t, err)
	assert.Equal(t, marketplaceAddedLine+pluginInstalledLine+installRestartLine+desktopSkippedLine, out.String())
	assert.Empty(t, errOut.String())
	listing, err := os.ReadDir(home)
	require.NoError(t, err)
	assert.Empty(t, listing)
}

func Test_claude_install_reports_a_missing_home_after_the_claude_code_lines(t *testing.T) {
	var out, errOut bytes.Buffer

	err := installDesktop(t, &toolCalls{}, "", &out, &errOut)

	require.ErrorIs(t, err, cli.ReportedError{})
	assert.Equal(t, marketplaceAddedLine+pluginInstalledLine+installRestartLine, out.String())
	assert.Equal(t, desktopNoHomeLine, errOut.String())
}

func Test_claude_install_prints_the_symbolic_link_refusal_for_a_dangling_link(t *testing.T) {
	home, folder := desktopHome(t)
	config := filepath.Join(folder, desktopConfigName)
	require.NoError(t, os.Symlink(filepath.Join(folder, "elsewhere.json"), config))
	var out, errOut bytes.Buffer

	err := installDesktop(t, &toolCalls{}, home, &out, &errOut)

	require.ErrorIs(t, err, cli.ReportedError{})
	assert.Equal(t, marketplaceAddedLine+pluginInstalledLine+installRestartLine, out.String())
	assert.Equal(t, desktopSymlinkLine(`"`+desktopQuarryBinary+`"`), errOut.String())
	assert.Equal(t, []string{desktopConfigName}, entryNames(t, folder))
	target, err := os.Readlink(config)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(folder, "elsewhere.json"), target)
}

func Test_claude_install_prints_the_claude_code_hint_before_the_claude_desktop_lines(t *testing.T) {
	home, _ := desktopHome(t)
	tool := (&toolCalls{}).lists(ourMarketplace, userPluginOff)
	var both bytes.Buffer

	err := installDesktop(t, tool, home, &both, &both)

	require.NoError(t, err)
	assert.Equal(t, marketplacePresentLine+
		"The quarry plugin is already installed for all your projects.\n"+
		"quarry: claude install: the quarry plugin is installed but turned off; "+
		"to turn it on, run claude plugin enable quarry@quarry\n"+
		desktopAddedLine+desktopQuitLine, both.String())
}

func Test_claude_install_returns_the_error_of_a_failed_claude_desktop_line_write(t *testing.T) {
	home, _ := desktopHome(t)
	var errOut bytes.Buffer

	err := installDesktop(t, &toolCalls{}, home, &failsAfterFirstWrite{err: errPipeClosed}, &errOut)

	require.ErrorIs(t, err, errPipeClosed)
	require.NotErrorIs(t, err, cli.ReportedError{})
}

// decodeConfig decodes a config document keeping numbers as written.
func decodeConfig(t *testing.T, doc []byte) map[string]any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(doc))
	dec.UseNumber()
	var decoded map[string]any
	require.NoError(t, dec.Decode(&decoded))
	return decoded
}

// splitQuarryEntry returns the config without mcpServers.quarry, and that entry.
func splitQuarryEntry(t *testing.T, doc []byte) (map[string]any, any) {
	t.Helper()
	decoded := decodeConfig(t, doc)
	servers, ok := decoded["mcpServers"].(map[string]any)
	require.True(t, ok, "mcpServers is not an object: %s", doc)
	entry := servers["quarry"]
	delete(servers, "quarry")
	return decoded, entry
}

// writeDesktopConfig writes body at config with mode 0o644, whatever the umask.
func writeDesktopConfig(t *testing.T, config, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(config, []byte(body), 0o600))
	require.NoError(t, os.Chmod(config, 0o644))
}

func entryNames(t *testing.T, folder string) []string {
	t.Helper()
	listing, err := os.ReadDir(folder)
	require.NoError(t, err)
	names := make([]string, 0, len(listing))
	for _, e := range listing {
		names = append(names, e.Name())
	}
	return names
}

func Test_claude_install_merges_into_an_existing_desktop_config_and_keeps_a_backup(t *testing.T) {
	cases := []struct {
		name     string
		existing string
		others   string
		literals []string
	}{
		{name: "an empty file", existing: "", others: `{"mcpServers": {}}`},
		{name: "whitespace only", existing: " \n\t\n", others: `{"mcpServers": {}}`},
		{
			name:     "an object with no mcpServers",
			existing: `{"globalShortcut": "Cmd+Shift+Space"}`,
			others:   `{"globalShortcut": "Cmd+Shift+Space", "mcpServers": {}}`,
		},
		{
			name:     "mcpServers null",
			existing: `{"globalShortcut": "Cmd+Shift+Space", "mcpServers": null}`,
			others:   `{"globalShortcut": "Cmd+Shift+Space", "mcpServers": {}}`,
		},
		{
			name:     "other servers with env secrets, large numbers and <>& characters",
			existing: otherServersConfig,
			others:   otherServersConfig,
			literals: []string{"ghp_secret123", "12345678901234567890", "a<b>&c"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home, folder := desktopHome(t)
			config := filepath.Join(folder, desktopConfigName)
			writeDesktopConfig(t, config, c.existing)
			var out, errOut bytes.Buffer

			err := installDesktop(t, &toolCalls{}, home, &out, &errOut)

			require.NoError(t, err)
			assert.Equal(t, marketplaceAddedLine+pluginInstalledLine+installRestartLine+desktopAddedLine+desktopQuitLine, out.String())
			assert.Empty(t, errOut.String())
			backup, err := os.ReadFile(filepath.Join(folder, desktopBackupName))
			require.NoError(t, err)
			assert.Equal(t, c.existing, string(backup))
			backupInfo, err := os.Stat(filepath.Join(folder, desktopBackupName))
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o600), backupInfo.Mode().Perm())
			configInfo, err := os.Stat(config)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o644), configInfo.Mode().Perm())
			written, err := os.ReadFile(config)
			require.NoError(t, err)
			others, entry := splitQuarryEntry(t, written)
			assert.Equal(t, decodeConfig(t, []byte(c.others)), others)
			assert.Equal(t, map[string]any{"command": desktopQuarryBinary, "args": []any{"mcp"}}, entry)
			for _, literal := range c.literals {
				assert.Contains(t, string(written), literal)
			}
			assert.Equal(t, []string{desktopConfigName, desktopBackupName}, entryNames(t, folder))
		})
	}
}

func Test_claude_install_reports_a_failed_backup_or_write_and_leaves_the_config_as_it_was(t *testing.T) {
	const existing = `{"globalShortcut": "Cmd+Shift+Space"}`
	cases := []struct {
		name     string
		fault    func(t *testing.T, folder string)
		wantLine string
		wantFold []string
	}{
		{
			name: "the backup cannot be saved",
			fault: func(t *testing.T, folder string) {
				t.Helper()
				require.NoError(t, os.MkdirAll(filepath.Join(folder, desktopBackupName, "kept"), 0o755))
			},
			wantLine: "quarry: claude install: cannot save " + desktopBackupShown + " (file exists), so " + desktopConfigShown +
				" is unchanged; check the permissions of " + desktopFolderShown + ", then run quarry claude install again\n",
			wantFold: []string{desktopConfigName, desktopBackupName},
		},
		{
			name: "the backup cannot be created in a read-only folder",
			fault: func(t *testing.T, folder string) {
				t.Helper()
				require.NoError(t, os.Chmod(folder, 0o555))
				t.Cleanup(func() { _ = os.Chmod(folder, 0o755) })
			},
			wantLine: "quarry: claude install: cannot save " + desktopBackupShown + " (permission denied), so " + desktopConfigShown +
				" is unchanged; check the permissions of " + desktopFolderShown + ", then run quarry claude install again\n",
			wantFold: []string{desktopConfigName},
		},
		{
			name: "the temp write or rename fails",
			fault: func(t *testing.T, folder string) {
				t.Helper()
				config := filepath.Join(folder, desktopConfigName)
				require.NoError(t, syscall.Chflags(config, userImmutableFlag))
				t.Cleanup(func() { _ = syscall.Chflags(config, 0) })
			},
			wantLine: "quarry: claude install: cannot write " + desktopConfigShown + " (operation not permitted), so it is unchanged; " +
				"check the permissions of " + desktopFolderShown + ", then run quarry claude install again\n",
			wantFold: []string{desktopConfigName, desktopBackupName},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home, folder := desktopHome(t)
			config := filepath.Join(folder, desktopConfigName)
			writeDesktopConfig(t, config, existing)
			c.fault(t, folder)
			before, err := os.ReadFile(config)
			require.NoError(t, err)
			var out, errOut bytes.Buffer

			err = installDesktop(t, &toolCalls{}, home, &out, &errOut)

			require.ErrorIs(t, err, cli.ReportedError{})
			assert.Equal(t, marketplaceAddedLine+pluginInstalledLine+installRestartLine, out.String())
			assert.Equal(t, c.wantLine, errOut.String())
			body, readErr := os.ReadFile(config)
			require.NoError(t, readErr)
			assert.Equal(t, before, body)
			info, statErr := os.Stat(config)
			require.NoError(t, statErr)
			assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
			assert.Equal(t, c.wantFold, entryNames(t, folder))
		})
	}
}

func desktopUpdatedLine(old string) string {
	return "Updated the quarry MCP server in Claude Desktop to start \"" + desktopQuarryBinary + "\" instead of \"" + old + "\".\n"
}

func Test_claude_install_repoints_a_stale_quarry_entry_and_keeps_its_env(t *testing.T) {
	cases := []struct {
		name string
		old  string
		said string
	}{
		{name: "another absolute path", old: "/usr/local/bin/quarry", said: "/usr/local/bin/quarry"},
		{name: "the bare name", old: "quarry", said: "quarry"},
		{name: "a path under home", old: "<home>/bin/quarry", said: "~/bin/quarry"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home, folder := desktopHome(t)
			config := filepath.Join(folder, desktopConfigName)
			existing := `{` + staleEntryRest + `, "quarry": {"command": "` + strings.ReplaceAll(c.old, "<home>", home) + `", ` + staleEntryTail + `}}}` + "\n"
			writeDesktopConfig(t, config, existing)
			var out, errOut bytes.Buffer

			err := installDesktop(t, &toolCalls{}, home, &out, &errOut)

			require.NoError(t, err)
			assert.Equal(t, marketplaceAddedLine+pluginInstalledLine+installRestartLine+desktopUpdatedLine(c.said)+desktopQuitLine, out.String())
			assert.Empty(t, errOut.String())
			backup, err := os.ReadFile(filepath.Join(folder, desktopBackupName))
			require.NoError(t, err)
			assert.Equal(t, existing, string(backup))
			info, err := os.Stat(config)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
			written, err := os.ReadFile(config)
			require.NoError(t, err)
			others, entry := splitQuarryEntry(t, written)
			assert.Equal(t, decodeConfig(t, []byte(`{`+staleEntryRest+`}}`)), others)
			assert.Equal(t, decodeConfig(t, []byte(`{"command": "`+desktopQuarryBinary+`", `+staleEntryTail+`}`)), entry)
			assert.Contains(t, string(written), "a<b>&c")
			assert.Equal(t, []string{desktopConfigName, desktopBackupName}, entryNames(t, folder))
		})
	}
}

func Test_claude_install_leaves_the_desktop_config_alone_when_it_already_starts_this_quarry(t *testing.T) {
	home, folder := desktopHome(t)
	config := filepath.Join(folder, desktopConfigName)
	existing := `{"mcpServers":{"quarry":{"command":"` + desktopQuarryBinary + `","args":["mcp"]}},"globalShortcut":"Cmd+Shift+Space"}`
	writeDesktopConfig(t, config, existing)
	var out, errOut bytes.Buffer

	err := installDesktop(t, &toolCalls{}, home, &out, &errOut)

	require.NoError(t, err)
	assert.Equal(t, marketplaceAddedLine+pluginInstalledLine+installRestartLine+desktopKeptLine, out.String())
	assert.Empty(t, errOut.String())
	body, err := os.ReadFile(config)
	require.NoError(t, err)
	assert.Equal(t, existing, string(body))
	assert.Equal(t, []string{desktopConfigName}, entryNames(t, folder))
}

func Test_claude_install_refuses_a_quarry_entry_that_does_not_run_quarry_mcp(t *testing.T) {
	home, folder := desktopHome(t)
	config := filepath.Join(folder, desktopConfigName)
	existing := `{"mcpServers": {"quarry": {"command": "/usr/local/bin/other-tool", "args": ["serve"]}}}` + "\n"
	writeDesktopConfig(t, config, existing)
	var out, errOut bytes.Buffer

	err := installDesktop(t, &toolCalls{}, home, &out, &errOut)

	require.ErrorIs(t, err, cli.ReportedError{})
	assert.Equal(t, marketplaceAddedLine+pluginInstalledLine+installRestartLine, out.String())
	assert.Equal(t, desktopForeignLine, errOut.String())
	body, readErr := os.ReadFile(config)
	require.NoError(t, readErr)
	assert.Equal(t, existing, string(body))
	assert.Equal(t, []string{desktopConfigName}, entryNames(t, folder))
}

const desktopRetryTail = ", then run quarry claude install again\n"

// desktopRefusalState is what a refused install must leave behind in the Claude folder.
type desktopRefusalState struct {
	entries []string
	mode    os.FileMode
	size    int64
	modTime time.Time
}

func desktopRefusalSnapshot(t *testing.T, folder string) desktopRefusalState {
	t.Helper()
	info, err := os.Lstat(filepath.Join(folder, desktopConfigName))
	require.NoError(t, err)
	return desktopRefusalState{entries: entryNames(t, folder), mode: info.Mode(), size: info.Size(), modTime: info.ModTime()}
}

func Test_claude_install_refuses_a_desktop_config_it_cannot_safely_change(t *testing.T) {
	const (
		backupBytes = `{"saved": "earlier backup"}`
		linkTarget  = "elsewhere.json"
		linkedBytes = `{"mcpServers": {}}`
	)
	backupBefore, linkedBefore, serversBefore, unreadableBefore := []byte(backupBytes), []byte(linkedBytes), []byte(`{"mcpServers": []}`), []byte(`{"globalShortcut": "x"}`)
	cases := []struct {
		name     string
		seed     func(t *testing.T, folder string)
		wantLine string
		verify   func(t *testing.T, folder string)
	}{
		{
			name: "the file is not valid JSON",
			seed: func(t *testing.T, folder string) {
				t.Helper()
				writeDesktopConfig(t, filepath.Join(folder, desktopConfigName), `{"globalShortcut": "x", "mcpServers": {`)
				require.NoError(t, os.WriteFile(filepath.Join(folder, desktopBackupName), []byte(backupBytes), 0o600))
			},
			wantLine: "quarry: claude install: cannot read " + desktopConfigShown +
				": it is not valid JSON (unexpected end of JSON input, at byte 39); fix it so Claude Desktop can read it too" + desktopRetryTail,
			verify: func(t *testing.T, folder string) {
				t.Helper()
				body, err := os.ReadFile(filepath.Join(folder, desktopBackupName))
				require.NoError(t, err)
				assert.Equal(t, backupBefore, body)
			},
		},
		{
			name: "the top level is an array",
			seed: func(t *testing.T, folder string) {
				t.Helper()
				writeDesktopConfig(t, filepath.Join(folder, desktopConfigName), `[1]`)
			},
			wantLine: "quarry: claude install: cannot add quarry to " + desktopConfigShown +
				": it holds a JSON array, not an object; fix it so Claude Desktop can read it too" + desktopRetryTail,
			verify: func(t *testing.T, folder string) {
				t.Helper()
				body, err := os.ReadFile(filepath.Join(folder, desktopConfigName))
				require.NoError(t, err)
				assert.Equal(t, []byte(`[1]`), body)
			},
		},
		{
			name: "mcpServers is an array",
			seed: func(t *testing.T, folder string) {
				t.Helper()
				writeDesktopConfig(t, filepath.Join(folder, desktopConfigName), `{"mcpServers": []}`)
			},
			wantLine: "quarry: claude install: cannot add quarry to " + desktopConfigShown +
				": its mcpServers is a JSON array, not an object; fix it so Claude Desktop can read it too" + desktopRetryTail,
			verify: func(t *testing.T, folder string) {
				t.Helper()
				body, err := os.ReadFile(filepath.Join(folder, desktopConfigName))
				require.NoError(t, err)
				assert.Equal(t, serversBefore, body)
			},
		},
		{
			name: "the file is a symbolic link",
			seed: func(t *testing.T, folder string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(folder, linkTarget), []byte(linkedBytes), 0o600))
				require.NoError(t, os.Symlink(linkTarget, filepath.Join(folder, desktopConfigName)))
			},
			wantLine: "quarry: claude install: " + desktopConfigShown + " is a symbolic link, so quarry leaves it alone; add " +
				`"quarry": {"command": "` + desktopQuarryBinary + `", "args": ["mcp"]}` +
				" under mcpServers in the file it links to yourself, then quit and reopen Claude Desktop\n",
			verify: func(t *testing.T, folder string) {
				t.Helper()
				target, err := os.Readlink(filepath.Join(folder, desktopConfigName))
				require.NoError(t, err)
				assert.Equal(t, linkTarget, target)
				body, err := os.ReadFile(filepath.Join(folder, linkTarget))
				require.NoError(t, err)
				assert.Equal(t, linkedBefore, body)
			},
		},
		{
			name: "the file is a named pipe",
			seed: func(t *testing.T, folder string) {
				t.Helper()
				require.NoError(t, syscall.Mkfifo(filepath.Join(folder, desktopConfigName), 0o600))
			},
			wantLine: "quarry: claude install: " + desktopConfigShown +
				" is not a file, so quarry leaves it alone; move it aside" + desktopRetryTail,
			verify: func(t *testing.T, folder string) {
				t.Helper()
				info, err := os.Lstat(filepath.Join(folder, desktopConfigName))
				require.NoError(t, err)
				assert.NotZero(t, info.Mode()&fs.ModeNamedPipe)
			},
		},
		{
			name: "the file cannot be read",
			seed: func(t *testing.T, folder string) {
				t.Helper()
				config := filepath.Join(folder, desktopConfigName)
				writeDesktopConfig(t, config, `{"globalShortcut": "x"}`)
				require.NoError(t, os.Chmod(config, 0o000))
				t.Cleanup(func() { _ = os.Chmod(config, 0o644) })
			},
			wantLine: "quarry: claude install: cannot read " + desktopConfigShown +
				" (permission denied); check its permissions" + desktopRetryTail,
			verify: func(t *testing.T, folder string) {
				t.Helper()
				config := filepath.Join(folder, desktopConfigName)
				require.NoError(t, os.Chmod(config, 0o644))
				body, err := os.ReadFile(config)
				require.NoError(t, err)
				assert.Equal(t, unreadableBefore, body)
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home, folder := desktopHome(t)
			c.seed(t, folder)
			before := desktopRefusalSnapshot(t, folder)
			var out, errOut bytes.Buffer

			err := installDesktop(t, &toolCalls{}, home, &out, &errOut)

			require.ErrorIs(t, err, cli.ReportedError{})
			assert.Equal(t, marketplaceAddedLine+pluginInstalledLine+installRestartLine, out.String())
			assert.Equal(t, c.wantLine, errOut.String())
			assert.Equal(t, before, desktopRefusalSnapshot(t, folder))
			c.verify(t, folder)
		})
	}
}

// desktopSymlinkLine is the symbolic-link refusal whose pasteable entry holds commandJSON, a JSON string.
func desktopSymlinkLine(commandJSON string) string {
	return "quarry: claude install: " + desktopConfigShown + " is a symbolic link, so quarry leaves it alone; add " +
		`"quarry": {"command": ` + commandJSON + `, "args": ["mcp"]}` +
		" under mcpServers in the file it links to yourself, then quit and reopen Claude Desktop\n"
}

// assertDesktopRefusal installs over a config holding body and asserts the refusal wantLine, with the
// config untouched and nothing else created.
func assertDesktopRefusal(t *testing.T, body, wantLine string) {
	t.Helper()
	home, folder := desktopHome(t)
	config := filepath.Join(folder, desktopConfigName)
	writeDesktopConfig(t, config, body)
	var out, errOut bytes.Buffer

	err := installDesktop(t, &toolCalls{}, home, &out, &errOut)

	require.ErrorIs(t, err, cli.ReportedError{})
	assert.Equal(t, marketplaceAddedLine+pluginInstalledLine+installRestartLine, out.String())
	assert.Equal(t, wantLine, errOut.String())
	written, readErr := os.ReadFile(config)
	require.NoError(t, readErr)
	assert.Equal(t, []byte(body), written)
	assert.Equal(t, []string{desktopConfigName}, entryNames(t, folder))
}

func Test_claude_install_names_the_byte_where_the_desktop_config_stops_being_json(t *testing.T) {
	const head = "quarry: claude install: cannot read " + desktopConfigShown + ": it is not valid JSON ("
	const tail = "); fix it so Claude Desktop can read it too" + desktopRetryTail
	cases := []struct {
		name string
		body string
		want string
	}{
		{name: "a byte order mark opens the file", body: "\ufeff{}", want: head + "invalid character '\\ufeff' looking for beginning of value, at byte 3" + tail},
		{name: "text follows the object", body: "{} x", want: head + "invalid character 'x' after top-level value, at byte 4" + tail},
		{name: "the file is not JSON at all", body: "not json", want: head + "invalid character 'o' in literal null (expecting 'u'), at byte 2" + tail},
		{name: "a comma ends the object", body: `{"a": 1,}`, want: head + "invalid character '}' looking for beginning of object key string, at byte 9" + tail},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assertDesktopRefusal(t, c.body, c.want)
		})
	}
}

func Test_claude_install_names_the_json_kind_when_the_desktop_config_is_not_an_object(t *testing.T) {
	const head = "quarry: claude install: cannot add quarry to " + desktopConfigShown + ": it holds a JSON "
	const tail = ", not an object; fix it so Claude Desktop can read it too" + desktopRetryTail
	cases := []struct {
		name string
		body string
		want string
	}{
		{name: "null", body: "null", want: head + "null" + tail},
		{name: "an array", body: `[{"mcpServers": {}}]`, want: head + "array" + tail},
		{name: "a string", body: `"mcpServers"`, want: head + "string" + tail},
		{name: "a number", body: "5", want: head + "number" + tail},
		{name: "true", body: "true", want: head + "boolean" + tail},
		{name: "false", body: "false", want: head + "boolean" + tail},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assertDesktopRefusal(t, c.body, c.want)
		})
	}
}

func Test_claude_install_names_the_json_kind_when_mcp_servers_is_not_an_object(t *testing.T) {
	const head = "quarry: claude install: cannot add quarry to " + desktopConfigShown + ": its mcpServers is a JSON "
	const tail = ", not an object; fix it so Claude Desktop can read it too" + desktopRetryTail
	cases := []struct {
		name string
		body string
		want string
	}{
		{name: "an array", body: `{"mcpServers": []}`, want: head + "array" + tail},
		{name: "a string", body: `{"mcpServers": "quarry"}`, want: head + "string" + tail},
		{name: "a number", body: `{"mcpServers": 5}`, want: head + "number" + tail},
		{name: "a boolean", body: `{"mcpServers": false}`, want: head + "boolean" + tail},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assertDesktopRefusal(t, c.body, c.want)
		})
	}
}

func Test_claude_install_prints_a_symbolic_link_refusal_whose_entry_is_pasteable_json(t *testing.T) {
	const exe = `/opt/a<b>&c"d\e/quarry`
	home, folder := desktopHome(t)
	require.NoError(t, os.Symlink(filepath.Join(folder, "elsewhere.json"), filepath.Join(folder, desktopConfigName)))
	var out, errOut bytes.Buffer

	err := installDesktopAs(t, &toolCalls{}, home, exe, &out, &errOut)

	require.ErrorIs(t, err, cli.ReportedError{})
	assert.Equal(t, desktopSymlinkLine(`"/opt/a<b>&c\"d\\e/quarry"`), errOut.String())
}

func Test_claude_install_prints_the_absolute_path_in_the_symbolic_link_refusal_for_a_binary_under_home(t *testing.T) {
	home, folder := desktopHome(t)
	exe := filepath.Join(home, "bin", "quarry")
	require.NoError(t, os.Symlink(filepath.Join(folder, "elsewhere.json"), filepath.Join(folder, desktopConfigName)))
	var out, errOut bytes.Buffer

	err := installDesktopAs(t, &toolCalls{}, home, exe, &out, &errOut)

	require.ErrorIs(t, err, cli.ReportedError{})
	assert.Equal(t, desktopSymlinkLine(`"`+exe+`"`), errOut.String())
}

func Test_claude_install_refuses_a_desktop_config_that_is_a_folder(t *testing.T) {
	home, folder := desktopHome(t)
	config := filepath.Join(folder, desktopConfigName)
	require.NoError(t, os.Mkdir(config, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(config, "inner"), []byte("kept"), 0o600))
	var out, errOut bytes.Buffer

	err := installDesktop(t, &toolCalls{}, home, &out, &errOut)

	require.ErrorIs(t, err, cli.ReportedError{})
	assert.Equal(t, marketplaceAddedLine+pluginInstalledLine+installRestartLine, out.String())
	assert.Equal(t, "quarry: claude install: "+desktopConfigShown+
		" is not a file, so quarry leaves it alone; move it aside"+desktopRetryTail, errOut.String())
	assert.Equal(t, []string{"inner"}, entryNames(t, config))
	assert.Equal(t, []string{desktopConfigName}, entryNames(t, folder))
}

func Test_claude_install_refuses_a_desktop_config_it_cannot_check_in_an_unsearchable_folder(t *testing.T) {
	home, folder := desktopHome(t)
	require.NoError(t, os.Chmod(folder, 0o000))
	t.Cleanup(func() { _ = os.Chmod(folder, 0o755) })
	var out, errOut bytes.Buffer

	err := installDesktop(t, &toolCalls{}, home, &out, &errOut)

	require.ErrorIs(t, err, cli.ReportedError{})
	assert.Equal(t, marketplaceAddedLine+pluginInstalledLine+installRestartLine, out.String())
	assert.Equal(t, "quarry: claude install: cannot read "+desktopConfigShown+
		" (permission denied); check its permissions"+desktopRetryTail, errOut.String())
}
