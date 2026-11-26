package workspace

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	sopsage "github.com/getsops/sops/v3/age"
	"gopkg.in/yaml.v3"
)

// Errors for changes to .sops.yaml the UI can recognize (and translate) by code.
var (
	ErrNoConfig         = errors.New("this repository has no .sops.yaml")
	ErrConfigLayout     = errors.New(".sops.yaml is written in a way Finnick does not change here: edit it by hand")
	ErrInvalidName      = errors.New("names use letters, digits, - and _, and start with a letter or digit")
	ErrNameTaken        = errors.New("another key in .sops.yaml already has this name")
	ErrInvalidRecipient = errors.New("not an age public key (age1...)")
	ErrPersonExists     = errors.New("this key already has a name in .sops.yaml")
	ErrPersonNotFound   = errors.New("this key is not in .sops.yaml")
	ErrRuleNotFound     = errors.New("no creation rule at this position")
	ErrLastRecipient    = errors.New("a rule must keep at least one recipient")
	ErrKeyGroups        = errors.New("rules with several key groups are edited by hand")
)

// ConfigOp is one change to .sops.yaml. Keys are age public keys; rules are positions in
// creation_rules.
//
//   - addPerson: name the key Key as Name (`- &Name Key` in the top-level keys list)
//   - removePerson: take Key out of every rule and drop its name
//   - grant / revoke: add Key to rule Rule, or take it out
type ConfigOp struct {
	Op   string `json:"op"`
	Name string `json:"name,omitempty"`
	Key  string `json:"key,omitempty"`
	Rule int    `json:"rule,omitempty"`
}

var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

func isAgeRecipient(s string) bool {
	if !strings.HasPrefix(s, "age1") {
		return false
	}
	_, err := sopsage.MasterKeyFromRecipient(s)
	return err == nil
}

// EditConfig applies ops to .sops.yaml, all or nothing, if the file is still at version. The file is
// changed as text, so everything the ops do not touch stays as it was; the result is read back and
// must say exactly what the ops mean before it replaces the file.
func (ws *Workspace) EditConfig(version string, ops []ConfigOp) (*Config, error) {
	ws.writeMu.Lock()
	defer ws.writeMu.Unlock()

	path := ws.ConfigPath()
	st, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoConfig
	}
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: it is not a regular file", ErrConfigLayout)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if Version(data) != version {
		return nil, ErrStale
	}
	before, err := parseConfig(path, data)
	if err != nil {
		return nil, err
	}
	t, err := newYAMLText(data)
	if err != nil {
		return nil, layoutErr(err)
	}
	want := modelOf(before)
	for _, op := range ops {
		if err := applyConfigOp(t, want, op); err != nil {
			return nil, layoutErr(err)
		}
	}
	out := t.bytes()
	after, err := parseConfig(path, out)
	if err != nil {
		return nil, layoutErr(err)
	}
	if !want.equal(modelOf(after)) {
		return nil, ErrConfigLayout
	}
	if emptyRules(after) > emptyRules(before) {
		return nil, ErrLastRecipient
	}
	if err := replaceFile(path, out); err != nil {
		return nil, err
	}
	ws.mu.Lock()
	ws.config = after
	ws.mu.Unlock()
	return after, nil
}

// layoutErr reports text the editor cannot change safely as ErrConfigLayout; known errors pass.
func layoutErr(err error) error {
	if errors.Is(err, errLayout) {
		return ErrConfigLayout
	}
	var yerr *yaml.TypeError
	if errors.As(err, &yerr) || strings.HasPrefix(err.Error(), "yaml:") {
		return fmt.Errorf("%w (%v)", ErrConfigLayout, err)
	}
	return err
}

func applyConfigOp(t *yamlText, want *model, op ConfigOp) error {
	if t.root == nil {
		return ErrNoConfig
	}
	key := strings.TrimSpace(op.Key)
	if !isAgeRecipient(key) {
		return ErrInvalidRecipient
	}
	switch op.Op {
	case "addPerson":
		if err := addPerson(t, op.Name, key); err != nil {
			return err
		}
		want.names[key] = op.Name
	case "removePerson":
		if err := removePerson(t, key); err != nil {
			return err
		}
		delete(want.names, key)
		for i := range want.rules {
			want.revoke(i, key)
		}
	case "grant":
		if op.Rule < 0 || op.Rule >= len(want.rules) {
			return ErrRuleNotFound
		}
		if err := grant(t, op.Rule, key); err != nil {
			return err
		}
		want.grant(op.Rule, key)
	case "revoke":
		if op.Rule < 0 || op.Rule >= len(want.rules) {
			return ErrRuleNotFound
		}
		if err := revoke(t, op.Rule, key); err != nil {
			return err
		}
		want.revoke(op.Rule, key)
	default:
		return fmt.Errorf("unknown operation %q", op.Op)
	}
	return nil
}

// anchors maps every anchor name in the document to its node.
func anchors(n *yaml.Node, out map[string]*yaml.Node) map[string]*yaml.Node {
	if out == nil {
		out = map[string]*yaml.Node{}
	}
	if n == nil {
		return out
	}
	if n.Anchor != "" {
		out[n.Anchor] = n
	}
	for _, c := range n.Content {
		anchors(c, out)
	}
	return out
}

// resolve is the key a scalar or an alias of one stands for.
func resolve(n *yaml.Node) string {
	if n.Kind == yaml.AliasNode && n.Alias != nil {
		n = n.Alias
	}
	if n.Kind != yaml.ScalarNode {
		return ""
	}
	return strings.TrimSpace(n.Value)
}

func addPerson(t *yamlText, name, key string) error {
	if !nameRe.MatchString(name) {
		return ErrInvalidName
	}
	all := anchors(t.root, nil)
	if _, taken := all[name]; taken {
		return ErrNameTaken
	}
	for _, n := range all {
		if n.Kind == yaml.ScalarNode && strings.TrimSpace(n.Value) == key {
			return ErrPersonExists
		}
	}
	item := "&" + name + " " + key
	ki, keys := entry(t.root, "keys")
	switch {
	case keys == nil:
		return addKeysList(t, item)
	case keys.Kind == yaml.SequenceNode && isBlock(keys) && len(keys.Content) > 0:
		return t.insertItem(keys, len(keys.Content), []string{item})
	case keys.Kind == yaml.SequenceNode:
		items, _, _, err := t.flowItems(keys)
		if err != nil {
			return err
		}
		return t.setFlowItems(keys, append(items, item))
	case keys.Kind == yaml.ScalarNode && keys.Tag == "!!null":
		k := t.root.Content[2*ki]
		kl := k.Line - 1
		if valueEnd(t.lines[kl], k.Column-1) != len(strings.TrimRight(t.lines[kl], " \t")) {
			return errLayout
		}
		return t.insert(kl+1, strings.Repeat(" ", k.Column-1+2)+"- "+item)
	}
	return errLayout
}

// addKeysList starts a top-level keys list above creation_rules, where aliases can refer to it.
func addKeysList(t *yamlText, item string) error {
	dash := 2
	at := len(t.lines)
	if ri, rules := entry(t.root, "creation_rules"); rules != nil {
		if rules.Kind == yaml.SequenceNode && isBlock(rules) {
			dash = rules.Column - 1
		}
		kl := t.root.Content[2*ri].Line - 1
		if at = t.headStart(kl, 0); at == 0 {
			at = kl // comments at the very top describe the file, not the rules: keep them first
		}
	}
	lines := []string{"keys:", strings.Repeat(" ", dash) + "- " + item, ""}
	if at > 0 && !blank(t.lines[at-1]) {
		lines = append([]string{""}, lines...)
	}
	if at == len(t.lines) {
		lines = lines[:len(lines)-1]
	}
	return t.insert(at, lines...)
}

// ageHolder is the mapping that holds the age keys of rule i: the rule, or its only key group.
func ageHolder(t *yamlText, i int) (*yaml.Node, error) {
	_, rules := entry(t.root, "creation_rules")
	if rules == nil || rules.Kind != yaml.SequenceNode || i < 0 || i >= len(rules.Content) {
		return nil, ErrRuleNotFound
	}
	if !isBlock(rules) {
		return nil, errLayout
	}
	r := rules.Content[i]
	if r.Kind != yaml.MappingNode || !isBlock(r) {
		return nil, errLayout
	}
	_, kg := entry(r, "key_groups")
	if kg == nil || (kg.Kind == yaml.SequenceNode && len(kg.Content) == 0) {
		return r, nil
	}
	if kg.Kind != yaml.SequenceNode || len(kg.Content) != 1 {
		return nil, ErrKeyGroups
	}
	g := kg.Content[0]
	if g.Kind != yaml.MappingNode || !isBlock(g) {
		return nil, errLayout
	}
	return g, nil
}

// ageKeys lists the keys an `age:` value names, whatever its form (list, alias, comma-separated).
func ageKeys(v *yaml.Node) []string {
	if v == nil {
		return nil
	}
	if v.Kind == yaml.SequenceNode {
		var out []string
		for _, c := range v.Content {
			out = append(out, resolve(c))
		}
		return out
	}
	if v.Kind == yaml.AliasNode {
		return []string{resolve(v)}
	}
	return splitKeys(v.Value)
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// refFor is how a rule at line names key: an alias when the key is anchored above, else the key.
func refFor(t *yamlText, key string, line int) string {
	for name, n := range anchors(t.root, nil) {
		if n.Kind == yaml.ScalarNode && strings.TrimSpace(n.Value) == key && n.Line < line {
			return "*" + name
		}
	}
	return key
}

func grant(t *yamlText, rule int, key string) error {
	h, err := ageHolder(t, rule)
	if err != nil {
		return err
	}
	idx, v := entry(h, "age")
	if contains(ageKeys(v), key) {
		return nil
	}
	ref := refFor(t, key, h.Line)
	switch {
	case v == nil:
		return t.addEntry(h, "age", "", "  - "+ref)
	case v.Kind == yaml.SequenceNode && isBlock(v) && len(v.Content) > 0:
		return t.insertItem(v, len(v.Content), []string{ref})
	case v.Kind == yaml.SequenceNode:
		items, _, _, err := t.flowItems(v)
		if err != nil {
			return err
		}
		return t.setFlowItems(v, append(items, ref))
	case v.Kind == yaml.AliasNode:
		return t.setValue(h, idx, "[*"+v.Value+", "+ref+"]")
	case v.Kind == yaml.ScalarNode && v.Tag == "!!null":
		return t.setValue(h, idx, "["+ref+"]")
	case v.Kind == yaml.ScalarNode:
		// A comma-separated string stays one: aliases cannot go inside it, the plain key can.
		text, err := scalarText(strings.Join(append(splitKeys(v.Value), key), ", "))
		if err != nil {
			return err
		}
		return t.setValue(h, idx, text)
	}
	return errLayout
}

func revoke(t *yamlText, rule int, key string) error {
	h, err := ageHolder(t, rule)
	if err != nil {
		return err
	}
	idx, v := entry(h, "age")
	keys := ageKeys(v)
	if !contains(keys, key) {
		return nil
	}
	var keep []string
	for _, k := range keys {
		if k != key {
			keep = append(keep, k)
		}
	}
	if len(keep) == 0 {
		if len(h.Content) == 2 {
			return ErrLastRecipient
		}
		return t.removeEntry(h, idx)
	}
	switch {
	case v.Kind == yaml.SequenceNode && isBlock(v):
		for {
			if h, err = ageHolder(t, rule); err != nil {
				return err
			}
			_, v = entry(h, "age")
			j := -1
			for i, c := range v.Content {
				if resolve(c) == key {
					j = i
					break
				}
			}
			if j < 0 {
				return nil
			}
			if err := t.removeItem(v, j); err != nil {
				return err
			}
		}
	case v.Kind == yaml.SequenceNode:
		items, _, _, err := t.flowItems(v)
		if err != nil {
			return err
		}
		var rest []string
		for i, c := range v.Content {
			if resolve(c) != key {
				rest = append(rest, items[i])
			}
		}
		return t.setFlowItems(v, rest)
	case v.Kind == yaml.ScalarNode:
		text, err := scalarText(strings.Join(keep, ", "))
		if err != nil {
			return err
		}
		return t.setValue(h, idx, text)
	}
	return errLayout
}

// mentions reports whether key appears anywhere under n.
func mentions(n *yaml.Node, key string) bool {
	if n.Kind == yaml.AliasNode || n.Kind == yaml.ScalarNode {
		for _, k := range splitKeys(resolve(n)) {
			if k == key {
				return true
			}
		}
		return false
	}
	for _, c := range n.Content {
		if mentions(c, key) {
			return true
		}
	}
	return false
}

func removePerson(t *yamlText, key string) error {
	found := false
	_, rules := entry(t.root, "creation_rules")
	n := 0
	if rules != nil {
		n = len(rules.Content)
	}
	for i := 0; i < n; i++ {
		h, err := ageHolder(t, i)
		if err != nil {
			if _, rules = entry(t.root, "creation_rules"); mentions(rules.Content[i], key) {
				return err
			}
			continue
		}
		_, v := entry(h, "age")
		if contains(ageKeys(v), key) {
			found = true
			if err := revoke(t, i, key); err != nil {
				return err
			}
		}
	}
	// The name itself, in the top-level keys list.
	for {
		_, keys := entry(t.root, "keys")
		if keys == nil || keys.Kind != yaml.SequenceNode {
			break
		}
		j := -1
		for i, c := range keys.Content {
			if c.Kind == yaml.ScalarNode && strings.TrimSpace(c.Value) == key {
				j = i
				break
			}
		}
		if j < 0 {
			break
		}
		found = true
		if isBlock(keys) {
			if err := t.removeItem(keys, j); err != nil {
				return err
			}
			continue
		}
		items, _, _, err := t.flowItems(keys)
		if err != nil {
			return err
		}
		if err := t.setFlowItems(keys, append(items[:j:j], items[j+1:]...)); err != nil {
			return err
		}
	}
	if !found {
		return ErrPersonNotFound
	}
	return nil
}

// model is what a config means for Finnick's checks: who each rule names and the names of keys.
type model struct {
	rules []modelRule
	names map[string]string // key → name
}

type modelRule struct {
	regex  string
	groups []map[string]bool // recipient keys per group
}

func modelOf(c *Config) *model {
	m := &model{names: map[string]string{}}
	for id, name := range c.aliases {
		m.names[id] = name
	}
	for _, r := range c.Rules {
		mr := modelRule{regex: r.PathRegex}
		for _, g := range r.KeyGroups {
			set := map[string]bool{}
			for _, rc := range g {
				set[rc.key()] = true
			}
			mr.groups = append(mr.groups, set)
		}
		m.rules = append(m.rules, mr)
	}
	return m
}

func (m *model) grant(i int, key string) {
	r := &m.rules[i]
	if len(r.groups) == 0 {
		r.groups = append(r.groups, map[string]bool{})
	}
	r.groups[0][Recipient{Type: "age", ID: key}.key()] = true
}

func (m *model) revoke(i int, key string) {
	for _, g := range m.rules[i].groups {
		delete(g, Recipient{Type: "age", ID: key}.key())
	}
}

func (m *model) equal(o *model) bool {
	if len(m.rules) != len(o.rules) || len(m.names) != len(o.names) {
		return false
	}
	for id, name := range m.names {
		if o.names[id] != name {
			return false
		}
	}
	for i, r := range m.rules {
		or := o.rules[i]
		var a, b []map[string]bool
		for _, g := range r.groups {
			if len(g) > 0 {
				a = append(a, g)
			}
		}
		for _, g := range or.groups {
			if len(g) > 0 {
				b = append(b, g)
			}
		}
		if r.regex != or.regex || len(a) != len(b) {
			return false
		}
		for j := range a {
			if len(a[j]) != len(b[j]) {
				return false
			}
			for k := range a[j] {
				if !b[j][k] {
					return false
				}
			}
		}
	}
	return true
}

// emptyRules counts rules (or key groups) that name nobody: SOPS cannot encrypt for them.
func emptyRules(c *Config) int {
	n := 0
	for _, r := range c.Rules {
		if len(r.KeyGroups) == 0 {
			n++
			continue
		}
		for _, g := range r.KeyGroups {
			if len(g) == 0 {
				n++
			}
		}
	}
	return n
}
