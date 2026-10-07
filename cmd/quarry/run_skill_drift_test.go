package main

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// driftSource is one file the skill ships, by repo-relative name.
type driftSource struct {
	name string
	text string
}

// driftCheck is what one kind of name check saw: the names it resolved against
// the product, and the uses that matched nothing.
type driftCheck struct {
	resolved   []string
	mismatches []string
}

// helpNode is one command in the help tree: its long flag names and its children.
type helpNode struct {
	flags    []string
	children map[string]helpNode
}

// skillDriftSources is SKILL.md, the README's Claude Code section, and every file
// under references/, read from disk.
func skillDriftSources(t *testing.T) []driftSource {
	t.Helper()
	return append([]driftSource{
		{name: skillPath, text: repoFile(t, skillPath)},
		{name: "README.md Claude Code section", text: readmeSection(t, readmeClaudeCodeHeading)},
	}, referenceSources(t)...)
}

// referenceSources is every file under references/, listed from disk and named by repo-relative path.
func referenceSources(t *testing.T) []driftSource {
	t.Helper()
	var sources []driftSource
	err := filepath.WalkDir(repoRoot+referencesDir, func(file string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel := path.Join(referencesDir, strings.TrimPrefix(filepath.ToSlash(file), repoRoot+referencesDir+"/"))
		sources = append(sources, driftSource{name: rel, text: repoFile(t, rel)})
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, sources)
	return sources
}

var (
	helpFlagLine    = regexp.MustCompile(`^\s+(?:-[A-Za-z], )?(--[A-Za-z][\w-]*)`)
	helpCommandLine = regexp.MustCompile(`^ {2}([a-z][a-z0-9-]*) {2,}\S`)
	flagInToken     = regexp.MustCompile(`(?:^|[^\w-])(--[a-z][a-z0-9-]*)`)
	commandWord     = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

// parseHelp is the long flag names listed under Flags: and Global Flags:, and the command names
// listed under Available Commands:, of one command's help text.
func parseHelp(help string) ([]string, []string) {
	var flags, commands []string
	section := ""
	for line := range strings.SplitSeq(help, "\n") {
		switch {
		case line == "Available Commands:" || line == "Flags:" || line == "Global Flags:":
			section = line
		case line == "":
			section = ""
		case section == "Flags:" || section == "Global Flags:":
			if match := helpFlagLine.FindStringSubmatch(line); match != nil {
				flags = append(flags, match[1])
			}
		case section == "Available Commands:":
			if match := helpCommandLine.FindStringSubmatch(line); match != nil {
				commands = append(commands, match[1])
			}
		}
	}
	return flags, commands
}

// helpTree is the command tree as `quarry <path> --help` prints it.
func helpTree(t *testing.T) helpNode {
	t.Helper()
	return helpNodeAt(t, nil)
}

// helpNodeAt is the help tree below the command at path.
func helpNodeAt(t *testing.T, path []string) helpNode {
	t.Helper()
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run(t.Context(), append(slices.Clone(path), "--help"), &stdout, &stderr), stderr.String())
	flags, commands := parseHelp(stdout.String())
	node := helpNode{flags: flags, children: map[string]helpNode{}}
	for _, name := range commands {
		node.children[name] = helpNodeAt(t, append(slices.Clone(path), name))
	}
	return node
}

// allFlags is every long flag name of node and the commands below it.
func (n helpNode) allFlags() map[string]bool {
	flags := map[string]bool{}
	for _, flag := range n.flags {
		flags[flag] = true
	}
	for _, child := range n.children {
		maps.Copy(flags, child.allFlags())
	}
	return flags
}

// codeUnit is one scanned piece of markdown: a code span, or a line inside a code fence.
type codeUnit struct {
	line int
	text string
}

// codeUnits is every code span and every fenced line of text, in order.
func codeUnits(text string) []codeUnit {
	var units []codeUnit
	inFence := false
	for i, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			units = append(units, codeUnit{line: i + 1, text: line})
			continue
		}
		parts := strings.Split(line, "`")
		for j := 1; j < len(parts)-1; j += 2 {
			units = append(units, codeUnit{line: i + 1, text: parts[j]})
		}
	}
	return units
}

// occurrence is one `quarry` token of a unit: its command path, and the node that path reached, nil
// when the path named no command.
type occurrence struct {
	path []string
	node *helpNode
}

// commandScan checks the quarry commands and flags one source uses against the help tree.
type commandScan struct {
	root     helpNode
	anyFlag  map[string]bool
	source   string
	findings *driftCheck
}

// commandMismatches resolves every `quarry <command> --flag` use in the code spans and fences of
// sources against tree. A flag is checked against the command it follows, never help prose.
func commandMismatches(tree helpNode, sources []driftSource) driftCheck {
	var check driftCheck
	anyFlag := tree.allFlags()
	for _, source := range sources {
		if strings.HasSuffix(source.name, ".sql") {
			continue
		}
		scan := commandScan{root: tree, anyFlag: anyFlag, source: source.name, findings: &check}
		scan.units(codeUnits(source.text))
	}
	check.resolved = slices.Compact(slices.Sorted(slices.Values(check.resolved)))
	return check
}

// units scans each unit, handing it the nearest quarry occurrence earlier on its physical line.
func (s *commandScan) units(units []codeUnit) {
	var onLine *occurrence
	lastLine := 0
	for _, unit := range units {
		if unit.line != lastLine {
			onLine, lastLine = nil, unit.line
		}
		if last := s.unit(unit, onLine); last != nil {
			onLine = last
		}
	}
}

// unit checks one unit and returns its last quarry occurrence, nil when it has none.
func (s *commandScan) unit(unit codeUnit, onLine *occurrence) *occurrence {
	tokens := strings.Fields(unit.text)
	if len(tokens) == 0 {
		return nil
	}
	opensWithFlag := flagInToken.MatchString(tokens[0])
	var own *occurrence
	for i, token := range tokens {
		if token == "quarry" {
			resolved := s.resolve(unit, tokens[i+1:])
			own = &resolved
			continue
		}
		for _, match := range flagInToken.FindAllStringSubmatch(token, -1) {
			// A flag binds to the nearest quarry before it; a flag-first unit binds to the line's quarry, else any command.
			switch {
			case own != nil:
				s.checkFlag(unit, match[1], own)
			case opensWithFlag && onLine != nil:
				s.checkFlag(unit, match[1], onLine)
			case opensWithFlag:
				s.checkAnyFlag(unit, match[1])
			}
		}
	}
	return own
}

// resolve descends the help tree along the lowercase words after a quarry token. A command without
// subcommands takes the rest as arguments.
func (s *commandScan) resolve(unit codeUnit, words []string) occurrence {
	node := s.root
	var path []string
	for _, word := range words {
		word = strings.TrimRight(word, ".,;")
		if len(node.children) == 0 || !commandWord.MatchString(word) {
			break
		}
		child, listed := node.children[word]
		if !listed {
			s.report(unit, fmt.Sprintf("unknown command %q", strings.Join(append(append([]string{"quarry"}, path...), word), " ")))
			return occurrence{path: path}
		}
		path = append(path, word)
		node = child
	}
	if len(path) > 0 {
		s.findings.resolved = append(s.findings.resolved, strings.Join(path, " "))
	}
	return occurrence{path: path, node: &node}
}

// checkFlag reports flag when the command the occurrence reached does not list it.
func (s *commandScan) checkFlag(unit codeUnit, flag string, to *occurrence) {
	if to.node == nil || slices.Contains(to.node.flags, flag) {
		return
	}
	s.report(unit, fmt.Sprintf("unknown flag %s for %q", flag, strings.Join(append([]string{"quarry"}, to.path...), " ")))
}

// checkAnyFlag reports flag when no command lists it.
func (s *commandScan) checkAnyFlag(unit codeUnit, flag string) {
	if !s.anyFlag[flag] {
		s.report(unit, fmt.Sprintf("unknown flag %s on any command", flag))
	}
}

func (s *commandScan) report(unit codeUnit, message string) {
	s.findings.mismatches = append(s.findings.mismatches, fmt.Sprintf("%s:%d: %s", s.source, unit.line, message))
}

// mcpToolNames is the name of every tool the MCP server lists.
func mcpToolNames(t *testing.T) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
	defer cancel()
	peer := startMCP(ctx, t, func(*cli.Env) {})
	listed, err := peer.session.ListTools(ctx, nil)
	require.NoError(t, err)
	require.NoError(t, peer.session.Close())
	peer.waitForExit(ctx, t)
	tools := make([]string, len(listed.Tools))
	for i, tool := range listed.Tools {
		tools[i] = tool.Name
	}
	return tools
}

func Test_every_quarry_name_the_skill_uses_exists(t *testing.T) {
	t.Run("quarry commands and flags", func(t *testing.T) {
		check := commandMismatches(helpTree(t), skillDriftSources(t))

		assert.Contains(t, check.resolved, "snapshots prune")
		assert.Empty(t, check.mismatches)
	})

	t.Run("MCP tool names", func(t *testing.T) {
		check := toolMismatches(t, skillDriftSources(t), mcpToolNames(t), helpTree(t))

		assert.Contains(t, check.resolved, "sync_status")
		assert.Empty(t, check.mismatches)
	})

	t.Run("tables and v_* views", func(t *testing.T) {
		check := relationMismatches(skillDriftSources(t), storeRelations(t))

		assert.Contains(t, check.resolved, "v_spending")
		assert.Empty(t, check.mismatches)
	})
}

func Test_drift_help_parser_reads_only_flag_sections(t *testing.T) {
	help := `A long text that mentions --limit and
  --csv at an indent.

Usage:
  quarry x [flags]

Available Commands:
  child       Does a thing, like --child-only

Flags:
  -h, --help       help for x
      --keep n     keep n, like --limit

Global Flags:
      --json   print JSON

Use "quarry x [command] --help" for more information.
`

	flags, commands := parseHelp(help)

	assert.Equal(t, []string{"--help", "--keep", "--json"}, flags)
	assert.Equal(t, []string{"child"}, commands)
}

func Test_drift_check_flags_crafted_command_text(t *testing.T) {
	tree := helpTree(t)
	cases := []struct {
		name, text string
		want       []string
	}{
		{"unknown_root_command", "`quarry bogus`", []string{`crafted:1: unknown command "quarry bogus"`}},
		{"unknown_subcommand", "`quarry snapshots bogus`", []string{`crafted:1: unknown command "quarry snapshots bogus"`}},
		{"sub_subcommand", "`quarry snapshots prune --dry-run`", nil},
		{"unknown_flag", "`quarry spend --bogus`", []string{`crafted:1: unknown flag --bogus for "quarry spend"`}},
		{"flag_only_in_the_commands_own_prose", "`quarry snapshots --from`", []string{`crafted:1: unknown flag --from for "quarry snapshots"`}},
		{"flag_on_another_command", "`quarry search --csv`", []string{`crafted:1: unknown flag --csv for "quarry search"`}},
		{"parenthesised_flag_binds_to_its_row", "`quarry accounts --json` (with `--since`)", []string{`crafted:1: unknown flag --since for "quarry accounts"`}},
		{"parenthesised_flag_of_its_row", "`quarry spend --json` (with `--by`)", nil},
		{"alternatives_are_one_flag", `quarry spend --by category\|payee`, nil},
		{"double_dash_and_dash_are_not_flags", "`quarry search -- -x`\n`quarry sql --json -`", nil},
		{"claude_line_ignored", "`claude plugin validate --strict plugin`", nil},
		{"quarry_line_with_the_same_flag", "`quarry spend --strict`", []string{`crafted:1: unknown flag --strict for "quarry spend"`}},
		{"go_env_ignored", "`$(go env GOPATH)/bin`", nil},
		{"colon_is_not_a_command", "`quarry: no store …; run quarry sync to build it`", nil},
		{"fence_line", "```\nquarry bogus\n```", []string{`crafted:2: unknown command "quarry bogus"`}},
		{"standalone_unknown_flag", "`--bogus`", []string{"crafted:1: unknown flag --bogus on any command"}},
		{"standalone_flag_of_some_command", "`--currency native`", nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			check := commandMismatches(tree, []driftSource{{name: "crafted", text: c.text}})

			assert.Equal(t, c.want, check.mismatches)
		})
	}
}

func Test_drift_check_resolves_crafted_command_text(t *testing.T) {
	tree := helpTree(t)
	cases := []struct {
		name, text string
		want       []string
	}{
		{"sub_subcommand", "`quarry snapshots prune --dry-run`", []string{"snapshots prune"}},
		{"colon_is_not_a_command", "`quarry: no store …; run quarry sync to build it`", []string{"sync"}},
		{"double_dash_and_dash_are_not_flags", "`quarry search -- -x`\n`quarry sql --json -`", []string{"search", "sql"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			check := commandMismatches(tree, []driftSource{{name: "crafted", text: c.text}})

			assert.Equal(t, c.want, check.resolved)
		})
	}
}
