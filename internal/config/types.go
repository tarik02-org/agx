package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
)

type ID string

type Source struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type Import struct {
	Path     string `yaml:"path"`
	Glob     string `yaml:"glob"`
	Optional bool   `yaml:"optional"`
}

type Package struct {
	Path     string `yaml:"path"     json:"path,omitempty"`
	Git      string `yaml:"git"      json:"git,omitempty"`
	Ref      string `yaml:"ref"      json:"ref,omitempty"`
	Subdir   string `yaml:"subdir"   json:"subdir,omitempty"`
	Override bool   `yaml:"override" json:"-"`
	Lockfile string `yaml:"-"        json:"-"`
	Source   Source `yaml:"-"        json:"-"`
	// Manifest marks definitions from package manifests rather than configuration fragments.
	Manifest bool `yaml:"-" json:"-"`
	// LockPath is the local path or Git source as recorded in the lockfile: relative to the
	// lockfile directory for configuration fragments, or to the package root for manifests.
	LockPath string `yaml:"-" json:"-"`
}

// LockSource returns the machine-independent source recorded in and compared against lockfiles.
func (p Package) LockSource() Package {
	out := Package{Path: p.Path, Git: p.Git, Ref: p.Ref, Subdir: p.Subdir}
	if p.LockPath != "" {
		if p.Path != "" {
			out.Path = p.LockPath
		} else {
			out.Git = p.LockPath
		}
	}
	return out
}

type FileResource struct {
	Path     string `yaml:"path"     json:"path"`
	Override bool   `yaml:"override" json:"-"`
	Source   Source `yaml:"-"        json:"source"`
}

type Connection struct {
	Transport       string            `yaml:"transport"        json:"transport"`
	Command         []string          `yaml:"command"          json:"command,omitempty"`
	URL             string            `yaml:"url"              json:"url,omitempty"`
	Cwd             string            `yaml:"cwd"              json:"cwd,omitempty"`
	Environment     map[string]string `yaml:"environment"      json:"environment,omitempty"`
	AuthEnvironment map[string]ID     `yaml:"auth_environment" json:"auth_environment,omitempty"`
	Headers         map[string]string `yaml:"headers"          json:"headers,omitempty"`
	Auth            ID                `yaml:"auth"             json:"auth,omitempty"`
	Override        bool              `yaml:"override"         json:"-"`
	Source          Source            `yaml:"-"                json:"source"`
}

type Auth struct {
	Kind     string   `yaml:"kind"      json:"kind"`
	Variable string   `yaml:"variable"  json:"variable,omitempty"`
	Command  []string `yaml:"command"   json:"command,omitempty"`
	Issuer   string   `yaml:"issuer"    json:"issuer,omitempty"`
	ClientID string   `yaml:"client_id" json:"client_id,omitempty"`
	Resource string   `yaml:"resource"  json:"resource,omitempty"`
	Scopes   []string `yaml:"scopes"    json:"scopes,omitempty"`
	Override bool     `yaml:"override"  json:"-"`
	Source   Source   `yaml:"-"         json:"source"`
}

type Selection struct {
	Skills       []ID `yaml:"skills"       json:"skills,omitempty"`
	Instructions []ID `yaml:"instructions" json:"instructions,omitempty"`
	Connections  []ID `yaml:"connections"  json:"connections,omitempty"`
	Bundles      []ID `yaml:"bundles"      json:"bundles,omitempty"`
}

type Bundle struct {
	Selection `       yaml:",inline"`
	Override  bool   `yaml:"override" json:"-"`
	Source    Source `yaml:"-"        json:"source"`
}

type Scope struct {
	Directory string    `yaml:"directory" json:"directory"`
	Add       Selection `yaml:"add"       json:"add"`
	Exclude   Selection `yaml:"exclude"   json:"exclude"`
	Source    Source    `yaml:"-"         json:"source"`
}

type Harness struct {
	Executable string `yaml:"executable" json:"executable"`
	Override   bool   `yaml:"override"   json:"-"`
}

type Daemon struct {
	LeaseSeconds int  `yaml:"lease_seconds" json:"lease_seconds"`
	RenewSeconds int  `yaml:"renew_seconds" json:"renew_seconds"`
	Override     bool `yaml:"override"      json:"-"`
}

type Document struct {
	Version      int                 `yaml:"version"`
	Imports      []Import            `yaml:"imports"`
	Lockfile     string              `yaml:"lockfile"`
	Packages     map[ID]Package      `yaml:"packages"`
	Skills       map[ID]FileResource `yaml:"skills"`
	Instructions map[ID]FileResource `yaml:"instructions"`
	Connections  map[ID]Connection   `yaml:"connections"`
	Auth         map[ID]Auth         `yaml:"auth"`
	Bundles      map[ID]Bundle       `yaml:"bundles"`
	Scopes       []Scope             `yaml:"scopes"`
	Harnesses    map[string]Harness  `yaml:"harnesses"`
	Daemon       Daemon              `yaml:"daemon"`
}

type Config struct {
	Root          string              `json:"root"`
	Packages      map[ID]Package      `json:"packages"`
	Skills        map[ID]FileResource `json:"skills"`
	Instructions  map[ID]FileResource `json:"instructions"`
	Connections   map[ID]Connection   `json:"connections"`
	Auth          map[ID]Auth         `json:"auth"`
	Bundles       map[ID]Bundle       `json:"bundles"`
	Scopes        []Scope             `json:"scopes"`
	Harnesses     map[string]Harness  `json:"harnesses"`
	Daemon        Daemon              `json:"daemon"`
	Files         []string            `json:"files"`
	PackageHashes map[ID]string       `json:"package_hashes"`

	// rootDefinitions maps definitions from configuration fragments to their override flag,
	// so package manifests merged later cannot replace them.
	rootDefinitions map[string]bool
	daemonSet       map[string]bool
}

type Resource struct {
	Kind string `json:"kind"`
	ID   ID     `json:"id"`
}

type Step struct {
	Change      string `json:"change"`
	Source      Source `json:"source"`
	Directory   string `json:"scope_directory"`
	BundleChain []ID   `json:"bundle_chain,omitempty"`
}

type Trace struct {
	Resource Resource `json:"resource"`
	Selected bool     `json:"selected"`
	Steps    []Step   `json:"steps"`
}

type Snapshot struct {
	ID                  string     `json:"id"`
	Directory           string     `json:"directory"`
	Resources           []Resource `json:"resources"`
	RequiredEnvironment []string   `json:"required_environment"`
	Traces              []Trace    `json:"traces,omitempty"`
	Config              *Config    `json:"-"`
}

func Canonical(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	// Resolve the existing prefix as scopes may name directories not created yet.
	suffix := []string{}
	for {
		real, err := filepath.EvalSymlinks(abs)
		if err == nil {
			real = actualCase(real)
			for i := len(suffix) - 1; i >= 0; i-- {
				real = filepath.Join(real, suffix[i])
			}
			return filepath.Clean(real), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", err
		}
		suffix = append(suffix, filepath.Base(abs))
		abs = parent
	}
}

func Digest(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func (c *Config) Resolve(directory string) (*Snapshot, error) {
	dir, err := Canonical(directory)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", dir)
	}
	scopes := append([]Scope(nil), c.Scopes...)
	sort.SliceStable(
		scopes,
		func(i, j int) bool { return pathDepth(scopes[i].Directory) < pathDepth(scopes[j].Directory) },
	)
	traces := map[Resource]*Trace{}
	for _, scope := range scopes {
		if !Contains(scope.Directory, dir) {
			continue
		}
		for _, change := range []struct {
			name      string
			selection Selection
		}{{"add", scope.Add}, {"exclude", scope.Exclude}} {
			err := c.expand(change.selection, nil, func(r Resource, chain []ID) {
				t := traces[r]
				if t == nil {
					t = &Trace{Resource: r}
					traces[r] = t
				}
				t.Selected = change.name == "add"
				t.Steps = append(
					t.Steps,
					Step{
						Change:      change.name,
						Source:      scope.Source,
						Directory:   scope.Directory,
						BundleChain: append([]ID(nil), chain...),
					},
				)
			})
			if err != nil {
				return nil, fmt.Errorf("%s:%d: %w", scope.Source.Path, scope.Source.Line, err)
			}
		}
	}
	s := &Snapshot{Directory: dir, Config: c, Resources: []Resource{}, RequiredEnvironment: []string{}}
	required := map[string]bool{}
	for r, t := range traces {
		s.Traces = append(s.Traces, *t)
		if !t.Selected {
			continue
		}
		s.Resources = append(s.Resources, r)
		if r.Kind == "connection" {
			conn := c.Connections[r.ID]
			auths := []ID{conn.Auth}
			for _, id := range conn.AuthEnvironment {
				auths = append(auths, id)
			}
			for _, id := range auths {
				if a, ok := c.Auth[id]; ok && a.Kind == "environment" {
					required[a.Variable] = true
				}
			}
		}
	}
	less := func(a, b Resource) bool {
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.ID < b.ID
	}
	sort.Slice(s.Resources, func(i, j int) bool { return less(s.Resources[i], s.Resources[j]) })
	sort.Slice(s.Traces, func(i, j int) bool { return less(s.Traces[i].Resource, s.Traces[j].Resource) })
	for name := range required {
		s.RequiredEnvironment = append(s.RequiredEnvironment, name)
	}
	sort.Strings(s.RequiredEnvironment)
	s.ID = Digest(struct {
		Config              *Config
		Directory, Platform string
	}{c.withoutPositions(), dir, runtime.GOOS + "/" + runtime.GOARCH})
	// Local resources are content inputs too, even when no package is declared.
	content := []string{s.ID}
	for _, r := range s.Resources {
		switch r.Kind {
		case "skill":
			h, err := TreeHash(c.Skills[r.ID].Path)
			if err != nil {
				return nil, err
			}
			content = append(content, h)
		case "instructions":
			b, err := os.ReadFile(c.Instructions[r.ID].Path)
			if err != nil {
				return nil, err
			}
			content = append(content, Digest(string(b)))
		}
	}
	s.ID = Digest(content)
	return s, nil
}

func Contains(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func pathDepth(path string) int {
	return len(strings.FieldsFunc(filepath.Clean(path), func(r rune) bool { return r == '/' || r == '\\' }))
}

func (c *Config) expand(s Selection, chain []ID, visit func(Resource, []ID)) error {
	for _, group := range []struct {
		kind string
		ids  []ID
	}{{"skill", s.Skills}, {"instructions", s.Instructions}, {"connection", s.Connections}} {
		for _, id := range group.ids {
			exists := false
			switch group.kind {
			case "skill":
				_, exists = c.Skills[id]
			case "instructions":
				_, exists = c.Instructions[id]
			case "connection":
				_, exists = c.Connections[id]
			}
			if !exists {
				return fmt.Errorf("unknown %s %q", group.kind, id)
			}
			visit(Resource{Kind: group.kind, ID: id}, chain)
		}
	}
	for _, id := range s.Bundles {
		for _, parent := range chain {
			if parent == id {
				return fmt.Errorf("bundle cycle: %v -> %s", chain, id)
			}
		}
		b, ok := c.Bundles[id]
		if !ok {
			return fmt.Errorf("unknown bundle %q", id)
		}
		if err := c.expand(b.Selection, append(append([]ID{}, chain...), id), visit); err != nil {
			return err
		}
	}
	return nil
}

// Source lines change with comment edits; they belong to explanations, not snapshot identity.
func (c *Config) withoutPositions() *Config {
	out := *c
	out.Skills = stripPositions(
		c.Skills,
		func(r FileResource) FileResource { r.Source = Source{Path: r.Source.Path}; return r },
	)
	out.Instructions = stripPositions(
		c.Instructions,
		func(r FileResource) FileResource { r.Source = Source{Path: r.Source.Path}; return r },
	)
	out.Connections = stripPositions(
		c.Connections,
		func(r Connection) Connection { r.Source = Source{Path: r.Source.Path}; return r },
	)
	out.Auth = stripPositions(c.Auth, func(r Auth) Auth { r.Source = Source{Path: r.Source.Path}; return r })
	out.Bundles = stripPositions(c.Bundles, func(r Bundle) Bundle { r.Source = Source{Path: r.Source.Path}; return r })
	out.Scopes = make([]Scope, len(c.Scopes))
	for i, s := range c.Scopes {
		s.Source = Source{Path: s.Source.Path}
		out.Scopes[i] = s
	}
	return &out
}

func stripPositions[T any](in map[ID]T, strip func(T) T) map[ID]T {
	out := make(map[ID]T, len(in))
	for id, v := range in {
		out[id] = strip(v)
	}
	return out
}

// Preserve distinct names on case-sensitive volumes while normalizing names that
// the filesystem resolves to the same directory on case-insensitive volumes.
// Components of directories that cannot be listed (traversable but not readable)
// or that vanished meanwhile are kept as given.
func actualCase(path string) string {
	volume := filepath.VolumeName(path)
	current := volume + string(filepath.Separator)
	rest := strings.TrimLeft(strings.TrimPrefix(path, volume), string(filepath.Separator))
	for _, part := range strings.Split(rest, string(filepath.Separator)) {
		if part == "" {
			continue
		}
		name := part
		entries, err := os.ReadDir(current)
		if err == nil && !slices.ContainsFunc(entries, func(e os.DirEntry) bool { return e.Name() == part }) {
			if target, err := os.Stat(filepath.Join(current, part)); err == nil {
				for _, entry := range entries {
					info, err := entry.Info()
					if err == nil && os.SameFile(target, info) {
						name = entry.Name()
						break
					}
				}
			}
		}
		current = filepath.Join(current, name)
	}
	return current
}
