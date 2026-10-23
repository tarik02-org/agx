package daemon

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"unicode/utf8"

	"connectrpc.com/connect"

	"github.com/tarik02-org/agx/internal/yaml"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/config/edit"
	"github.com/tarik02-org/agx/internal/configedit"
)

const (
	// maxFileContent is the largest file ReadFile returns.
	maxFileContent = 1 << 20
	// maxSkillFiles bounds a skill's file list.
	maxSkillFiles = 2000
)

func (s *Server) ListDefinitions(
	context.Context,
	*connect.Request[v1.ListDefinitionsRequest],
) (*connect.Response[v1.ListDefinitionsResponse], error) {
	c, err := s.reload()
	if err != nil {
		return nil, configError(err)
	}
	d := newDescriber(c)
	out := &v1.ListDefinitionsResponse{Definitions: []*v1.Definition{}}
	for _, kind := range configedit.Kinds {
		for _, id := range definitionIDs(c, kind) {
			out.Definitions = append(out.Definitions, d.definition(kind, id))
		}
	}
	return connect.NewResponse(out), nil
}

// definitionIDs lists the IDs of a kind's definitions, sorted.
func definitionIDs(c *config.Config, kind string) []string {
	var ids []string
	switch kind {
	case edit.Skills:
		ids = names(c.Skills)
	case edit.Instructions:
		ids = names(c.Instructions)
	case edit.MCP:
		ids = names(c.MCP)
	case configedit.Secrets:
		ids = names(c.Secrets)
	case edit.Bundles:
		ids = names(c.Bundles)
	case configedit.Packages:
		ids = names(c.Packages)
	case configedit.Harnesses:
		ids = slices.Sorted(maps.Keys(c.Harnesses))
	}
	return slices.DeleteFunc(ids, func(id string) bool {
		_, ok := configedit.Lookup(c, kind, id)
		return !ok
	})
}

func names[V any](m map[config.ID]V) []string {
	out := []string{}
	for id := range m {
		out = append(out, string(id))
	}
	slices.Sort(out)
	return out
}

// describer builds definitions, opening each fragment once.
type describer struct {
	c     *config.Config
	files map[string]*edit.File
	modes map[string]*v1.ConfigurationFile
}

func newDescriber(c *config.Config) *describer {
	return &describer{
		c:     c,
		files: map[string]*edit.File{},
		modes: map[string]*v1.ConfigurationFile{},
	}
}

func (d *describer) definition(kind, id string) *v1.Definition {
	at, _ := configedit.Lookup(d.c, kind, id)
	out := &v1.Definition{
		Kind: kind,
		Id:   id,
		Source: &v1.SourceLocation{
			Path:   at.Source.Path,
			Line:   uint32(at.Source.Line),   //nolint:gosec // lines fit
			Column: uint32(at.Source.Column), //nolint:gosec // columns fit
		},
		Package: string(configedit.Owner(d.c, at.Source, id)),
	}
	fragment := slices.Contains(d.c.Files, at.Source.Path)
	out.ReadOnly = !fragment || at.Inline || d.mode(at.Source.Path).ReadOnly
	if fragment && !at.Inline {
		out.Yaml = d.yaml(at.Source.Path, kind, id)
	}
	switch kind {
	case edit.Skills:
		skill := d.c.Skills[config.ID(id)]
		if skill.Generated() {
			out.Description = skill.Description
			break
		}
		out.Path = skill.Path
		if document, err := os.ReadFile(filepath.Join(skill.Path, "SKILL.md")); err == nil {
			_, out.Description = config.FrontMatter(document)
		}
	case edit.Instructions:
		out.Path = d.c.Instructions[config.ID(id)].Path
	case edit.MCP:
		out.Description = d.c.MCP[config.ID(id)].Description
	}
	return out
}

func (d *describer) mode(path string) *v1.ConfigurationFile {
	if m, ok := d.modes[path]; ok {
		return m
	}
	m := configurationFile(path)
	d.modes[path] = m
	return m
}

// yaml renders a definition's value as its fragment holds it; empty when it cannot be found.
func (d *describer) yaml(path, kind, id string) string {
	f, ok := d.files[path]
	if !ok {
		var err error
		if f, err = edit.Open(path, ""); err != nil {
			f = nil
		}
		d.files[path] = f
	}
	if f == nil {
		return ""
	}
	node := f.Definition(kind, id)
	if node == nil {
		return ""
	}
	b, err := yaml.Dump(node, yaml.WithV4Defaults(), yaml.WithIndent(2), yaml.WithLineWidth(-1))
	if err != nil {
		return ""
	}
	return string(b)
}

func (s *Server) GetSkill(
	_ context.Context,
	req *connect.Request[v1.GetSkillRequest],
) (*connect.Response[v1.GetSkillResponse], error) {
	c, err := s.reload()
	if err != nil {
		return nil, configError(err)
	}
	skill, ok := c.Skills[config.ID(req.Msg.SkillId)]
	if !ok {
		return nil, rpcError(connect.CodeNotFound, "unknown skill "+req.Msg.SkillId)
	}
	out := &v1.GetSkillResponse{Definition: newDescriber(c).definition(edit.Skills, req.Msg.SkillId)}
	if skill.Generated() {
		out.SkillMd = skill.Content
		return connect.NewResponse(out), nil
	}
	document, err := os.ReadFile(filepath.Join(skill.Path, "SKILL.md"))
	if err != nil {
		return nil, rpcError(connect.CodeFailedPrecondition, fmt.Sprintf("skill %s: %v", req.Msg.SkillId, err))
	}
	out.SkillMd = string(document)
	if out.Files, err = skillFiles(skill.Path); err != nil {
		return nil, rpcError(connect.CodeInternal, fmt.Sprintf("skill %s: %v", req.Msg.SkillId, err))
	}
	return connect.NewResponse(out), nil
}

// skillFiles lists the files below dir, relative and /-separated, sorted, without Git metadata.
func skillFiles(dir string) ([]string, error) {
	out := []string{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		if len(out) == maxSkillFiles {
			return fs.SkipAll
		}
		return nil
	})
	slices.Sort(out)
	return out, err
}

func (s *Server) ReadFile(
	_ context.Context,
	req *connect.Request[v1.ReadFileRequest],
) (*connect.Response[v1.ReadFileResponse], error) {
	c, err := s.reload()
	if err != nil {
		return nil, configError(err)
	}
	requested, err := config.ExpandHome(req.Msg.Path)
	if err != nil || !filepath.IsAbs(requested) {
		return nil, rpcError(connect.CodeInvalidArgument, "path must be absolute or start with ~/")
	}
	requested = filepath.Clean(requested)
	path, err := config.Canonical(requested)
	if err != nil {
		return nil, rpcError(connect.CodeInvalidArgument, err.Error())
	}
	readOnly := false
	if !slices.Contains(c.Files, path) {
		local, found := configedit.ContentFile(c, s.paths.Cache, path)
		if !found {
			return nil, rpcError(
				connect.CodeInvalidArgument,
				req.Msg.Path+" is not a configuration fragment or a file of a configured skill or instructions definition",
			)
		}
		readOnly = !local
	}
	content, binary, err := readText(path)
	if err != nil {
		return nil, err
	}
	out := &v1.ReadFileResponse{
		Path:     path,
		Content:  content,
		Binary:   binary,
		ReadOnly: readOnly || configurationFile(requested).ReadOnly || configurationFile(path).ReadOnly,
	}
	return connect.NewResponse(out), nil
}

// readText reads a regular file as text, or reports it binary when it is too large or not UTF-8.
func readText(path string) (string, bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, rpcError(connect.CodeNotFound, path+" does not exist")
	} else if err != nil {
		return "", false, rpcError(connect.CodeFailedPrecondition, err.Error())
	}
	if !info.Mode().IsRegular() {
		return "", false, rpcError(connect.CodeInvalidArgument, path+" is not a regular file")
	}
	if info.Size() > maxFileContent {
		return "", true, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false, rpcError(connect.CodeFailedPrecondition, err.Error())
	}
	if len(b) > maxFileContent || !utf8.Valid(b) {
		return "", true, nil
	}
	return string(b), false, nil
}
