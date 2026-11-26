package workspace

import (
	"errors"
	"strings"

	"gopkg.in/yaml.v3"
)

// errLayout means .sops.yaml is written in a way the text editor below does not change safely.
var errLayout = errors.New("this part of .sops.yaml is written in a way Finnick does not change: edit it by hand")

// yamlText edits a YAML document as lines of text, guided by the positions of a yaml.v3 parse.
// Only the lines a change is about are touched, so comments, blank lines, quoting and key order
// elsewhere stay byte for byte. Every change parses the text again, so positions are always fresh:
// callers must look nodes up again after each change.
type yamlText struct {
	lines []string
	crlf  bool
	root  *yaml.Node // the top-level block mapping, nil for an empty document
}

func newYAMLText(data []byte) (*yamlText, error) {
	s := string(data)
	t := &yamlText{crlf: strings.Contains(s, "\r\n")}
	s = strings.TrimRight(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	if s != "" {
		t.lines = strings.Split(s, "\n")
	}
	return t, t.parse()
}

func (t *yamlText) bytes() []byte {
	s := strings.Join(t.lines, "\n") + "\n"
	if t.crlf {
		s = strings.ReplaceAll(s, "\n", "\r\n")
	}
	return []byte(s)
}

func (t *yamlText) parse() error {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(strings.Join(t.lines, "\n")), &doc); err != nil {
		return err
	}
	t.root = nil
	if doc.Kind == 0 || len(doc.Content) == 0 {
		return nil
	}
	root := doc.Content[0]
	if root.Kind == yaml.ScalarNode && root.Tag == "!!null" {
		return nil // only comments
	}
	if root.Kind != yaml.MappingNode || !isBlock(root) {
		return errLayout
	}
	t.root = root
	return nil
}

func (t *yamlText) insert(at int, lines ...string) error {
	t.lines = append(t.lines[:at], append(lines, t.lines[at:]...)...)
	return t.parse()
}

func (t *yamlText) remove(first, last int) {
	t.lines = append(t.lines[:first], t.lines[last+1:]...)
	// Do not leave two blank lines where one item stood between them.
	if first > 0 && first < len(t.lines) && blank(t.lines[first-1]) && blank(t.lines[first]) {
		t.lines = append(t.lines[:first], t.lines[first+1:]...)
	}
}

func isBlock(n *yaml.Node) bool { return n.Style&yaml.FlowStyle == 0 }

func blank(line string) bool { return strings.TrimSpace(line) == "" }

func isComment(line string) bool { return strings.HasPrefix(strings.TrimSpace(line), "#") }

// indent is the number of leading spaces of line, or -1 for a blank line.
func indent(line string) int {
	if blank(line) {
		return -1
	}
	return len(line) - len(strings.TrimLeft(line, " "))
}

// blockEnd is the last line of the block that starts at line start and continues with lines
// indented deeper than ind (nested values, block scalars, comments inside them).
func (t *yamlText) blockEnd(start, ind int) int {
	end := start
	for j := start + 1; j < len(t.lines); j++ {
		in := indent(t.lines[j])
		if in < 0 {
			continue
		}
		if in <= ind {
			break
		}
		end = j
	}
	return end
}

// headStart moves up from line over the comment lines right above it at the same indentation.
func (t *yamlText) headStart(line, ind int) int {
	for line > 0 && isComment(t.lines[line-1]) && indent(t.lines[line-1]) == ind {
		line--
	}
	return line
}

// entry finds key in mapping m and returns its index and value.
func entry(m *yaml.Node, key string) (int, *yaml.Node) {
	if m == nil || m.Kind != yaml.MappingNode {
		return -1, nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return i / 2, m.Content[i+1]
		}
	}
	return -1, nil
}

// itemSpan returns the lines of item i of the block sequence seq: from the comments right above
// it to its last line.
func (t *yamlText) itemSpan(seq *yaml.Node, i int) (first, last int, err error) {
	dash := seq.Column - 1
	start := seq.Content[i].Line - 1
	if start < 0 || start >= len(t.lines) {
		return 0, 0, errLayout
	}
	line := t.lines[start]
	if indent(line) != dash || line[dash] != '-' {
		return 0, 0, errLayout
	}
	return t.headStart(start, dash), t.blockEnd(start, dash), nil
}

// spaced reports whether every item of the block sequence seq is separated from the one before by a
// blank line, so new items follow suit.
func (t *yamlText) spaced(seq *yaml.Node) bool {
	if len(seq.Content) < 2 {
		return false
	}
	for i := 1; i < len(seq.Content); i++ {
		if first, _, err := t.itemSpan(seq, i); err != nil || first == 0 || !blank(t.lines[first-1]) {
			return false
		}
	}
	return true
}

// insertItem adds an item at position i of a block sequence (i == len appends). body is the item's
// text: its first line goes after the dash, the others are indented under it.
func (t *yamlText) insertItem(seq *yaml.Node, i int, body []string) error {
	if !isBlock(seq) || len(seq.Content) == 0 {
		return errLayout
	}
	pad := strings.Repeat(" ", seq.Column-1)
	lines := []string{pad + "- " + body[0]}
	for _, b := range body[1:] {
		if b == "" {
			lines = append(lines, "")
		} else {
			lines = append(lines, pad+"  "+b)
		}
	}
	spaced := t.spaced(seq)
	if i < len(seq.Content) {
		first, _, err := t.itemSpan(seq, i)
		if err != nil {
			return err
		}
		if spaced {
			lines = append(lines, "")
		}
		return t.insert(first, lines...)
	}
	_, last, err := t.itemSpan(seq, len(seq.Content)-1)
	if err != nil {
		return err
	}
	if spaced {
		lines = append([]string{""}, lines...)
	}
	return t.insert(last+1, lines...)
}

// removeItem deletes item i of a block sequence with the comments right above it.
func (t *yamlText) removeItem(seq *yaml.Node, i int) error {
	first, last, err := t.itemSpan(seq, i)
	if err != nil {
		return err
	}
	t.remove(first, last)
	return t.parse()
}

// flowItems splits a flow sequence written on one line into the source text of its items, and
// returns where the brackets are.
func (t *yamlText) flowItems(seq *yaml.Node) (items []string, open, close int, err error) {
	line := t.lines[seq.Line-1]
	open = seq.Column - 1
	if open >= len(line) || line[open] != '[' {
		return nil, 0, 0, errLayout
	}
	depth, start := 0, open+1
	var quote byte
	for j := open; j < len(line); j++ {
		c := line[j]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '[' || c == '{':
			depth++
		case c == ']' || c == '}':
			depth--
			if depth == 0 {
				if s := strings.TrimSpace(line[start:j]); s != "" {
					items = append(items, s)
				}
				if len(items) != len(seq.Content) {
					return nil, 0, 0, errLayout
				}
				return items, open, j, nil
			}
		case c == ',' && depth == 1:
			items = append(items, strings.TrimSpace(line[start:j]))
			start = j + 1
		}
	}
	return nil, 0, 0, errLayout // spans several lines
}

// setFlowItems rewrites a one-line flow sequence with the given item texts.
func (t *yamlText) setFlowItems(seq *yaml.Node, items []string) error {
	_, open, close, err := t.flowItems(seq)
	if err != nil {
		return err
	}
	l := seq.Line - 1
	t.lines[l] = t.lines[l][:open] + "[" + strings.Join(items, ", ") + "]" + t.lines[l][close+1:]
	return t.parse()
}

// entryEnd is the last line of entry idx of a block mapping.
func (t *yamlText) entryEnd(m *yaml.Node, idx int) int {
	k, v := m.Content[2*idx], m.Content[2*idx+1]
	end := t.blockEnd(k.Line-1, k.Column-1)
	if v.Kind == yaml.SequenceNode && isBlock(v) && len(v.Content) > 0 {
		// A sequence may sit at the same indentation as its key.
		if _, last, err := t.itemSpan(v, len(v.Content)-1); err == nil && last > end {
			end = last
		}
	}
	return end
}

// addEntry appends `key: inline` to a block mapping, followed by more lines indented under the key.
func (t *yamlText) addEntry(m *yaml.Node, key, inline string, more ...string) error {
	if !isBlock(m) || len(m.Content) == 0 {
		return errLayout
	}
	col := m.Content[0].Column - 1
	pad := strings.Repeat(" ", col)
	first := pad + key + ":"
	if inline != "" {
		first += " " + inline
	}
	lines := []string{first}
	for _, l := range more {
		lines = append(lines, pad+l)
	}
	return t.insert(t.entryEnd(m, len(m.Content)/2-1)+1, lines...)
}

// removeEntry deletes entry idx of a block mapping, with the comments right above it.
func (t *yamlText) removeEntry(m *yaml.Node, idx int) error {
	k := m.Content[2*idx]
	kl, col := k.Line-1, k.Column-1
	end := t.entryEnd(m, idx)
	prefix := t.lines[kl][:col]
	if strings.TrimSpace(prefix) == "" {
		t.remove(t.headStart(kl, col), end)
		return t.parse()
	}
	// The key shares its line with the dash of a sequence item: hand the dash to the next entry.
	if 2*idx+2 >= len(m.Content) {
		return errLayout
	}
	next := m.Content[2*idx+2]
	nl := next.Line - 1
	if next.Column-1 != col || indent(t.lines[nl]) != col {
		return errLayout
	}
	t.lines[nl] = prefix + t.lines[nl][col:]
	t.lines = append(t.lines[:kl], t.lines[nl:]...)
	return t.parse()
}

// setValue replaces the value of entry idx of a block mapping with a one-line text.
func (t *yamlText) setValue(m *yaml.Node, idx int, text string) error {
	k, v := m.Content[2*idx], m.Content[2*idx+1]
	kl := k.Line - 1
	end := t.entryEnd(m, idx)
	line := t.lines[kl]
	if v.Line-1 == kl && end == kl && !(v.Tag == "!!null" && v.Value == "") {
		vc := v.Column - 1
		t.lines[kl] = line[:vc] + text + line[valueEnd(line, vc):]
		return t.parse()
	}
	colon := strings.IndexByte(line[k.Column-1:], ':')
	if colon < 0 {
		return errLayout
	}
	t.lines[kl] = line[:k.Column-1+colon+1] + " " + text
	t.lines = append(t.lines[:kl+1], t.lines[end+1:]...)
	return t.parse()
}

// valueEnd is where a one-line value that starts at from ends: before a trailing comment.
func valueEnd(line string, from int) int {
	var quote byte
	depth := 0
	for j := from; j < len(line); j++ {
		c := line[j]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '[' || c == '{':
			depth++
		case c == ']' || c == '}':
			depth--
		case c == '#' && depth <= 0 && j > from && (line[j-1] == ' ' || line[j-1] == '\t'):
			return len(strings.TrimRight(line[:j], " \t"))
		}
	}
	return len(strings.TrimRight(line, " \t"))
}

// scalarText renders s as a one-line YAML scalar, quoted only when it needs to be.
func scalarText(s string) (string, error) {
	if strings.ContainsAny(s, "\n\r") {
		return "", errLayout
	}
	b, err := yaml.Marshal(s)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(string(b), "\n"), nil
}
