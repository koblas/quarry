// Package docs_test checks the documentation site against the code without
// building it: every quarry command and flag that --help prints has an entry
// on reference/cli.md, every MCP tool and parameter one on
// reference/mcp-tools.md, the Claude guide carries every `claude plugin` line
// quarry claude install and uninstall run, every public page is in mkdocs.yml's nav, no public page
// cites an internal reference, and no page is named so that macOS's
// case-insensitive filesystem reads it as a CLAUDE.md. internal/config checks
// reference/config.md against the keys it reads.
package docs_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// internalDirs are the directories under docs/ that mkdocs.yml's exclude_docs keeps off the site.
var internalDirs = []string{"adr", "prior-art", "product", "security", "specs"}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)

	return string(b)
}

// publicPages is every Markdown page the site publishes, relative to docs/.
func publicPages(t *testing.T) []string {
	t.Helper()
	var pages []string
	err := filepath.WalkDir(".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && slices.Contains(internalDirs, p) {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(p, ".md") {
			pages = append(pages, filepath.ToSlash(p))
		}

		return nil
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(pages), 5, "found too few public pages")

	return pages
}

func Test_every_public_page_is_in_the_nav(t *testing.T) {
	nav := read(t, "../mkdocs.yml")

	for _, p := range publicPages(t) {
		entry := regexp.MustCompile(`(?m)(^|[\s:])` + regexp.QuoteMeta(p) + `\s*$`)
		assert.Regexp(t, entry, nav, "docs/%s is not in mkdocs.yml nav", p)
	}
}

func Test_no_page_is_named_claude_md(t *testing.T) {
	err := filepath.WalkDir(".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		assert.NotEqual(t, "claude.md", strings.ToLower(d.Name()), "docs/%s is read as a CLAUDE.md on macOS", p)

		return nil
	})
	require.NoError(t, err)
}

// internalRefs belong to the design documents, not the site: spec sections, scenario ids,
// ADR numbers and not-built marks.
var internalRefs = []*regexp.Regexp{
	regexp.MustCompile(`§`),
	regexp.MustCompile(`\bSCENARIO-\d+\b`),
	regexp.MustCompile(`\bADR[- ]?\d+\b`),
	regexp.MustCompile(`not built: #\d+`),
	regexp.MustCompile(`\bdocs/(specs|product|adr)/`),
}

func Test_no_public_page_cites_an_internal_reference(t *testing.T) {
	for _, p := range publicPages(t) {
		for i, line := range strings.Split(read(t, p), "\n") {
			for _, re := range internalRefs {
				assert.NotRegexp(t, re, line, "docs/%s:%d cites an internal reference", p, i+1)
			}
		}
	}
}

// command is one quarry command as --help shows it: its words after "quarry" and its own flags.
type command struct {
	path  []string
	flags []string
}

// help is what quarry <path> --help prints.
func help(t *testing.T, path []string) string {
	t.Helper()
	var out bytes.Buffer
	err := cli.Execute(t.Context(), append(slices.Clone(path), "--help"), cli.Env{Stdout: &out, Stderr: &out})
	require.NoError(t, err, "quarry %s --help", strings.Join(path, " "))

	return out.String()
}

// section is the lines of help under the heading line title, up to the first blank line.
func section(help, title string) []string {
	_, after, found := strings.Cut(help, "\n"+title+"\n")
	if !found {
		return nil
	}
	body, _, _ := strings.Cut(after, "\n\n")

	return strings.Split(body, "\n")
}

var flagName = regexp.MustCompile(`--([a-z][a-z-]*)`)

// commands walks the tree under path, depth first, path itself included.
func commands(t *testing.T, path []string) []command {
	t.Helper()
	text := help(t, path)
	cmd := command{path: path}
	for _, line := range section(text, "Flags:") {
		if m := flagName.FindStringSubmatch(line); m != nil && m[1] != "help" {
			cmd.flags = append(cmd.flags, m[1])
		}
	}
	all := []command{cmd}
	for _, line := range section(text, "Available Commands:") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] == "help" {
			continue
		}
		all = append(all, commands(t, append(slices.Clone(path), fields[0]))...)
	}

	return all
}

// pageSection is the text of page from the heading line heading to the next heading of any level.
func pageSection(page, heading string) (string, bool) {
	_, after, found := strings.Cut(page, "\n"+heading+"\n")
	if !found {
		return "", false
	}
	if i := strings.Index(after, "\n#"); i >= 0 {
		after = after[:i]
	}

	return after, true
}

func Test_every_command_and_flag_is_on_the_cli_reference(t *testing.T) {
	page := read(t, "reference/cli.md")
	all := commands(t, []string{})
	require.GreaterOrEqual(t, len(all), 15, "found too few commands")

	for _, cmd := range all {
		heading := "### `quarry " + strings.Join(cmd.path, " ") + "`"
		if len(cmd.path) == 0 {
			heading = "## Global flags"
		}
		text, found := pageSection(page, heading)
		if !assert.True(t, found, "docs/reference/cli.md has no heading %s", heading) {
			continue
		}
		for _, flag := range cmd.flags {
			assert.Contains(t, text, "`--"+flag+"`", "%s: flag --%s is not under its heading", heading, flag)
		}
	}
}

func Test_the_claude_guide_carries_every_claude_plugin_command_the_cli_runs(t *testing.T) {
	guide := read(t, "guides/use-with-claude.md")
	var lines []string
	for _, name := range []string{"install", "uninstall"} {
		for line := range strings.SplitSeq(help(t, []string{"claude", name}), "\n") {
			if strings.HasPrefix(line, "  claude plugin ") {
				lines = append(lines, strings.TrimSpace(line))
			}
		}
	}
	require.Len(t, lines, 4)

	for _, line := range lines {
		assert.Regexp(t, `(?m)^\s*`+regexp.QuoteMeta(line)+`$`, guide, "docs/guides/use-with-claude.md does not carry %q", line)
	}
}
