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

	"google.golang.org/protobuf/types/known/structpb"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/configedit"
	"github.com/tarik02-org/agx/internal/packages"
	"github.com/tarik02-org/agx/internal/platform"
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
	for _, kind := range config.Kinds {
		for _, id := range definitionIDs(c, kind) {
			out.Definitions = append(out.Definitions, d.definition(kind, id))
		}
	}
	return connect.NewResponse(out), nil
}

// definitionIDs lists the IDs of a kind's definitions, sorted.
func definitionIDs(c *config.Config, kind config.Kind) []string {
	var ids []string
	switch kind {
	case config.KindSkills:
		ids = names(c.Skills)
	case config.KindInstructions:
		ids = names(c.Instructions)
	case config.KindMCPServers:
		ids = names(c.MCP)
	case config.KindSecrets:
		ids = names(c.Secrets)
	case config.KindBundles:
		ids = names(c.Bundles)
	case config.KindPackages:
		ids = names(c.Packages)
	case config.KindHarnesses:
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
}

func newDescriber(c *config.Config, icons *mcpIcons) *describer {
	return &describer{
		c:     c,
		icons: icons,
	}
}

func (d *describer) definition(kind config.Kind, id string) *v1.Definition {
	at, _ := configedit.Lookup(d.c, kind, id)
	out := &v1.Definition{
		Kind: string(kind),
		Id:   id,
		Source: &v1.SourceLocation{
			Path:   at.Source.Path,
			Local:  at.Source.Local,
			Line:   uint32(at.Source.Line),   //nolint:gosec // lines fit
			Column: uint32(at.Source.Column), //nolint:gosec // columns fit
		},
		Package: string(configedit.Owner(d.c, at.Source, id)),
	}
	out.ReadOnly = !at.Source.Local || at.Inline
	if at.Source.Local && !at.Inline {
		if definitions, ok := d.c.Local.Document[string(kind)].(map[string]any); ok {
			out.Value, _ = structpb.NewValue(definitions[id])
		}
	}
	switch kind {
	case config.KindSkills:
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
	case config.KindInstructions:
		out.Path = d.c.Instructions[config.ID(id)].Path
	case config.KindMCPServers:
		out.Description = d.c.MCP[config.ID(id)].Description
		out.Icon = d.icons.get(id)
	default:
	}
	return out
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
	out := &v1.GetSkillResponse{Definition: newDescriber(c, s.icons).definition(config.KindSkills, req.Msg.SkillId)}
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
	path, err := platform.Canonical(requested)
	if err != nil {
		return nil, rpcError(connect.CodeInvalidArgument, err.Error())
	}
	local, found := configedit.ContentFile(c, s.paths, path)
	if !found {
		return nil, rpcError(connect.CodeInvalidArgument, "path is not a configured resource file")
	}
	content, binary, err := readText(path)
	if err != nil {
		return nil, err
	}
	out := &v1.ReadFileResponse{
		Path:     path,
		Content:  content,
		Binary:   binary,
		ReadOnly: !local,
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

func (s *Server) UpdateConfiguration(
	ctx context.Context,
	req *connect.Request[v1.UpdateConfigurationRequest],
) (*connect.Response[v1.UpdateConfigurationResponse], error) {
	s.editMu.Lock()
	defer s.editMu.Unlock()
	c, err := s.reload()
	if err != nil {
		return nil, configError(err)
	}
	if c.Local.Revision != req.Msg.ExpectedRevision {
		return nil, rpcError(connect.CodeAborted, config.ErrConflict.Error())
	}
	session, err := configedit.New(s.paths, s.builtins, c)
	if err != nil {
		return nil, editError(err)
	}
	for i, change := range req.Msg.Changes {
		if err := s.change(ctx, session, change); err != nil {
			return nil, editError(fmt.Errorf("change %d: %w", i+1, err))
		}
	}
	if _, err := session.Commit(ctx); err != nil {
		return nil, editError(err)
	}
	_, _ = s.reload()
	return connect.NewResponse(&v1.UpdateConfigurationResponse{Revision: session.Local.Revision}), nil
}

func (s *Server) WriteResourceFile(
	_ context.Context,
	req *connect.Request[v1.WriteResourceFileRequest],
) (*connect.Response[v1.WriteResourceFileResponse], error) {
	s.editMu.Lock()
	defer s.editMu.Unlock()
	c, err := s.reload()
	if err != nil {
		return nil, configError(err)
	}
	if err := configedit.WriteFile(c, s.paths, req.Msg.Path, []byte(req.Msg.Content)); err != nil {
		return nil, editError(err)
	}
	return connect.NewResponse(&v1.WriteResourceFileResponse{}), nil
}

func (s *Server) change(
	ctx context.Context,
	session *configedit.Session,
	change *v1.ConfigChange,
) error {
	switch change := change.Change.(type) {
	case *v1.ConfigChange_Define:
		d := change.Define
		if d.Value == nil {
			return invalidf("definition needs a value")
		}
		return session.Define(config.Kind(d.Kind), d.Id, d.Value.AsInterface())
	case *v1.ConfigChange_Undefine:
		return session.Undefine(config.Kind(change.Undefine.Kind), change.Undefine.Id)
	case *v1.ConfigChange_Enable:
		return selectionChange(session, change.Enable, true)
	case *v1.ConfigChange_Disable:
		return selectionChange(session, change.Disable, false)
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
		if _, err := session.DeclarePackage(id, source); err != nil {
			return err
		}
		return session.Install(ctx)
	}
	return invalidf("empty change")
}

func invalidf(format string, args ...any) error {
	return &configedit.Error{Code: configedit.InvalidRequest, Err: fmt.Errorf(format, args...)}
}

// selectionChange enables or disables a resource as agx use and unuse do.
func selectionChange(session *configedit.Session, c *v1.SelectionChange, enable bool) error {
	kind := config.Kind(c.Kind)
	if !slices.Contains(config.SelectionKinds, kind) {
		return invalidf("kind must be one of %s", config.JoinKinds(config.SelectionKinds))
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
		if directory, err = platform.Canonical(expanded); err != nil {
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
	var err error
	if enable {
		_, err = session.Enable(kind, c.Id, target)
	} else {
		_, err = session.Disable(kind, c.Id, target)
	}
	return err
}

// editError maps edit failures to Connect codes and configuration diagnostics.
func editError(err error) error {
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		return connectErr
	}
	var edited *configedit.Error
	if errors.As(err, &edited) {
		switch edited.Code {
		case configedit.ConfigInvalid:
			return configInvalid(err.Error(), err)
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
	case errors.Is(err, packages.ErrStaleLock):
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
	if _, err := session.DeclarePackage(id, source); err != nil {
		return nil, editError(err)
	}
	if err := session.Install(ctx); err != nil {
		return nil, editError(err)
	}
	loaded, err := session.Load()
	if err != nil {
		return nil, configInvalid(fmt.Sprintf("package %s: %v", id, err), err)
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
	root, err := platform.Canonical(dir)
	if err != nil {
		return nil, rpcError(connect.CodeInternal, err.Error())
	}
	path, err := platform.Canonical(filepath.Join(root, rel))
	if err != nil || path == root || !config.Contains(root, path) {
		return nil, rpcError(connect.CodeInvalidArgument, req.Msg.Path+" is outside the skill")
	}
	content, binary, err := readText(path)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&v1.ReadPreviewFileResponse{Content: content, Binary: binary}), nil
}
