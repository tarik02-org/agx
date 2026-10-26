package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/tarik02-org/agx/internal/platform"
)

type ID string

// Kind is a kind of definition, named by its top-level section; the selection kinds are also
// the keys of selection lists.
type Kind string

const (
	KindSkills       Kind = "skills"
	KindInstructions Kind = "instructions"
	KindMCPServers   Kind = "mcp"
	KindBundles      Kind = "bundles"
	KindSecrets      Kind = "secrets"
	KindPackages     Kind = "packages"
	KindHarnesses    Kind = "harnesses"
)

var (
	// Kinds lists every kind of definition.
	Kinds = []Kind{KindSkills, KindInstructions, KindMCPServers, KindSecrets, KindBundles, KindPackages, KindHarnesses}
	// SelectionKinds are the kinds selection lists hold, in the order documents write them.
	SelectionKinds = []Kind{KindSkills, KindInstructions, KindMCPServers, KindBundles}
)

// kindNames holds each kind's name in messages and the agx command managing it.
var kindNames = map[Kind]struct{ label, command string }{
	KindSkills:       {"skill", "skill"},
	KindInstructions: {"instructions", "instructions"},
	KindMCPServers:   {"MCP server", "mcp"},
	KindBundles:      {"bundle", "bundles"},
	KindSecrets:      {"secret", "secret"},
	KindPackages:     {"package", "package"},
	KindHarnesses:    {"harness", "harness"},
}

// Label names the kind in messages.
func (k Kind) Label() string { return kindNames[k].label }

// Command is the agx command managing the kind.
func (k Kind) Command() string { return kindNames[k].command }

// JoinKinds lists kinds for messages, such as "skills, mcp".
func JoinKinds(kinds []Kind) string {
	names := []string{}
	for _, k := range kinds {
		names = append(names, string(k))
	}
	return strings.Join(names, ", ")
}

type Source struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type Package struct {
	Path     string `yaml:"path"     json:"path,omitempty"   jsonschema_description:"Local package directory."`
	Git      string `yaml:"git"      json:"git,omitempty"    jsonschema_description:"Git repository URL."`
	Ref      string `yaml:"ref"      json:"ref,omitempty"    jsonschema_description:"Git branch, tag, or commit, locked at install; a full 40-character commit pins the source without a lock entry."`
	Hash     string `yaml:"hash"     json:"hash,omitempty"   jsonschema_description:"Content hash verified at install; pins the source without a lock entry."`
	Subdir   string `yaml:"subdir"   json:"subdir,omitempty" jsonschema_description:"Package root within the source."`
	Override bool   `yaml:"override" json:"-"                jsonschema_description:"Replace an existing definition."`
	Lockfile string `yaml:"-"        json:"-"`
	Source   Source `yaml:"-"        json:"-"`
	// Manifest marks definitions from package manifests rather than configuration fragments.
	Manifest bool `yaml:"-" json:"-"`
	// Anonymous marks the Git source of a skill or instructions definition: plain content
	// without a manifest, whose resources point into its checkout.
	Anonymous bool `yaml:"-" json:"anonymous,omitempty"`
	// Parent is the package whose manifest declared an anonymous package; empty for fragments.
	Parent ID `yaml:"-" json:"-"`
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

// Pinned reports whether the source determines its content without a lock entry: a full commit,
// a content hash, or a directory inside the read-only Nix store.
func (p Package) Pinned() bool {
	return IsCommit(p.Ref) || p.Hash != "" || p.Path != "" && InNixStore(p.Path)
}

// GitSource fetches a skill or instructions file from a repository as an anonymous package.
type GitSource struct {
	Git  string `yaml:"git"  json:"git,omitempty"  jsonschema_description:"Git repository holding the resource, fetched like a package; path is then relative to the checkout."`
	Ref  string `yaml:"ref"  json:"ref,omitempty"  jsonschema_description:"git: branch, tag, or commit, locked at install; a full 40-character commit pins the source without a lock entry."`
	Hash string `yaml:"hash" json:"hash,omitempty" jsonschema_description:"git: content hash of the checkout, verified at install; pins the source without a lock entry."`
}

// Skill is a skill directory: local, from a Git source, or generated from configuration.
type Skill struct {
	Path        string `yaml:"path"        json:"path,omitempty"        jsonschema_description:"Skill directory containing SKILL.md, relative to this file; with git, relative to the checkout."`
	GitSource   `                     yaml:",inline"`
	Name        string               `yaml:"name"        json:"name,omitempty"        jsonschema_description:"content: native skill name, lowercase letters, digits, and hyphens; defaults to the ID's last segment."`
	Description string               `yaml:"description" json:"description,omitempty" jsonschema_description:"content: what the skill does and when to use it."`
	Frontmatter map[string]any       `yaml:"frontmatter" json:"frontmatter,omitempty" jsonschema_description:"content: extra SKILL.md front matter, passed through."`
	Content     string               `yaml:"content"     json:"content,omitempty"     jsonschema_description:"SKILL.md body of a skill generated from configuration."`
	Files       map[string]SkillFile `yaml:"files"       json:"files,omitempty"       jsonschema_description:"content: other files by path relative to the skill root; a trailing slash copies a directory."`
	Override    bool                 `yaml:"override"    json:"-"                     jsonschema_description:"Replace an existing definition."`
	// Package is the anonymous package of a Git skill; until it is placed, Path is relative to
	// its checkout.
	Package ID `yaml:"-" json:"package,omitempty"`
	// Inline marks definitions written in a selection list, whose IDs may not be reused.
	Inline bool   `yaml:"-" json:"-"`
	Source Source `yaml:"-" json:"source"`
}

// Instructions is guidance text: a file, a file from a Git source, or text in configuration.
type Instructions struct {
	Path      string `yaml:"path"     json:"path,omitempty"    jsonschema_description:"Instructions file, relative to this file; with git, relative to the checkout."`
	GitSource `       yaml:",inline"`
	Text      string `yaml:"text"     json:"text,omitempty"    jsonschema_description:"Instructions written in configuration."`
	Order     int    `yaml:"order"    json:"order,omitempty"   jsonschema_description:"Sorts selected instructions, lower first, before resolution order; defaults to 0."`
	Override  bool   `yaml:"override" json:"-"                 jsonschema_description:"Replace an existing definition."`
	Package   ID     `yaml:"-"        json:"package,omitempty"`
	Inline    bool   `yaml:"-"        json:"-"`
	Source    Source `yaml:"-"        json:"source"`
}

// MCPServer is an MCP server together with the account and settings used to reach it. Load is
// LoadNative or LoadOnDemand once merged.
type MCPServer struct {
	// Transport follows from url (http) or command (stdio) once merged.
	Transport   string           `yaml:"-"           json:"transport"`
	Command     []string         `yaml:"command"     json:"command,omitempty"     jsonschema_description:"stdio: server command and arguments."`
	URL         string           `yaml:"url"         json:"url,omitempty"         jsonschema_description:"http: server URL; http or https."`
	Cwd         string           `yaml:"cwd"         json:"cwd,omitempty"         jsonschema_description:"stdio: working directory; defaults to the launch directory."`
	Environment map[string]Value `yaml:"environment" json:"environment,omitempty" jsonschema_description:"stdio: environment variables for the server."`
	Headers     map[string]Value `yaml:"headers"     json:"headers,omitempty"     jsonschema_description:"http: extra request headers."`
	Auth        MCPAuth          `yaml:"auth"        json:"auth"`
	Description string           `yaml:"description" json:"description,omitempty" jsonschema_description:"What the server offers, shown by agx mcp tools."`
	Load        string           `yaml:"load"        json:"load"                  jsonschema_description:"native: in the harness's tool list (default); on_demand: reached with agx mcp call." jsonschema:"enum=native,enum=on_demand"`
	Share       MCPShare         `yaml:"share"       json:"share,omitzero"`
	Override    bool             `yaml:"override"    json:"-"                     jsonschema_description:"Replace an existing definition."`
	Inline      bool             `yaml:"-"           json:"-"`
	Source      Source           `yaml:"-"           json:"source"`
}

// Load modes of an MCP server: a native server is in the harness's tool list; an on-demand
// one is left out of it and reached with agx mcp call.
const (
	LoadNative   = "native"
	LoadOnDemand = "on_demand"
)

// Share modes of a stdio MCP server: whether harness sessions get a process each, or join one
// process per launch directory, per repository root, or for everything. Merging leaves none
// empty, so snapshots of configurations that do not share keep their identity.
const (
	ShareNone       = "none"
	ShareDirectory  = "directory"
	ShareRepository = "repo"
	ShareGlobal     = "global"
)

// MCPShare is whether a stdio MCP server's sessions share one process: a share mode, or
// {type, idle} to also say how long the process outlives its last session.
type MCPShare struct {
	// Type is a share mode other than none; empty once merged for none.
	Type string `json:"type,omitempty"`
	// Idle is how long a shared process stays after its last session; nil for the default.
	Idle *time.Duration `json:"idle,omitempty"`
}

// ShareMode is the server's share mode, none when it does not share.
func (c MCPServer) ShareMode() string {
	if c.Share.Type == "" {
		return ShareNone
	}
	return c.Share.Type
}

// MCPAuth is the credential an HTTP MCP server sends as its bearer token: a tagged value
// (!secret, !env, or !file), an inline secret owned by the entry, or none. Merging stores an
// inline secret, including the implicit on-demand OAuth of an HTTP entry without auth, in
// Config.Secrets under the entry's ID and points Secret at it.
type MCPAuth struct {
	Value
	None bool `json:"none,omitempty"`
	// Inline is the entry's own secret as configured; nil for references, none, and stdio.
	Inline *Secret `json:"-"`
}

// Secret defines how a credential is obtained.
type Secret struct {
	Kind     string    `yaml:"kind"      json:"kind"                jsonschema:"required"`
	Command  []string  `yaml:"command"   json:"command,omitempty"                         jsonschema_description:"Command whose trimmed stdout is the secret."`
	File     string    `yaml:"file"      json:"file,omitempty"                            jsonschema_description:"sops: encrypted file, relative to the declaring fragment."`
	Key      SecretKey `yaml:"key"       json:"key,omitempty"                             jsonschema_description:"sops: dotted key path or list of segments to extract; omit to read the whole file."`
	AgeKey   Value     `yaml:"age_key"   json:"age_key,omitzero"                          jsonschema_description:"sops: age identity passed to sops as SOPS_AGE_KEY."`
	Issuer   string    `yaml:"issuer"    json:"issuer,omitempty"                          jsonschema_description:"OAuth issuer URL; discovered when omitted."`
	ClientID string    `yaml:"client_id" json:"client_id,omitempty"                       jsonschema_description:"OAuth public client ID."`
	Resource string    `yaml:"resource"  json:"resource,omitempty"                        jsonschema_description:"OAuth resource indicator."`
	Scopes   []string  `yaml:"scopes"    json:"scopes,omitempty"                          jsonschema_description:"OAuth scopes to request."`
	Override bool      `yaml:"override"  json:"-"                                         jsonschema_description:"Replace an existing definition."`
	// Owner is the MCP server whose auth declares this secret inline; empty for top-level
	// secrets. It keeps the credentials of an inline secret apart from a top-level one.
	Owner ID `yaml:"-" json:"owner,omitempty"`
	// Implicit marks the on-demand OAuth of an HTTP MCP server that omits auth.
	Implicit bool   `yaml:"-" json:"implicit,omitempty"`
	Source   Source `yaml:"-" json:"source"`
}

// Selection lists resources by ID once inline definitions are merged.
type Selection struct {
	Skills       []ID `json:"skills,omitempty"`
	Instructions []ID `json:"instructions,omitempty"`
	MCP          []ID `json:"mcp,omitempty"`
	Bundles      []ID `json:"bundles,omitempty"`
}

// SelectionSpec is a selection as written: references, paths, and inline definitions.
type SelectionSpec struct {
	Skills       []SkillItem        `yaml:"skills"       jsonschema_description:"Skill IDs, skill directory paths, or inline skill definitions."`
	Instructions []InstructionsItem `yaml:"instructions" jsonschema_description:"Instructions IDs, file paths, or inline instructions definitions."`
	MCP          []MCPItem          `yaml:"mcp"          jsonschema_description:"MCP server IDs or inline MCP server definitions with an id."`
	Bundles      []ID               `yaml:"bundles"      jsonschema_description:"Bundle IDs."`
}

type Bundle struct {
	Selection
	Source Source `json:"source"`
}

type BundleSpec struct {
	SelectionSpec `     yaml:",inline"`
	Override      bool `yaml:"override" jsonschema_description:"Replace an existing definition."`
}

type Scope struct {
	// Directory is canonical, or empty for a rule that applies everywhere.
	Directory string `json:"directory"`
	// Harnesses limits the rule to launches of these harnesses or definitions based on them.
	Harnesses []string  `json:"harnesses,omitempty"`
	Add       Selection `json:"add"`
	Remove    Selection `json:"remove"`
	Source    Source    `json:"source"`
}

type ScopeSpec struct {
	Directory string        `yaml:"directory" jsonschema_description:"Directory the rule applies to, with its descendants; omitted, the rule applies everywhere, before every directory rule."`
	Harnesses []string      `yaml:"harnesses" jsonschema_description:"Apply only to launches of these harnesses or definitions based on them."`
	Add       SelectionSpec `yaml:"add"       jsonschema_description:"Resources to select."`
	Remove    SelectionSpec `yaml:"remove"    jsonschema_description:"Resources to deselect."`
}

// Daemon settings each come from at most one fragment.
type Daemon struct {
	LeaseSeconds int   `yaml:"lease_seconds" json:"lease_seconds"       jsonschema:"minimum=3" jsonschema_description:"Launch lease length; defaults to 60."`
	RenewSeconds int   `yaml:"renew_seconds" json:"renew_seconds"       jsonschema:"minimum=1" jsonschema_description:"Lease renewal interval, shorter than the lease; defaults to 20."`
	Autostart    *bool `yaml:"autostart"     json:"autostart,omitempty"                        jsonschema_description:"Start the daemon in the background when a command needs it; defaults to true. AGX_NO_AUTOSTART=1 overrides it."`
}

type Document struct {
	Imports      []string            `yaml:"imports"      jsonschema_description:"Fragments loaded before this file, relative to it: paths, which must exist, or glob patterns, which may match nothing, loaded in lexical order."`
	Packages     map[ID]Package      `yaml:"packages"     jsonschema_description:"Package sources by ID."`
	Skills       map[ID]Skill        `yaml:"skills"       jsonschema_description:"Skills: directories containing SKILL.md, Git sources, or generated from content."`
	Instructions map[ID]Instructions `yaml:"instructions" jsonschema_description:"Instructions applied to launches: files, Git sources, or text."`
	MCP          map[ID]MCPServer    `yaml:"mcp"          jsonschema_description:"MCP servers with the account and settings used to reach them."`
	Secrets      map[ID]Secret       `yaml:"secrets"      jsonschema_description:"How credentials are obtained."`
	Bundles      map[ID]BundleSpec   `yaml:"bundles"      jsonschema_description:"Named selections of resources."`
	Scopes       []ScopeSpec         `yaml:"scopes"       jsonschema_description:"Directory rules that add or remove resources."`
	Harnesses    map[string]Harness  `yaml:"harnesses"    jsonschema_description:"Built-in harness customizations and new definitions."`
	Daemon       Daemon              `yaml:"daemon"       jsonschema_description:"Launch lease timing and automatic daemon startup."`
}

type Config struct {
	Root          string                       `json:"root"`
	Packages      map[ID]Package               `json:"packages"`
	Skills        map[ID]Skill                 `json:"skills"`
	Instructions  map[ID]Instructions          `json:"instructions"`
	MCP           map[ID]MCPServer             `json:"mcp"`
	Secrets       map[ID]Secret                `json:"secrets"`
	Bundles       map[ID]Bundle                `json:"bundles"`
	Scopes        []Scope                      `json:"scopes"`
	Harnesses     map[string]HarnessDefinition `json:"harnesses"`
	Daemon        Daemon                       `json:"daemon"`
	Files         []string                     `json:"files"`
	PackageHashes map[ID]string                `json:"package_hashes"`
	// Missing holds the declared packages that are not installed, which loading skips along
	// with every resource they would define.
	Missing map[ID]MissingPackage `json:"missing,omitempty"`

	// rootDefinitions maps definitions from configuration fragments to their override flag,
	// so package manifests merged later cannot replace them.
	rootDefinitions map[string]bool
	// daemonSet records the fragment that set each daemon setting.
	daemonSet map[string]string
	builtins  Builtins
}

// MissingPackage is a package that is not installed. Root is the declared package the warning
// names: the package itself, or the missing package it belongs to.
type MissingPackage struct {
	Root   ID     `json:"root"`
	Reason string `json:"reason"`
}

// missingOwner reports the missing package a reference of kind into id would have come from:
// the package whose namespace holds it, or the Git source of a skill or instructions
// definition left out because it is not installed.
func (c *Config) missingOwner(kind string, id ID) (ID, bool) {
	if m, ok := c.Missing[ID(kind+":"+string(id))]; ok {
		return m.Root, true
	}
	for prefix := string(id); ; {
		i := strings.LastIndex(prefix, "/")
		if i <= 0 {
			return "", false
		}
		prefix = prefix[:i]
		if m, ok := c.Missing[ID(prefix)]; ok {
			return m.Root, true
		}
	}
}

// MissingWarnings describes each missing package once, sorted.
func (c *Config) MissingWarnings(ids []ID) []string {
	warnings := []string{}
	for _, id := range ids {
		if m, ok := c.Missing[id]; ok && m.Root == id {
			warnings = append(warnings, m.Reason)
		}
	}
	return warnings
}

// AllMissing lists the missing packages warnings name, sorted.
func (c *Config) AllMissing() []ID {
	ids := []ID{}
	for id, m := range c.Missing {
		if m.Root == id {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}

type Resource struct {
	Kind string `json:"kind"`
	ID   ID     `json:"id"`
}

type Step struct {
	Change    string `json:"change"`
	Source    Source `json:"source"`
	Directory string `json:"scope_directory"`
	// Harness names the definition whose add/remove produced the step; empty for scopes.
	Harness     string `json:"harness,omitempty"`
	BundleChain []ID   `json:"bundle_chain,omitempty"`
}

type Trace struct {
	Resource Resource `json:"resource"`
	Selected bool     `json:"selected"`
	// Order is a selected instructions definition's sort key; zero otherwise.
	Order int    `json:"order,omitempty"`
	Steps []Step `json:"steps"`
}

type Snapshot struct {
	ID                  string     `json:"id"`
	Directory           string     `json:"directory"`
	Harness             string     `json:"harness,omitempty"`
	Resources           []Resource `json:"resources"`
	RequiredEnvironment []string   `json:"required_environment"`
	Traces              []Trace    `json:"traces,omitempty"`
	// Missing lists the missing packages whose resources the selection would have added.
	Missing []ID `json:"missing,omitempty"`
	// Injections lists tagged values the launch resolves, as configured; never their values.
	Injections []Injection `json:"injections,omitempty"`
	Config     *Config     `json:"-"`
}

func Canonical(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	// Resolve the existing prefix as scopes may name directories not created yet, or
	// ones this process may not read, such as a home directory seen from a build sandbox.
	suffix := []string{}
	for {
		real, err := filepath.EvalSymlinks(abs)
		if err == nil {
			real = platform.ActualCase(real)
			for i := len(suffix) - 1; i >= 0; i-- {
				real = filepath.Join(real, suffix[i])
			}
			return filepath.Clean(real), nil
		}
		if !os.IsNotExist(err) && !os.IsPermission(err) {
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

// Resolve selects resources for a directory and harness. An empty harness applies only rules
// without a harness filter and no harness's own add/remove.
func (c *Config) Resolve(directory, harness string) (*Snapshot, error) {
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
	var h ResolvedHarness
	if harness != "" {
		h, err = c.Harness(harness)
		if err != nil {
			return nil, err
		}
	}
	chain := h.Chain
	traces := map[Resource]*Trace{}
	// selectedAt records when each resource was last newly selected, which orders instructions:
	// shallower scopes first, harness rules last, and list order within a rule.
	selectedAt := map[Resource]int{}
	sequence := 0
	missing := map[ID]bool{}
	apply := func(change string, selection Selection, step Step) error {
		err := c.expandSkipping(selection, nil, func(r Resource, bundles []ID) {
			t := traces[r]
			if t == nil {
				t = &Trace{Resource: r}
				traces[r] = t
			}
			sequence++
			if change == "add" && !t.Selected {
				selectedAt[r] = sequence
			}
			t.Selected = change == "add"
			step := step
			step.Change = change
			step.BundleChain = append([]ID(nil), bundles...)
			t.Steps = append(t.Steps, step)
		}, func(pkg ID) {
			if change == "add" {
				missing[pkg] = true
			}
		})
		if err != nil {
			return errorAt(step.Source, "%w", err)
		}
		return nil
	}
	scopes := append([]Scope(nil), c.Scopes...)
	sort.SliceStable(scopes, func(i, j int) bool { return scopes[i].depth() < scopes[j].depth() })
	for _, scope := range scopes {
		if scope.Directory != "" && !Contains(scope.Directory, dir) {
			continue
		}
		if len(scope.Harnesses) > 0 && !slices.ContainsFunc(scope.Harnesses, func(name string) bool {
			return slices.Contains(chain, name)
		}) {
			continue
		}
		step := Step{Source: scope.Source, Directory: scope.Directory}
		if err := apply("add", scope.Add, step); err != nil {
			return nil, err
		}
		if err := apply("remove", scope.Remove, step); err != nil {
			return nil, err
		}
	}
	// A harness's own selection follows directory scopes so it holds everywhere for that harness.
	for _, name := range chain {
		selections := c.Harnesses[name].Selections
		for _, change := range []string{"add", "remove"} {
			for _, s := range selections {
				selection := s.Add
				if change == "remove" {
					selection = s.Remove
				}
				if err := apply(change, selection, Step{Source: s.Source, Harness: name}); err != nil {
					return nil, err
				}
			}
		}
	}
	s := &Snapshot{
		Directory:           dir,
		Harness:             harness,
		Config:              c,
		Resources:           []Resource{},
		RequiredEnvironment: []string{},
		Missing:             slices.Sorted(maps.Keys(missing)),
	}
	for r, t := range traces {
		if t.Selected {
			s.Resources = append(s.Resources, r)
			if r.Kind == "instructions" {
				t.Order = c.Instructions[r.ID].Order
			}
		}
		s.Traces = append(s.Traces, *t)
	}
	less := func(a, b Resource) bool {
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.ID < b.ID
	}
	sort.Slice(s.Traces, func(i, j int) bool { return less(s.Traces[i].Resource, s.Traces[j].Resource) })
	// Instructions concatenate in this order, so it is part of the snapshot.
	sort.Slice(s.Resources, func(i, j int) bool {
		a, b := s.Resources[i], s.Resources[j]
		if a.Kind == "instructions" && b.Kind == "instructions" {
			if oa, ob := c.Instructions[a.ID].Order, c.Instructions[b.ID].Order; oa != ob {
				return oa < ob
			}
			return selectedAt[a] < selectedAt[b]
		}
		return less(a, b)
	})
	injections := s.MCPInjections()
	if harness != "" {
		injections = append(injections, h.Injections()...)
	}
	required := map[string]bool{}
	for _, i := range injections {
		if name := c.Variable(i.Value); name != "" {
			required[name] = true
		}
		if i.Value.Tagged() {
			s.Injections = append(s.Injections, i)
		}
	}
	for name := range required {
		s.RequiredEnvironment = append(s.RequiredEnvironment, name)
	}
	sort.Strings(s.RequiredEnvironment)
	s.ID = Digest(struct {
		Config                       *Config
		Directory, Harness, Platform string
	}{c.withoutPositions(), dir, harness, runtime.GOOS + "/" + runtime.GOARCH})
	// Local resources are content inputs too, even when no package is declared.
	content := []string{s.ID}
	for _, r := range s.Resources {
		switch r.Kind {
		case "skill":
			h, err := c.Skills[r.ID].TreeHash()
			if err != nil {
				return nil, fmt.Errorf("skill %s: %w", r.ID, err)
			}
			content = append(content, h)
		case "instructions":
			b, err := c.Instructions[r.ID].Read()
			if err != nil {
				return nil, fmt.Errorf("instructions %s: %w", r.ID, err)
			}
			content = append(content, Digest(string(b)))
		}
	}
	s.ID = Digest(content)
	return s, nil
}

// MCPInjections lists the values of selected MCP servers, including their auth secrets.
func (s *Snapshot) MCPInjections() []Injection {
	out := []Injection{}
	for _, r := range s.Resources {
		if r.Kind != "mcp" {
			continue
		}
		server := s.Config.MCP[r.ID]
		if server.Auth.Tagged() {
			auth := injection("mcp "+string(r.ID)+" auth", server.Auth.Value)
			if secret := s.Config.Secrets[server.Auth.Secret]; server.Auth.Secret != "" && secret.Owner != "" {
				auth.Text = secret.Kind
				if secret.Implicit {
					auth.Text += " (on demand)"
				}
			}
			out = append(out, auth)
		}
		out = append(out, server.Injections(r.ID)...)
	}
	return out
}

// MCPEnvironment lists caller environment variables that only selected MCP servers
// receive, which the harness must not inherit.
func (s *Snapshot) MCPEnvironment() []string {
	names := []string{}
	for _, i := range s.MCPInjections() {
		if name := s.Config.Variable(i.Value); name != "" && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func Contains(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// depth orders rules from shallowest to deepest; a rule without a directory applies everywhere
// and comes before every directory rule.
func (s Scope) depth() int {
	if s.Directory == "" {
		return -1
	}
	return pathDepth(s.Directory)
}

func pathDepth(path string) int {
	return len(strings.FieldsFunc(filepath.Clean(path), func(r rune) bool { return r == '/' || r == '\\' }))
}

func (c *Config) expand(s Selection, chain []ID, visit func(Resource, []ID)) error {
	return c.expandSkipping(s, chain, visit, nil)
}

// expandSkipping expands like expand, leaving out references into missing packages and
// reporting their package to skipped, when set.
func (c *Config) expandSkipping(
	s Selection,
	chain []ID,
	visit func(Resource, []ID),
	skipped func(ID),
) error {
	for _, group := range []struct {
		kind, label string
		ids         []ID
	}{{"skill", "skill", s.Skills}, {"instructions", "instructions", s.Instructions}, {"mcp", "MCP server", s.MCP}} {
		for _, id := range group.ids {
			exists := false
			switch group.kind {
			case "skill":
				_, exists = c.Skills[id]
			case "instructions":
				_, exists = c.Instructions[id]
			case "mcp":
				_, exists = c.MCP[id]
			}
			if !exists {
				if pkg, ok := c.missingOwner(group.kind, id); ok {
					if skipped != nil {
						skipped(pkg)
					}
					continue
				}
				return fmt.Errorf("unknown %s %q", group.label, id)
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
			if pkg, missing := c.missingOwner("bundle", id); missing {
				if skipped != nil {
					skipped(pkg)
				}
				continue
			}
			return fmt.Errorf("unknown bundle %q", id)
		}
		if err := c.expandSkipping(b.Selection, append(append([]ID{}, chain...), id), visit, skipped); err != nil {
			return err
		}
	}
	return nil
}

// SelectableSkills lists, sorted, the skills some scope or harness rule adds, directly or through
// bundles: those a launch can select. Skills a package defines but no rule adds are left out, so
// installing native plugins does not enable them.
func (c *Config) SelectableSkills() []ID {
	selected := map[ID]bool{}
	visit := func(r Resource, _ []ID) {
		if r.Kind == "skill" {
			selected[r.ID] = true
		}
	}
	// Validation already checked every selection, so expansion cannot fail here.
	for _, scope := range c.Scopes {
		_ = c.expand(scope.Add, nil, visit)
	}
	for _, h := range c.Harnesses {
		for _, s := range h.Selections {
			_ = c.expand(s.Add, nil, visit)
		}
	}
	return slices.Sorted(maps.Keys(selected))
}

// Source lines change with comment edits; they belong to explanations, not snapshot identity.
func (c *Config) withoutPositions() *Config {
	out := *c
	out.Skills = stripPositions(
		c.Skills,
		func(r Skill) Skill { r.Source = Source{Path: r.Source.Path}; return r },
	)
	out.Instructions = stripPositions(
		c.Instructions,
		func(r Instructions) Instructions { r.Source = Source{Path: r.Source.Path}; return r },
	)
	out.MCP = stripPositions(
		c.MCP,
		func(r MCPServer) MCPServer { r.Source = Source{Path: r.Source.Path}; return r },
	)
	out.Secrets = stripPositions(
		c.Secrets,
		func(r Secret) Secret { r.Source = Source{Path: r.Source.Path}; return r },
	)
	out.Bundles = stripPositions(c.Bundles, func(r Bundle) Bundle { r.Source = Source{Path: r.Source.Path}; return r })
	out.Harnesses = make(map[string]HarnessDefinition, len(c.Harnesses))
	for name, h := range c.Harnesses {
		h.Source = Source{Path: h.Source.Path}
		h.Selections = append([]HarnessSelection(nil), h.Selections...)
		for i := range h.Selections {
			h.Selections[i].Source = Source{Path: h.Selections[i].Source.Path}
		}
		out.Harnesses[name] = h
	}
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
