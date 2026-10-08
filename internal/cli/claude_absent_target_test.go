package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/claudeplugin"
	"github.com/koblas/quarry/internal/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	codeSkippedLine         = "Skipped Claude Code: no claude command on your PATH.\n"
	desktopNotAFolderLine   = "Skipped Claude Desktop: " + desktopFolderShown + " is not a folder.\n"
	neitherPresentHead      = "found neither Claude Code (no claude command on your PATH) nor Claude Desktop ("
	neitherInstallTail      = "); install either one, open it once, then run quarry claude install again\n"
	neitherUninstallTail    = "), so there is nothing to uninstall; if claude is installed, add it to your PATH, then run quarry claude uninstall again\n"
	desktopFolderCheckFault = "cannot check for Claude Desktop at " + desktopFolderShown + " (permission denied); check its permissions, then run quarry claude "
)

// runWithDesktop runs quarry claude args over tool with the process's claude lookup and home, and
// the fixed quarry binary.
func runWithDesktop(t *testing.T, tool *toolCalls, lookPath claudeplugin.LookPath, home string, args ...string) (string, string, error) {
	t.Helper()
	var out, errOut bytes.Buffer

	err := cli.Execute(t.Context(), append([]string{"claude"}, args...), cli.Env{
		Stdout:     &out,
		Stderr:     &errOut,
		RunTool:    tool.run,
		LookPath:   lookPath,
		Home:       home,
		Executable: func() (string, error) { return desktopQuarryBinary, nil },
	})
	return out.String(), errOut.String(), err
}

// homeWithClaudeAsFile returns a home whose Claude Desktop path is a regular file.
func homeWithClaudeAsFile(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	support := filepath.Join(home, "Library", "Application Support")
	require.NoError(t, os.MkdirAll(support, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(support, "Claude"), []byte("not a folder"), 0o600))
	return home
}

// homeWithUnsearchableSupport returns a home whose Claude Desktop folder exists but cannot be
// stat'ed, because the folder above it denies search.
func homeWithUnsearchableSupport(t *testing.T) string {
	t.Helper()
	home, folder := desktopHome(t)
	support := filepath.Dir(folder)
	require.NoError(t, os.Chmod(support, 0o000))
	t.Cleanup(func() { _ = os.Chmod(support, 0o755) })
	return home
}

// homeWithQuarryEntry returns a home whose Claude Desktop config holds quarry's own entry.
func homeWithQuarryEntry(t *testing.T) string {
	t.Helper()
	home, folder := desktopHome(t)
	writeDesktopConfig(t, filepath.Join(folder, desktopConfigName), `{"mcpServers": {`+quarryEntry+`}}`)
	return home
}

func Test_claude_skips_the_absent_target_and_handles_the_present_one(t *testing.T) {
	cases := []struct {
		name     string
		verb     string
		tool     *toolCalls
		lookPath claudeplugin.LookPath
		home     func(t *testing.T) string
		want     string
	}{
		{
			name: "install without claude adds Claude Desktop", verb: "install", tool: &toolCalls{},
			lookPath: findsAllBut("claude"), home: func(t *testing.T) string { h, _ := desktopHome(t); return h },
			want: codeSkippedLine + desktopAddedLine + desktopQuitLine,
		},
		{
			name: "uninstall without claude removes from Claude Desktop", verb: "uninstall", tool: &toolCalls{},
			lookPath: findsAllBut("claude"), home: homeWithQuarryEntry,
			want: codeSkippedLine + desktopRemovedLine + desktopQuitUnloadLine,
		},
		{
			name: "install skips Claude Desktop when Claude is a file", verb: "install", tool: &toolCalls{},
			home: homeWithClaudeAsFile,
			want: marketplaceAddedLine + pluginInstalledLine + installRestartLine + desktopNotAFolderLine,
		},
		{
			name: "install skips a Claude Desktop folder that does not exist", verb: "install", tool: &toolCalls{},
			home: func(t *testing.T) string { return t.TempDir() },
			want: marketplaceAddedLine + pluginInstalledLine + installRestartLine + desktopSkippedLine,
		},
		{
			name: "uninstall skips a Claude Desktop folder that does not exist", verb: "uninstall",
			tool: (&toolCalls{}).lists(ourMarketplace, userPluginOn),
			home: func(t *testing.T) string { return t.TempDir() },
			want: codeUninstalledLines + desktopSkippedLine,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stdout, stderr, err := runWithDesktop(t, c.tool, c.lookPath, c.home(t), c.verb)

			require.NoError(t, err)
			assert.Equal(t, c.want, stdout)
			assert.Empty(t, stderr)
		})
	}
}

func Test_claude_refuses_when_neither_claude_code_nor_claude_desktop_is_present(t *testing.T) {
	cases := []struct {
		name string
		verb string
		home func(t *testing.T) string
		want string
	}{
		{
			name: "install, Claude Desktop folder missing", verb: "install",
			home: func(t *testing.T) string { return t.TempDir() },
			want: "quarry: claude install: " + neitherPresentHead + desktopFolderShown + " does not exist" + neitherInstallTail,
		},
		{
			name: "uninstall, Claude Desktop folder missing", verb: "uninstall",
			home: func(t *testing.T) string { return t.TempDir() },
			want: "quarry: claude uninstall: " + neitherPresentHead + desktopFolderShown + " does not exist" + neitherUninstallTail,
		},
		{
			name: "install, Claude is a file", verb: "install", home: homeWithClaudeAsFile,
			want: "quarry: claude install: " + neitherPresentHead + desktopFolderShown + " is not a folder" + neitherInstallTail,
		},
		{
			name: "uninstall, Claude is a file", verb: "uninstall", home: homeWithClaudeAsFile,
			want: "quarry: claude uninstall: " + neitherPresentHead + desktopFolderShown + " is not a folder" + neitherUninstallTail,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tool := &toolCalls{}

			stdout, stderr, err := runWithDesktop(t, tool, findsAllBut("claude"), c.home(t), c.verb)

			require.ErrorIs(t, err, cli.ReportedError{})
			assert.Empty(t, stdout)
			assert.Equal(t, c.want, stderr)
			assert.Empty(t, tool.argv)
		})
	}
}

func Test_claude_refuses_a_claude_desktop_it_cannot_look_for(t *testing.T) {
	cases := []struct {
		name       string
		verb       string
		tool       *toolCalls
		lookPath   claudeplugin.LookPath
		home       func(t *testing.T) string
		wantStdout string
		wantStderr string
	}{
		{
			name: "install without a home", verb: "install", tool: &toolCalls{},
			home:       func(*testing.T) string { return "" },
			wantStdout: marketplaceAddedLine + pluginInstalledLine + installRestartLine, wantStderr: desktopNoHomeLine,
		},
		{
			name: "uninstall with an unsearchable folder", verb: "uninstall",
			tool: (&toolCalls{}).lists(ourMarketplace, userPluginOn), home: homeWithUnsearchableSupport,
			wantStdout: codeUninstalledLines,
			wantStderr: "quarry: claude uninstall: " + desktopFolderCheckFault + "uninstall again\n",
		},
		{
			name: "install with an unsearchable folder", verb: "install", tool: &toolCalls{}, home: homeWithUnsearchableSupport,
			wantStdout: marketplaceAddedLine + pluginInstalledLine + installRestartLine,
			wantStderr: "quarry: claude install: " + desktopFolderCheckFault + "install again\n",
		},
		{
			name: "install without claude or a home", verb: "install", tool: &toolCalls{}, lookPath: findsAllBut("claude"),
			home:       func(*testing.T) string { return "" },
			wantStdout: codeSkippedLine, wantStderr: desktopNoHomeLine,
		},
		{
			name: "install without claude and with an unsearchable folder", verb: "install", tool: &toolCalls{},
			lookPath: findsAllBut("claude"), home: homeWithUnsearchableSupport,
			wantStdout: codeSkippedLine,
			wantStderr: "quarry: claude install: " + desktopFolderCheckFault + "install again\n",
		},
		{
			name: "uninstall without claude and with an unsearchable folder", verb: "uninstall", tool: &toolCalls{},
			lookPath: findsAllBut("claude"), home: homeWithUnsearchableSupport,
			wantStdout: codeSkippedLine,
			wantStderr: "quarry: claude uninstall: " + desktopFolderCheckFault + "uninstall again\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := c.home(t)

			stdout, stderr, err := runWithDesktop(t, c.tool, c.lookPath, home, c.verb)

			require.ErrorIs(t, err, cli.ReportedError{})
			assert.Equal(t, c.wantStdout, stdout)
			assert.Equal(t, c.wantStderr, stderr)
		})
	}
}
