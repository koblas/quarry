package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/koblas/quarry/internal/platform/accountmask"
	"github.com/koblas/quarry/internal/platform/homepath"
	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/platform/tomlstr"
	toml "github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

// What a bad value shows when the file introduced it with a header, not right of "=".
const (
	gotTable     = "a table"
	gotTableList = "a list of tables"
)

// setting names a known key: its table, its name, and the line shown when the user wrote the
// table as a plain value. A masked setting holds account numbers, so a refusal masks its value.
type setting struct {
	table, name, example string
	masked               bool
}

// show is text as a refusal of s prints it.
func (s setting) show(text string) string {
	if s.masked {
		return accountmask.Mask(text)
	}

	return text
}

// ignoreExample is a findings.ignore list as a user writes it, right of the equal sign.
const ignoreExample = `["duplicate:txn-4410+txn-4412"]`

// accountsExample is an accounts.registered list as a user writes it, right of the equal sign.
const accountsExample = `["acct-12"]`

var (
	keepSetting   = setting{table: "snapshots", name: "keep", example: "snapshots.keep = 12"}
	pathSetting   = setting{table: "quicken", name: "path", example: `quicken.path = "~/Documents/Home.quicken"`}
	ignoreSetting = setting{table: "findings", name: "ignore", example: "findings.ignore = " + ignoreExample}

	reportingSetting = setting{table: "reporting", name: "currency", example: `reporting.currency = "CAD"`}

	registeredSetting    = setting{table: "accounts", name: "registered", example: "accounts.registered = " + accountsExample, masked: true}
	nonRegisteredSetting = setting{table: "accounts", name: "non-registered", example: "accounts.registered = " + accountsExample, masked: true}
)

func (s setting) String() string { return strings.Join(s.key(), ".") }

func (s setting) key() []string { return []string{s.table, s.name} }

// file is one config file read from disk.
type file struct {
	home, path, shown string
	data              []byte
}

// parse validates the whole file: syntax first, then snapshots.keep, then
// quicken.path, then findings.ignore, then reporting.currency, then accounts.registered,
// then accounts.non-registered, then an id in both account lists, then acb.adjustment, then unknown keys.
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

	ignore, err := doc.idList(ignoreSetting, "finding ids", ignoreExample)
	if err != nil {
		return Config{}, err
	}

	currency, err := doc.currency()
	if err != nil {
		return Config{}, err
	}

	registered, err := doc.idList(registeredSetting, "account ids", accountsExample)
	if err != nil {
		return Config{}, err
	}

	nonRegistered, err := doc.idList(nonRegisteredSetting, "account ids", accountsExample)
	if err != nil {
		return Config{}, err
	}

	if err := doc.notInBoth(registered, nonRegistered); err != nil {
		return Config{}, err
	}

	adjustments, err := doc.adjustments()
	if err != nil {
		return Config{}, err
	}

	return Config{
		Adjustments:      adjustments,
		Path:             f.path,
		Keep:             keep,
		QuickenPath:      homepath.Expand(f.home, quickenPath),
		Ignore:           ignore,
		Currency:         currency,
		Registered:       registered,
		NonRegistered:    nonRegistered,
		Warnings:         doc.unknownKeys(f.shown),
		WarningsAbsolute: doc.unknownKeys(f.path),
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
		return nil, f.cannotRead(err.Error(), err)
	}
	line, _ := decodeErr.Position()
	message, _, _ := strings.Cut(strings.TrimPrefix(decodeErr.Error(), "toml: "), "\n")

	return nil, f.cannotRead("line "+strconv.Itoa(line)+": "+maskNamedKey(message), err)
}

// namedKeyMessage matches the go-toml messages that echo a key or table name, which under
// [accounts] can be an account number; no other syntax message echoes a value.
var namedKeyMessage = regexp.MustCompile(`^(key |table )(.+?)( is already defined| should be a table, not a value| already exists as an array of tables| already exists)$`)

// maskNamedKey is message with the key or table name in it masked, or message unchanged when it
// names none.
func maskNamedKey(message string) string {
	parts := namedKeyMessage.FindStringSubmatch(message)
	if parts == nil {
		return message
	}

	return parts[1] + accountmask.Mask(parts[2]) + parts[3]
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

// idList is the list s holds as written, file order and duplicates kept, nil when unset:
// a list whose items are all strings, which noun names as in "must be a list of <noun> in
// quotes, such as <example>". An item that is not a string is refused by its place.
func (d document) idList(s setting, noun, example string) ([]string, error) {
	value, present, err := d.lookup(s)
	if err != nil || !present {
		return nil, err
	}
	items, isList := value.([]any)
	written, isWritten := d.written(s.key())
	if !isList || !isWritten {
		return nil, d.badValue(s.String()+" must be a list of "+noun+" in quotes, such as "+example, s.show(d.got(s.key())))
	}
	var ids []string
	for i, item := range items {
		id, isString := item.(string)
		if !isString {
			// The item is masked before " as item n" is added: n is a digit too.
			return nil, d.badValue(s.String()+" must hold only "+noun+" in quotes", s.show(itemText(arrayItems(written.value), items, i))+" as item "+strconv.Itoa(i+1))
		}
		ids = append(ids, id)
	}

	return ids, nil
}

// notInBoth refuses the first id of registered, in file order, that nonRegistered also lists,
// showing it masked.
func (d document) notInBoth(registered, nonRegistered []string) error {
	for _, id := range registered {
		if slices.Contains(nonRegistered, id) {
			return d.badValue("an account must be in only one of "+registeredSetting.String()+" and "+nonRegisteredSetting.String(), tomlstr.BasicString(accountmask.Mask(id))+" in both")
		}
	}

	return nil
}

// currency is reporting.currency in any letter case: money.CAD when unset, else a string
// money.ParseCurrency reads.
func (d document) currency() (money.Currency, error) {
	value, present, err := d.lookup(reportingSetting)
	if err != nil || !present {
		return money.CAD, err
	}
	if text, isString := value.(string); isString {
		if currency, ok := money.ParseCurrency(text); ok {
			return currency, nil
		}
	}

	return money.Native, d.badValue(reportingSetting.String()+" must be CAD, USD or native", d.got(reportingSetting.key()))
}

// itemText is items[i] as the file wrote it, collapsed to one line, from the split text
// raw; when raw and the decoded items disagree in count it is the decoded value.
func itemText(raw []string, items []any, i int) string {
	if len(raw) != len(items) {
		return fmt.Sprint(items[i])
	}

	return strings.Join(strings.Fields(raw[i]), " ")
}

// written is the key-value entry at key, which a header or dotted keys never make.
func (d document) written(key []string) (entry, bool) {
	for _, e := range d.entries {
		if e.kind == unstable.KeyValue && slices.Equal(e.key, key) {
			return e, true
		}
	}

	return entry{}, false
}

// lookup finds s in the values. A table written as a plain value is refused.
func (d document) lookup(s setting) (any, bool, error) {
	entry, found := d.tree[s.table]
	if !found {
		return nil, false, nil
	}
	table, isTable := entry.(map[string]any)
	if !isTable {
		return nil, false, d.badValue(s.table+" must be a table, such as "+s.example, s.show(d.got([]string{s.table})))
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
var knownKeys = [][]string{
	{keepSetting.table},
	keepSetting.key(),
	{pathSetting.table},
	pathSetting.key(),
	{ignoreSetting.table},
	ignoreSetting.key(),
	{reportingSetting.table},
	reportingSetting.key(),
	{registeredSetting.table},
	registeredSetting.key(),
	nonRegisteredSetting.key(),
	{adjustmentSetting.table},
	adjustmentSetting.key(),
	append(adjustmentSetting.key(), securityKey),
	append(adjustmentSetting.key(), dateKey),
	append(adjustmentSetting.key(), returnOfCapitalKey),
	append(adjustmentSetting.key(), reinvestedDistributionKey),
}

// unknownKeys is one warning per key the file has beyond the known ones, in
// file order, matched by exact spelling, each naming the file as path. A table is named once, not once per child.
func (d document) unknownKeys(path string) []string {
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
		warnings = append(warnings, path+": unknown key "+keyText(e.key)+"; quarry ignores it")
	}

	return warnings
}

// bareKey matches a key part TOML allows unquoted.
var bareKey = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// keyText writes key as TOML would, a part that is not bare as a basic string, so it stays on one
// line; a name under accounts may be an account number and is masked.
func keyText(key []string) string {
	parts := make([]string, len(key))
	for i, part := range key {
		parts[i] = keyPartText(part)
		if i > 0 && key[0] == registeredSetting.table {
			parts[i] = accountmask.Mask(parts[i])
		}
	}

	return strings.Join(parts, ".")
}

// keyPartText is part bare when TOML allows it, else tomlstr.BasicString(part).
func keyPartText(part string) string {
	if bareKey.MatchString(part) {
		return part
	}

	return tomlstr.BasicString(part)
}
