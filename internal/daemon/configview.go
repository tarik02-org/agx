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
	out := &v1.GetConfigurationResponse{Root: c.Root, Home: home}
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
	for _, path := range c.Files {
		out.Files = append(out.Files, configurationFile(path))
	}
	return connect.NewResponse(out), nil
}

// configurationFile says whether agx may edit a configuration file: not through a symlink,
// which belongs to whatever made it, and not without write permission.
func configurationFile(path string) *v1.ConfigurationFile {
	file := &v1.ConfigurationFile{Path: path}
	info, err := os.Lstat(path)
	if err != nil {
		file.ReadOnly = true
		return file
	}
	if info.Mode()&os.ModeSymlink != 0 {
		file.ReadOnly = true
		file.LinkTarget, _ = os.Readlink(path)
		return file
	}
	if f, err := os.OpenFile(path, os.O_WRONLY, 0); err != nil {
		file.ReadOnly = true
	} else {
		_ = f.Close()
	}
	return file
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
	out := &v1.CompleteDirectoryResponse{Directories: []string{}}
	for _, entry := range entries {
		entryName := entry.Name()
		// Hidden directories show once the prefix asks for them.
		if !entry.IsDir() || !strings.HasPrefix(entryName, name) ||
			strings.HasPrefix(entryName, ".") && !strings.HasPrefix(name, ".") {
			continue
		}
		out.Directories = append(out.Directories, filepath.Join(parent, entryName))
		if len(out.Directories) == maxCompletions {
			break
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
				Line:   uint32(at.Line),
				Column: uint32(at.Column),
			}, //nolint:gosec // lines fit
		})
	}
	return out
}
