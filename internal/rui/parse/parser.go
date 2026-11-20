// Package parse implements the deliberately bounded RUI v1 YAML authoring
// subset. It has no third-party dependencies, no aliases/includes/tags and no
// runtime execution. A general YAML compiler may replace its tokenizer in M2
// without changing the public AST or M1 semantic validator.
package parse

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"roundos/sdk/rui/spec"
)

const MaxSourceBytes = 128 << 10
const MaxLines = 2500
const MaxDepth = 16

type Diagnostic struct {
	Pos     spec.Pos
	Message string
}

func (e Diagnostic) Error() string {
	return fmt.Sprintf("%d:%d: %s", e.Pos.Line, e.Pos.Column, e.Message)
}
func fail(p spec.Pos, format string, args ...any) error {
	return Diagnostic{p, fmt.Sprintf(format, args...)}
}

type line struct {
	indent  int
	content string
	pos     spec.Pos
}
type node struct {
	kind     string
	pos      spec.Pos
	raw      string
	quoted   bool
	mapping  map[string]*node
	order    []string
	sequence []*node
}

func (n *node) get(k string) *node {
	if n == nil {
		return nil
	}
	return n.mapping[k]
}

// Parse accepts indentation-based YAML mappings/sequences, comments, single
// and double quoted scalars, and single-key flow binding maps `{ bind: key }`.
// Unsupported YAML features fail closed rather than being misinterpreted.
func Parse(data []byte) (*spec.Document, error) {
	if len(data) > MaxSourceBytes {
		return nil, fail(spec.Pos{Line: 1, Column: 1}, "source exceeds %d bytes", MaxSourceBytes)
	}
	if !utf8.Valid(data) {
		return nil, fail(spec.Pos{Line: 1, Column: 1}, "invalid UTF-8")
	}
	if strings.ContainsRune(string(data), '\x00') {
		return nil, fail(spec.Pos{Line: 1, Column: 1}, "NUL is not allowed")
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if strings.ContainsRune(text, '\r') {
		return nil, fail(spec.Pos{Line: 1, Column: 1}, "bare CR is not allowed")
	}
	rows := strings.Split(text, "\n")
	if len(rows) > MaxLines {
		return nil, fail(spec.Pos{Line: 1, Column: 1}, "too many lines")
	}
	lines := make([]line, 0, len(rows))
	for i, r := range rows {
		if strings.ContainsRune(r, '\t') {
			return nil, fail(spec.Pos{Line: i + 1, Column: 1}, "tabs are not allowed")
		}
		cut := len(r) - len(strings.TrimLeft(r, " "))
		if cut%2 != 0 {
			return nil, fail(spec.Pos{Line: i + 1, Column: 1}, "indentation must use pairs of spaces")
		}
		content, err := stripComment(r[cut:])
		if err != nil {
			return nil, fail(spec.Pos{Line: i + 1, Column: cut + 1}, "%v", err)
		}
		content = strings.TrimSpace(content)
		if content == "" {
			continue
		}
		pos := spec.Pos{Line: i + 1, Column: cut + 1}
		if strings.HasPrefix(content, "---") || strings.HasPrefix(content, "...") {
			return nil, fail(pos, "document markers and multiple documents are not supported")
		}
		if strings.HasPrefix(content, "!") || strings.HasPrefix(content, "&") || strings.HasPrefix(content, "*") || strings.HasPrefix(content, "%") {
			return nil, fail(pos, "YAML tags, aliases and directives are forbidden")
		}
		lines = append(lines, line{cut, content, pos})
	}
	if len(lines) == 0 {
		return nil, fail(spec.Pos{Line: 1, Column: 1}, "document is empty")
	}
	if lines[0].indent != 0 {
		return nil, fail(lines[0].pos, "root must start at column 1")
	}
	root, next, err := parseBlock(lines, 0, 0, 0)
	if err != nil {
		return nil, err
	}
	if next < len(lines) {
		return nil, fail(lines[next].pos, "unexpected trailing document content")
	}
	return decodeDocument(root)
}
func stripComment(s string) (string, error) {
	quote := rune(0)
	escape := false
	for i, r := range s {
		if quote == '"' && escape {
			escape = false
			continue
		}
		if quote == '"' && r == '\\' {
			escape = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if r == '#' && (i == 0 || s[i-1] == ' ') {
			return s[:i], nil
		}
	}
	if quote != 0 {
		return "", fmt.Errorf("unterminated quoted scalar")
	}
	return s, nil
}
func splitKV(s string) (string, string, bool) {
	quote := rune(0)
	escape := false
	for i, r := range s {
		if quote == '"' && escape {
			escape = false
			continue
		}
		if quote == '"' && r == '\\' {
			escape = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if r == ':' && (i == len(s)-1 || s[i+1] == ' ') {
			return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:]), true
		}
	}
	return "", "", false
}
func parseBlock(lines []line, from, indent, depth int) (*node, int, error) {
	if depth > MaxDepth {
		return nil, from, fail(lines[from].pos, "maximum nesting depth exceeded")
	}
	if from >= len(lines) {
		return nil, from, fmt.Errorf("internal parser bounds error")
	}
	if lines[from].indent != indent {
		return nil, from, fail(lines[from].pos, "invalid indentation: expected %d spaces", indent)
	}
	if strings.HasPrefix(lines[from].content, "- ") {
		return parseSeq(lines, from, indent, depth)
	}
	return parseMap(lines, from, indent, depth)
}
func parseMap(lines []line, from, indent, depth int) (*node, int, error) {
	out := &node{kind: "map", pos: lines[from].pos, mapping: map[string]*node{}}
	i := from
	for i < len(lines) && lines[i].indent >= indent {
		l := lines[i]
		if l.indent > indent {
			return nil, i, fail(l.pos, "unexpected mapping indentation")
		}
		if strings.HasPrefix(l.content, "- ") {
			return nil, i, fail(l.pos, "sequence item inside mapping without a key")
		}
		key, value, ok := splitKV(l.content)
		if !ok || !validKey(key) {
			return nil, i, fail(l.pos, "expected `key: value` with an identifier key")
		}
		if _, exists := out.mapping[key]; exists {
			return nil, i, fail(l.pos, "duplicate key %q", key)
		}
		out.order = append(out.order, key)
		i++
		var child *node
		if value == "" && i < len(lines) && lines[i].indent > indent {
			if lines[i].indent != indent+2 {
				return nil, i, fail(lines[i].pos, "nested content must indent exactly two spaces")
			}
			var err error
			child, i, err = parseBlock(lines, i, indent+2, depth+1)
			if err != nil {
				return nil, i, err
			}
		} else {
			if value == "" {
				return nil, i, fail(l.pos, "field %q has an empty/null value", key)
			}
			pos := spec.Pos{Line: l.pos.Line, Column: l.pos.Column + strings.Index(l.content, ":") + 2}
			var err error
			child, err = parseScalar(value, pos)
			if err != nil {
				return nil, i, err
			}
		}
		out.mapping[key] = child
	}
	return out, i, nil
}
func parseSeq(lines []line, from, indent, depth int) (*node, int, error) {
	out := &node{kind: "seq", pos: lines[from].pos}
	i := from
	for i < len(lines) && lines[i].indent >= indent {
		l := lines[i]
		if l.indent != indent {
			return nil, i, fail(l.pos, "unexpected sequence indentation")
		}
		if !strings.HasPrefix(l.content, "- ") {
			return nil, i, fail(l.pos, "expected sequence entry starting `- `")
		}
		content := strings.TrimSpace(l.content[2:])
		if content == "" {
			return nil, i, fail(l.pos, "bare `-` entries are unsupported; put the first key on the same line")
		}
		j := i + 1
		for j < len(lines) && lines[j].indent > indent {
			j++
		}
		if _, _, ok := splitKV(content); ok {
			sub := make([]line, 0, j-i)
			sub = append(sub, line{indent + 2, content, spec.Pos{Line: l.pos.Line, Column: l.pos.Column + 2}})
			sub = append(sub, lines[i+1:j]...)
			item, used, err := parseBlock(sub, 0, indent+2, depth+1)
			if err != nil {
				return nil, i, err
			}
			if used != len(sub) {
				return nil, i, fail(sub[used].pos, "invalid sequence entry")
			}
			out.sequence = append(out.sequence, item)
		} else {
			if j != i+1 {
				return nil, i, fail(l.pos, "scalar entry cannot have nested fields")
			}
			item, err := parseScalar(content, spec.Pos{Line: l.pos.Line, Column: l.pos.Column + 2})
			if err != nil {
				return nil, i, err
			}
			out.sequence = append(out.sequence, item)
		}
		i = j
	}
	return out, i, nil
}
func parseScalar(s string, pos spec.Pos) (*node, error) {
	if len(s) > 2048 {
		return nil, fail(pos, "scalar exceeds 2048 bytes")
	}
	if strings.HasPrefix(s, "{") {
		if !strings.HasSuffix(s, "}") {
			return nil, fail(pos, "unclosed flow mapping")
		}
		inner := strings.TrimSpace(s[1 : len(s)-1])
		key, value, ok := splitKV(inner)
		if !ok || key != "bind" || strings.Contains(value, ",") {
			return nil, fail(pos, "only single-key flow mapping `{ bind: name }` is allowed")
		}
		child, err := parseScalar(value, pos)
		if err != nil {
			return nil, err
		}
		return &node{kind: "map", pos: pos, mapping: map[string]*node{"bind": child}, order: []string{"bind"}}, nil
	}
	if strings.HasPrefix(s, "[") || strings.HasPrefix(s, "|") || strings.HasPrefix(s, ">") || strings.HasPrefix(s, "!") || strings.HasPrefix(s, "&") || strings.HasPrefix(s, "*") || strings.Contains(s, "<<:") {
		return nil, fail(pos, "unsupported YAML syntax (aliases, tags, block scalars, flow sequences are forbidden)")
	}
	n := &node{kind: "scalar", pos: pos, raw: s}
	if strings.HasPrefix(s, `"`) {
		decoded, err := strconv.Unquote(s)
		if err != nil {
			return nil, fail(pos, "invalid double-quoted string: %v", err)
		}
		n.raw = decoded
		n.quoted = true
	} else if strings.HasPrefix(s, "'") {
		if len(s) < 2 || !strings.HasSuffix(s, "'") {
			return nil, fail(pos, "invalid single-quoted string")
		}
		n.raw = strings.ReplaceAll(s[1:len(s)-1], "''", "'")
		n.quoted = true
	} else {
		if strings.ContainsAny(s, "{}[]\n") || strings.HasSuffix(s, ":") || strings.Contains(s, ": ") {
			return nil, fail(pos, "ambiguous plain scalar; quote it")
		}
		switch strings.ToLower(s) {
		case "null", "~", "yes", "no", "on", "off", "nan", ".nan", ".inf", "inf":
			return nil, fail(pos, "ambiguous YAML scalar %q; use an explicitly quoted string or true/false", s)
		}
	}
	return n, nil
}
func validKey(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}
