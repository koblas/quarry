package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_claude_install_installs_claude_desktop_after_a_claude_code_failure(t *testing.T) {
	cases := []struct {
		name       string
		tool       *toolCalls
		wantStderr string
	}{
		{
			name: "a foreign quarry marketplace",
			tool: (&toolCalls{}).lists(foreignMarketplace, "[]"),
			wantStderr: `quarry: claude install: Claude Code has a marketplace named "quarry" that is not koblas/quarry on GitHub; ` +
				"remove it with claude plugin marketplace remove quarry, then run quarry claude install again\n",
		},
		{
			name: "a step that exits non-zero",
			tool: (&toolCalls{}).reply(addMarketplaceArgv, toolReply{output: "denied\n", status: 1}),
			wantStderr: "denied\n" +
				"quarry: claude install: claude plugin marketplace add --scope user koblas/quarry exited with status 1; see its message above\n",
		},
		{
			name: "a plugin list that is not json",
			tool: (&toolCalls{}).lists(ourMarketplace, "not json"),
			wantStderr: "quarry: claude install: cannot read what claude plugin list --json printed; " +
				"update Claude Code, or run the two commands in quarry claude install --help yourself\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home, folder := desktopHome(t)
			var out, errOut bytes.Buffer

			err := installDesktop(t, c.tool, home, &out, &errOut)

			require.ErrorIs(t, err, cli.ReportedError{})
			assert.Equal(t, desktopAddedLine+desktopQuitLine, out.String())
			assert.Equal(t, c.wantStderr, errOut.String())
			written, err := os.ReadFile(filepath.Join(folder, desktopConfigName))
			require.NoError(t, err)
			var decoded map[string]any
			require.NoError(t, json.Unmarshal(written, &decoded))
			assert.Equal(t, map[string]any{
				"mcpServers": map[string]any{
					"quarry": map[string]any{"command": desktopQuarryBinary, "args": []any{"mcp"}},
				},
			}, decoded)
		})
	}
}

const (
	desktopStoppedInstallLine   = "quarry: claude install: stopped before quarry changed Claude Desktop; run quarry claude install again\n"
	desktopStoppedUninstallLine = "quarry: claude uninstall: stopped before quarry changed Claude Desktop; run quarry claude uninstall again\n"

	desktopOurEntryConfig = `{"mcpServers":{"quarry":{"command":"` + desktopQuarryBinary + `","args":["mcp"]}}}`
)

// runCancellable runs quarry claude verb over home under a context the tool's cancel reply can end.
func runCancellable(t *testing.T, tool *toolCalls, home, verb string) (string, string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	tool.cancel = cancel
	if tool.cancelBefore {
		cancel()
	}

	err := cli.Execute(ctx, []string{"claude", verb}, cli.Env{
		Stdout:     &out,
		Stderr:     &errOut,
		RunTool:    tool.run,
		Home:       home,
		Executable: func() (string, error) { return desktopQuarryBinary, nil },
	})

	return out.String(), errOut.String(), err
}

func Test_claude_install_reports_d13_when_interrupted_after_claude_code_finished(t *testing.T) {
	home, folder := desktopHome(t)
	tool := (&toolCalls{}).lists(ourMarketplace, "[]").reply(installPluginArgv, toolReply{cancel: true})

	stdout, stderr, err := runCancellable(t, tool, home, "install")

	require.ErrorIs(t, err, cli.ReportedError{})
	assert.Equal(t, marketplacePresentLine+pluginInstalledLine+installRestartLine, stdout)
	assert.Equal(t, desktopStoppedInstallLine, stderr)
	listing, err := os.ReadDir(folder)
	require.NoError(t, err)
	assert.Empty(t, listing)
}

func Test_claude_uninstall_reports_d13_when_interrupted_after_claude_code_finished(t *testing.T) {
	home, folder := desktopHome(t)
	config := filepath.Join(folder, desktopConfigName)
	require.NoError(t, os.WriteFile(config, []byte(desktopOurEntryConfig), 0o600))
	tool := (&toolCalls{}).lists(ourMarketplace, userPluginOn).reply(removeMarketplaceArgv, toolReply{cancel: true})

	stdout, stderr, err := runCancellable(t, tool, home, "uninstall")

	require.ErrorIs(t, err, cli.ReportedError{})
	assert.Equal(t, pluginUninstalledLine+marketplaceRemovedLine+uninstallRestartLine, stdout)
	assert.Equal(t, desktopStoppedUninstallLine, stderr)
	kept, err := os.ReadFile(config)
	require.NoError(t, err)
	assert.JSONEq(t, desktopOurEntryConfig, string(kept))
	listing, err := os.ReadDir(folder)
	require.NoError(t, err)
	assert.Len(t, listing, 1)
}

func Test_claude_install_prints_both_failures_in_target_order(t *testing.T) {
	home, folder := desktopHome(t)
	foreign := `{"mcpServers":{"quarry":{"command":"/usr/bin/other","args":[]}}}`
	require.NoError(t, os.WriteFile(filepath.Join(folder, desktopConfigName), []byte(foreign), 0o600))
	tool := (&toolCalls{}).reply(addMarketplaceArgv, toolReply{output: "denied\n", status: 1})
	var shared bytes.Buffer

	err := installDesktop(t, tool, home, &shared, &shared)

	require.ErrorIs(t, err, cli.ReportedError{})
	assert.Equal(t, "denied\n"+
		"quarry: claude install: claude plugin marketplace add --scope user koblas/quarry exited with status 1; see its message above\n"+
		desktopForeignLine, shared.String())
	kept, err := os.ReadFile(filepath.Join(folder, desktopConfigName))
	require.NoError(t, err)
	assert.Equal(t, foreign, string(kept))
}

func Test_claude_install_does_not_touch_claude_desktop_after_an_interrupt_during_a_code_step(t *testing.T) {
	cases := []struct {
		name       string
		tool       *toolCalls
		wantStdout string
		wantAfter  string
	}{
		{
			name:       "during a step",
			tool:       (&toolCalls{}).reply(installPluginArgv, toolReply{cancel: true, ctxErr: true}),
			wantStdout: marketplaceAddedLine,
			wantAfter:  "claude plugin install --scope user quarry@quarry",
		},
		{
			name:      "before the command runs",
			tool:      &toolCalls{cancelBefore: true},
			wantAfter: "claude plugin marketplace list --json",
		},
		{
			name:      "between two children",
			tool:      (&toolCalls{}).reply(pluginListArgv, toolReply{output: "[]", cancel: true}),
			wantAfter: "claude plugin marketplace add --scope user koblas/quarry",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home, folder := desktopHome(t)

			stdout, stderr, err := runCancellable(t, c.tool, home, "install")

			require.ErrorIs(t, err, cli.ReportedError{})
			assert.Equal(t, c.wantStdout, stdout)
			assert.Equal(t, "quarry: claude install: stopped before "+c.wantAfter+" finished; run quarry claude install again\n", stderr)
			listing, err := os.ReadDir(folder)
			require.NoError(t, err)
			assert.Empty(t, listing)
		})
	}
}

func Test_claude_install_returns_the_stdout_write_error_of_claude_desktop_after_a_claude_code_failure(t *testing.T) {
	home, _ := desktopHome(t)
	tool := (&toolCalls{}).reply(installPluginArgv, toolReply{status: 1})
	stdout := &failsAfterFirstWrite{err: errPipeClosed}

	err := installDesktop(t, tool, home, stdout, &bytes.Buffer{})

	require.ErrorIs(t, err, errPipeClosed)
	assert.NotErrorIs(t, err, cli.ReportedError{})
}
