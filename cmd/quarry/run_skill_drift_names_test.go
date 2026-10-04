// The skill's MCP tool names, views and links are checked against the product, so these tests
// live in package main beside the other cmd/quarry tests.
package main

import (
	"fmt"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// mcpNotTools are the names SKILL.md section 9 spells in code that are commands the MCP server
// cannot run, not tools.
var mcpNotTools = []string{"sync"}

var (
	viewName    = regexp.MustCompile(`\bv_[a-z][a-z0-9_]*\b`)
	cteName     = regexp.MustCompile(`(?i)\b(\w+)\s+AS\s*\(`)
	fromKeyword = regexp.MustCompile(`(?i)\b(?:from|join)\s+`)
	listEnd     = regexp.MustCompile(`(?i)\s(?:where|group|order|having|limit|union|on|using|left|right|inner|full|cross|join|window)\b|[);]`)
	firstWord   = regexp.MustCompile(`^\s*([A-Za-z_]\w*)`)
	selectWord  = regexp.MustCompile(`(?i)\bselect\b`)
)

// toolMismatches resolves every code span of SKILL.md section 9 against tools. The names in
// mcpNotTools must instead be absent from tools and be commands of root.
func toolMismatches(t *testing.T, sources []driftSource, tools []string, root helpNode) driftCheck {
	t.Helper()
	var check driftCheck
	for _, source := range sources {
		if source.name != skillPath {
			continue
		}
		for _, unit := range codeUnits(splitSkill(t, source.text).bodies[skillHeadings[8]]) {
			name := strings.TrimSpace(unit.text)
			switch _, isCommand := root.children[name]; {
			case slices.Contains(mcpNotTools, name) && slices.Contains(tools, name):
				check.mismatches = append(check.mismatches, fmt.Sprintf("%s: %q is an MCP tool, but section 9 says the server cannot run it", source.name, name))
			case slices.Contains(mcpNotTools, name) && !isCommand:
				check.mismatches = append(check.mismatches, fmt.Sprintf("%s: %q is not a quarry command", source.name, name))
			case slices.Contains(mcpNotTools, name):
			case slices.Contains(tools, name):
				check.resolved = append(check.resolved, name)
			default:
				check.mismatches = append(check.mismatches, fmt.Sprintf("%s: unknown MCP tool %q", source.name, name))
			}
		}
	}
	return check
}

// relationMismatches resolves every v_* name in sources, and every relation in the FROM and JOIN
// lists of their SQL, against relations. A relation a query defines with WITH is not checked.
func relationMismatches(sources []driftSource, relations []string) driftCheck {
	var check driftCheck
	known := map[string]bool{}
	for _, relation := range relations {
		known[strings.ToLower(relation)] = true
	}
	for _, source := range sources {
		sql := sqlOf(source)
		named := viewName.FindAllString(source.text, -1)
		for _, list := range relationLists(sql) {
			for item := range strings.SplitSeq(list, ",") {
				if match := firstWord.FindStringSubmatch(item); match != nil {
					named = append(named, match[1])
				}
			}
		}
		defined := map[string]bool{}
		for _, match := range cteName.FindAllStringSubmatch(sql, -1) {
			defined[strings.ToLower(match[1])] = true
		}
		for _, name := range slices.Compact(slices.Sorted(slices.Values(named))) {
			switch lower := strings.ToLower(name); {
			case known[lower]:
				check.resolved = append(check.resolved, lower)
			case !defined[lower]:
				check.mismatches = append(check.mismatches, fmt.Sprintf("%s: unknown relation %s", source.name, name))
			}
		}
	}
	check.resolved = slices.Compact(slices.Sorted(slices.Values(check.resolved)))
	return check
}

// relationLists is the text after each FROM or JOIN in sql, up to the keyword or bracket that ends the list.
func relationLists(sql string) []string {
	keywords := fromKeyword.FindAllStringIndex(sql, -1)
	lists := make([]string, 0, len(keywords))
	for _, keyword := range keywords {
		list := sql[keyword[1]:]
		if end := listEnd.FindStringIndex(list); end != nil {
			list = list[:end[0]]
		}
		lists = append(lists, list)
	}
	return lists
}

// sqlOf is the SQL a source holds with comment lines and line breaks removed: the whole of a .sql
// file, or the code fences of a markdown file that open as sql or hold a SELECT.
func sqlOf(source driftSource) string {
	if strings.HasSuffix(source.name, ".sql") {
		return withoutCommentLines(source.text)
	}
	var sql []string
	fence := make([]string, 0, strings.Count(source.text, "\n"))
	info, inFence := "", false
	for line := range strings.SplitSeq(source.text, "\n") {
		switch trimmed := strings.TrimSpace(line); {
		case strings.HasPrefix(trimmed, "```") && !inFence:
			info, inFence, fence = strings.TrimPrefix(trimmed, "```"), true, fence[:0]
		case strings.HasPrefix(trimmed, "```"):
			inFence = false
			if body := strings.Join(fence, "\n"); strings.EqualFold(info, "sql") || selectWord.MatchString(body) {
				sql = append(sql, body)
			}
		case inFence:
			fence = append(fence, line)
		}
	}
	return withoutCommentLines(strings.Join(sql, "\n"))
}

// withoutCommentLines is sql less its `--` lines, on one line.
func withoutCommentLines(sql string) string {
	var kept []string
	for line := range strings.SplitSeq(sql, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			kept = append(kept, line)
		}
	}
	return strings.Join(strings.Fields(strings.Join(kept, " ")), " ")
}

// linkMismatches resolves each markdown link relative to its file and each backticked references/
// path relative to the skill directory. Anchors and URLs are skipped.
func linkMismatches(sources []driftSource, exists func(rel string) bool) driftCheck {
	var check driftCheck
	resolve := func(source driftSource, line int, rel string) {
		if exists(rel) {
			check.resolved = append(check.resolved, rel)
			return
		}
		check.mismatches = append(check.mismatches, fmt.Sprintf("%s:%d: %s does not exist", source.name, line, rel))
	}
	for _, source := range sources {
		if strings.HasSuffix(source.name, ".sql") {
			continue
		}
		for i, line := range strings.Split(source.text, "\n") {
			for _, match := range skillLinkTarget.FindAllStringSubmatch(line, -1) {
				target, _, _ := strings.Cut(match[1], "#")
				if target != "" && !strings.Contains(target, "://") {
					resolve(source, i+1, path.Join(path.Dir(source.name), target))
				}
			}
		}
		for _, unit := range codeUnits(source.text) {
			if strings.HasPrefix(unit.text, "references/") && !strings.ContainsAny(unit.text, " \t") {
				resolve(source, unit.line, path.Join(path.Dir(referencesDir), unit.text))
			}
		}
	}
	check.resolved = slices.Compact(slices.Sorted(slices.Values(check.resolved)))
	return check
}

// repoFileExists is whether rel names a file or directory of the repository.
func repoFileExists(rel string) bool {
	_, err := os.Stat("../../" + rel)
	return err == nil
}

func Test_skill_links_and_reference_paths_resolve(t *testing.T) {
	check := linkMismatches(skillDriftSources(t), repoFileExists)

	assert.Contains(t, check.resolved, "plugin/skills/quarry/references/schema.md")
	assert.Contains(t, check.resolved, "plugin/skills/quarry/references/sql/spending-trend.sql")
	assert.Empty(t, check.mismatches)
}

func Test_drift_check_flags_crafted_name_text(t *testing.T) {
	tree := helpNode{children: map[string]helpNode{"sync": {}}}
	tools := []string{"sync_status", "query"}
	relations := []string{"transactions", "v_spending"}
	skill := func(section9 string) driftSource {
		return driftSource{name: skillPath, text: "---\nname: x\n---\n# T\n\nintro\n\n" + skillHeadings[8] + "\n\n" + section9 + "\n"}
	}
	cases := []struct {
		name  string
		check func() driftCheck
		want  []string
	}{
		{"known_tool", func() driftCheck { return toolMismatches(t, []driftSource{skill("Use `sync_status`.")}, tools, tree) }, nil},
		{
			"unknown_tool", func() driftCheck { return toolMismatches(t, []driftSource{skill("Use `bogus_tool`.")}, tools, tree) },
			[]string{`plugin/skills/quarry/SKILL.md: unknown MCP tool "bogus_tool"`},
		},
		{"sync_named_as_a_command", func() driftCheck {
			return toolMismatches(t, []driftSource{skill("It cannot run `sync`.")}, tools, tree)
		}, nil},
		{"sync_named_as_a_tool", func() driftCheck {
			return toolMismatches(t, []driftSource{skill("It cannot run `sync`.")}, append(slices.Clone(tools), "sync"), tree)
		}, []string{`plugin/skills/quarry/SKILL.md: "sync" is an MCP tool, but section 9 says the server cannot run it`}},
		{
			"sync_not_a_command", func() driftCheck {
				return toolMismatches(t, []driftSource{skill("It cannot run `sync`.")}, tools, helpNode{})
			},
			[]string{`plugin/skills/quarry/SKILL.md: "sync" is not a quarry command`},
		},
		{"known_view", func() driftCheck {
			return relationMismatches([]driftSource{{name: "a.md", text: "Read `v_spending`."}}, relations)
		}, nil},
		{
			"unknown_view", func() driftCheck {
				return relationMismatches([]driftSource{{name: "a.md", text: "Read `v_net_worth`."}}, relations)
			},
			[]string{"a.md: unknown relation v_net_worth"},
		},
		{"from_list", func() driftCheck {
			return relationMismatches([]driftSource{{name: "a.sql", text: "WITH params AS (SELECT 1 AS x)\nSELECT 1\nFROM v_bogus b, params p\nWHERE p.x = 1"}}, relations)
		}, []string{"a.sql: unknown relation v_bogus"}},
		{"join_table", func() driftCheck {
			return relationMismatches([]driftSource{{name: "a.sql", text: "SELECT 1 FROM v_spending s JOIN transactions t ON t.id = s.id"}}, relations)
		}, nil},
		{"unknown_joined_table", func() driftCheck {
			return relationMismatches([]driftSource{{name: "a.sql", text: "SELECT 1 FROM v_spending s JOIN bogus t ON t.id = s.id"}}, relations)
		}, []string{"a.sql: unknown relation bogus"}},
		{"sql_fence_from_list", func() driftCheck {
			return relationMismatches([]driftSource{{name: "a.md", text: "```sql\nSELECT 1 FROM bogus\n```"}}, relations)
		}, []string{"a.md: unknown relation bogus"}},
		{"from_in_prose_is_not_a_relation", func() driftCheck {
			return relationMismatches([]driftSource{{name: "a.md", text: "Read it from the dashboard."}}, relations)
		}, nil},
		{"dead_link", func() driftCheck {
			return linkMismatches([]driftSource{{name: skillPath, text: "[x](references/bogus.md)"}}, repoFileExists)
		}, []string{"plugin/skills/quarry/SKILL.md:1: plugin/skills/quarry/references/bogus.md does not exist"}},
		{"live_link", func() driftCheck {
			return linkMismatches([]driftSource{{name: skillPath, text: "[x](references/schema.md)"}}, repoFileExists)
		}, nil},
		{"dead_backticked_path", func() driftCheck {
			return linkMismatches([]driftSource{{name: skillPath, text: "Run `references/sql/bogus.sql`."}}, repoFileExists)
		}, []string{"plugin/skills/quarry/SKILL.md:1: plugin/skills/quarry/references/sql/bogus.sql does not exist"}},
		{"live_backticked_path", func() driftCheck {
			return linkMismatches([]driftSource{{name: skillPath, text: "Run `references/sql/spending-trend.sql`."}}, repoFileExists)
		}, nil},
		{"anchor_and_url", func() driftCheck {
			return linkMismatches([]driftSource{{name: skillPath, text: "[a](#frag) [b](https://example.com/x)"}}, repoFileExists)
		}, nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, c.check().mismatches)
		})
	}
}
