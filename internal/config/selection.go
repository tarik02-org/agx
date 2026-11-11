package config

import (
	"path"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v4"
)

// item holds what a selection entry was written as: a scalar reference or path, or an inline
// definition, with its position for errors and explanations.
type item struct {
	scalar       string
	inline       bool
	line, column int
}

func decodeItem(node *yaml.Node, inline any) (item, error) {
	i := item{line: node.Line, column: node.Column}
	if node.Kind != yaml.MappingNode {
		i.scalar = node.Value
		return i, nil
	}
	i.inline = true
	return i, node.Load(inline, yaml.WithKnownFields())
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
	i, err := decodeItem(node, &fields)
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
	i, err := decodeItem(node, &fields)
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
	i, err := decodeItem(node, &fields)
	fields.item = i
	*s = MCPItem(fields)
	return err
}

// AgentItem is an agents selection entry: an ID, a Markdown file path, or an inline definition.
type AgentItem struct {
	ID    ID `yaml:"id"      jsonschema_description:"Subagent ID; defaults to the file stem of path."`
	Agent `     yaml:",inline"`
	item  item
}

type agentItemFields AgentItem

func (s *AgentItem) UnmarshalYAML(node *yaml.Node) error {
	var fields agentItemFields
	i, err := decodeItem(node, &fields)
	fields.item = i
	*s = AgentItem(fields)
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
	for _, entry := range spec.Agents {
		id, err := m.agentItem(entry)
		if err != nil {
			return out, err
		}
		out.Agents = append(out.Agents, id)
	}
	for _, id := range spec.Bundles {
		out.Bundles = append(out.Bundles, qualify(m.ns, id))
	}
	return out, nil
}

func (m *merger) itemAt(i item) Source {
	at := Source{Path: m.path, Line: i.line, Column: i.column, Local: m.at.at().Local}
	if at.Local {
		at.Line, at.Column = 0, 0
	}
	return at
}

func (m *merger) skillItem(entry SkillItem) (ID, error) {
	if !entry.item.inline && !IsPath(entry.item.scalar) {
		return qualify(m.ns, ID(entry.item.scalar)), nil
	}
	at, id, s := m.itemAt(entry.item), entry.ID, entry.Skill
	if !entry.item.inline {
		s = Skill{Path: entry.item.scalar}
	}
	if id == "" {
		var err error
		if id, err = deriveID(at, DerivedID(KindSkills, s.Path, s.Git)); err != nil {
			return "", err
		}
	}
	return qualify(m.ns, id), m.addSkill(id, s, at, true)
}

func (m *merger) instructionsItem(entry InstructionsItem) (ID, error) {
	if !entry.item.inline && !IsPath(entry.item.scalar) {
		return qualify(m.ns, ID(entry.item.scalar)), nil
	}
	at, id, i := m.itemAt(entry.item), entry.ID, entry.Instructions
	if !entry.item.inline {
		i = Instructions{Path: entry.item.scalar}
	}
	if id == "" {
		var err error
		if id, err = deriveID(at, DerivedID(KindInstructions, i.Path, "")); err != nil {
			return "", err
		}
	}
	return qualify(m.ns, id), m.addInstructions(id, i, at, true)
}

func (m *merger) agentItem(entry AgentItem) (ID, error) {
	if !entry.item.inline && !IsPath(entry.item.scalar) {
		return qualify(m.ns, ID(entry.item.scalar)), nil
	}
	at, id, a := m.itemAt(entry.item), entry.ID, entry.Agent
	if !entry.item.inline {
		a = Agent{Path: entry.item.scalar}
	}
	if id == "" {
		var err error
		if id, err = deriveID(at, DerivedID(KindAgents, a.Path, "")); err != nil {
			return "", err
		}
	}
	return qualify(m.ns, id), m.addAgent(id, a, at, true)
}

func (m *merger) mcpItem(entry MCPItem) (ID, error) {
	if !entry.item.inline {
		return qualify(m.ns, ID(entry.item.scalar)), nil
	}
	return qualify(m.ns, entry.ID), m.addMCP(entry.ID, entry.MCPServer, m.itemAt(entry.item), true)
}

func deriveID(at Source, name string) (ID, error) {
	if !validIDSegment.MatchString(name) {
		return "", errorAt(at, "cannot derive an ID from %q; use a mapping with an id", name)
	}
	return ID(name), nil
}

// DerivedID is the ID a path or inline selection entry of kind defines when it names none: the
// folder name of a skill, or its repository's name when it is a whole repository, and the file
// stem of instructions or a subagent.
func DerivedID(kind Kind, file, git string) string {
	if kind == KindSkills && git != "" && file == "" {
		return RepositoryName(git)
	}
	base := path.Base(filepath.ToSlash(filepath.Clean(file)))
	if kind == KindInstructions || kind == KindAgents {
		return strings.TrimSuffix(base, path.Ext(base))
	}
	return base
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
