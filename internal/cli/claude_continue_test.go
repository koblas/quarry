package cli_test

import (
	"bytes"
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
