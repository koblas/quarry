package cli_test

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/koblas/quarry/internal/claudeplugin"
	"github.com/koblas/quarry/internal/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	absentTempRoot = "/quarry-test-temp-that-does-not-exist"

	codeLines = marketplaceAddedLine + pluginInstalledLine + installRestartLine
)

// outsideTemp points the temporary directory at an absent absolute path, so a home made by
// t.TempDir before this call is not under it. Call it after desktopHome.
func outsideTemp(t *testing.T) {
	t.Helper()
	t.Setenv("TMPDIR", absentTempRoot)
}

// findsQuarryAt is a LookPath that resolves quarry to path and every other command under /opt/bin.
func findsQuarryAt(path string) claudeplugin.LookPath {
	return func(file string) (string, error) {
		if file == "quarry" {
			return path, nil
		}
		return "/opt/bin/" + file, nil
	}
}

// installDesktopFinding runs quarry claude install with the running quarry at exe and lookPath as the
// process's command lookup.
func installDesktopFinding(t *testing.T, home, exe string, lookPath claudeplugin.LookPath, stdout, stderr io.Writer) error {
	t.Helper()
	return cli.Execute(t.Context(), []string{"claude", "install"}, cli.Env{
		Stdout:     stdout,
		Stderr:     stderr,
		RunTool:    (&toolCalls{}).run,
		LookPath:   lookPath,
		Home:       home,
		Executable: func() (string, error) { return exe, nil },
	})
}

// pathQuarryFile writes a quarry that is not the running one under home/bin and returns its path.
func pathQuarryFile(t *testing.T, home string) string {
	t.Helper()
	path := filepath.Join(home, "bin", "quarry")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"), 0o600))
	return path
}

const desktopPathWarning = "quarry: claude install: warning: Claude Desktop starts \"" + desktopQuarryBinary +
	"\", but the quarry on your PATH is \"~/bin/quarry\"; " +
	"run quarry claude install with the quarry you want Claude Desktop to start\n"

func Test_claude_install_warns_when_the_quarry_on_path_is_not_the_one_desktop_will_start(t *testing.T) {
	cases := []struct {
		name       string
		config     string
		wantStdout string
	}{
		{name: "a new entry", config: "", wantStdout: codeLines + desktopAddedLine + desktopQuitLine},
		{
			name: "an entry started from elsewhere", config: `{"mcpServers":{"quarry":{"command":"/usr/local/bin/quarry","args":["mcp"]}}}`,
			wantStdout: codeLines + desktopUpdatedLine("/usr/local/bin/quarry") + desktopQuitLine,
		},
		{
			name: "an entry that already starts this quarry", config: `{"mcpServers":{"quarry":{"command":"` + desktopQuarryBinary + `","args":["mcp"]}}}`,
			wantStdout: codeLines + desktopKeptLine,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home, folder := desktopHome(t)
			outsideTemp(t)
			onPath := pathQuarryFile(t, home)
			if c.config != "" {
				writeDesktopConfig(t, filepath.Join(folder, desktopConfigName), c.config)
			}
			var out, errOut bytes.Buffer

			err := installDesktopFinding(t, home, desktopQuarryBinary, findsQuarryAt(onPath), &out, &errOut)

			require.NoError(t, err)
			assert.Equal(t, c.wantStdout, out.String())
			assert.Equal(t, desktopPathWarning, errOut.String())
		})
	}
}

func Test_claude_install_escapes_quotes_backslashes_and_control_bytes_in_the_path_warning(t *testing.T) {
	home, _ := desktopHome(t)
	outsideTemp(t)
	const running = "/opt/q\"uote\\back\x01ctl/quarry"
	onPath := filepath.Join(home, "bin", "p\"q\\r\x01s", "quarry")
	require.NoError(t, os.MkdirAll(filepath.Dir(onPath), 0o755))
	require.NoError(t, os.WriteFile(onPath, []byte("#!/bin/sh\n"), 0o600))
	var out, errOut bytes.Buffer

	err := installDesktopFinding(t, home, running, findsQuarryAt(onPath), &out, &errOut)

	require.NoError(t, err)
	assert.Equal(t, `quarry: claude install: warning: Claude Desktop starts "/opt/q\"uote\\back\x01ctl/quarry", `+
		`but the quarry on your PATH is "~/bin/p\"q\\r\x01s/quarry"; `+
		"run quarry claude install with the quarry you want Claude Desktop to start\n", errOut.String())
}

func Test_claude_install_does_not_warn_when_the_quarry_on_path_cannot_be_checked(t *testing.T) {
	home, _ := desktopHome(t)
	outsideTemp(t)
	var out, errOut bytes.Buffer

	err := installDesktopFinding(t, home, desktopQuarryBinary, findsQuarryAt(filepath.Join(home, "bin", "missing")), &out, &errOut)

	require.NoError(t, err)
	assert.Equal(t, codeLines+desktopAddedLine+desktopQuitLine, out.String())
	assert.Empty(t, errOut.String())
}

func Test_claude_install_does_not_warn_when_the_shell_finds_no_quarry(t *testing.T) {
	home, _ := desktopHome(t)
	var out, errOut bytes.Buffer

	err := installDesktopFinding(t, home, desktopQuarryBinary, findsAllBut("quarry"), &out, &errOut)

	require.NoError(t, err)
	assert.Equal(t, codeLines+desktopAddedLine+desktopQuitLine, out.String())
	assert.Equal(t, `quarry: claude install: warning: the plugin starts "quarry" from your PATH, and your PATH has none; `+
		"add the directory holding quarry to your PATH\n", errOut.String())
}

func Test_claude_install_does_not_warn_when_claude_desktop_is_missing(t *testing.T) {
	home := t.TempDir()
	outsideTemp(t)
	onPath := pathQuarryFile(t, home)
	var out, errOut bytes.Buffer

	err := installDesktopFinding(t, home, desktopQuarryBinary, findsQuarryAt(onPath), &out, &errOut)

	require.NoError(t, err)
	assert.Equal(t, codeLines+desktopSkippedLine, out.String())
	assert.Empty(t, errOut.String())
}

func Test_claude_install_does_not_warn_when_it_refuses_the_desktop_config(t *testing.T) {
	home, folder := desktopHome(t)
	outsideTemp(t)
	onPath := pathQuarryFile(t, home)
	require.NoError(t, os.Symlink(filepath.Join(folder, "elsewhere.json"), filepath.Join(folder, desktopConfigName)))
	var out, errOut bytes.Buffer

	err := installDesktopFinding(t, home, desktopQuarryBinary, findsQuarryAt(onPath), &out, &errOut)

	require.ErrorIs(t, err, cli.ReportedError{})
	assert.Equal(t, desktopSymlinkLine(`"`+desktopQuarryBinary+`"`), errOut.String())
}

func Test_claude_install_starts_the_quarry_on_path_when_it_is_the_running_binary_by_another_name(t *testing.T) {
	home, folder := desktopHome(t)
	outsideTemp(t)
	cellar := filepath.Join(home, "Cellar", "quarry", "1.0", "bin", "quarry")
	require.NoError(t, os.MkdirAll(filepath.Dir(cellar), 0o755))
	require.NoError(t, os.WriteFile(cellar, []byte("#!/bin/sh\n"), 0o600))
	link := filepath.Join(home, "bin", "quarry")
	require.NoError(t, os.MkdirAll(filepath.Dir(link), 0o755))
	require.NoError(t, os.Symlink(cellar, link))
	var out, errOut bytes.Buffer

	err := installDesktopFinding(t, home, cellar, findsQuarryAt(link), &out, &errOut)

	require.NoError(t, err)
	assert.Equal(t, codeLines+"Added the quarry MCP server to Claude Desktop; it starts \"~/bin/quarry\".\n"+desktopQuitLine, out.String())
	assert.Empty(t, errOut.String())
	written, err := os.ReadFile(filepath.Join(folder, desktopConfigName))
	require.NoError(t, err)
	assert.Contains(t, string(written), link)
	assert.NotContains(t, string(written), "Cellar")
}

func Test_claude_install_refuses_a_temporary_quarry_binary_and_leaves_desktop_alone(t *testing.T) {
	cases := []struct {
		name string
		temp string
		exe  string
	}{
		{name: "under the temporary directory", temp: "/quarry-test-temp", exe: "/quarry-test-temp/quarry"},
		{name: "in a go run build directory", temp: absentTempRoot, exe: "/var/folders/xy/go-build123/b001/exe/quarry"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home, folder := desktopHome(t)
			t.Setenv("TMPDIR", c.temp)
			var out, errOut bytes.Buffer

			err := installDesktopAs(t, &toolCalls{}, home, c.exe, &out, &errOut)

			require.ErrorIs(t, err, cli.ReportedError{})
			assert.Equal(t, codeLines, out.String())
			assert.Equal(t, "quarry: claude install: this quarry runs from a temporary build (\""+c.exe+"\"), "+
				"which will be gone when Claude Desktop starts it; run quarry claude install from an installed quarry, not go run\n", errOut.String())
			assert.Empty(t, entryNames(t, folder))
		})
	}
}

func Test_claude_install_escapes_quotes_backslashes_and_control_bytes_in_the_temporary_build_refusal(t *testing.T) {
	home, _ := desktopHome(t)
	outsideTemp(t)
	var out, errOut bytes.Buffer

	err := installDesktopAs(t, &toolCalls{}, home, "/opt/go-build/q\"uote\\back\x01ctl/quarry", &out, &errOut)

	require.ErrorIs(t, err, cli.ReportedError{})
	assert.Equal(t, `quarry: claude install: this quarry runs from a temporary build ("/opt/go-build/q\"uote\\back\x01ctl/quarry"), `+
		"which will be gone when Claude Desktop starts it; run quarry claude install from an installed quarry, not go run\n", errOut.String())
}

func Test_claude_install_prints_a_temporary_binary_under_home_as_tilde(t *testing.T) {
	home, folder := desktopHome(t)
	var out, errOut bytes.Buffer

	err := installDesktopAs(t, &toolCalls{}, home, filepath.Join(home, "build", "quarry"), &out, &errOut)

	require.ErrorIs(t, err, cli.ReportedError{})
	assert.Equal(t, "quarry: claude install: this quarry runs from a temporary build (\"~/build/quarry\"), "+
		"which will be gone when Claude Desktop starts it; run quarry claude install from an installed quarry, not go run\n", errOut.String())
	assert.Empty(t, entryNames(t, folder))
}

func Test_claude_install_names_the_config_to_edit_when_it_cannot_find_its_own_binary(t *testing.T) {
	home, folder := desktopHome(t)
	var out, errOut bytes.Buffer
	failure := &fs.PathError{Op: "readlink", Path: "/proc/self/exe", Err: syscall.ENOENT}

	err := cli.Execute(t.Context(), []string{"claude", "install"}, cli.Env{
		Stdout:     &out,
		Stderr:     &errOut,
		RunTool:    (&toolCalls{}).run,
		Home:       home,
		Executable: func() (string, error) { return "", failure },
	})

	require.ErrorIs(t, err, cli.ReportedError{})
	require.NotErrorIs(t, err, failure)
	assert.Equal(t, codeLines, out.String())
	assert.Equal(t, "quarry: claude install: cannot tell where this quarry binary is (no such file or directory), "+
		"so quarry cannot add it to Claude Desktop; add \"quarry\": {\"command\": \"<the path command -v quarry prints>\", \"args\": [\"mcp\"]} "+
		"under mcpServers in "+desktopConfigShown+" yourself\n", errOut.String())
	assert.Empty(t, entryNames(t, folder))
}
