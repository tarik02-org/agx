package daemon

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"connectrpc.com/connect"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/config"
)

// maxCompletions bounds CompleteDirectory's answer.
const maxCompletions = 50

func (s *Server) GetConfiguration(
	context.Context,
	*connect.Request[v1.GetConfigurationRequest],
) (*connect.Response[v1.GetConfigurationResponse], error) {
	c, err := s.reload()
	if err != nil {
		return nil, configError(err)
	}
	home, _ := os.UserHomeDir()
	// Configuration paths are canonical, so the home they start with is too.
	if canonical, err := filepath.EvalSymlinks(home); err == nil {
		home = canonical
	}
	out := &v1.GetConfigurationResponse{Root: c.Root, Home: home, Revision: c.Local.Revision, Overlays: c.Files}
	for _, name := range c.HarnessNames() {
		h, err := c.Harness(name)
		if err != nil {
			continue
		}
		out.Harnesses = append(out.Harnesses, &v1.ConfiguredHarness{
			Name:    name,
			Builtin: h.Builtin,
			Base:    c.Harnesses[name].Base,
			Hidden:  h.Hidden,
		})
	}
	for _, scope := range c.Scopes {
		if scope.Directory != "" && !slices.Contains(out.ScopeDirectories, scope.Directory) {
			out.ScopeDirectories = append(out.ScopeDirectories, scope.Directory)
		}
	}
	slices.Sort(out.ScopeDirectories)
	return connect.NewResponse(out), nil
}

func (s *Server) CompleteDirectory(
	_ context.Context,
	req *connect.Request[v1.CompleteDirectoryRequest],
) (*connect.Response[v1.CompleteDirectoryResponse], error) {
	prefix, err := config.ExpandHome(req.Msg.Prefix)
	if err != nil || !filepath.IsAbs(prefix) {
		return nil, rpcError(connect.CodeInvalidArgument, "prefix must be absolute or start with ~/")
	}
	parent, name := prefix, ""
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		parent, name = filepath.Dir(prefix), filepath.Base(prefix)
	}
	// A parent that does not exist or cannot be read has no completions.
	entries, _ := os.ReadDir(parent)
	out := &v1.CompleteDirectoryResponse{Directories: []string{}, Files: []string{}}
	for _, entry := range entries {
		entryName := entry.Name()
		// Hidden entries show once the prefix asks for them.
		if !strings.HasPrefix(entryName, name) ||
			strings.HasPrefix(entryName, ".") && !strings.HasPrefix(name, ".") {
			continue
		}
		switch {
		case entry.IsDir() && len(out.Directories) < maxCompletions:
			out.Directories = append(out.Directories, filepath.Join(parent, entryName))
		case !entry.IsDir() && req.Msg.Files && len(out.Files) < maxCompletions:
			out.Files = append(out.Files, filepath.Join(parent, entryName))
		}
	}
	return connect.NewResponse(out), nil
}

// harnessSettings describes the resolved native settings of a snapshot's harness.
func harnessSettings(snapshot *config.Snapshot) []*v1.HarnessSetting {
	if snapshot.Harness == "" {
		return nil
	}
	h, err := snapshot.Config.Harness(snapshot.Harness)
	if err != nil {
		return nil
	}
	sources := map[string]config.Source{}
	for _, s := range h.SettingSources {
		sources[config.FormatSettingPath(s.Path)] = s.Source
	}
	out := []*v1.HarnessSetting{}
	for _, leaf := range config.SettingLeaves(h.Settings) {
		value, err := json.Marshal(leaf.Value)
		if err != nil {
			continue
		}
		at := sources[config.FormatSettingPath(leaf.Path)]
		out = append(out, &v1.HarnessSetting{
			Path:      leaf.Path,
			ValueJson: string(value),
			Source: &v1.SourceLocation{
				Path:   at.Path,
				Local:  at.Local,
				Line:   uint32(at.Line),
				Column: uint32(at.Column),
			}, //nolint:gosec // lines fit
		})
	}
	return out
}
