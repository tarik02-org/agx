package edit

import (
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tarik02-org/agx/internal/yaml"

	"github.com/tarik02-org/agx/internal/config"
)

// Kinds are the selection list keys, which are also the names of their top-level sections.
const (
	Skills       = "skills"
	Instructions = "instructions"
	MCP          = "mcp"
	Bundles      = "bundles"
)

// Scalar is a plain string node.
func Scalar(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

// Tagged is a scalar with a tag such as !secret.
func Tagged(tag, value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value}
}

// Mapping builds a block mapping from alternating keys and values.
func Mapping(pairs ...any) *yaml.Node {
	m := mapping()
	for i := 0; i+1 < len(pairs); i += 2 {
		Set(m, pairs[i].(string), node(pairs[i+1]))
	}
	return m
}

// Sequence builds a sequence of strings or nodes, in flow style when flow is set.
func Sequence(flow bool, items ...any) *yaml.Node {
	s := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	if flow {
		s.Style = yaml.FlowStyle
	}
	for _, item := range items {
		s.Content = append(s.Content, node(item))
	}
	return s
}

func node(v any) *yaml.Node {
	switch v := v.(type) {
	case *yaml.Node:
		return v
	case string:
		return Scalar(v)
	}
	panic("edit: unsupported node value")
}

func mapping() *yaml.Node { return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"} }

// block lets an empty flow collection grow in block style: {} stays readable as entries arrive.
func block(n *yaml.Node) {
	if len(n.Content) == 0 {
		n.Style &^= yaml.FlowStyle
	}
}

// Get returns the value of key in mapping m, or nil.
func Get(m *yaml.Node, key string) *yaml.Node {
	if i := index(m, key); i >= 0 {
		return m.Content[i+1]
	}
	return nil
}

func index(m *yaml.Node, key string) int {
	if m == nil || m.Kind != yaml.MappingNode {
		return -1
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return i
		}
	}
	return -1
}

// Set replaces the value of key, or appends the key.
func Set(m *yaml.Node, key string, value *yaml.Node) {
	if i := index(m, key); i >= 0 {
		m.Content[i+1] = value
		return
	}
	block(m)
	m.Content = append(m.Content, Scalar(key), value)
}

// Delete removes key from m and reports whether it was present.
func Delete(m *yaml.Node, key string) bool {
	i := index(m, key)
	if i < 0 {
		return false
	}
	m.Content = slices.Delete(m.Content, i, i+2)
	return true
}

func (f *File) root() *yaml.Node { return f.doc.Content[0] }

// Section returns the mapping under a top-level key, creating it when create is set.
func (f *File) Section(name string, create bool) *yaml.Node {
	return child(f.root(), name, create)
}

// child returns the mapping under key in m; a missing or null value becomes a mapping when
// create is set.
func child(m *yaml.Node, key string, create bool) *yaml.Node {
	v := Get(m, key)
	if v != nil && v.Kind == yaml.MappingNode {
		return v
	}
	if !create || v != nil && !null(v) {
		return nil
	}
	v = mapping()
	Set(m, key, v)
	return v
}

func null(n *yaml.Node) bool { return n.Kind == yaml.ScalarNode && n.ShortTag() == "!!null" }

// Definition returns the definition of id under a top-level section, or nil.
func (f *File) Definition(section, id string) *yaml.Node { return Get(f.Section(section, false), id) }

// Define adds a definition under a top-level section; it must not exist yet.
func (f *File) Define(section, id string, value *yaml.Node) {
	Set(f.Section(section, true), id, value)
}

// Undefine removes a top-level definition, and its section when it becomes empty.
func (f *File) Undefine(section, id string) bool {
	m := f.Section(section, false)
	if !Delete(m, id) {
		return false
	}
	if len(m.Content) == 0 {
		Delete(f.root(), section)
	}
	return true
}

// Rule is one entry of a fragment's scopes.
type Rule struct {
	Node *yaml.Node
	// Directory is canonical, or empty for a rule that applies everywhere.
	Directory string
	Harnesses []string
}

// Matches reports whether the rule has this directory and harness filter.
func (r Rule) Matches(directory string, harnesses []string) bool {
	return r.Directory == directory && slices.Equal(sortedUnique(r.Harnesses), sortedUnique(harnesses))
}

// Rules lists the fragment's scope rules; rules whose directory cannot be read are skipped, as
// loading reports them.
func (f *File) Rules() []Rule {
	scopes := Get(f.root(), "scopes")
	if scopes == nil || scopes.Kind != yaml.SequenceNode {
		return nil
	}
	out := []Rule{}
	for _, n := range scopes.Content {
		if n.Kind != yaml.MappingNode {
			continue
		}
		r := Rule{Node: n}
		if d := Get(n, "directory"); d != nil && d.Value != "" {
			directory, err := config.ScopeDirectory(f.Path, d.Value)
			if err != nil {
				continue
			}
			r.Directory = directory
		}
		if h := Get(n, "harnesses"); h != nil {
			for _, name := range h.Content {
				r.Harnesses = append(r.Harnesses, name.Value)
			}
		}
		out = append(out, r)
	}
	return out
}

// AddRule appends a scope rule for directory as written, or everywhere when it is empty.
func (f *File) AddRule(directory string, harnesses []string) *yaml.Node {
	scopes := Get(f.root(), "scopes")
	if scopes == nil || scopes.Kind != yaml.SequenceNode {
		scopes = Sequence(false)
		Set(f.root(), "scopes", scopes)
	}
	rule := mapping()
	if directory != "" {
		Set(rule, "directory", Scalar(directory))
	}
	if len(harnesses) > 0 {
		names := []any{}
		for _, name := range harnesses {
			names = append(names, name)
		}
		Set(rule, "harnesses", Sequence(true, names...))
	}
	block(scopes)
	scopes.Content = append(scopes.Content, rule)
	f.touched[rule] = true
	return rule
}

// List returns the kind list of a selection mapping (a rule's add or remove, a bundle, or a
// harness's add or remove), creating it when create is set. A new list is written in flow style
// unless its siblings use block style.
func (f *File) List(selection *yaml.Node, kind string, create bool) *yaml.Node {
	if selection == nil {
		return nil
	}
	if v := Get(selection, kind); v != nil && v.Kind == yaml.SequenceNode {
		return v
	} else if !create || v != nil && !null(v) {
		return nil
	}
	flow := true
	for i := 1; i < len(selection.Content); i += 2 {
		if sibling := selection.Content[i]; sibling.Kind == yaml.SequenceNode && sibling.Style&yaml.FlowStyle == 0 &&
			len(sibling.Content) > 0 {
			flow = false
		}
	}
	list := Sequence(flow)
	setOrdered(selection, kind, list, selectionKinds)
	return list
}

// selectionKinds is the order of a selection's lists as configuration documents write them.
var selectionKinds = []string{"skills", "instructions", "mcp", "bundles"}

// setOrdered sets key like Set, but places a new key before the first sibling that order puts
// after it, so a list recreated after its last entry was removed returns to its usual place.
func setOrdered(m *yaml.Node, key string, value *yaml.Node, order []string) {
	rank := slices.Index(order, key)
	if index(m, key) >= 0 || rank < 0 {
		Set(m, key, value)
		return
	}
	block(m)
	for i := 0; i+1 < len(m.Content); i += 2 {
		if slices.Index(order, m.Content[i].Value) > rank {
			m.Content = slices.Insert(m.Content, i, Scalar(key), value)
			return
		}
	}
	m.Content = append(m.Content, Scalar(key), value)
}

// Selection returns the add or remove mapping of a rule or harness, creating it when create is set.
func (f *File) Selection(owner *yaml.Node, change string, create bool) *yaml.Node {
	f.touched[owner] = true
	v := Get(owner, change)
	if v != nil && v.Kind == yaml.MappingNode {
		return v
	}
	if !create || v != nil && !null(v) {
		return nil
	}
	v = mapping()
	setOrdered(owner, change, v, []string{"add", "remove"})
	return v
}

// Find returns the position of the entry for id in list, or -1.
func Find(kind string, list *yaml.Node, id string) int {
	if list == nil {
		return -1
	}
	for i, entry := range list.Content {
		if found, _ := EntryID(kind, entry); found == id {
			return i
		}
	}
	return -1
}

// Append adds an ID reference to a list.
func Append(list *yaml.Node, id string) {
	list.Content = append(list.Content, Scalar(id))
}

// RemoveAt drops the entry at i from a list.
func RemoveAt(list *yaml.Node, i int) {
	list.Content = slices.Delete(list.Content, i, i+1)
}

// EntryID is the ID a selection entry refers to, or defines when defines is set: a path or an
// inline definition. It follows the derivation rules of configuration loading.
func EntryID(kind string, entry *yaml.Node) (id string, defines bool) {
	switch entry.Kind {
	case yaml.ScalarNode:
		if kind == Bundles || !config.IsPath(entry.Value) {
			return entry.Value, false
		}
		return derived(kind, entry.Value), true
	case yaml.MappingNode:
		if id := Get(entry, "id"); id != nil {
			return id.Value, true
		}
		p := ""
		if v := Get(entry, "path"); v != nil {
			p = v.Value
		}
		if git := Get(entry, "git"); kind == Skills && git != nil && p == "" {
			return config.RepositoryName(git.Value), true
		}
		return derived(kind, p), true
	default:
		return "", false
	}
}

func derived(kind, value string) string {
	base := path.Base(filepath.ToSlash(filepath.Clean(value)))
	if kind == Instructions {
		return strings.TrimSuffix(base, path.Ext(base))
	}
	return base
}

// Reference is one selection entry naming an ID.
type Reference struct {
	// Where describes the list, such as "scope ~/work add" or "bundle dev".
	Where   string
	Defines bool
	// Enables marks a plain ID in a scope rule's add list, the entry use and skill add write.
	Enables bool
	list    *yaml.Node
	entry   *yaml.Node
	owner   *yaml.Node
}

// References lists the selection entries for id of a kind: in scope rules, bundles, and harness
// selections.
func (f *File) References(kind, id string) []Reference {
	out := []Reference{}
	collect := func(where string, owner, selection *yaml.Node, enables bool) {
		list := f.List(selection, kind, false)
		if list == nil {
			return
		}
		for _, entry := range list.Content {
			if found, defines := EntryID(kind, entry); found == id {
				out = append(out, Reference{
					Where:   where,
					Defines: defines,
					Enables: enables && !defines,
					list:    list,
					entry:   entry,
					owner:   owner,
				})
			}
		}
	}
	for _, r := range f.Rules() {
		where := "scope everywhere"
		if d := Get(r.Node, "directory"); d != nil && d.Value != "" {
			where = "scope " + d.Value
		}
		if len(r.Harnesses) > 0 {
			where += " (" + strings.Join(r.Harnesses, ", ") + ")"
		}
		for _, change := range []string{"add", "remove"} {
			collect(where+" "+change, r.Node, Get(r.Node, change), change == "add")
		}
	}
	if bundles := f.Section(Bundles, false); bundles != nil {
		for i := 0; i+1 < len(bundles.Content); i += 2 {
			collect("bundle "+bundles.Content[i].Value, nil, bundles.Content[i+1], false)
		}
	}
	if harnesses := f.Section("harnesses", false); harnesses != nil {
		for i := 0; i+1 < len(harnesses.Content); i += 2 {
			h := harnesses.Content[i+1]
			for _, change := range []string{"add", "remove"} {
				collect("harness "+harnesses.Content[i].Value+" "+change, h, Get(h, change), false)
			}
		}
	}
	return out
}

// Remove drops a referencing entry from its list.
func (f *File) Remove(r Reference) {
	if i := slices.Index(r.list.Content, r.entry); i >= 0 {
		RemoveAt(r.list, i)
	}
	if r.owner != nil {
		f.touched[r.owner] = true
	}
}

// Prune drops the empty lists, add and remove mappings, and scope rules that edits left behind.
// Only rules and harnesses an edit touched are pruned, so empty rules written by hand stay.
func (f *File) Prune() {
	for owner := range f.touched {
		for _, change := range []string{"add", "remove"} {
			selection := Get(owner, change)
			if selection == nil || selection.Kind != yaml.MappingNode {
				continue
			}
			for _, kind := range []string{Skills, Instructions, MCP, Bundles} {
				if list := Get(
					selection,
					kind,
				); list != nil && list.Kind == yaml.SequenceNode &&
					len(list.Content) == 0 {
					Delete(selection, kind)
				}
			}
			if len(selection.Content) == 0 {
				Delete(owner, change)
			}
		}
	}
	scopes := Get(f.root(), "scopes")
	if scopes == nil || scopes.Kind != yaml.SequenceNode {
		return
	}
	before := len(scopes.Content)
	scopes.Content = slices.DeleteFunc(scopes.Content, func(rule *yaml.Node) bool {
		return f.touched[rule] && Get(rule, "add") == nil && Get(rule, "remove") == nil
	})
	if len(scopes.Content) == 0 && before > 0 {
		Delete(f.root(), "scopes")
	}
}

// Root returns the top-level mapping, for edits outside the selection helpers.
func (f *File) Root() *yaml.Node { return f.root() }

// Touch marks a rule or harness as changed so Prune considers it.
func (f *File) Touch(owner *yaml.Node) { f.touched[owner] = true }
