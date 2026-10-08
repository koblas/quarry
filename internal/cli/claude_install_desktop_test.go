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
	return cli.Execute(t.Context(), []string{"claude", "install"}, cli.Env{
		Stdout:     stdout,
		Stderr:     stderr,
		RunTool:    tool.run,
		Home:       home,
		Executable: func() (string, error) { return desktopQuarryBinary, nil },
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

func Test_claude_install_returns_the_desktop_error_unreported_when_the_config_is_not_a_regular_file(t *testing.T) {
	home, folder := desktopHome(t)
	config := filepath.Join(folder, desktopConfigName)
	require.NoError(t, os.Symlink(filepath.Join(folder, "elsewhere.json"), config))
	var out, errOut bytes.Buffer

	err := installDesktop(t, &toolCalls{}, home, &out, &errOut)

	require.ErrorIs(t, err, fs.ErrExist)
	require.NotErrorIs(t, err, cli.ReportedError{})
	assert.Equal(t, marketplaceAddedLine+pluginInstalledLine+installRestartLine, out.String())
	assert.Empty(t, errOut.String())
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
				assert.Equal(t, backupBytes, string(body))
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
				assert.Equal(t, `[1]`, string(body))
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
				assert.Equal(t, `{"mcpServers": []}`, string(body))
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
				assert.Equal(t, linkedBytes, string(body))
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
				assert.Equal(t, `{"globalShortcut": "x"}`, string(body))
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
