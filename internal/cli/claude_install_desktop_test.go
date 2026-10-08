package cli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	desktopConfigName   = "claude_desktop_config.json"
	desktopQuarryBinary = "/opt/homebrew/bin/quarry"

	desktopAddedLine = "Added the quarry MCP server to Claude Desktop; it starts \"" + desktopQuarryBinary + "\".\n"
	desktopQuitLine  = "Quit and reopen Claude Desktop to load it.\n"

	desktopNoHomeLine = "quarry: claude install: cannot find your home directory ($HOME is not set), " +
		"so quarry cannot look for Claude Desktop; set HOME, then run quarry claude install again\n"
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

func Test_claude_install_returns_the_desktop_error_unreported_when_a_config_is_already_present(t *testing.T) {
	home, folder := desktopHome(t)
	config := filepath.Join(folder, desktopConfigName)
	require.NoError(t, os.WriteFile(config, []byte(`{"mcpServers":{}}`), 0o600))
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
