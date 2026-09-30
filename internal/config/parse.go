package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/koblas/quarry/internal/platform/homepath"
	toml "github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

// What a bad value shows when the file introduced it with a header, not right of "=".
const (
	gotTable     = "a table"
	gotTableList = "a list of tables"
)

// setting names a known key: its table, its name in that table, and the
// line shown to a user who wrote the table as a plain value.
type setting struct {
	table, name, example string
}

var (
	keepSetting = setting{table: "snapshots", name: "keep", example: "snapshots.keep = 12"}
	pathSetting = setting{table: "quicken", name: "path", example: `quicken.path = "~/Documents/Home.quicken"`}
)

func (s setting) String() string { return strings.Join(s.key(), ".") }

func (s setting) key() []string { return []string{s.table, s.name} }

// file is one config file read from disk.
type file struct {
	home, path, shown string
	data              []byte
}

// parse validates the whole file: syntax first, then snapshots.keep, then
// quicken.path, then unknown keys.
func (f file) parse() (Config, error) {
	tree, err := f.tree()
	if err != nil {
		return Config{}, err
	}
	doc := document{file: f, tree: tree, entries: f.entries()}
	keep, err := doc.keep()
	if err != nil {
		return Config{}, err
	}
	quickenPath, err := doc.quickenPath()
	if err != nil {
		return Config{}, err
	}

	return Config{
		Path:        f.path,
		Keep:        keep,
		QuickenPath: homepath.Expand(f.home, quickenPath),
		Warnings:    doc.unknownKeys(),
	}, nil
}

// tree decodes the file into generic values, which accept any shape, so the
// only error is malformed TOML, refused naming its line.
func (f file) tree() (map[string]any, error) {
	var tree map[string]any
	err := toml.Unmarshal(f.data, &tree)
	if err == nil {
		return tree, nil
	}
	decodeErr, ok := errors.AsType[*toml.DecodeError](err)
	if !ok {
		// unreachable: go-toml v2.4.3 decode_fused.go raises only parser and tracker errors and wrapError makes them *toml.DecodeError; 23 malformed scalars, keys and strings probed, all *toml.DecodeError.
		return nil, f.refuse("cannot read "+f.shown+": "+err.Error(), err)
	}
	line, _ := decodeErr.Position()
	message, _, _ := strings.Cut(strings.TrimPrefix(decodeErr.Error(), "toml: "), "\n")

	return nil, f.refuse("cannot read "+f.shown+": line "+strconv.Itoa(line)+": "+message, err)
}

// entry is one header or key of the file at its full key path, exactly as written:
// a key under a header or inside an inline table carries its parents' names.
type entry struct {
	kind unstable.Kind // Table, ArrayTable or KeyValue
	key  []string
	// value is a KeyValue's value as written, up to the end of the line's expression.
	value string
}

// entries lists every header and key of the file in file order. The parser reads the
// file's own spelling, so a key's letter case is never folded.
func (f file) entries() []entry {
	var parser unstable.Parser
	parser.Reset(f.data)
	var entries []entry
	var header []string
	for parser.NextExpression() {
		expr := parser.Expression()
		if expr.Kind == unstable.KeyValue {
			entries = f.appendKeyValue(entries, header, expr)
			continue
		}
		if expr.Kind == unstable.Table || expr.Kind == unstable.ArrayTable {
			header = keyParts(expr)
			entries = append(entries, entry{kind: expr.Kind, key: header})
		}
	}

	return entries
}

// appendKeyValue adds the key of expr under parent, then the keys of an inline table it holds.
func (f file) appendKeyValue(entries []entry, parent []string, expr *unstable.Node) []entry {
	key := append(slices.Clone(parent), keyParts(expr)...)
	entries = append(entries, entry{kind: unstable.KeyValue, key: key, value: f.valueText(expr)})
	if value := expr.Value(); value.Kind == unstable.InlineTable {
		for children := value.Children(); children.Next(); {
			entries = f.appendKeyValue(entries, key, children.Node())
		}
	}

	return entries
}

// keyParts are the names in the key of a header or key-value expression.
func keyParts(expr *unstable.Node) []string {
	var parts []string
	for part := expr.Key(); part.Next(); {
		parts = append(parts, string(part.Node().Data))
	}

	return parts
}

// valueText is the text right of the "=" of expr, to the end of the expression, so a
// container is whole and a trailing comment is left out.
func (f file) valueText(expr *unstable.Node) string {
	var last unstable.Range
	for part := expr.Key(); part.Next(); {
		last = part.Node().Raw
	}
	start := int(last.Offset + last.Length)
	for start < len(f.data) && (f.data[start] == ' ' || f.data[start] == '\t') {
		start++
	}
	start++ // the equal sign
	for start < len(f.data) && (f.data[start] == ' ' || f.data[start] == '\t') {
		start++
	}

	return string(f.data[start : expr.Raw.Offset+expr.Raw.Length])
}

// document is a file with its values and every key as written, for checks that need both.
type document struct {
	file

	tree    map[string]any
	entries []entry
}

// keep is snapshots.keep: DefaultKeep when unset, else a whole number of 1 or more.
func (d document) keep() (int, error) {
	value, present, err := d.lookup(keepSetting)
	if err != nil || !present {
		return DefaultKeep, err
	}
	if n, ok := value.(int64); ok && n >= 1 {
		return int(n), nil
	}

	return 0, d.badValue(keepSetting.String()+" must be a whole number of 1 or more", d.got(keepSetting.key()))
}

// quickenPath is quicken.path as written: "" when unset, else a string that
// is a full path or starts with "~/".
func (d document) quickenPath() (string, error) {
	value, present, err := d.lookup(pathSetting)
	if err != nil || !present {
		return "", err
	}
	text, isString := value.(string)
	if !isString {
		return "", d.badValue(pathSetting.String()+" must be a path in quotes", d.got(pathSetting.key()))
	}
	if !filepath.IsAbs(text) && !strings.HasPrefix(text, "~/") {
		return "", d.badValue(pathSetting.String()+" must be a full path or start with ~/", d.got(pathSetting.key()))
	}

	return text, nil
}

// lookup finds s in the values. A table written as a plain value is refused.
func (d document) lookup(s setting) (any, bool, error) {
	entry, found := d.tree[s.table]
	if !found {
		return nil, false, nil
	}
	table, isTable := entry.(map[string]any)
	if !isTable {
		return nil, false, d.badValue(s.table+" must be a table, such as "+s.example, d.got([]string{s.table}))
	}
	value, present := table[s.name]

	return value, present, nil
}

// got is the value at key as the file wrote it, collapsed to one line. A value a header
// introduced, or dotted keys built, is named by kind, since it has no text right of "=".
func (d document) got(key []string) string {
	for _, e := range d.entries {
		if !slices.Equal(e.key, key) {
			continue
		}
		if e.kind == unstable.ArrayTable {
			return gotTableList
		}
		if e.kind == unstable.KeyValue {
			return strings.Join(strings.Fields(e.value), " ")
		}
	}

	return gotTable
}

// knownKeys are the paths quarry reads; a header or key at any other path is unknown.
var knownKeys = [][]string{{keepSetting.table}, keepSetting.key(), {pathSetting.table}, pathSetting.key()}

// unknownKeys is one warning per key the file has beyond the known ones, in
// file order, matched by exact spelling. A table is named once, not once per child.
func (d document) unknownKeys() []string {
	var warnings []string
	var reported [][]string
	for _, e := range d.entries {
		if slices.ContainsFunc(knownKeys, func(known []string) bool { return slices.Equal(known, e.key) }) {
			continue
		}
		if slices.ContainsFunc(reported, func(parent []string) bool { return slices.Equal(parent, e.key[:min(len(parent), len(e.key))]) }) {
			continue
		}
		reported = append(reported, e.key)
		warnings = append(warnings, d.shown+": unknown key "+keyText(e.key)+"; quarry ignores it")
	}

	return warnings
}

// bareKey matches a key part TOML allows unquoted.
var bareKey = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// keyText writes key as TOML would: a part that is not bare is a basic string, so a dot,
// quote, newline or empty name inside one part stays visible and on one line.
func keyText(key []string) string {
	parts := make([]string, len(key))
	for i, part := range key {
		parts[i] = keyPartText(part)
	}

	return strings.Join(parts, ".")
}

// keyPartText is part bare when TOML allows it, else a basic string with control characters escaped.
func keyPartText(part string) string {
	if bareKey.MatchString(part) {
		return part
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range part {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case unicode.IsControl(r):
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')

	return b.String()
}
