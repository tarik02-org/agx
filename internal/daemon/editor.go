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
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"connectrpc.com/connect"

	"go.yaml.in/yaml/v4"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/config/edit"
	"github.com/tarik02-org/agx/internal/configedit"
	"github.com/tarik02-org/agx/internal/packages"
	"github.com/tarik02-org/agx/internal/securefs"
)

const (
	// maxFileContent is the largest file ReadFile and ReadPreviewFile return.
	maxFileContent = 1 << 20
	// maxSkillFiles bounds a skill's file list.
	maxSkillFiles = 2000
	// previewLifetime is how long a previewed source stays readable.
	previewLifetime = 30 * time.Minute
)

func (s *Server) ListDefinitions(
	context.Context,
	*connect.Request[v1.ListDefinitionsRequest],
) (*connect.Response[v1.ListDefinitionsResponse], error) {
	c, err := s.reload()
	if err != nil {
		return nil, configError(err)
	}
	d := newDescriber(c, s.icons)
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
	icons *mcpIcons
	files map[string]*edit.File
	modes map[string]*v1.ConfigurationFile
}

func newDescriber(c *config.Config, icons *mcpIcons) *describer {
	return &describer{
		c:     c,
		icons: icons,
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
		out.Color, out.Icon = skillInterface(skill.Path)
	case edit.Instructions:
		out.Path = d.c.Instructions[config.ID(id)].Path
	case edit.MCP:
		out.Description = d.c.MCP[config.ID(id)].Description
		out.Icon = d.icons.get(id)
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
	out := &v1.GetSkillResponse{Definition: newDescriber(c, s.icons).definition(edit.Skills, req.Msg.SkillId)}
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

func (s *Server) PlanChanges(
	ctx context.Context,
	req *connect.Request[v1.PlanChangesRequest],
) (*connect.Response[v1.PlanChangesResponse], error) {
	s.editMu.Lock()
	defer s.editMu.Unlock()
	plan, err := s.plan(ctx, req.Msg.Changes)
	if err != nil {
		return nil, err
	}
	out := &v1.PlanChangesResponse{Files: []*v1.PlannedFile{}}
	for _, f := range plan.Files {
		out.Files = append(out.Files, &v1.PlannedFile{Path: f.Path, Diff: f.Diff(), BeforeSha256: f.BeforeSHA256()})
	}
	return connect.NewResponse(out), nil
}

func (s *Server) ApplyChanges(
	ctx context.Context,
	req *connect.Request[v1.ApplyChangesRequest],
) (*connect.Response[v1.ApplyChangesResponse], error) {
	expected := map[string]string{}
	for _, f := range req.Msg.Expected {
		if _, ok := expected[f.Path]; ok {
			return nil, rpcError(connect.CodeInvalidArgument, "expected names "+f.Path+" twice")
		}
		expected[f.Path] = f.BeforeSha256
	}
	s.editMu.Lock()
	defer s.editMu.Unlock()
	plan, err := s.plan(ctx, req.Msg.Changes)
	if err != nil {
		return nil, err
	}
	if err := plan.Check(expected); err != nil {
		return nil, editError(err)
	}
	written, err := plan.Write()
	// The configuration's inputs changed, so this loads what was written.
	_, _ = s.reload()
	if err != nil {
		var edited *configedit.Error
		if errors.As(err, &edited) {
			return nil, editError(err)
		}
		return nil, rpcError(
			connect.CodeInternal,
			fmt.Sprintf("writing the change failed after %d of %d files: %v", len(written), len(plan.Files), err),
		)
	}
	return connect.NewResponse(&v1.ApplyChangesResponse{Written: written}), nil
}

// plan applies changes in order to a session over the current configuration and plans it.
func (s *Server) plan(ctx context.Context, changes []*v1.ConfigChange) (*configedit.Plan, error) {
	c, err := s.reload()
	if err != nil {
		return nil, configError(err)
	}
	session, err := configedit.New(s.paths, s.builtins, c)
	if err != nil {
		return nil, rpcError(connect.CodeInternal, err.Error())
	}
	// Definitions the changes create take their rules beside them, as the CLI's add commands do.
	defined := map[string]string{}
	for i, change := range changes {
		if err := s.change(ctx, session, change, defined); err != nil {
			return nil, editError(fmt.Errorf("change %d: %w", i+1, err))
		}
	}
	plan, err := session.Plan()
	if err != nil {
		return nil, editError(err)
	}
	return plan, nil
}

func (s *Server) change(
	ctx context.Context,
	session *configedit.Session,
	change *v1.ConfigChange,
	defined map[string]string,
) error {
	switch change := change.Change.(type) {
	case *v1.ConfigChange_Define:
		d := change.Define
		value, err := definitionValue(d.Yaml)
		if err != nil {
			return err
		}
		file := session.Root
		if d.File != "" {
			if file, err = absoluteFile(d.File); err != nil {
				return err
			}
		}
		if at, exists := configedit.Lookup(session.Config, d.Kind, d.Id); exists {
			file = at.Source.Path
		}
		if err := session.Define(d.Kind, d.Id, value, file); err != nil {
			return err
		}
		defined[d.Kind+"\x00"+d.Id] = file
		return nil
	case *v1.ConfigChange_Undefine:
		return session.Undefine(change.Undefine.Kind, change.Undefine.Id)
	case *v1.ConfigChange_Enable:
		return selectionChange(session, change.Enable, true, defined)
	case *v1.ConfigChange_Disable:
		return selectionChange(session, change.Disable, false, defined)
	case *v1.ConfigChange_WriteFile:
		return session.WriteFile(change.WriteFile.Path, []byte(change.WriteFile.Content))
	case *v1.ConfigChange_AddPackage:
		p := change.AddPackage
		if p.Git == "" {
			return invalidf("add_package needs a git source")
		}
		source := configedit.Source{Git: p.Git, Ref: p.Ref, Subdir: p.Subdir, Name: config.RepositoryName(p.Git)}
		id, err := configedit.PackageID(session.Config, source, p.Id)
		if err != nil {
			return err
		}
		if _, err := session.DeclarePackage(id, source, session.Root); err != nil {
			return err
		}
		return session.Install(ctx, session.Root)
	}
	return invalidf("empty change")
}

func invalidf(format string, args ...any) error {
	return &configedit.Error{Code: configedit.InvalidRequest, Err: fmt.Errorf(format, args...)}
}

// selectionChange enables or disables a resource as agx use and unuse do.
func selectionChange(session *configedit.Session, c *v1.SelectionChange, enable bool, defined map[string]string) error {
	if !slices.Contains(configedit.SelectionKinds, c.Kind) {
		return invalidf("kind must be one of %s", strings.Join(configedit.SelectionKinds, ", "))
	}
	if c.Id == "" {
		return invalidf("a selection change needs an ID")
	}
	directory := ""
	if c.Directory != "" {
		expanded, err := config.ExpandHome(c.Directory)
		if err != nil || !filepath.IsAbs(expanded) {
			return invalidf("directory must be absolute or start with ~/")
		}
		if directory, err = config.Canonical(expanded); err != nil {
			return err
		}
		// Only a rule that enables needs its directory, so a mistyped one writes no dead rule.
		if enable {
			info, err := os.Stat(directory)
			switch {
			case errors.Is(err, fs.ErrNotExist):
				return &configedit.Error{
					Code: configedit.NotFound,
					Err:  fmt.Errorf("scope directory %s does not exist", directory),
				}
			case err != nil:
				return err
			case !info.IsDir():
				return invalidf("scope is not a directory: %s", directory)
			}
		}
	}
	target := configedit.NewScope(directory, c.Harnesses)
	fallback, ok := defined[c.Kind+"\x00"+c.Id]
	if !ok {
		fallback = session.RuleFile(c.Kind, c.Id)
	}
	var err error
	if enable {
		_, err = session.Enable(c.Kind, c.Id, target, fallback)
	} else {
		_, err = session.Disable(c.Kind, c.Id, target, fallback)
	}
	return err
}

// absoluteFile resolves a fragment path a change names.
func absoluteFile(path string) (string, error) {
	expanded, err := config.ExpandHome(path)
	if err != nil || !filepath.IsAbs(expanded) {
		return "", invalidf("file must be absolute or start with ~/: %s", path)
	}
	return config.Canonical(expanded)
}

// allowedTags are the tags a definition may use besides YAML's own.
var allowedTags = []string{"!env", "!file", "!secret"}

// definitionValue parses a definition's YAML: a mapping or a scalar, tagged only with agx's tags.
func definitionValue(text string) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		return nil, invalidf("definition YAML: %v", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 {
		return nil, invalidf("definition YAML is empty")
	}
	value := doc.Content[0]
	if value.Kind != yaml.MappingNode && value.Kind != yaml.ScalarNode {
		return nil, invalidf("a definition is a mapping or a scalar")
	}
	var check func(*yaml.Node) error
	check = func(n *yaml.Node) error {
		if n.Kind == yaml.AliasNode || n.Anchor != "" {
			return invalidf("definition YAML may not use anchors or aliases")
		}
		if strings.HasPrefix(n.Tag, "!") && !strings.HasPrefix(n.Tag, "!!") && !slices.Contains(allowedTags, n.Tag) {
			return invalidf("definition YAML uses tag %s; only %s are allowed", n.Tag, strings.Join(allowedTags, ", "))
		}
		for _, child := range n.Content {
			if err := check(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := check(value); err != nil {
		return nil, err
	}
	return value, nil
}

// editError maps an edit error to a Connect error whose reason the UI branches on.
func editError(err error) error {
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		return connectErr
	}
	if code, ok := configedit.CodeOf(err); ok {
		switch code {
		case configedit.ConfigInvalid:
			return reasonError(connect.CodeFailedPrecondition, v1.ErrorDetail_REASON_CONFIG_INVALID, err.Error())
		case configedit.ReadOnly:
			return rpcError(connect.CodeFailedPrecondition, err.Error())
		case configedit.Conflict:
			return rpcError(connect.CodeAborted, err.Error())
		case configedit.NotFound:
			return rpcError(connect.CodeNotFound, err.Error())
		case configedit.InvalidRequest:
			return rpcError(connect.CodeInvalidArgument, err.Error())
		}
	}
	switch {
	case errors.Is(err, packages.ErrBusy), errors.Is(err, packages.ErrStaleLock):
		return rpcError(connect.CodeAborted, err.Error())
	case errors.Is(err, packages.ErrUnknown):
		return rpcError(connect.CodeNotFound, err.Error())
	case errors.Is(err, packages.ErrFetch):
		return rpcError(connect.CodeFailedPrecondition, err.Error())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return rpcError(connect.CodeDeadlineExceeded, err.Error())
	}
	return rpcError(connect.CodeInternal, err.Error())
}

// listRefsTimeout bounds asking a Git server for its refs.
const listRefsTimeout = 20 * time.Second

// ListSourceRefs lists a Git source's branches and tags for picking a package's ref.
func (s *Server) ListSourceRefs(
	ctx context.Context,
	req *connect.Request[v1.ListSourceRefsRequest],
) (*connect.Response[v1.ListSourceRefsResponse], error) {
	source := strings.TrimSpace(req.Msg.Git)
	if source == "" {
		return nil, rpcError(connect.CodeInvalidArgument, "git is required")
	}
	ctx, cancel := context.WithTimeout(ctx, listRefsTimeout)
	defer cancel()
	refs, err := packages.RemoteRefs(ctx, source)
	if err != nil {
		return nil, editError(err)
	}
	out := &v1.ListSourceRefsResponse{Refs: make([]*v1.SourceRef, 0, len(refs))}
	for _, ref := range refs {
		out.Refs = append(out.Refs, &v1.SourceRef{
			Name:    ref.Name,
			Tag:     ref.Tag,
			Commit:  ref.Commit,
			Default: ref.Default,
		})
	}
	return connect.NewResponse(out), nil
}

// inspectTimeout bounds checking a Git source out.
const inspectTimeout = 2 * time.Minute

// InspectSource checks a Git source out at its pins, for filling in a package's revision, hash,
// and subdirectory.
func (s *Server) InspectSource(
	ctx context.Context,
	req *connect.Request[v1.InspectSourceRequest],
) (*connect.Response[v1.InspectSourceResponse], error) {
	source := strings.TrimSpace(req.Msg.Git)
	if source == "" {
		return nil, rpcError(connect.CodeInvalidArgument, "git is required")
	}
	ctx, cancel := context.WithTimeout(ctx, inspectTimeout)
	defer cancel()
	inspection, err := packages.Inspect(
		ctx, s.paths.Cache, source, strings.TrimSpace(req.Msg.Ref),
		strings.Trim(strings.TrimSpace(req.Msg.Subdir), "/"),
	)
	if err != nil {
		return nil, editError(err)
	}
	return connect.NewResponse(&v1.InspectSourceResponse{
		Revision:    inspection.Revision,
		Hash:        inspection.Hash,
		Directories: inspection.Directories,
	}), nil
}

// preview is a fetched source and the directories of its skills.
type preview struct {
	source  *v1.PackageSource
	skills  map[string]string
	expires time.Time
}

// previews holds the sources PreviewSource fetched, until they expire.
type previews struct {
	mu    sync.Mutex
	items map[string]*preview
}

func (p *previews) add(item *preview) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prune(time.Now())
	if p.items == nil {
		p.items = map[string]*preview{}
	}
	id := securefs.Token()
	p.items[id] = item
	return id
}

func (p *previews) get(id string) *preview {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prune(time.Now())
	return p.items[id]
}

func (p *previews) prune(now time.Time) {
	maps.DeleteFunc(p.items, func(_ string, item *preview) bool { return now.After(item.expires) })
}

func (s *Server) PreviewSource(
	ctx context.Context,
	req *connect.Request[v1.PreviewSourceRequest],
) (*connect.Response[v1.PreviewSourceResponse], error) {
	source, err := configedit.ParseSource(strings.TrimSpace(req.Msg.Source), configedit.GitHub, false)
	if err != nil {
		return nil, editError(err)
	}
	c, err := s.reload()
	if err != nil {
		return nil, configError(err)
	}
	session, err := configedit.New(s.paths, s.builtins, c)
	if err != nil {
		return nil, rpcError(connect.CodeInternal, err.Error())
	}
	id, err := configedit.PackageID(c, source, "")
	if err != nil {
		return nil, editError(err)
	}
	// A derived ID another source took gets a suffix, so the preview can be added as it is.
	for base, n := id, 2; ; n++ {
		p, exists := c.Packages[id]
		if !exists || p.Git == source.Git && p.Ref == source.Ref && p.Subdir == source.Subdir {
			break
		}
		id = config.ID(fmt.Sprintf("%s-%d", base, n))
	}
	if _, err := session.DeclarePackage(id, source, session.Root); err != nil {
		return nil, editError(err)
	}
	if err := session.Install(ctx, session.Root); err != nil {
		return nil, editError(err)
	}
	loaded, err := session.Load(session.Root)
	if err != nil {
		return nil, reasonError(
			connect.CodeFailedPrecondition,
			v1.ErrorDetail_REASON_CONFIG_INVALID,
			fmt.Sprintf("package %s: %v", id, err),
		)
	}
	item := &preview{
		source:  &v1.PackageSource{Id: string(id), Git: source.Git, Ref: source.Ref, Subdir: source.Subdir},
		skills:  map[string]string{},
		expires: time.Now().Add(previewLifetime),
	}
	out := &v1.PreviewSourceResponse{Package: item.source, Skills: []*v1.PreviewSkill{}}
	prefix := string(id) + "/"
	for _, skillID := range slices.Sorted(maps.Keys(loaded.Skills)) {
		name, ok := strings.CutPrefix(string(skillID), prefix)
		skill := loaded.Skills[skillID]
		if !ok || strings.Contains(name, "/") || skill.Generated() {
			continue
		}
		found := &v1.PreviewSkill{Name: name}
		if document, err := os.ReadFile(filepath.Join(skill.Path, "SKILL.md")); err == nil {
			_, found.Description = config.FrontMatter(document)
			found.SkillMd = string(document)
		}
		if found.Files, err = skillFiles(skill.Path); err != nil {
			return nil, rpcError(connect.CodeInternal, fmt.Sprintf("skill %s: %v", skillID, err))
		}
		item.skills[name] = skill.Path
		out.Skills = append(out.Skills, found)
	}
	out.PreviewId = s.previews.add(item)
	return connect.NewResponse(out), nil
}

func (s *Server) ReadPreviewFile(
	_ context.Context,
	req *connect.Request[v1.ReadPreviewFileRequest],
) (*connect.Response[v1.ReadPreviewFileResponse], error) {
	item := s.previews.get(req.Msg.PreviewId)
	if item == nil {
		return nil, rpcError(connect.CodeNotFound, "unknown or expired preview; preview the source again")
	}
	dir, ok := item.skills[req.Msg.Skill]
	if !ok {
		return nil, rpcError(connect.CodeNotFound, "the preview has no skill "+req.Msg.Skill)
	}
	rel := filepath.FromSlash(req.Msg.Path)
	if !filepath.IsLocal(rel) {
		return nil, rpcError(connect.CodeInvalidArgument, "path must be relative to the skill, without ..")
	}
	root, err := config.Canonical(dir)
	if err != nil {
		return nil, rpcError(connect.CodeInternal, err.Error())
	}
	path, err := config.Canonical(filepath.Join(root, rel))
	if err != nil || path == root || !config.Contains(root, path) {
		return nil, rpcError(connect.CodeInvalidArgument, req.Msg.Path+" is outside the skill")
	}
	content, binary, err := readText(path)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&v1.ReadPreviewFileResponse{Content: content, Binary: binary}), nil
}
