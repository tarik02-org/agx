package config

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/tarik02-org/agx/internal/yaml"
)

// item holds what a selection entry was written as: a scalar reference or path, or an inline
// definition, with its position for errors and explanations.
type item struct {
	scalar       string
	inline       bool
	line, column int
}

func decodeItem(node *yaml.Node, kind string, inline any) (item, error) {
	i := item{line: node.Line, column: node.Column}
	switch node.Kind {
	case yaml.ScalarNode:
		if node.ShortTag() != "!!str" || node.Value == "" {
			return i, fmt.Errorf("%s entry must be an ID, a path, or an inline definition", kind)
		}
		i.scalar = node.Value
		return i, nil
	case yaml.MappingNode:
		i.inline = true
		return i, node.Load(inline, yaml.WithKnownFields())
	default:
		return i, fmt.Errorf("%s entry must be an ID, a path, or an inline definition", kind)
	}
}

// SkillItem is a skills selection entry: an ID, a skill directory path, or an inline definition.
type SkillItem struct {
	ID    ID `yaml:"id"      jsonschema_description:"Skill ID; defaults to the folder name of path."`
	Skill `     yaml:",inline"`
	item  item
}

type skillItemFields SkillItem

func (s *SkillItem) UnmarshalYAML(node *yaml.Node) error {
	var fields skillItemFields
	i, err := decodeItem(node, "skills", &fields)
	fields.item = i
	*s = SkillItem(fields)
	return err
}

// InstructionsItem is an instructions selection entry: an ID, a file path, or an inline definition.
type InstructionsItem struct {
	ID           ID `yaml:"id"      jsonschema_description:"Instructions ID; defaults to the file stem of path."`
	Instructions `     yaml:",inline"`
	item         item
}

type instructionsItemFields InstructionsItem

func (s *InstructionsItem) UnmarshalYAML(node *yaml.Node) error {
	var fields instructionsItemFields
	i, err := decodeItem(node, "instructions", &fields)
	fields.item = i
	*s = InstructionsItem(fields)
	return err
}

// MCPItem is an mcp selection entry: an ID or an inline definition with an explicit id.
type MCPItem struct {
	ID        ID `yaml:"id"      jsonschema_description:"MCP server ID, used for logins and native server names."`
	MCPServer `     yaml:",inline"`
	item      item
}

type mcpItemFields MCPItem

func (s *MCPItem) UnmarshalYAML(node *yaml.Node) error {
	var fields mcpItemFields
	i, err := decodeItem(node, "mcp", &fields)
	fields.item = i
	*s = MCPItem(fields)
	return err
}

// selection merges a selection as written: references are qualified, and paths and inline
// definitions are defined under their IDs.
func (m *merger) selection(spec SelectionSpec) (Selection, error) {
	out := Selection{}
	for _, entry := range spec.Skills {
		id, err := m.skillItem(entry)
		if err != nil {
			return out, err
		}
		out.Skills = append(out.Skills, id)
	}
	for _, entry := range spec.Instructions {
		id, err := m.instructionsItem(entry)
		if err != nil {
			return out, err
		}
		out.Instructions = append(out.Instructions, id)
	}
	for _, entry := range spec.MCP {
		id, err := m.mcpItem(entry)
		if err != nil {
			return out, err
		}
		out.MCP = append(out.MCP, id)
	}
	for _, id := range spec.Bundles {
		if !validID(id) {
			return out, fmt.Errorf("%s: invalid bundle ID %q", m.path, id)
		}
		out.Bundles = append(out.Bundles, qualify(m.ns, id))
	}
	return out, nil
}

// reference qualifies an ID entry, or reports that the entry is a path or inline definition.
func (m *merger) reference(i item, kind string) (ID, bool, error) {
	if i.inline || IsPath(i.scalar) {
		return "", false, nil
	}
	if !validID(ID(i.scalar)) {
		return "", false, fmt.Errorf("%s: invalid %s ID %q", locate(m.at(i)), kind, i.scalar)
	}
	return qualify(m.ns, ID(i.scalar)), true, nil
}

func (m *merger) at(i item) Source { return Source{Path: m.path, Line: i.line, Column: i.column} }

func (m *merger) skillItem(entry SkillItem) (ID, error) {
	at := m.at(entry.item)
	if id, ok, err := m.reference(entry.item, "skill"); ok || err != nil {
		return id, err
	}
	id, s := entry.ID, entry.Skill
	if !entry.item.inline {
		s = Skill{Path: entry.item.scalar}
	}
	if s.Override {
		return "", fmt.Errorf("%s: inline definitions cannot set override; define the skill at top level", locate(at))
	}
	if id == "" {
		var err error
		switch {
		case s.Generated():
			return "", fmt.Errorf("%s: an inline skill with content needs an id", locate(at))
		case s.Git != "" && s.Path == "":
			id, err = deriveID(at, RepositoryName(s.Git))
		default:
			id, err = deriveID(at, path.Base(filepath.ToSlash(filepath.Clean(s.Path))))
		}
		if err != nil {
			return "", err
		}
	}
	name := qualify(m.ns, id)
	return name, m.addSkill(id, s, at, true)
}

func (m *merger) instructionsItem(entry InstructionsItem) (ID, error) {
	at := m.at(entry.item)
	if id, ok, err := m.reference(entry.item, "instructions"); ok || err != nil {
		return id, err
	}
	id, i := entry.ID, entry.Instructions
	if !entry.item.inline {
		i = Instructions{Path: entry.item.scalar}
	}
	if i.Override {
		return "", fmt.Errorf(
			"%s: inline definitions cannot set override; define the instructions at top level",
			locate(at),
		)
	}
	if id == "" {
		if i.Text != "" || i.Path == "" {
			return "", fmt.Errorf("%s: inline instructions with text need an id", locate(at))
		}
		base := path.Base(filepath.ToSlash(filepath.Clean(i.Path)))
		var err error
		id, err = deriveID(at, strings.TrimSuffix(base, path.Ext(base)))
		if err != nil {
			return "", err
		}
	}
	return qualify(m.ns, id), m.addInstructions(id, i, at, true)
}

func (m *merger) mcpItem(entry MCPItem) (ID, error) {
	at := m.at(entry.item)
	if !entry.item.inline && IsPath(entry.item.scalar) {
		return "", fmt.Errorf(
			"%s: MCP servers are referenced by ID or defined inline with an id, not by path",
			locate(at),
		)
	}
	if id, ok, err := m.reference(entry.item, "MCP server"); ok || err != nil {
		return id, err
	}
	if entry.ID == "" {
		return "", fmt.Errorf("%s: an inline MCP server needs an id; it names logins and native servers", locate(at))
	}
	if entry.Override {
		return "", fmt.Errorf(
			"%s: inline definitions cannot set override; define the MCP server at top level",
			locate(at),
		)
	}
	return qualify(m.ns, entry.ID), m.addMCP(entry.ID, entry.MCPServer, at, true)
}

func deriveID(at Source, name string) (ID, error) {
	if !validIDSegment.MatchString(name) {
		return "", fmt.Errorf("%s: cannot derive an ID from %q; use a mapping with an id", locate(at), name)
	}
	return ID(name), nil
}

// RepositoryName is the last segment of a Git URL or path, without .git. A repository named just
// "skills" is named after its owner instead, so anthropics/skills and mattpocock/skills don't
// collide.
func RepositoryName(source string) string {
	segments := strings.FieldsFunc(strings.TrimSuffix(filepath.ToSlash(source), ".git"), func(r rune) bool {
		return r == '/' || r == ':'
	})
	if len(segments) == 0 {
		return ""
	}
	name := strings.TrimSuffix(segments[len(segments)-1], ".git")
	if name == "skills" && len(segments) > 1 {
		return segments[len(segments)-2]
	}
	return name
}
