package config

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/tarik02-org/agx/internal/platform"
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
const DefaultConfig = "fragments: [agx.d/*.yaml]\n"

// Options configures Load.
type Options struct {
	Builtins Builtins
	State    string
	Local    *Local
	// Cache is the package cache. Without it, Load reads only configuration fragments, without
	// loading packages or validating the result.
	Cache string
	// Inputs, when set, records the files loading read, including after a failure, so callers
	// can skip reloading until one of them changes.
	Inputs *Inputs
}

func read(root string, builtins Builtins, inputs *Inputs, local *Local) (*Config, error) {
	dir, err := RootDir(root)
	if err != nil {
		return nil, err
	}
	root, err = platform.Canonical(root)
	if err != nil {
		return nil, err
	}
	c := &Config{
		Root:          root,
		Local:         local,
		Packages:      map[ID]Package{},
		Skills:        map[ID]Skill{},
		Instructions:  map[ID]Instructions{},
		MCP:           map[ID]MCPServer{},
		Secrets:       map[ID]Secret{},
		Bundles:       map[ID]Bundle{},
		Harnesses:     map[string]HarnessDefinition{},
		PackageHashes: map[ID]string{},
		builtins:      builtins,
	}
	if local != nil && len(local.Document) > 0 {
		d, at, err := local.decode(root, builtins)
		if err != nil {
			return nil, err
		}
		if err := c.merge(d, at, root, "", false); err != nil {
			return nil, err
		}
	}
	inputs.file(root)
	b, err := os.ReadFile(root)
	if errors.Is(err, fs.ErrNotExist) {
		b, err = []byte(DefaultConfig), nil
	}
	if err != nil {
		return nil, err
	}
	r, positions, err := decode[RootFile](root, b, RootSchema, builtins)
	if err != nil {
		return nil, err
	}
	c.Daemon = Daemon{
		LeaseSeconds: cmp.Or(r.Daemon.LeaseSeconds, 60),
		RenewSeconds: cmp.Or(r.Daemon.RenewSeconds, 20),
		Autostart:    r.Daemon.Autostart,
	}
	// A glob matching the root, or two entries matching one fragment, load it once.
	seen := map[string]bool{root: true}
	for i, entry := range r.Fragments {
		at := positions.at("fragments", strconv.Itoa(i))
		p, err := ExpandHome(entry)
		if err != nil {
			return nil, errorAt(at, "fragment %w", err)
		}
		p = relative(dir, p)
		files := []string{p}
		if isGlob(entry) {
			if files, err = globFiles(p); err != nil {
				return nil, errorAt(at, "fragment %s: %w", entry, err)
			}
			inputs.glob(p, files)
		} else if _, err := os.Stat(p); errors.Is(err, fs.ErrNotExist) {
			return nil, errorAt(at, "fragment %s: %s does not exist", entry, p)
		}
		for _, path := range files {
			if path, err = platform.Canonical(path); err != nil {
				return nil, err
			}
			if seen[path] {
				continue
			}
			seen[path] = true
			inputs.file(path)
			b, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			d, positions, err := decode[Document](path, b, FragmentSchema, builtins)
			if err != nil {
				return nil, err
			}
			if err := c.merge(d, positions, path, "", false); err != nil {
				return nil, err
			}
			c.Files = append(c.Files, path)
		}
	}
	c.Files = append(c.Files, root)
	if err := c.validateHarnesses(); err != nil {
		return nil, err
	}
	return c, nil
}

// RootDir is the directory the root's fragments are relative to: the root's own, before resolving
// a link to the root, so a root linked from elsewhere, such as the Nix store, loads the
// fragments beside the link.
func RootDir(root string) (string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	return platform.Canonical(filepath.Dir(abs))
}

// isGlob reports whether a fragment entry is a pattern rather than a literal path.
func isGlob(path string) bool { return strings.ContainsAny(path, "*?[") }

// Load reads the configuration fragments from root and, given a cache, the installed packages
// they declare, and validates the result.
func Load(root string, opts Options) (*Config, error) {
	inputs, cache := cmp.Or(opts.Inputs, &Inputs{}), opts.Cache
	local := opts.Local
	inputs.localState, inputs.localRoot = opts.State, root
	if local == nil {
		var err error
		local, err = ReadLocal(context.Background(), opts.State, root)
		if err != nil {
			inputs.localRevision = "error:" + err.Error()
			return nil, fmt.Errorf("load local configuration: %w", err)
		}
	}
	if opts.State != "" {
		inputs.localState, inputs.localRoot, inputs.localRevision = opts.State, root, fmt.Sprint(local.Revision)
	}
	c, err := read(root, opts.Builtins, inputs, local)
	if err != nil || cache == "" {
		return c, err
	}
	c.Missing = map[ID]MissingPackage{}
	// skip leaves a package out because it is not installed; a launch goes on without it.
	skip := func(id ID, format string) (string, error) {
		reason := fmt.Sprintf(format, id) + "; run agx install"
		if p := c.Packages[id]; p.Anonymous {
			kind, resource, _ := strings.Cut(string(id), ":")
			reason = fmt.Sprintf("%s %s: its Git source is not installed; run agx install", kind, resource)
		}
		c.Missing[id] = MissingPackage{Root: id, Reason: reason}
		return "", nil
	}
	locked := map[ID][]ID{}
	installed := func(id ID, p Package) (string, error) {
		hash := p.Hash
		switch {
		case p.Pinned() && hash == "":
			pin := PinPath(cache, p)
			inputs.file(pin)
			var err error
			if hash, err = ReadPin(pin); errors.Is(err, fs.ErrNotExist) {
				return skip(id, "package %s is not installed")
			} else if err != nil {
				return "", fmt.Errorf("package %s: %w", id, err)
			}
		case !p.Pinned():
			if !p.Local {
				inputs.file(p.Lockfile)
			}
			lock, err := c.PackageLock(p)
			var newer *securefs.NewerFormatError
			switch {
			case errors.As(err, &newer):
				return "", err
			case errors.Is(err, fs.ErrNotExist):
				return skip(id, "package %s is not installed")
			case err != nil:
				return "", &SourceError{Path: p.Lockfile, Err: fmt.Errorf("%w; run agx install", err)}
			}
			entry, ok := lock.Packages[id]
			switch {
			case !ok:
				return skip(id, "package %s is not installed")
			case Digest(entry.Source) != Digest(p.LockSource()):
				return skip(id, "package %s changed since it was installed")
			case !ValidHash(entry.Hash):
				return "", fmt.Errorf("invalid cache hash for %s", id)
			}
			hash, locked[id] = entry.Hash, append([]ID{}, entry.Dependencies...)
			slices.Sort(locked[id])
		}
		dir := filepath.Join(cache, "packages", hash)
		inputs.file(dir)
		if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
			return skip(id, "package %s is not installed")
		} else if err != nil {
			return "", fmt.Errorf("package %s cache: %w", id, err)
		}
		// Installation renames a hashed copy into place, so an entry that exists is complete.
		c.PackageHashes[id] = hash
		return dir, nil
	}
	merged := func(id ID, p Package, deps []ID) error {
		if expected, ok := locked[id]; ok && !p.Anonymous && Digest(deps) != Digest(expected) {
			return fmt.Errorf("stale dependency lock for %s", id)
		}
		return nil
	}
	if err := c.MergePackages(installed, merged); err != nil {
		return nil, err
	}
	// Definitions whose content is a missing Git source are left out, like a missing package's.
	for name, s := range c.Skills {
		if _, ok := c.Missing[s.Package]; ok && s.Package != "" {
			delete(c.Skills, name)
			continue
		}
		for _, hash := range c.PackageHashes {
			rel, err := filepath.Rel(filepath.Join(cache, "packages", hash), s.Path)
			if !s.Generated() && err == nil && filepath.IsLocal(rel) {
				s.installed = Digest([]string{hash, filepath.ToSlash(rel)})
				c.Skills[name] = s
				break
			}
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

// MergePackages merges the declared packages and their dependencies, parents before what they
// declare. locate returns the directory holding a package's content, or "" to leave it out with
// everything it declares; merged receives each merged package with its dependencies.
func (c *Config) MergePackages(
	locate func(ID, Package) (string, error),
	merged func(ID, Package, []ID) error,
) error {
	done := map[ID]bool{}
	var visit func(ID, []ID) error
	visit = func(id ID, chain []ID) error {
		if slices.Contains(chain, id) {
			return fmt.Errorf("package dependency cycle: %v -> %s", chain, id)
		}
		if done[id] {
			return nil
		}
		done[id] = true
		p, ok := c.Packages[id]
		if !ok {
			return fmt.Errorf("unknown package %s", id)
		}
		// What a missing package declares, its dependencies and Git sources, is missing with it.
		if owner, ok := c.missingParent(id, p); ok {
			c.Missing[id] = MissingPackage{Root: owner}
			return nil
		}
		dir, err := locate(id, p)
		if err != nil || dir == "" {
			return err
		}
		deps := []ID{}
		if p.Anonymous {
			err = c.addContent(id, dir)
		} else {
			var d Document
			if d, err = c.AddPackage(id, dir); err == nil {
				deps = c.dependencies(id, d)
			}
		}
		if err != nil {
			return err
		}
		if err := merged(id, p, deps); err != nil {
			return err
		}
		for _, dep := range deps {
			if err := visit(dep, append(chain, id)); err != nil {
				return err
			}
		}
		return nil
	}
	ids := slices.Collect(maps.Keys(c.Packages))
	// Parents merge before what they declare, so a missing parent is known when its declarations
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
		if err := visit(id, nil); err != nil {
			return err
		}
	}
	return nil
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
	manifest := filepath.Join(dir, "agx.package.yaml")
	b, err := os.ReadFile(manifest)
	if !errors.Is(err, fs.ErrNotExist) {
		if err != nil {
			return Document{}, err
		}
		d, positions, err := decode[Document](manifest, b, ManifestSchema, c.builtins)
		if err != nil {
			return d, err
		}
		return d, c.merge(d, positions, manifest, string(id), true)
	}
	d := Document{Skills: map[ID]Skill{}}
	skills, err := FindSkills(dir)
	if err != nil {
		return d, err
	}
	for _, found := range skills {
		if previous, taken := d.Skills[found.ID]; taken {
			return d, fmt.Errorf(
				"package %s: skills %s and %s share the ID %s; add agx.package.yaml to name them",
				dir, previous.Path, found.Path, found.ID,
			)
		}
		d.Skills[found.ID] = Skill{Path: found.Path}
	}
	if err := c.merge(d, positions{"": {Path: manifest, Line: 1, Column: 1}}, manifest, string(id), true); err != nil {
		return d, err
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

// addContent points the resources of an anonymous package at its installed checkout.
func (c *Config) addContent(id ID, dir string) error {
	for name, s := range c.Skills {
		if s.Package != id {
			continue
		}
		path, err := securefs.Contained(dir, filepath.FromSlash(cmp.Or(s.Path, ".")))
		if err != nil {
			return errorAt(s.Source, "skill %s: %w", name, err)
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
			return errorAt(i.Source, "instructions %s: %w", name, err)
		}
		i.Path = path
		c.Instructions[name] = i
	}
	return nil
}

// dependencies lists the packages a manifest declares, including the anonymous Git sources of its
// resources, and moves those locked with it into its lockfile.
func (c *Config) dependencies(id ID, d Document) []ID {
	lockfile := c.Packages[id].Lockfile
	deps := []ID{}
	for dep := range d.Packages {
		qualified := qualify(string(id), dep)
		// Configuration overrides of a dependency keep their own lockfile.
		if value := c.Packages[qualified]; value.Manifest {
			value.Lockfile = lockfile
			value.Local = c.Packages[id].Local
			c.Packages[qualified] = value
		}
		deps = append(deps, qualified)
	}
	for name, p := range c.Packages {
		if p.Anonymous && p.Parent == id {
			p.Lockfile = lockfile
			p.Local = c.Packages[id].Local
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
	return securefs.WriteCacheJSON(path, pin{Version: pinVersion, Hash: hash})
}

// merger merges one file: a configuration fragment, or a package manifest under its namespace.
type merger struct {
	c        *Config
	at       positions
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

func (c *Config) merge(d Document, at positions, path, ns string, manifest bool) error {
	m := &merger{c: c, at: at, path: path, base: filepath.Dir(path), ns: ns, manifest: manifest}
	if c.rootDefinitions == nil {
		c.rootDefinitions = map[string]bool{}
	}
	for _, id := range sortedKeys(d.Packages) {
		p := d.Packages[id]
		name := qualify(ns, id)
		at := m.at.at(string(KindPackages), string(id))
		existing, exists := c.Packages[name]
		if apply, err := m.conflict(
			KindPackages,
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
		if err := m.addSkill(id, d.Skills[id], m.at.at(string(KindSkills), string(id)), false); err != nil {
			return err
		}
	}
	for _, id := range sortedKeys(d.Instructions) {
		if err := m.addInstructions(
			id,
			d.Instructions[id],
			m.at.at(string(KindInstructions), string(id)),
			false,
		); err != nil {
			return err
		}
	}
	for _, id := range sortedKeys(d.Secrets) {
		secret := d.Secrets[id]
		name := qualify(ns, id)
		at := m.at.at(string(KindSecrets), string(id))
		existing, exists := c.Secrets[name]
		if existing.Owner != "" {
			return errorAt(
				at, "secret %s conflicts with the auth of MCP server %s, which uses that ID; rename one",
				name,
				existing.Owner,
			)
		}
		if apply, err := m.conflict(
			KindSecrets,
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
			return errorAt(at, "secret %s: %w", name, err)
		}
		c.Secrets[name] = secret
	}
	for _, id := range sortedKeys(d.MCP) {
		if err := m.addMCP(id, d.MCP[id], m.at.at(string(KindMCPServers), string(id)), false); err != nil {
			return err
		}
	}
	for _, id := range sortedKeys(d.Bundles) {
		b := d.Bundles[id]
		name := qualify(ns, id)
		at := m.at.at(string(KindBundles), string(id))
		existing, exists := c.Bundles[name]
		if apply, err := m.conflict(
			KindBundles,
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
		at := m.at.at("scopes", strconv.Itoa(i))
		directory := ""
		if s.Directory != "" {
			var err error
			if directory, err = ScopeDirectory(path, s.Directory); err != nil {
				return errorAt(at, "scope directory %w", err)
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
	return m.harnesses(d)
}

// ScopeDirectory canonicalizes a scope directory as written in file.
func ScopeDirectory(file, written string) (string, error) {
	directory, err := ExpandHome(written)
	if err != nil {
		return "", err
	}
	return platform.Canonical(relative(filepath.Dir(file), directory))
}

// conflict decides whether a definition applies. Configuration fragments merge before package
// manifests, so precedence is decided by origin rather than order: a configuration override wins
// over package definitions, and packages can never replace configuration definitions. Inline
// definitions are never replaced, so a reference never silently changes meaning.
func (m *merger) conflict(kind Kind, id ID, p prior, override, inline bool, at Source) (bool, error) {
	label := kind.Label()
	if p.exists && (p.inline || inline) {
		return false, errorAt(
			at, "%s %s is also defined at %s; define it once, under top-level %s:, and reference its ID elsewhere",
			label, id, locate(p.source), kind,
		)
	}
	key := string(kind) + "\x00" + string(id)
	rootOverride, fromRoot := m.c.rootDefinitions[key]
	if m.manifest && fromRoot {
		if rootOverride {
			return false, nil
		}
		return false, errorAt(
			at,
			"package definition %s conflicts with configuration at %s; set override: true on the configuration definition",
			id,
			locate(p.source),
		)
	}
	if p.exists && p.source.Local && !at.Local && !m.manifest {
		override = true
	}
	if p.exists && !override {
		return false, errorAt(
			at, "conflicting definition of %s %s, also defined at %s; set override: true",
			label, id, locate(p.source),
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
	v.Source.Local = m.at.at().Local
	if v.Source.Local {
		v.Source.Line, v.Source.Column = 0, 0
	}
	if v.Secret != "" {
		v.Secret = qualify(m.ns, v.Secret)
	}
	if v.File != "" {
		file, err := m.file(v.File)
		if err != nil {
			return v, errorAt(v.Source, "!file %s: %w", v.File, err)
		}
		v.File = file
	}
	return v, nil
}

func (m *merger) placePackage(name ID, p *Package, at Source) error {
	p.Source = at
	p.Manifest = m.manifest
	// The lockfile beside a fragment records the unpinned packages it declares.
	p.Lockfile = filepath.Join(m.base, "agx.lock.json")
	p.Local = at.Local
	if p.Local {
		p.Lockfile = ""
	}
	if p.Path != "" {
		var err error
		p.Path, err = m.file(p.Path)
		if err != nil {
			return errorAt(at, "package %s: %w", name, err)
		}
		p.LockPath = slashRel(m.base, p.Path)
	}
	if filepath.IsAbs(p.Subdir) || strings.HasPrefix(filepath.Clean(p.Subdir), "..") {
		return errorAt(at, "invalid subdir for %s", name)
	}
	if p.Git != "" {
		if strings.HasPrefix(p.Git, "-") {
			return errorAt(at, "invalid git source for %s", name)
		}
		transport := gitTransport(p.Git)
		if m.manifest && transport != "https" && transport != "ssh" {
			return errorAt(at, "package %s must use an https, ssh, or scp-style Git source", name)
		}
		if transport == "file" && !strings.HasPrefix(strings.ToLower(p.Git), "file:") {
			git, err := ExpandHome(p.Git)
			if err != nil {
				return errorAt(at, "package %s: %w", name, err)
			}
			p.Git = relative(m.base, git)
			p.LockPath = slashRel(m.base, p.Git)
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
		return errorAt(at, "%s %s: path %s must stay inside the Git checkout", kind, id, path)
	}
	return nil
}

func (m *merger) addSkill(id ID, s Skill, at Source, inline bool) error {
	name := qualify(m.ns, id)
	existing, exists := m.c.Skills[name]
	if apply, err := m.conflict(
		KindSkills,
		name,
		prior{exists, existing.Inline, existing.Source},
		s.Override,
		inline,
		at,
	); err != nil ||
		!apply {
		return err
	}
	if s.Generated() && s.Name == "" {
		s.Name = string(name)[strings.LastIndex(string(name), "/")+1:]
		if !skillNamePattern.MatchString(s.Name) || len(s.Name) > maxSkillName {
			return errorAt(
				at,
				"skill %s: name %q must be lowercase letters, digits, and single hyphens, at most %d characters; set name",
				name,
				s.Name,
				maxSkillName,
			)
		}
	}
	for key, f := range s.Files {
		f.Directory = strings.HasSuffix(f.Path, "/") || strings.HasSuffix(f.Path, `\`)
		s.Files[key] = f
	}
	if err := checkSkillFiles(name, s.Files); err != nil {
		return errorAt(at, "%w", err)
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
				return errorAt(at, "skill %s: files %s: %w", name, key, err)
			}
			s.Files[key] = f
		}
	default:
		if s.Path, err = m.file(s.Path); err != nil {
			err = errorAt(at, "skill %s: %w", name, err)
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
		KindInstructions,
		name,
		prior{exists, existing.Inline, existing.Source},
		i.Override,
		inline,
		at,
	); err != nil ||
		!apply {
		return err
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
			err = errorAt(at, "instructions %s: %w", name, err)
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
		KindMCPServers,
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
	transport := "stdio"
	if server.URL != "" {
		transport = "http"
	}
	server.Transport = transport
	server.Load = cmp.Or(server.Load, LoadNative)
	if server.Share.Type == ShareNone {
		if server.Share.Idle != nil {
			return errorAt(at, "MCP server %s: share idle needs a share type other than none", name)
		}
		server.Share.Type = ""
	}
	if server.Cwd != "" {
		cwd, err := m.expand(server.Cwd)
		if err != nil {
			return errorAt(at, "MCP server %s: cwd %w", name, err)
		}
		server.Cwd = relative(m.base, cwd)
	}
	if len(server.Command) > 0 && strings.HasPrefix(server.Command[0], "~") {
		command := slices.Clone(server.Command)
		var err error
		if command[0], err = m.expand(command[0]); err != nil {
			return errorAt(at, "MCP server %s: command %w", name, err)
		}
		server.Command = command
	}
	// A replaced definition takes its inline secret with it.
	if previous, ok := c.Secrets[name]; ok && previous.Owner == name {
		delete(c.Secrets, name)
	}
	if server.Auth.Tagged() {
		placed, err := m.place(server.Auth.Value)
		if err != nil {
			return err
		}
		server.Auth.Value = placed
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
			return errorAt(
				at, "MCP server %s %s, but secret %s is defined too; set auth: !secret %s or auth: none, or rename one",
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
			return errorAt(at, "MCP server %s auth: %w", name, err)
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
		// Apply the same symlink rules as hashing and copying, so a valid skill can always be
		// launched. Installation applied them to package content already.
		if r.installed == "" {
			if err := r.tree(func(string, int64, fs.FileMode, io.Reader) error { return nil }); err != nil {
				return errorAt(r.Source, "skill %s: %w", id, err)
			}
		}
		if !r.Generated() {
			if _, err := os.ReadFile(filepath.Join(r.Path, "SKILL.md")); err != nil {
				return errorAt(r.Source, "skill %s: %w", id, err)
			}
		}
	}
	for _, id := range sortedKeys(c.Instructions) {
		r := c.Instructions[id]
		if _, err := r.Read(); err != nil {
			return errorAt(r.Source, "instructions %s: %w", id, err)
		}
	}
	for id, secret := range c.Secrets {
		label := "secret " + string(id)
		if secret.Owner != "" {
			label = "MCP server " + string(secret.Owner) + " auth"
		}
		at := secret.Source
		// The schema checks the fields of each kind.
		switch secret.Kind {
		case "sops":
			// The encrypted file belongs to the configuration, so a missing one is an error
			// rather than a runtime state: committing it is what makes the configuration portable.
			info, err := os.Stat(secret.File)
			if err != nil {
				return errorAt(at, "%s: cannot read sops file: %w", label, err)
			}
			if !info.Mode().IsRegular() {
				return errorAt(at, "%s: sops file %s is not a regular file", label, secret.File)
			}
		case "oauth":
			if secret.Issuer != "" {
				if err := validateURL(secret.Issuer); err != nil {
					return errorAt(at, "%s: %w", label, err)
				}
			}
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
			return errorAt(i.Source, "%s: unknown secret %s", i.Target, i.Value.Secret)
		}
		if secret.Owner != "" {
			return errorAt(
				i.Source, "%s: secret %s is the inline auth of MCP server %s; declare it under secrets: to share it",
				i.Target,
				i.Value.Secret,
				secret.Owner,
			)
		}
		if secret.Kind == "oauth" {
			return errorAt(
				i.Source, "%s: OAuth secret %s can only be an MCP server's auth",
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
				return errorAt(
					secret.AgeKey.Source, "%s: age_key forms a cycle: %s",
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
		at := server.Source
		if server.Transport == "http" {
			if err := validateURL(server.URL); err != nil {
				return errorAt(at, "MCP server %s: %w", id, err)
			}
		}
		for name := range server.Headers {
			switch strings.ToLower(name) {
			case "authorization", "proxy-authorization", "cookie", "host", "mcp-session-id":
				return errorAt(
					at, "MCP server %s: header %s is managed by agx; use auth for bearer credentials",
					id,
					name,
				)
			}
		}
		if ref := server.Auth.Secret; ref != "" {
			secret, ok := c.Secrets[ref]
			switch {
			case !ok:
				return errorAt(at, "MCP server %s: unknown secret %s", id, ref)
			case secret.Owner != "" && secret.Owner != id:
				return errorAt(
					at,
					"MCP server %s: secret %s is the inline auth of MCP server %s; declare it under secrets: to share it",
					id,
					ref,
					secret.Owner,
				)
			case secret.Kind == "oauth":
				// A stored OAuth credential is bound to one server origin.
				origin := strings.ToLower(serverOrigin(server.URL))
				if other, seen := oauthOrigins[ref]; seen && other.origin != origin {
					return errorAt(
						at, "MCP servers %s and %s share OAuth secret %s but have different origins",
						other.id, id, ref,
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
				return errorAt(s.Source, "%w", err)
			}
		}
	}
	for name, h := range c.Harnesses {
		for _, s := range h.Selections {
			for _, sel := range []Selection{s.Add, s.Remove} {
				if err := c.expand(sel, nil, visit); err != nil {
					return errorAt(s.Source, "harness %s: %w", name, err)
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

// ReadLock reads a portable package lockfile.
func ReadLock(path string) (Lock, error) {
	var lock Lock
	b, err := os.ReadFile(path)
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
	h := newTreeHash()
	err := securefs.Walk(src, func(rel string, info fs.FileInfo, content io.Reader) error {
		return h.add(rel, info.Size(), content)
	})
	if err != nil {
		return "", err
	}
	return h.sum(), nil
}

type treeHash struct{ hash.Hash }

func newTreeHash() treeHash { return treeHash{sha256.New()} }

// add hashes one entry of a walk; content is nil for a directory.
func (h treeHash) add(rel string, size int64, content io.Reader) error {
	if content == nil {
		// Directory sizes vary across filesystems; only file bytes matter.
		_, _ = fmt.Fprintf(h, "%s\x00dir\x000\x00", rel)
		return nil
	}
	_, _ = fmt.Fprintf(h, "%s\x00file\x00%d\x00", rel, size)
	_, err := io.Copy(h, content)
	return err
}

func (h treeHash) sum() string { return hex.EncodeToString(h.Sum(nil)) }

// VerifyPackages hashes the installed content of every loaded package again. Loading trusts the
// cache, whose entries installation creates from content it hashed.
func (c *Config) VerifyPackages(cache string) error {
	for _, id := range sortedKeys(c.PackageHashes) {
		hash := c.PackageHashes[id]
		dir := filepath.Join(cache, "packages", hash)
		cached, err := TreeHash(dir)
		if err != nil {
			return fmt.Errorf("package %s cache: %w", id, err)
		}
		if cached != hash {
			return fmt.Errorf("package %s: cache entry %s does not match its content hash", id, dir)
		}
	}
	return nil
}

var contentHashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func ValidHash(value string) bool { return contentHashPattern.MatchString(value) }
