package config

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"go.yaml.in/yaml/v4"

	"github.com/tarik02-org/agx/internal/securefs"
)

var (
	validIDSegment   = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
	validEnvironment = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

type LockedPackage struct {
	Source       Package `json:"source"`
	Revision     string  `json:"revision"`
	Hash         string  `json:"hash"`
	Dependencies []ID    `json:"dependencies"`
}
type Lock struct {
	Version  int                  `json:"version"`
	Packages map[ID]LockedPackage `json:"packages"`
}

// DefaultConfig is used when the root configuration file does not exist.
const DefaultConfig = "imports: [agx.d/*.yaml]\n"

// Options configures Load.
type Options struct {
	Builtins Builtins
	// Cache is the package cache. Without it, Load reads only configuration fragments, without
	// loading packages or validating the result.
	Cache string
	// Overlay replaces fragments and lockfiles while loading.
	Overlay Overlay
	// Inputs, when set, records the files loading read, including after a failure, so callers
	// can skip reloading until one of them changes.
	Inputs *Inputs
}

// Overlay maps canonical paths to contents that replace those files while loading, so an edit
// can be validated before anything is written.
type Overlay map[string][]byte

func (o Overlay) readFile(path string) ([]byte, error) {
	if b, ok := o[path]; ok {
		return b, nil
	}
	return os.ReadFile(path)
}

func (o Overlay) exists(path string) (bool, error) {
	if _, ok := o[path]; ok {
		return true, nil
	}
	_, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// glob adds overlay files matching pattern, such as a new drop-in fragment, to the files on disk.
func (o Overlay) glob(pattern string) ([]string, error) {
	files, err := globFiles(pattern)
	if err != nil {
		return nil, err
	}
	for path := range o {
		if matched, _ := filepath.Match(pattern, path); matched && !slices.Contains(files, path) {
			files = append(files, path)
		}
	}
	slices.Sort(files)
	return files, nil
}

func read(root string, builtins Builtins, inputs *Inputs, overlay Overlay) (*Config, error) {
	root, err := Canonical(root)
	if err != nil {
		return nil, err
	}
	c := &Config{
		Root:          root,
		Packages:      map[ID]Package{},
		Skills:        map[ID]Skill{},
		Instructions:  map[ID]Instructions{},
		MCP:           map[ID]MCPServer{},
		Secrets:       map[ID]Secret{},
		Bundles:       map[ID]Bundle{},
		Harnesses:     map[string]HarnessDefinition{},
		PackageHashes: map[ID]string{},
		Daemon:        Daemon{LeaseSeconds: 60, RenewSeconds: 20},
		builtins:      builtins,
	}
	active, seen := map[string]bool{}, map[string]bool{}
	var load func(string) error
	load = func(path string) error {
		path, err := Canonical(path)
		if err != nil {
			return err
		}
		if active[path] {
			return fmt.Errorf("import cycle at %s", path)
		}
		if seen[path] {
			return nil
		}
		active[path] = true
		defer delete(active, path)
		inputs.file(path)
		d, node, err := decode(path, overlay)
		if path == root && errors.Is(err, fs.ErrNotExist) {
			d, node, err = decodeBytes(path, []byte(DefaultConfig))
		}
		if err != nil {
			return err
		}
		for i, imp := range d.Imports {
			at := path
			if imports := position(node, path, "imports"); len(node.Content) > 0 {
				if list := mappingValue(node.Content[0], "imports"); list != nil && i < len(list.Content) {
					imports.Line = list.Content[i].Line
				}
				at = locate(imports)
			}
			p, err := ExpandHome(imp)
			if err != nil {
				return fmt.Errorf("%s: import %w", at, err)
			}
			p = relative(filepath.Dir(path), p)
			if !isGlob(imp) {
				if found, err := overlay.exists(p); err == nil && !found {
					return fmt.Errorf("%s: import %s: %s does not exist", at, imp, p)
				}
				if err := load(p); err != nil {
					return err
				}
				continue
			}
			files, err := overlay.glob(p)
			if err != nil {
				return fmt.Errorf("%s: import %s: %w", at, imp, err)
			}
			inputs.glob(p, files)
			for _, match := range files {
				if err := load(match); err != nil {
					return err
				}
			}
		}
		if err := c.merge(d, node, path, "", false); err != nil {
			return err
		}
		c.Files = append(c.Files, path)
		seen[path] = true
		return nil
	}
	if err := load(root); err != nil {
		return nil, err
	}
	if err := c.validateHarnesses(); err != nil {
		return nil, err
	}
	return c, nil
}

// isGlob reports whether an import is a pattern rather than a literal path.
func isGlob(path string) bool { return strings.ContainsAny(path, "*?[") }

// Load reads the configuration fragments from root and, given a cache, the installed packages
// they declare, and validates the result.
func Load(root string, opts Options) (*Config, error) {
	inputs, overlay, cache := cmp.Or(opts.Inputs, &Inputs{}), opts.Overlay, opts.Cache
	c, err := read(root, opts.Builtins, inputs, overlay)
	if err != nil || cache == "" {
		return c, err
	}
	loaded := map[ID]bool{}
	c.Missing = map[ID]MissingPackage{}
	// skip leaves a package out because it is not installed; a launch goes on without it.
	skip := func(id ID, format string) error {
		reason := fmt.Sprintf(format, id) + "; run agx install"
		if p := c.Packages[id]; p.Anonymous {
			kind, resource, _ := strings.Cut(string(id), ":")
			reason = fmt.Sprintf("%s %s: its Git source is not installed; run agx install", kind, resource)
		}
		c.Missing[id] = MissingPackage{Root: id, Reason: reason}
		loaded[id] = true
		return nil
	}
	var load func(ID, []ID) error
	load = func(id ID, chain []ID) error {
		for _, parent := range chain {
			if parent == id {
				return fmt.Errorf("package dependency cycle: %v -> %s", chain, id)
			}
		}
		if loaded[id] {
			return nil
		}
		p := c.Packages[id]
		// What a missing package declares, its dependencies and Git sources, is missing with it.
		if owner, ok := c.missingParent(id, p); ok {
			c.Missing[id] = MissingPackage{Root: owner}
			loaded[id] = true
			return nil
		}
		var entry LockedPackage
		hash := p.Hash
		switch {
		case p.Pinned() && hash == "":
			pin := PinPath(cache, p)
			inputs.file(pin)
			var err error
			if hash, err = ReadPin(pin); errors.Is(err, fs.ErrNotExist) {
				return skip(id, "package %s is not installed")
			} else if err != nil {
				return fmt.Errorf("package %s: %w", id, err)
			}
		case !p.Pinned():
			inputs.file(p.Lockfile)
			lock, err := ReadLock(p.Lockfile, overlay)
			var newer *securefs.NewerFormatError
			switch {
			case errors.As(err, &newer):
				return err
			case errors.Is(err, fs.ErrNotExist):
				return skip(id, "package %s is not installed")
			case err != nil:
				return fmt.Errorf("%s: %w; run agx install", p.Lockfile, err)
			}
			var ok bool
			entry, ok = lock.Packages[id]
			switch {
			case !ok:
				return skip(id, "package %s is not installed")
			case Digest(entry.Source) != Digest(p.LockSource()):
				return skip(id, "package %s changed since it was installed")
			}
			if !ValidHash(entry.Hash) {
				return fmt.Errorf("invalid cache hash for %s", id)
			}
			hash = entry.Hash
		}
		dir := filepath.Join(cache, "packages", hash)
		inputs.file(dir)
		if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
			return skip(id, "package %s is not installed")
		} else if err != nil {
			return fmt.Errorf("package %s cache: %w", id, err)
		}
		cached, err := TreeHash(dir)
		if err != nil {
			return fmt.Errorf("package %s cache: %w", id, err)
		}
		if cached != hash {
			return fmt.Errorf("package %s cache content does not match its lock", id)
		}
		c.PackageHashes[id] = hash
		if p.Anonymous {
			if err := c.AddContent(id, dir); err != nil {
				return err
			}
			loaded[id] = true
			return nil
		}
		d, err := c.AddPackage(id, dir)
		if err != nil {
			return err
		}
		deps := c.Dependencies(id, d)
		if !p.Pinned() {
			expected := append([]ID{}, entry.Dependencies...)
			sort.Slice(expected, func(i, j int) bool { return expected[i] < expected[j] })
			if Digest(deps) != Digest(expected) {
				return fmt.Errorf("stale dependency lock for %s", id)
			}
		}
		for _, dep := range deps {
			if err := load(dep, append(chain, id)); err != nil {
				return err
			}
		}
		loaded[id] = true
		return nil
	}
	ids := []ID{}
	for id := range c.Packages {
		ids = append(ids, id)
	}
	// Parents load before what they declare, so a missing parent is known when its declarations
	// come up: named packages by depth, then the Git sources of resources.
	anonymous := func(id ID) int {
		if strings.Contains(string(id), ":") {
			return 1
		}
		return 0
	}
	slices.SortFunc(ids, func(a, b ID) int {
		return cmp.Or(
			cmp.Compare(anonymous(a), anonymous(b)),
			cmp.Compare(strings.Count(string(a), "/"), strings.Count(string(b), "/")),
			cmp.Compare(a, b),
		)
	})
	for _, id := range ids {
		if err := load(id, nil); err != nil {
			return nil, err
		}
	}
	// Definitions whose content is a missing Git source are left out, like a missing package's.
	for name, s := range c.Skills {
		if _, ok := c.Missing[s.Package]; ok && s.Package != "" {
			delete(c.Skills, name)
		}
	}
	for name, i := range c.Instructions {
		if _, ok := c.Missing[i.Package]; ok && i.Package != "" {
			delete(c.Instructions, name)
		}
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

// missingParent reports the missing package that declared package id: the package of an
// anonymous Git source, or the package whose namespace holds a dependency.
func (c *Config) missingParent(id ID, p Package) (ID, bool) {
	if p.Anonymous && p.Parent != "" {
		if m, ok := c.Missing[p.Parent]; ok {
			return m.Root, true
		}
	}
	if i := strings.LastIndex(string(id), "/"); i > 0 {
		if m, ok := c.Missing[id[:i]]; ok {
			return m.Root, true
		}
	}
	return "", false
}

// AddPackage merges an installed package under its ID and returns its manifest. A package
// without agx.package.yaml declares each directory holding SKILL.md as a skill named after its
// folder, located at that SKILL.md.
func (c *Config) AddPackage(id ID, dir string) (Document, error) {
	d, node, manifest, err := decodePackage(dir)
	if err != nil {
		return d, err
	}
	if err := c.merge(d, node, manifest, string(id), true); err != nil {
		return d, err
	}
	if len(node.Content) > 0 {
		return d, nil
	}
	for name := range d.Skills {
		qualified := qualify(string(id), name)
		if s := c.Skills[qualified]; s.Source.Path == manifest {
			s.Source = Source{Path: filepath.Join(s.Path, "SKILL.md"), Line: 1, Column: 1}
			c.Skills[qualified] = s
		}
	}
	return d, nil
}

// decodePackage decodes a package's manifest, or discovers its skills when it has none. It
// returns the manifest path, which anchors the package's paths.
func decodePackage(dir string) (Document, *yaml.Node, string, error) {
	manifest := filepath.Join(dir, "agx.package.yaml")
	d, node, err := decode(manifest, nil)
	if !errors.Is(err, fs.ErrNotExist) {
		return d, node, manifest, err
	}
	d = Document{Skills: map[ID]Skill{}}
	skills, err := FindSkills(dir)
	if err != nil {
		return d, nil, manifest, err
	}
	for _, found := range skills {
		if previous, taken := d.Skills[found.ID]; taken {
			return d, nil, manifest, fmt.Errorf(
				"package %s: skills %s and %s share the ID %s; add agx.package.yaml to name them",
				dir, previous.Path, found.Path, found.ID,
			)
		}
		d.Skills[found.ID] = Skill{Path: found.Path}
	}
	return d, &yaml.Node{}, manifest, nil
}

// AddContent points the resources of an anonymous package at its installed checkout.
func (c *Config) AddContent(id ID, dir string) error {
	for name, s := range c.Skills {
		if s.Package != id {
			continue
		}
		path, err := securefs.Contained(dir, filepath.FromSlash(cmp.Or(s.Path, ".")))
		if err != nil {
			return fmt.Errorf("%s: skill %s: %w", locate(s.Source), name, err)
		}
		s.Path = path
		c.Skills[name] = s
	}
	for name, i := range c.Instructions {
		if i.Package != id {
			continue
		}
		path, err := securefs.Contained(dir, filepath.FromSlash(i.Path))
		if err != nil {
			return fmt.Errorf("%s: instructions %s: %w", locate(i.Source), name, err)
		}
		i.Path = path
		c.Instructions[name] = i
	}
	return nil
}

// Dependencies lists the packages a manifest declares, including the anonymous Git sources of its
// resources, and moves those locked with it into its lockfile.
func (c *Config) Dependencies(id ID, d Document) []ID {
	lockfile := c.Packages[id].Lockfile
	deps := []ID{}
	for dep := range d.Packages {
		qualified := qualify(string(id), dep)
		// Configuration overrides of a dependency keep their own lockfile.
		if value := c.Packages[qualified]; value.Manifest {
			value.Lockfile = lockfile
			c.Packages[qualified] = value
		}
		deps = append(deps, qualified)
	}
	for name, p := range c.Packages {
		if p.Anonymous && p.Parent == id {
			p.Lockfile = lockfile
			c.Packages[name] = p
			deps = append(deps, name)
		}
	}
	slices.Sort(deps)
	return deps
}

// PinPath locates the cache record of the content a pinned commit or /nix/store source installed;
// a hash pin names its cache entry directly.
func PinPath(cache string, p Package) string {
	key := Digest(Package{Path: p.Path, Git: p.Git, Ref: p.Ref, Subdir: p.Subdir})
	return filepath.Join(cache, "packages", "pins", key+".json")
}

// pinVersion is the format version of pin records; agx rejects records of other versions.
const pinVersion = 1

type pin struct {
	Version int    `json:"version"`
	Hash    string `json:"hash"`
}

func ReadPin(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var p pin
	if err := securefs.CheckFormat(path, b, pinVersion); err != nil {
		return "", err
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return "", fmt.Errorf("invalid pin record %s; run agx install", path)
	}
	if p.Version != pinVersion || !ValidHash(p.Hash) {
		return "", fmt.Errorf("invalid pin record %s; run agx install", path)
	}
	return p.Hash, nil
}

func WritePin(path, hash string) error {
	return securefs.WriteJSON(path, pin{Version: pinVersion, Hash: hash})
}

func decode(path string, overlay Overlay) (Document, *yaml.Node, error) {
	b, err := overlay.readFile(path)
	if err != nil {
		return Document{}, nil, err
	}
	return decodeBytes(path, b)
}

func decodeBytes(path string, b []byte) (Document, *yaml.Node, error) {
	var d Document
	var n yaml.Node
	if err := yaml.Unmarshal(b, &n); err != nil {
		return d, nil, yamlError(path, b, nil, err)
	}
	// Decoding ignores tags on plain strings, so they are checked first.
	tagErr := checkTags(path, &n, nil)
	if tagErr != nil && !errors.As(tagErr, new(misplacedTagError)) {
		return d, nil, tagErr
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&d); errors.Is(err, io.EOF) {
		return d, nil, fmt.Errorf("%s: file is empty; write {} for a fragment that declares nothing", path)
	} else if err != nil {
		return d, nil, yamlError(path, b, &n, err)
	}
	if tagErr != nil {
		return d, nil, tagErr
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return d, nil, fmt.Errorf("%s: expected one YAML document", path)
	}
	return d, &n, nil
}

// merger merges one file: a configuration fragment, or a package manifest under its namespace.
type merger struct {
	c        *Config
	node     *yaml.Node
	path     string
	base     string
	ns       string
	manifest bool
}

// prior describes an existing definition with the ID being merged.
type prior struct {
	exists, inline bool
	source         Source
}

var kindLabels = map[string]string{
	"packages": "package", "skills": "skill", "instructions": "instructions", "mcp": "MCP server",
	"secrets": "secret", "bundles": "bundle",
}

func (c *Config) merge(d Document, node *yaml.Node, path, ns string, manifest bool) error {
	base := filepath.Dir(path)
	if manifest &&
		(len(d.Imports) > 0 || len(d.Scopes) > 0 || len(d.Harnesses) > 0 || d.Daemon != (Daemon{})) {
		return fmt.Errorf("%s: package manifests may only declare resources, bundles, secrets, and dependencies", path)
	}
	m := &merger{c: c, node: node, path: path, base: base, ns: ns, manifest: manifest}
	if c.rootDefinitions == nil {
		c.rootDefinitions = map[string]bool{}
	}
	for _, id := range sortedKeys(d.Packages) {
		p := d.Packages[id]
		name := qualify(ns, id)
		at := m.source("packages", string(id))
		existing, exists := c.Packages[name]
		if apply, err := m.conflict(
			"packages",
			name,
			prior{exists: exists, source: existing.Source},
			p.Override,
			false,
			at,
		); err != nil {
			return err
		} else if !apply {
			continue
		}
		if err := m.placePackage(name, &p, at); err != nil {
			return err
		}
		c.Packages[name] = p
	}
	for _, id := range sortedKeys(d.Skills) {
		if err := m.addSkill(id, d.Skills[id], m.source("skills", string(id)), false); err != nil {
			return err
		}
	}
	for _, id := range sortedKeys(d.Instructions) {
		if err := m.addInstructions(id, d.Instructions[id], m.source("instructions", string(id)), false); err != nil {
			return err
		}
	}
	for _, id := range sortedKeys(d.Secrets) {
		secret := d.Secrets[id]
		name := qualify(ns, id)
		at := m.source("secrets", string(id))
		existing, exists := c.Secrets[name]
		if existing.Owner != "" {
			return fmt.Errorf(
				"%s: secret %s conflicts with the auth of MCP server %s, which uses that ID; rename one",
				locate(at),
				name,
				existing.Owner,
			)
		}
		if apply, err := m.conflict(
			"secrets",
			name,
			prior{exists: exists, source: existing.Source},
			secret.Override,
			false,
			at,
		); err != nil {
			return err
		} else if !apply {
			continue
		}
		secret.Source = at
		if err := m.placeSecret(&secret); err != nil {
			return fmt.Errorf("%s: secret %s: %w", locate(at), name, err)
		}
		c.Secrets[name] = secret
	}
	for _, id := range sortedKeys(d.MCP) {
		if err := m.addMCP(id, d.MCP[id], m.source("mcp", string(id)), false); err != nil {
			return err
		}
	}
	for _, id := range sortedKeys(d.Bundles) {
		b := d.Bundles[id]
		name := qualify(ns, id)
		at := m.source("bundles", string(id))
		existing, exists := c.Bundles[name]
		if apply, err := m.conflict(
			"bundles",
			name,
			prior{exists: exists, source: existing.Source},
			b.Override,
			false,
			at,
		); err != nil {
			return err
		} else if !apply {
			continue
		}
		selection, err := m.selection(b.SelectionSpec)
		if err != nil {
			return err
		}
		c.Bundles[name] = Bundle{Selection: selection, Source: at}
	}
	for i, s := range d.Scopes {
		at := Source{Path: path, Line: 1, Column: 1}
		if len(node.Content) > 0 {
			n := node.Content[0]
			for j := 0; j+1 < len(n.Content); j += 2 {
				if n.Content[j].Value == "scopes" && i < len(n.Content[j+1].Content) {
					pos := n.Content[j+1].Content[i]
					at.Line = pos.Line
					at.Column = pos.Column
				}
			}
		}
		directory := ""
		if s.Directory != "" {
			var err error
			if directory, err = ScopeDirectory(path, s.Directory); err != nil {
				return fmt.Errorf("%s: scope directory %w", locate(at), err)
			}
		}
		add, err := m.selection(s.Add)
		if err != nil {
			return err
		}
		remove, err := m.selection(s.Remove)
		if err != nil {
			return err
		}
		c.Scopes = append(c.Scopes, Scope{
			Directory: directory, Harnesses: s.Harnesses, Add: add, Remove: remove, Source: at,
		})
	}
	if err := m.harnesses(d); err != nil {
		return err
	}
	if c.daemonSet == nil {
		c.daemonSet = map[string]string{}
	}
	for _, field := range []struct {
		name string
		set  bool
	}{
		{"lease_seconds", d.Daemon.LeaseSeconds != 0},
		{"renew_seconds", d.Daemon.RenewSeconds != 0},
		{"autostart", d.Daemon.Autostart != nil},
	} {
		if !field.set {
			continue
		}
		if previous, ok := c.daemonSet[field.name]; ok {
			return fmt.Errorf("%s: daemon.%s is already set in %s; set it in one fragment", path, field.name, previous)
		}
		c.daemonSet[field.name] = path
	}
	if d.Daemon.LeaseSeconds != 0 {
		c.Daemon.LeaseSeconds = d.Daemon.LeaseSeconds
	}
	if d.Daemon.RenewSeconds != 0 {
		c.Daemon.RenewSeconds = d.Daemon.RenewSeconds
	}
	if d.Daemon.Autostart != nil {
		c.Daemon.Autostart = d.Daemon.Autostart
	}
	return nil
}

// mappingValue returns the value of key in mapping n, or nil.
func mappingValue(n *yaml.Node, key string) *yaml.Node {
	if n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

// ScopeDirectory canonicalizes a scope directory as written in file.
func ScopeDirectory(file, written string) (string, error) {
	directory, err := ExpandHome(written)
	if err != nil {
		return "", err
	}
	return Canonical(relative(filepath.Dir(file), directory))
}

// source locates a top-level definition.
func (m *merger) source(section, key string) Source {
	out := Source{Path: m.path, Line: 1, Column: 1}
	if len(m.node.Content) == 0 {
		return out
	}
	root := m.node.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != section {
			continue
		}
		n := root.Content[i+1]
		for j := 0; j+1 < len(n.Content); j += 2 {
			if n.Content[j].Value == key {
				return Source{m.path, n.Content[j].Line, n.Content[j].Column}
			}
		}
	}
	return out
}

// conflict decides whether a definition applies. Configuration fragments merge before package
// manifests, so precedence is decided by origin rather than order: a configuration override wins
// over package definitions, and packages can never replace configuration definitions. Inline
// definitions are never replaced, so a reference never silently changes meaning.
func (m *merger) conflict(kind string, id ID, p prior, override, inline bool, at Source) (bool, error) {
	if !validID(id) {
		return false, fmt.Errorf("%s: invalid ID %q", locate(at), id)
	}
	label := kindLabels[kind]
	if p.exists && (p.inline || inline) {
		return false, fmt.Errorf(
			"%s: %s %s is also defined at %s; define it once, under top-level %s:, and reference its ID elsewhere",
			locate(at), label, id, locate(p.source), kind,
		)
	}
	key := kind + "\x00" + string(id)
	rootOverride, fromRoot := m.c.rootDefinitions[key]
	if m.manifest && fromRoot {
		if rootOverride {
			return false, nil
		}
		return false, fmt.Errorf(
			"%s: package definition %s conflicts with configuration at %s; set override: true on the configuration definition",
			locate(at),
			id,
			locate(p.source),
		)
	}
	if p.exists && !override {
		return false, fmt.Errorf(
			"%s: conflicting definition of %s %s, also defined at %s; set override: true",
			locate(at), label, id, locate(p.source),
		)
	}
	if !m.manifest {
		m.c.rootDefinitions[key] = override
	}
	return true, nil
}

// file resolves a path written in this file: relative to a configuration fragment, with ~
// expanded, or contained in the package root for manifests.
func (m *merger) file(path string) (string, error) {
	if m.manifest {
		if strings.HasPrefix(path, "~") {
			return "", fmt.Errorf("%s: package manifests cannot use home-relative paths", path)
		}
		return securefs.Contained(m.base, path)
	}
	expanded, err := ExpandHome(path)
	if err != nil {
		return "", err
	}
	return relative(m.base, expanded), nil
}

// placeSecret anchors a file or sops secret's file to this file, like a !file path, and places its
// age key like any injected value.
func (m *merger) placeSecret(secret *Secret) error {
	if secret.AgeKey != (Value{}) {
		ageKey, err := m.place(secret.AgeKey)
		if err != nil {
			return err
		}
		secret.AgeKey = ageKey
	}
	if secret.File == "" {
		return nil
	}
	file, err := m.file(secret.File)
	if err != nil {
		return fmt.Errorf("file %s: %w", secret.File, err)
	}
	secret.File = file
	return nil
}

// expand expands ~ in a path that is not resolved against this file.
func (m *merger) expand(path string) (string, error) {
	if m.manifest && strings.HasPrefix(path, "~") {
		return "", fmt.Errorf("%s: package manifests cannot use home-relative paths", path)
	}
	return ExpandHome(path)
}

// place qualifies a value's secret like other references and anchors a !file path to this file;
// package files must stay inside the package, as resource paths do.
func (m *merger) place(v Value) (Value, error) {
	v.Source.Path = m.path
	if v.Secret != "" {
		v.Secret = qualify(m.ns, v.Secret)
	}
	if v.File != "" {
		file, err := m.file(v.File)
		if err != nil {
			return v, fmt.Errorf("%s:%d: !file %s: %w", m.path, v.Source.Line, v.File, err)
		}
		v.File = file
	}
	return v, nil
}

func (m *merger) placePackage(name ID, p *Package, at Source) error {
	p.Source = at
	if (p.Path == "") == (p.Git == "") {
		return fmt.Errorf("%s: package %s needs exactly one of path or git", locate(at), name)
	}
	if err := checkGitSource(p.Ref, p.Hash, p.Git != ""); err != nil {
		return fmt.Errorf("%s: package %s: %w", locate(at), name, err)
	}
	p.Manifest = m.manifest
	// The lockfile beside a fragment records the unpinned packages it declares.
	p.Lockfile = filepath.Join(m.base, "agx.lock.json")
	if p.Path != "" {
		var err error
		p.Path, err = m.file(p.Path)
		if err != nil {
			return fmt.Errorf("%s: package %s: %w", locate(at), name, err)
		}
		p.LockPath = slashRel(filepath.Dir(p.Lockfile), p.Path)
	}
	if filepath.IsAbs(p.Subdir) || strings.HasPrefix(filepath.Clean(p.Subdir), "..") {
		return fmt.Errorf("%s: invalid subdir for %s", locate(at), name)
	}
	if p.Git != "" {
		if strings.HasPrefix(p.Git, "-") {
			return fmt.Errorf("%s: invalid git source for %s", locate(at), name)
		}
		transport := gitTransport(p.Git)
		if m.manifest && transport != "https" && transport != "ssh" {
			return fmt.Errorf("%s: package %s must use an https, ssh, or scp-style Git source", locate(at), name)
		}
		if transport == "file" && !strings.HasPrefix(strings.ToLower(p.Git), "file:") {
			git, err := ExpandHome(p.Git)
			if err != nil {
				return fmt.Errorf("%s: package %s: %w", locate(at), name, err)
			}
			p.Git = relative(m.base, git)
			p.LockPath = slashRel(filepath.Dir(p.Lockfile), p.Git)
		}
	}
	return nil
}

// anonymous declares the Git source of a skill or instructions definition as a package. Its ID
// contains a colon, which declared package IDs cannot, and names the resource it holds.
func (m *merger) anonymous(kind string, resource ID, g GitSource, at Source) (ID, error) {
	id := ID(kind + ":" + string(resource))
	p := Package{Git: g.Git, Ref: g.Ref, Hash: g.Hash, Anonymous: true, Parent: ID(m.ns)}
	if err := m.placePackage(id, &p, at); err != nil {
		return "", err
	}
	m.c.Packages[id] = p
	return id, nil
}

// checkoutPath validates a path inside a Git checkout.
func checkoutPath(at Source, kind string, id ID, path string) error {
	if path != "" && (strings.HasPrefix(path, "~") || !filepath.IsLocal(filepath.FromSlash(path))) {
		return fmt.Errorf("%s: %s %s: path %s must stay inside the Git checkout", locate(at), kind, id, path)
	}
	return nil
}

func (m *merger) addSkill(id ID, s Skill, at Source, inline bool) error {
	name := qualify(m.ns, id)
	existing, exists := m.c.Skills[name]
	if apply, err := m.conflict(
		"skills",
		name,
		prior{exists, existing.Inline, existing.Source},
		s.Override,
		inline,
		at,
	); err != nil ||
		!apply {
		return err
	}
	if s.Git == "" && (s.Ref != "" || s.Hash != "") {
		return fmt.Errorf("%s: skill %s: ref and hash apply only with git", locate(at), name)
	}
	if s.Generated() && s.Name == "" {
		s.Name = string(name)[strings.LastIndex(string(name), "/")+1:]
	}
	for key, f := range s.Files {
		f.Directory = strings.HasSuffix(f.Path, "/") || strings.HasSuffix(f.Path, `\`)
		s.Files[key] = f
	}
	if err := checkSkill(name, s); err != nil {
		return fmt.Errorf("%s: %w", locate(at), err)
	}
	// A replaced definition takes its anonymous package with it.
	if existing.Package != "" {
		delete(m.c.Packages, existing.Package)
	}
	s.Package = ""
	var err error
	switch {
	case s.Git != "":
		if err := checkoutPath(at, "skill", name, s.Path); err != nil {
			return err
		}
		s.Package, err = m.anonymous("skill", name, s.GitSource, at)
	case s.Generated():
		for _, key := range sortedKeys(s.Files) {
			f := s.Files[key]
			if f.Path == "" {
				continue
			}
			if f.Path, err = m.file(f.Path); err != nil {
				return fmt.Errorf("%s: skill %s: files %s: %w", locate(at), name, key, err)
			}
			s.Files[key] = f
		}
	default:
		if s.Path, err = m.file(s.Path); err != nil {
			err = fmt.Errorf("%s: skill %s: %w", locate(at), name, err)
		}
	}
	if err != nil {
		return err
	}
	s.Inline, s.Override, s.Source = inline, false, at
	m.c.Skills[name] = s
	return nil
}

func (m *merger) addInstructions(id ID, i Instructions, at Source, inline bool) error {
	name := qualify(m.ns, id)
	existing, exists := m.c.Instructions[name]
	if apply, err := m.conflict(
		"instructions",
		name,
		prior{exists, existing.Inline, existing.Source},
		i.Override,
		inline,
		at,
	); err != nil ||
		!apply {
		return err
	}
	if i.Git == "" && (i.Ref != "" || i.Hash != "") {
		return fmt.Errorf("%s: instructions %s: ref and hash apply only with git", locate(at), name)
	}
	if err := checkInstructions(name, i); err != nil {
		return fmt.Errorf("%s: %w", locate(at), err)
	}
	if existing.Package != "" {
		delete(m.c.Packages, existing.Package)
	}
	i.Package = ""
	var err error
	switch {
	case i.Git != "":
		if err := checkoutPath(at, "instructions", name, i.Path); err != nil {
			return err
		}
		i.Package, err = m.anonymous("instructions", name, i.GitSource, at)
	case i.Path != "":
		if i.Path, err = m.file(i.Path); err != nil {
			err = fmt.Errorf("%s: instructions %s: %w", locate(at), name, err)
		}
	}
	if err != nil {
		return err
	}
	i.Inline, i.Override, i.Source = inline, false, at
	m.c.Instructions[name] = i
	return nil
}

func (m *merger) addMCP(id ID, server MCPServer, at Source, inline bool) error {
	c := m.c
	name := qualify(m.ns, id)
	existing, exists := c.MCP[name]
	if apply, err := m.conflict(
		"mcp",
		name,
		prior{exists, existing.Inline, existing.Source},
		server.Override,
		inline,
		at,
	); err != nil ||
		!apply {
		return err
	}
	server.Source, server.Inline, server.Override = at, inline, false
	where := locate(at)
	transport := "stdio"
	switch {
	case server.URL != "" && len(server.Command) > 0:
		return fmt.Errorf(
			"%s: MCP server %s sets both url and command; use url for HTTP or command for stdio",
			where,
			name,
		)
	case server.URL != "":
		transport = "http"
	case len(server.Command) == 0:
		return fmt.Errorf("%s: MCP server %s needs url (HTTP) or command (stdio)", where, name)
	}
	server.Transport = transport
	switch server.Load {
	case "":
		server.Load = LoadNative
	case LoadNative, LoadOnDemand:
	default:
		return fmt.Errorf("%s: MCP server %s: load must be native or on_demand", where, name)
	}
	switch server.Share.Type {
	case ShareNone:
		if server.Share.Idle != nil {
			return fmt.Errorf("%s: MCP server %s: share idle needs a share type other than none", where, name)
		}
		server.Share.Type = ""
	case "":
	case ShareDirectory, ShareRepository, ShareGlobal:
		if transport == "http" {
			return fmt.Errorf(
				"%s: MCP server %s: share applies to stdio servers; HTTP servers already serve many sessions",
				where,
				name,
			)
		}
	default:
		return fmt.Errorf("%s: MCP server %s: share must be none, directory, repo, or global", where, name)
	}
	if server.Cwd != "" {
		cwd, err := m.expand(server.Cwd)
		if err != nil {
			return fmt.Errorf("%s: MCP server %s: cwd %w", where, name, err)
		}
		server.Cwd = relative(m.base, cwd)
	}
	if len(server.Command) > 0 && strings.HasPrefix(server.Command[0], "~") {
		command := slices.Clone(server.Command)
		var err error
		if command[0], err = m.expand(command[0]); err != nil {
			return fmt.Errorf("%s: MCP server %s: command %w", where, name, err)
		}
		server.Command = command
	}
	// A replaced definition takes its inline secret with it.
	if previous, ok := c.Secrets[name]; ok && previous.Owner == name {
		delete(c.Secrets, name)
	}
	if server.Auth.Secret != "" {
		server.Auth.Secret = qualify(m.ns, server.Auth.Secret)
	}
	if transport == "http" && server.Auth == (MCPAuth{}) {
		server.Auth.Inline = &Secret{Kind: "oauth", Implicit: true}
	}
	if server.Auth.Inline != nil {
		// Inline secrets share the ID space of top-level ones, as agx secret addresses both.
		if _, taken := c.Secrets[name]; taken {
			what := "has inline auth"
			if server.Auth.Inline.Implicit {
				what = "logs in on demand under its ID"
			}
			return fmt.Errorf(
				"%s: MCP server %s %s, but secret %s is defined too; set auth: !secret %s or auth: none, or rename one",
				where,
				name,
				what,
				name,
				name,
			)
		}
		secret := *server.Auth.Inline
		secret.Owner = name
		secret.Source = server.Source
		if err := m.placeSecret(&secret); err != nil {
			return fmt.Errorf("%s: MCP server %s auth: %w", where, name, err)
		}
		c.Secrets[name] = secret
		server.Auth.Secret = name
	}
	for _, values := range []map[string]Value{server.Environment, server.Headers} {
		for key, v := range values {
			placed, err := m.place(v)
			if err != nil {
				return err
			}
			values[key] = placed
		}
	}
	c.MCP[name] = server
	return nil
}

func (c *Config) Validate() error {
	if c.Daemon.LeaseSeconds < 3 || c.Daemon.RenewSeconds < 1 || c.Daemon.RenewSeconds >= c.Daemon.LeaseSeconds {
		return fmt.Errorf("daemon renewal must be positive and shorter than lease (minimum 3 seconds)")
	}
	for _, id := range sortedKeys(c.Skills) {
		r := c.Skills[id]
		// Apply the same symlink rules as hashing and copying, so a valid skill can always be launched.
		if _, err := r.TreeHash(); err != nil {
			return fmt.Errorf("%s: skill %s: %w", locate(r.Source), id, err)
		}
		if !r.Generated() {
			if _, err := os.ReadFile(filepath.Join(r.Path, "SKILL.md")); err != nil {
				return fmt.Errorf("%s: skill %s: %w", locate(r.Source), id, err)
			}
		}
	}
	for _, id := range sortedKeys(c.Instructions) {
		r := c.Instructions[id]
		if _, err := r.Read(); err != nil {
			return fmt.Errorf("%s: instructions %s: %w", locate(r.Source), id, err)
		}
	}
	for id, secret := range c.Secrets {
		label := "secret " + string(id)
		if secret.Owner != "" {
			label = "MCP server " + string(secret.Owner) + " auth"
		}
		at := locate(secret.Source)
		if secret.Kind != "sops" && len(secret.Key) > 0 {
			return fmt.Errorf("%s: %s: key applies only to sops secrets", at, label)
		}
		if secret.Kind != "sops" && secret.Kind != "file" && secret.File != "" {
			return fmt.Errorf("%s: %s: file applies only to file and sops secrets", at, label)
		}
		if secret.Kind != "sops" && secret.AgeKey != (Value{}) {
			return fmt.Errorf("%s: %s: age_key applies only to sops secrets", at, label)
		}
		switch secret.Kind {
		case "environment", "daemon_environment":
			if !validEnvironment.MatchString(secret.Variable) {
				return fmt.Errorf("%s: %s needs a valid variable", at, label)
			}
		case "command":
			if len(secret.Command) == 0 || secret.Command[0] == "" {
				return fmt.Errorf("%s: %s needs a command", at, label)
			}
		case "file":
			// The file is runtime state, such as a decrypted secret under /run, so a missing one
			// fails the launch that needs it rather than the configuration.
			if secret.File == "" {
				return fmt.Errorf("%s: %s needs a file", at, label)
			}
		case "keychain":
		case "sops":
			// The encrypted file belongs to the configuration, so a missing one is an error
			// rather than a runtime state: committing it is what makes the configuration portable.
			if secret.File == "" {
				return fmt.Errorf("%s: %s needs a file", at, label)
			}
			info, err := os.Stat(secret.File)
			if err != nil {
				return fmt.Errorf("%s: %s: cannot read sops file: %w", at, label, err)
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("%s: %s: sops file %s is not a regular file", at, label, secret.File)
			}
			// A key written into configuration sits next to the file it decrypts, which
			// defeats encrypting it.
			if secret.AgeKey != (Value{}) && !secret.AgeKey.Tagged() {
				return fmt.Errorf(
					"%s: %s: age_key must be !secret ID, !env NAME, or !file PATH, not a literal key",
					at,
					label,
				)
			}
		case "oauth":
			if secret.Issuer != "" {
				if err := validateURL(secret.Issuer); err != nil {
					return fmt.Errorf("%s: %s: %w", at, label, err)
				}
			}
		default:
			if secret.Owner != "" {
				return fmt.Errorf(
					"%s: %s: unknown kind %q; reference a secret with auth: !secret %s",
					at, label, secret.Kind, secret.Kind,
				)
			}
			return fmt.Errorf("%s: %s: unknown kind %q", at, label, secret.Kind)
		}
	}
	// checkSecret validates a !secret reference; OAuth tokens need login and refresh per
	// server, so they authenticate only through an HTTP MCP server's auth field.
	checkSecret := func(i Injection) error {
		if i.Value.Secret == "" {
			return nil
		}
		secret, ok := c.Secrets[i.Value.Secret]
		if !ok {
			return fmt.Errorf("%s: %s: unknown secret %s", locate(i.Source), i.Target, i.Value.Secret)
		}
		if secret.Owner != "" {
			return fmt.Errorf(
				"%s: %s: secret %s is the inline auth of MCP server %s; declare it under secrets: to share it",
				locate(i.Source),
				i.Target,
				i.Value.Secret,
				secret.Owner,
			)
		}
		if secret.Kind == "oauth" {
			return fmt.Errorf(
				"%s: %s: OAuth secret %s can only be an MCP server's auth",
				locate(i.Source),
				i.Target,
				i.Value.Secret,
			)
		}
		return nil
	}
	for _, id := range sortedKeys(c.Secrets) {
		secret := c.Secrets[id]
		if !secret.AgeKey.Tagged() {
			continue
		}
		target := "secret " + string(id) + " age_key"
		if secret.Owner != "" {
			target = "MCP server " + string(secret.Owner) + " auth age_key"
		}
		if err := checkSecret(injection(target, secret.AgeKey)); err != nil {
			return err
		}
		// A sops secret whose age key decrypts through itself would never resolve.
		chain := []string{string(id)}
		for next := secret.AgeKey.Secret; next != ""; next = c.Secrets[next].AgeKey.Secret {
			chain = append(chain, string(next))
			if next == id {
				return fmt.Errorf(
					"%s: %s: age_key forms a cycle: %s",
					locate(secret.AgeKey.Source),
					target,
					strings.Join(chain, " -> "),
				)
			}
			if len(chain) > len(c.Secrets) {
				// The chain loops without returning here; that loop reports itself.
				break
			}
		}
	}
	oauthOrigins := map[ID]struct {
		id     ID
		origin string
	}{}
	for _, id := range sortedKeys(c.MCP) {
		server := c.MCP[id]
		at := locate(server.Source)
		switch server.Transport {
		case "stdio":
			if server.Command[0] == "" || server.Auth != (MCPAuth{}) || len(server.Headers) > 0 {
				return fmt.Errorf(
					"%s: stdio MCP server %s needs a command and cannot set headers or auth",
					at,
					id,
				)
			}
		case "http":
			if err := validateURL(server.URL); err != nil {
				return fmt.Errorf("%s: MCP server %s: %w", at, id, err)
			}
			if len(server.Environment) > 0 || server.Cwd != "" {
				return fmt.Errorf("%s: HTTP MCP server %s cannot set environment or cwd", at, id)
			}
		}
		for name := range server.Environment {
			if !validEnvironment.MatchString(name) {
				return fmt.Errorf("%s: MCP server %s: invalid environment name %q", at, id, name)
			}
		}
		for name := range server.Headers {
			switch strings.ToLower(name) {
			case "authorization", "proxy-authorization", "cookie", "host", "mcp-session-id":
				return fmt.Errorf(
					"%s: MCP server %s: header %s is managed by agx; use auth for bearer credentials",
					at,
					id,
					name,
				)
			}
		}
		if ref := server.Auth.Secret; ref != "" {
			secret, ok := c.Secrets[ref]
			switch {
			case !ok:
				return fmt.Errorf("%s: MCP server %s: unknown secret %s", at, id, ref)
			case secret.Owner != "" && secret.Owner != id:
				return fmt.Errorf(
					"%s: MCP server %s: secret %s is the inline auth of MCP server %s; declare it under secrets: to share it",
					at,
					id,
					ref,
					secret.Owner,
				)
			case secret.Kind == "oauth":
				// A stored OAuth credential is bound to one server origin.
				origin := strings.ToLower(serverOrigin(server.URL))
				if other, seen := oauthOrigins[ref]; seen && other.origin != origin {
					return fmt.Errorf(
						"%s: MCP servers %s and %s share OAuth secret %s but have different origins",
						at, other.id, id, ref,
					)
				}
				oauthOrigins[ref] = struct {
					id     ID
					origin string
				}{id, origin}
			}
		}
		for _, i := range server.Injections(id) {
			if err := checkSecret(i); err != nil {
				return err
			}
		}
	}
	for _, name := range sortedKeys(c.Harnesses) {
		h, err := c.Harness(name)
		if err != nil {
			return err
		}
		for _, i := range h.Injections() {
			if err := checkSecret(i); err != nil {
				return err
			}
		}
	}
	visit := func(Resource, []ID) {}
	for id, b := range c.Bundles {
		if err := c.expand(b.Selection, []ID{id}, visit); err != nil {
			return err
		}
	}
	for _, s := range c.Scopes {
		for _, sel := range []Selection{s.Add, s.Remove} {
			if err := c.expand(sel, nil, visit); err != nil {
				return fmt.Errorf("%s:%d: %w", s.Source.Path, s.Source.Line, err)
			}
		}
	}
	for name, h := range c.Harnesses {
		for _, s := range h.Selections {
			for _, sel := range []Selection{s.Add, s.Remove} {
				if err := c.expand(sel, nil, visit); err != nil {
					return fmt.Errorf("%s: harness %s: %w", locate(s.Source), name, err)
				}
			}
		}
	}
	return nil
}

func serverOrigin(value string) string {
	u, err := url.Parse(value)
	if err != nil {
		return value
	}
	return u.Scheme + "://" + u.Host
}

func validateURL(value string) error {
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" {
		return fmt.Errorf("invalid HTTP URL")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("URL must use http or https")
	}
	return nil
}

func validID(id ID) bool {
	for _, segment := range strings.Split(string(id), "/") {
		if !validIDSegment.MatchString(segment) {
			return false
		}
	}
	return true
}

// gitTransport classifies a Git source like Git does: URLs by scheme, transport::address
// remote helpers, scp-style host:path as ssh, and everything else as a local path.
func gitTransport(source string) string {
	if i := strings.Index(source, "://"); i > 0 {
		return strings.ToLower(source[:i])
	}
	if strings.Contains(source, "::") {
		return "helper"
	}
	colon := strings.IndexByte(source, ':')
	slash := strings.IndexAny(source, `/\`)
	// A single character before the colon is a Windows drive letter.
	if colon > 1 && (slash < 0 || colon < slash) {
		return "ssh"
	}
	return "file"
}

func slashRel(base, target string) string {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return filepath.ToSlash(target)
	}
	return filepath.ToSlash(rel)
}

func qualify(ns string, id ID) ID {
	if ns == "" {
		return id
	}
	return ID(ns + "/" + string(id))
}

func relative(base, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(base, path)
}

// ReadLock reads a lockfile, or its replacement in overlay.
func ReadLock(path string, overlay Overlay) (Lock, error) {
	var lock Lock
	b, err := overlay.readFile(path)
	if err != nil {
		return lock, err
	}
	if err := securefs.CheckFormat(path, b, 1); err != nil {
		return lock, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&lock); err != nil {
		return lock, err
	}
	if lock.Version != 1 || lock.Packages == nil {
		return lock, fmt.Errorf("unsupported lockfile")
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return lock, fmt.Errorf("trailing lockfile data")
	}
	return lock, nil
}

// TreeHash walks as securefs.CopyTree copies, so a source tree and its materialized copy hash
// identically. Executable bits are excluded because Windows checkouts and umasks cannot
// reproduce them.
func TreeHash(src string) (string, error) {
	h := sha256.New()
	err := securefs.Walk(src, func(rel string, info fs.FileInfo, content io.Reader) error {
		if content == nil {
			// Directory sizes vary across filesystems; only file bytes matter.
			_, _ = fmt.Fprintf(h, "%s\x00dir\x000\x00", rel)
			return nil
		}
		_, _ = fmt.Fprintf(h, "%s\x00file\x00%d\x00", rel, info.Size())
		_, err := io.Copy(h, content)
		return err
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

var contentHashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func ValidHash(value string) bool { return contentHashPattern.MatchString(value) }
