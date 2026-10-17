package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"go.yaml.in/yaml/v4"

	"github.com/tarik02/weft/internal/securefs"
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
const DefaultConfig = "version: 1\nimports:\n  - glob: weft.d/*.yaml\n"

func Read(root string) (*Config, error) {
	return read(root, &Inputs{})
}

func read(root string, inputs *Inputs) (*Config, error) {
	root, err := Canonical(root)
	if err != nil {
		return nil, err
	}
	c := &Config{
		Root:          root,
		Packages:      map[ID]Package{},
		Skills:        map[ID]FileResource{},
		Instructions:  map[ID]FileResource{},
		Connections:   map[ID]Connection{},
		Auth:          map[ID]Auth{},
		Bundles:       map[ID]Bundle{},
		Harnesses:     map[string]Harness{},
		PackageHashes: map[ID]string{},
		Daemon:        Daemon{LeaseSeconds: 60, RenewSeconds: 20},
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
		d, node, err := decode(path)
		if path == root && errors.Is(err, fs.ErrNotExist) {
			d, node, err = decodeBytes(path, []byte(DefaultConfig))
		}
		if err != nil {
			return err
		}
		for _, imp := range d.Imports {
			if (imp.Path == "") == (imp.Glob == "") {
				return fmt.Errorf("%s: import needs exactly one of path or glob", path)
			}
			if imp.Glob != "" {
				if imp.Optional {
					return fmt.Errorf(
						"%s: import %s: optional applies only to path imports; globs may match nothing",
						path,
						imp.Glob,
					)
				}
				pattern := relative(filepath.Dir(path), imp.Glob)
				files, err := globFiles(pattern)
				if err != nil {
					return fmt.Errorf("%s: import %s: %w", path, imp.Glob, err)
				}
				inputs.glob(pattern, files)
				for _, match := range files {
					if err := load(match); err != nil {
						return err
					}
				}
				continue
			}
			p := relative(filepath.Dir(path), imp.Path)
			if imp.Optional {
				inputs.file(p)
				if _, err := os.Stat(p); errors.Is(err, fs.ErrNotExist) {
					continue
				} else if err != nil {
					return fmt.Errorf("%s: import %s: %w", path, imp.Path, err)
				}
			}
			if err := load(p); err != nil {
				return err
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
	return c, nil
}

func Load(root, cache string) (*Config, error) {
	c, _, err := LoadTracked(root, cache)
	return c, err
}

// LoadTracked also returns the files it read, including after a failure, so callers can skip
// reloading until one of them changes.
func LoadTracked(root, cache string) (*Config, *Inputs, error) {
	inputs := &Inputs{}
	c, err := loadConfig(root, cache, inputs)
	return c, inputs, err
}

func loadConfig(root, cache string, inputs *Inputs) (*Config, error) {
	c, err := read(root, inputs)
	if err != nil {
		return nil, err
	}
	loaded := map[ID]bool{}
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
		inputs.file(p.Lockfile)
		lock, err := ReadLock(p.Lockfile)
		if err != nil {
			return fmt.Errorf("%s: %w; run weft install", p.Lockfile, err)
		}
		entry, ok := lock.Packages[id]
		if !ok || Digest(entry.Source) != Digest(p.LockSource()) {
			return fmt.Errorf("package %s has a missing or stale lock; run weft install", id)
		}
		if !ValidHash(entry.Hash) {
			return fmt.Errorf("invalid cache hash for %s", id)
		}
		dir := filepath.Join(cache, "packages", entry.Hash)
		inputs.file(dir)
		if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("package %s is not installed; run weft install", id)
		} else if err != nil {
			return fmt.Errorf("package %s cache: %w", id, err)
		}
		hash, err := TreeHash(dir)
		if err != nil {
			return fmt.Errorf("package %s cache: %w", id, err)
		}
		if hash != entry.Hash {
			return fmt.Errorf("package %s cache content does not match its lock", id)
		}
		d, node, err := decode(filepath.Join(dir, "weft.package.yaml"))
		if err != nil {
			return err
		}
		if err := c.merge(d, node, filepath.Join(dir, "weft.package.yaml"), string(id), true); err != nil {
			return err
		}
		c.PackageHashes[id] = entry.Hash
		deps := []ID{}
		for dep := range d.Packages {
			qualified := qualify(string(id), dep)
			deps = append(deps, qualified)
			// Configuration overrides of a dependency keep their own lockfile.
			if value := c.Packages[qualified]; value.Manifest {
				value.Lockfile = p.Lockfile
				c.Packages[qualified] = value
			}
		}
		sort.Slice(deps, func(i, j int) bool { return deps[i] < deps[j] })
		expected := append([]ID{}, entry.Dependencies...)
		sort.Slice(expected, func(i, j int) bool { return expected[i] < expected[j] })
		if Digest(deps) != Digest(expected) {
			return fmt.Errorf("stale dependency lock for %s", id)
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
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		if err := load(id, nil); err != nil {
			return nil, err
		}
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

func DecodeManifest(path string) (Document, *yaml.Node, error) { return decode(path) }
func (c *Config) AddManifest(d Document, node *yaml.Node, path string, namespace ID) error {
	return c.merge(d, node, path, string(namespace), true)
}

func decode(path string) (Document, *yaml.Node, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Document{}, nil, err
	}
	return decodeBytes(path, b)
}

func decodeBytes(path string, b []byte) (Document, *yaml.Node, error) {
	var d Document
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&d); errors.Is(err, io.EOF) {
		return d, nil, fmt.Errorf("%s: file is empty; expected a document with version: 1", path)
	} else if err != nil {
		return d, nil, fmt.Errorf("%s: %w", path, err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return d, nil, fmt.Errorf("%s: expected one YAML document", path)
	}
	if d.Version != 1 {
		return d, nil, fmt.Errorf("%s: version must be 1", path)
	}
	var n yaml.Node
	if err := yaml.Unmarshal(b, &n); err != nil {
		return d, nil, err
	}
	return d, &n, nil
}

func (c *Config) merge(d Document, node *yaml.Node, path, ns string, manifest bool) error {
	base := filepath.Dir(path)
	if manifest &&
		(len(d.Imports) > 0 || len(d.Scopes) > 0 || len(d.Harnesses) > 0 || d.Lockfile != "" || d.Daemon != (Daemon{})) {
		return fmt.Errorf("%s: package manifests may only declare resources, bundles, auth, and dependencies", path)
	}
	source := func(section, key string) Source {
		out := Source{Path: path, Line: 1, Column: 1}
		if len(node.Content) == 0 {
			return out
		}
		root := node.Content[0]
		for i := 0; i+1 < len(root.Content); i += 2 {
			if root.Content[i].Value != section {
				continue
			}
			n := root.Content[i+1]
			for j := 0; j+1 < len(n.Content); j += 2 {
				if n.Content[j].Value == key {
					return Source{path, n.Content[j].Line, n.Content[j].Column}
				}
			}
		}
		return out
	}
	if c.rootDefinitions == nil {
		c.rootDefinitions = map[string]bool{}
	}
	// Configuration fragments merge before package manifests, so precedence is decided by
	// origin rather than order: a configuration override wins over package definitions, and
	// packages can never replace configuration definitions.
	conflict := func(kind string, id ID, exists, override bool) (bool, error) {
		if !validID(id) {
			return false, fmt.Errorf("%s: invalid ID %q", path, id)
		}
		key := kind + "\x00" + string(id)
		rootOverride, fromRoot := c.rootDefinitions[key]
		if manifest && fromRoot {
			if rootOverride {
				return false, nil
			}
			return false, fmt.Errorf(
				"%s: package definition %s conflicts with configuration; set override: true on the configuration definition",
				path,
				id,
			)
		}
		if exists && !override {
			return false, fmt.Errorf("%s: conflicting definition %s; set override: true", path, id)
		}
		if !manifest {
			c.rootDefinitions[key] = override
		}
		return true, nil
	}
	for id, p := range d.Packages {
		name := qualify(ns, id)
		_, exists := c.Packages[name]
		if apply, err := conflict("packages", name, exists, p.Override); err != nil {
			return err
		} else if !apply {
			continue
		}
		if (p.Path == "") == (p.Git == "") {
			return fmt.Errorf("package %s needs exactly one of path or git", name)
		}
		p.Manifest = manifest
		p.Lockfile = relative(base, d.Lockfile)
		if d.Lockfile == "" {
			p.Lockfile = filepath.Join(base, "weft.lock.json")
		}
		if p.Path != "" {
			if manifest {
				packageRoot, err := filepath.EvalSymlinks(base)
				if err != nil {
					return err
				}
				p.Path, err = securefs.Contained(base, p.Path)
				if err != nil {
					return err
				}
				p.LockPath = slashRel(packageRoot, p.Path)
			} else {
				p.Path = relative(base, p.Path)
				p.LockPath = slashRel(filepath.Dir(p.Lockfile), p.Path)
			}
		}
		if filepath.IsAbs(p.Subdir) || strings.HasPrefix(filepath.Clean(p.Subdir), "..") {
			return fmt.Errorf("invalid subdir for %s", name)
		}
		if p.Git != "" {
			if strings.HasPrefix(p.Git, "-") {
				return fmt.Errorf("invalid git source for %s", name)
			}
			transport := gitTransport(p.Git)
			if manifest && transport != "https" && transport != "ssh" {
				return fmt.Errorf("%s: package %s must use an https, ssh, or scp-style Git source", path, name)
			}
			if transport == "file" && !strings.HasPrefix(strings.ToLower(p.Git), "file:") {
				p.Git = relative(base, p.Git)
				p.LockPath = slashRel(filepath.Dir(p.Lockfile), p.Git)
			}
		}
		p.Source = source("packages", string(id))
		c.Packages[name] = p
	}
	for _, section := range []struct {
		name string
		from map[ID]FileResource
		to   map[ID]FileResource
	}{{"skills", d.Skills, c.Skills}, {"instructions", d.Instructions, c.Instructions}} {
		for id, r := range section.from {
			name := qualify(ns, id)
			_, exists := section.to[name]
			if apply, err := conflict(section.name, name, exists, r.Override); err != nil {
				return err
			} else if !apply {
				continue
			}
			if r.Path == "" {
				return fmt.Errorf("%s %s needs a path", section.name, name)
			}
			var err error
			if manifest {
				r.Path, err = securefs.Contained(base, r.Path)
			} else {
				r.Path = relative(base, r.Path)
			}
			if err != nil {
				return err
			}
			r.Source = source(section.name, string(id))
			section.to[name] = r
		}
	}
	for id, a := range d.Auth {
		name := qualify(ns, id)
		_, exists := c.Auth[name]
		if apply, err := conflict("auth", name, exists, a.Override); err != nil {
			return err
		} else if !apply {
			continue
		}
		a.Source = source("auth", string(id))
		c.Auth[name] = a
	}
	for id, conn := range d.Connections {
		name := qualify(ns, id)
		_, exists := c.Connections[name]
		if apply, err := conflict("connections", name, exists, conn.Override); err != nil {
			return err
		} else if !apply {
			continue
		}
		if conn.Cwd != "" {
			conn.Cwd = relative(base, conn.Cwd)
		}
		if conn.Auth != "" {
			conn.Auth = qualify(ns, conn.Auth)
		}
		for key, id := range conn.AuthEnvironment {
			conn.AuthEnvironment[key] = qualify(ns, id)
		}
		conn.Source = source("connections", string(id))
		c.Connections[name] = conn
	}
	qualifySelection := func(s Selection) Selection {
		for _, ids := range [][]ID{s.Skills, s.Instructions, s.Connections, s.Bundles} {
			for i, id := range ids {
				ids[i] = qualify(ns, id)
			}
		}
		return s
	}
	for id, b := range d.Bundles {
		name := qualify(ns, id)
		_, exists := c.Bundles[name]
		if apply, err := conflict("bundles", name, exists, b.Override); err != nil {
			return err
		} else if !apply {
			continue
		}
		b.Selection = qualifySelection(b.Selection)
		b.Source = source("bundles", string(id))
		c.Bundles[name] = b
	}
	for i, s := range d.Scopes {
		if s.Directory == "" {
			return fmt.Errorf("%s: scope directory is required", path)
		}
		var err error
		s.Directory, err = Canonical(relative(base, s.Directory))
		if err != nil {
			return err
		}
		s.Source = Source{Path: path, Line: 1, Column: 1}
		if len(node.Content) > 0 {
			n := node.Content[0]
			for j := 0; j+1 < len(n.Content); j += 2 {
				if n.Content[j].Value == "scopes" && i < len(n.Content[j+1].Content) {
					pos := n.Content[j+1].Content[i]
					s.Source.Line = pos.Line
					s.Source.Column = pos.Column
				}
			}
		}
		c.Scopes = append(c.Scopes, s)
	}
	for name, h := range d.Harnesses {
		if name != "codex" && name != "claude" && name != "pi" {
			return fmt.Errorf("unknown harness %q", name)
		}
		if h.Executable != "" && !filepath.IsAbs(h.Executable) {
			return fmt.Errorf("harness executable must be absolute: %s", name)
		}
		if _, exists := c.Harnesses[name]; exists && !h.Override {
			return fmt.Errorf("%s: conflicting harness %s; set override: true", path, name)
		}
		c.Harnesses[name] = h
	}
	if c.daemonSet == nil {
		c.daemonSet = map[string]bool{}
	}
	// Daemon settings merge field by field; only fields set again need an override.
	for _, field := range []struct {
		name   string
		value  int
		target *int
	}{
		{"lease_seconds", d.Daemon.LeaseSeconds, &c.Daemon.LeaseSeconds},
		{"renew_seconds", d.Daemon.RenewSeconds, &c.Daemon.RenewSeconds},
	} {
		if field.value == 0 {
			continue
		}
		if c.daemonSet[field.name] && !d.Daemon.Override {
			return fmt.Errorf("%s: conflicting daemon.%s; set daemon.override: true", path, field.name)
		}
		*field.target = field.value
		c.daemonSet[field.name] = true
	}
	return nil
}

func (c *Config) Validate() error {
	if c.Daemon.LeaseSeconds < 3 || c.Daemon.RenewSeconds < 1 || c.Daemon.RenewSeconds >= c.Daemon.LeaseSeconds {
		return fmt.Errorf("daemon renewal must be positive and shorter than lease (minimum 3 seconds)")
	}
	for id, r := range c.Skills {
		// Apply the same symlink rules as hashing and copying, so a valid skill can always be launched.
		if _, err := TreeHash(r.Path); err != nil {
			return fmt.Errorf("skill %s: %w", id, err)
		}
		if _, err := os.ReadFile(filepath.Join(r.Path, "SKILL.md")); err != nil {
			return fmt.Errorf("skill %s: %w", id, err)
		}
	}
	for id, r := range c.Instructions {
		if _, err := os.ReadFile(r.Path); err != nil {
			return fmt.Errorf("instructions %s: %w", id, err)
		}
	}
	for id, a := range c.Auth {
		switch a.Kind {
		case "environment", "daemon_environment":
			if !validEnvironment.MatchString(a.Variable) {
				return fmt.Errorf("auth %s needs a valid variable", id)
			}
		case "command":
			if len(a.Command) == 0 || a.Command[0] == "" {
				return fmt.Errorf("auth %s needs a command", id)
			}
		case "keychain":
		case "oauth":
			if a.Issuer != "" {
				if err := validateURL(a.Issuer); err != nil {
					return fmt.Errorf("auth %s: %w", id, err)
				}
			}
		default:
			return fmt.Errorf("auth %s: unknown kind %q", id, a.Kind)
		}
	}
	for id, conn := range c.Connections {
		switch conn.Transport {
		case "stdio":
			if len(conn.Command) == 0 || conn.Command[0] == "" || conn.URL != "" || conn.Auth != "" ||
				len(conn.Headers) > 0 {
				return fmt.Errorf(
					"stdio connection %s needs command and auth_environment; no URL/headers/bearer auth",
					id,
				)
			}
		case "http":
			if err := validateURL(conn.URL); err != nil {
				return fmt.Errorf("connection %s: %w", id, err)
			}
			if len(conn.Command) > 0 || len(conn.Environment) > 0 || len(conn.AuthEnvironment) > 0 {
				return fmt.Errorf("HTTP connection %s cannot have command/environment", id)
			}
		default:
			return fmt.Errorf("connection %s: transport must be stdio or http", id)
		}
		for name := range conn.Environment {
			if !validEnvironment.MatchString(name) {
				return fmt.Errorf("invalid environment name for %s", id)
			}
		}
		ids := []ID{}
		if conn.Auth != "" {
			ids = append(ids, conn.Auth)
		}
		for name, auth := range conn.AuthEnvironment {

			if source, ok := c.Auth[auth]; ok && source.Kind == "oauth" {
				return fmt.Errorf(
					"connection %s: OAuth sources require HTTP auth; use an environment, keychain or helper source for stdio credentials",
					id,
				)
			}
			if !validEnvironment.MatchString(name) {
				return fmt.Errorf("invalid auth environment name for %s", id)
			}
			if _, ok := conn.Environment[name]; ok {
				return fmt.Errorf("duplicate environment name for %s", id)
			}
			ids = append(ids, auth)
		}
		for _, auth := range ids {
			if _, ok := c.Auth[auth]; !ok {
				return fmt.Errorf("connection %s: unknown auth %s", id, auth)
			}
		}
		for name := range conn.Headers {
			switch strings.ToLower(name) {
			case "authorization", "proxy-authorization", "cookie", "host", "mcp-session-id":
				return fmt.Errorf("connection %s: header %s must not be configured as a literal", id, name)
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
		for _, sel := range []Selection{s.Add, s.Exclude} {
			if err := c.expand(sel, nil, visit); err != nil {
				return fmt.Errorf("%s:%d: %w", s.Source.Path, s.Source.Line, err)
			}
		}
	}
	return nil
}

func validateURL(value string) error {
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" {
		return fmt.Errorf("invalid HTTP URL")
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1") {
		return nil
	}
	return fmt.Errorf("URL must use HTTPS (HTTP permitted only for loopback)")
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

func ReadLock(path string) (Lock, error) {
	var lock Lock
	b, err := os.ReadFile(path)
	if err != nil {
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

// TreeHash follows the rules of securefs.CopyTree, so a source tree and its materialized
// copy hash identically: internal symlinks are followed, escaping or cyclic ones rejected.
// Executable bits are excluded because Windows checkouts and umasks cannot reproduce them.
func TreeHash(src string) (string, error) {
	root, err := filepath.EvalSymlinks(src)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	active := map[string]bool{}
	var walk func(from, rel string) error
	walk = func(from, rel string) error {
		real, err := filepath.EvalSymlinks(from)
		if err != nil {
			return err
		}
		if !securefs.Within(root, real) {
			return fmt.Errorf("symlink escapes package: %s", from)
		}
		info, err := os.Stat(real)
		if err != nil {
			return err
		}
		if info.IsDir() {
			if active[real] {
				return fmt.Errorf("symlink cycle at %s", from)
			}
			active[real] = true
			defer delete(active, real)
			// Directory sizes vary across filesystems; only file bytes matter.
			_, _ = fmt.Fprintf(h, "%s\x00dir\x000\x00", rel)
			entries, err := os.ReadDir(real)
			if err != nil {
				return err
			}
			for _, e := range entries {
				if e.Name() == ".git" {
					continue
				}
				if err := walk(filepath.Join(real, e.Name()), path.Join(rel, e.Name())); err != nil {
					return err
				}
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported file %s", from)
		}
		_, _ = fmt.Fprintf(h, "%s\x00file\x00%d\x00", rel, info.Size())
		f, err := os.Open(real)
		if err != nil {
			return err
		}
		_, err = io.Copy(h, f)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		return closeErr
	}
	if err := walk(root, "."); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

var contentHashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func ValidHash(value string) bool { return contentHashPattern.MatchString(value) }
