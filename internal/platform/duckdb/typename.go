package duckdb

import "strings"

// Kinds of typeNode that hold other types; every other kind is a scalar
// type's name as the driver spells it ("INTEGER", "DECIMAL", "TIMESTAMPTZ").
const (
	kindList   = "LIST"
	kindArray  = "ARRAY"
	kindStruct = "STRUCT"
	kindMap    = "MAP"
	kindUnion  = "UNION"
)

// typeNode is a parsed DuckDB column type name. A nil *typeNode is a type
// whose name did not parse; its accessors answer as if nothing is known.
type typeNode struct {
	kind       string
	elem       *typeNode   // LIST and ARRAY
	key, value *typeNode   // MAP
	fields     []typeField // STRUCT and UNION, in declared order
}

// typeField is one named member of a STRUCT or UNION type.
type typeField struct {
	name string
	typ  *typeNode
}

func (t *typeNode) kindName() string {
	if t == nil {
		return ""
	}
	return t.kind
}

func (t *typeNode) elemType() *typeNode {
	if t == nil {
		return nil
	}
	return t.elem
}

func (t *typeNode) mapTypes() (*typeNode, *typeNode) {
	if t == nil {
		return nil, nil
	}
	return t.key, t.value
}

// member returns the type of the STRUCT or UNION member called name, nil if
// there is none.
func (t *typeNode) member(name string) *typeNode {
	if t == nil {
		return nil
	}
	for _, f := range t.fields {
		if f.name == name {
			return f.typ
		}
	}
	return nil
}

// parseTypeName parses a driver type name: scalars, DECIMAL, MAP, STRUCT,
// UNION and [] or [N] suffixes; false for anything else.
func parseTypeName(name string) (*typeNode, bool) {
	p := typeParser{s: name}
	t, ok := p.parseType()
	return t, ok && p.i == len(p.s)
}

// typeParser is a recursive-descent parser over one type name.
type typeParser struct {
	s string
	i int
}

func (p *typeParser) parseType() (*typeNode, bool) {
	name := p.word()
	var t *typeNode
	switch name {
	case "":
		return nil, false
	case kindStruct, kindUnion:
		fields, ok := p.parseFields()
		if !ok {
			return nil, false
		}
		t = &typeNode{kind: name, fields: fields}
	case kindMap:
		key, value, ok := p.parseMapTypes()
		if !ok {
			return nil, false
		}
		t = &typeNode{kind: name, key: key, value: value}
	case "DECIMAL":
		if !p.eat("(") || p.word() == "" || !p.eat(",") || p.word() == "" || !p.eat(")") {
			return nil, false
		}
		t = &typeNode{kind: name}
	default:
		t = &typeNode{kind: name}
	}

	for p.eat("[") {
		kind := kindList
		if p.word() != "" {
			kind = kindArray
		}
		if !p.eat("]") {
			return nil, false
		}
		t = &typeNode{kind: kind, elem: t}
	}
	return t, true
}

func (p *typeParser) parseFields() ([]typeField, bool) {
	if !p.eat("(") {
		return nil, false
	}
	var fields []typeField
	for {
		name, ok := p.quotedName()
		if !ok || !p.eat(" ") {
			return nil, false
		}
		typ, ok := p.parseType()
		if !ok {
			return nil, false
		}
		fields = append(fields, typeField{name: name, typ: typ})
		if p.eat(")") {
			return fields, true
		}
		if !p.eat(", ") {
			return nil, false
		}
	}
}

func (p *typeParser) parseMapTypes() (*typeNode, *typeNode, bool) {
	if !p.eat("(") {
		return nil, nil, false
	}
	key, ok := p.parseType()
	if !ok || !p.eat(", ") {
		return nil, nil, false
	}
	value, ok := p.parseType()
	if !ok || !p.eat(")") {
		return nil, nil, false
	}
	return key, value, true
}

// quotedName reads a double-quoted member name, "" standing for one quote.
func (p *typeParser) quotedName() (string, bool) {
	if !p.eat(`"`) {
		return "", false
	}
	var b strings.Builder
	for p.i < len(p.s) {
		c := p.s[p.i]
		p.i++
		if c != '"' {
			b.WriteByte(c)
			continue
		}
		if !p.eat(`"`) {
			return b.String(), true
		}
		b.WriteByte('"')
	}
	return "", false
}

// word reads a run of upper-case letters, digits and underscores.
func (p *typeParser) word() string {
	start := p.i
	for p.i < len(p.s) {
		c := p.s[p.i]
		if (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '_' {
			break
		}
		p.i++
	}
	return p.s[start:p.i]
}

func (p *typeParser) eat(token string) bool {
	if !strings.HasPrefix(p.s[p.i:], token) {
		return false
	}
	p.i += len(token)
	return true
}
