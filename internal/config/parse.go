package config

import (
	"bytes"
	"errors"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

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

// Mirrors of the known keys with each value left as written, one per setting
// so a bad shape in the other table cannot fail this one's decode.
type (
	snapshotsTable struct {
		Keep unstable.RawMessage `toml:"keep"`
	}
	quickenTable struct {
		Path unstable.RawMessage `toml:"path"`
	}
	keepDocument struct {
		Snapshots snapshotsTable `toml:"snapshots"`
	}
	pathDocument struct {
		Quicken quickenTable `toml:"quicken"`
	}
	knownKeys struct {
		Snapshots snapshotsTable `toml:"snapshots"`
		Quicken   quickenTable   `toml:"quicken"`
	}
)

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
	keep, err := f.keep(tree)
	if err != nil {
		return Config{}, err
	}
	quickenPath, err := f.quickenPath(tree)
	if err != nil {
		return Config{}, err
	}

	return Config{
		Path:        f.path,
		Keep:        keep,
		QuickenPath: homepath.Expand(f.home, quickenPath),
		Warnings:    f.unknownKeys(),
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

// keep is snapshots.keep: DefaultKeep when unset, else a whole number of 1 or more.
func (f file) keep(tree map[string]any) (int, error) {
	value, present, err := f.lookup(tree, keepSetting)
	if err != nil || !present {
		return DefaultKeep, err
	}
	if n, ok := value.(int64); ok && n >= 1 {
		return int(n), nil
	}

	return 0, f.badValue(keepSetting.String()+" must be a whole number of 1 or more", f.got(value, keepSetting.key(), f.rawKeep))
}

// quickenPath is quicken.path as written: "" when unset, else a string that
// is a full path or starts with "~/".
func (f file) quickenPath(tree map[string]any) (string, error) {
	value, present, err := f.lookup(tree, pathSetting)
	if err != nil || !present {
		return "", err
	}
	text, isString := value.(string)
	if !isString {
		return "", f.badValue(pathSetting.String()+" must be a path in quotes", f.got(value, pathSetting.key(), f.rawPath))
	}
	if !filepath.IsAbs(text) && !strings.HasPrefix(text, "~/") {
		return "", f.badValue(pathSetting.String()+" must be a full path or start with ~/", f.got(value, pathSetting.key(), f.rawPath))
	}

	return text, nil
}

// lookup finds s in tree. A table written as a plain value is refused.
func (f file) lookup(tree map[string]any, s setting) (any, bool, error) {
	entry, found := tree[s.table]
	if !found {
		return nil, false, nil
	}
	table, isTable := entry.(map[string]any)
	if !isTable {
		return nil, false, f.badValue(s.table+" must be a table, such as "+s.example, f.got(entry, []string{s.table}, func() unstable.RawMessage { return f.rawTable(s.table) }))
	}
	value, present := table[s.name]

	return value, present, nil
}

// got is value as the file wrote it, collapsed to one line. A value a header
// introduced is named by kind, since its text is the body under the header.
func (f file) got(value any, key []string, raw func() unstable.RawMessage) string {
	switch value.(type) {
	case map[string]any:
		return gotTable
	case []any:
		if f.hasArrayTableHeader(key) {
			return gotTableList
		}
	}

	return strings.Join(strings.Fields(string(raw())), " ")
}

// hasArrayTableHeader reports whether the file opens a [[key]] header; an
// inline array of inline tables decodes to the same value but has none.
func (f file) hasArrayTableHeader(key []string) bool {
	var parser unstable.Parser
	parser.Reset(f.data)
	for parser.NextExpression() {
		expr := parser.Expression()
		if expr.Kind != unstable.ArrayTable {
			continue
		}
		var header []string
		for part := expr.Key(); part.Next(); {
			header = append(header, string(part.Node().Data))
		}
		if slices.Equal(header, key) {
			return true
		}
	}

	return false
}

func (f file) rawKeep() unstable.RawMessage {
	var doc keepDocument
	f.decodeRaw(&doc)

	return doc.Snapshots.Keep
}

func (f file) rawPath() unstable.RawMessage {
	var doc pathDocument
	f.decodeRaw(&doc)

	return doc.Quicken.Path
}

// rawTable is the value written for the top-level key name.
func (f file) rawTable(name string) unstable.RawMessage {
	var doc map[string]unstable.RawMessage
	f.decodeRaw(&doc)

	return doc[name]
}

// decodeRaw fills target's RawMessage fields from the file.
func (f file) decodeRaw(target any) {
	// Any other [[table]] header in the file fails the decode, but only after the
	// keys above it are filled, and those are the only ones read.
	_ = toml.NewDecoder(bytes.NewReader(f.data)).EnableUnmarshalerInterface().Decode(target)
}

// unknownKeys is one warning per key the file has beyond the known ones, in
// file order. A table is named once, not once per child.
func (f file) unknownKeys() []string {
	err := toml.NewDecoder(bytes.NewReader(f.data)).DisallowUnknownFields().EnableUnmarshalerInterface().Decode(&knownKeys{})
	missing, ok := errors.AsType[*toml.StrictMissingError](err)
	if !ok {
		return nil
	}
	var warnings []string
	var reported [][]string
	for _, decodeErr := range missing.Errors {
		key := decodeErr.Key()
		if slices.ContainsFunc(reported, func(parent []string) bool { return slices.Equal(parent, key[:min(len(parent), len(key))]) }) {
			continue
		}
		reported = append(reported, key)
		warnings = append(warnings, f.shown+": unknown key "+strings.Join(key, ".")+"; quarry ignores it")
	}

	return warnings
}
