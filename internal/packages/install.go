package packages

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gofrs/flock"

	"github.com/tarik02/agx/internal/config"
	"github.com/tarik02/agx/internal/securefs"
)

type Result struct {
	PackageIDs []config.ID `json:"package_ids"`
	Lockfiles  []string    `json:"lockfiles"`
}

func Install(
	ctx context.Context,
	root, cache string,
	frozen bool,
	updates []config.ID,
	updateAll bool,
) (Result, error) {
	result := Result{PackageIDs: []config.ID{}, Lockfiles: []string{}}
	if err := os.MkdirAll(filepath.Join(cache, "packages"), 0o700); err != nil {
		return result, err
	}
	lock := flock.New(filepath.Join(cache, "install.lock"))
	ok, err := lock.TryLockContext(ctx, 100000000)
	if err != nil {
		return result, err
	}
	if !ok {
		return result, fmt.Errorf("package installer is busy")
	}
	defer func() { _ = lock.Unlock() }()
	c, err := config.Read(root)
	if err != nil {
		return result, err
	}
	wanted := map[config.ID]bool{}
	for _, id := range updates {
		wanted[id] = true
	}
	locks := map[string]config.Lock{}
	loaded := map[config.ID]bool{}
	var install func(config.ID, []config.ID) error
	install = func(id config.ID, chain []config.ID) error {
		for _, parent := range chain {
			if parent == id {
				return fmt.Errorf("package dependency cycle: %v -> %s", chain, id)
			}
		}
		if loaded[id] {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		p, ok := c.Packages[id]
		if !ok {
			return fmt.Errorf("unknown package %s", id)
		}
		l, ok := locks[p.Lockfile]
		if !ok {
			l, err = config.ReadLock(p.Lockfile)
			if err != nil {
				if !os.IsNotExist(err) {
					return err
				}
				if frozen {
					return fmt.Errorf("missing lockfile %s", p.Lockfile)
				}
				l = config.Lock{Version: 1, Packages: map[config.ID]config.LockedPackage{}}
			}
			locks[p.Lockfile] = l
		}
		previous, locked := l.Packages[id]
		source := p.LockSource()
		sourceMatches := locked && config.Digest(previous.Source) == config.Digest(source)
		if frozen && !sourceMatches {
			return fmt.Errorf("stale lock for %s", id)
		}
		hash, revision, dest := "", "", ""
		if p.Git != "" && sourceMatches && !updateAll && !wanted[id] {
			candidate := filepath.Join(cache, "packages", previous.Hash)
			if config.ValidHash(previous.Hash) {
				if cached, err := config.TreeHash(candidate); err == nil && cached == previous.Hash {
					hash = previous.Hash
					revision = previous.Revision
					dest = candidate
				}
			}
		}
		if dest == "" {
			stage, err := os.MkdirTemp(filepath.Join(cache, "packages"), ".install-*")
			if err != nil {
				return err
			}
			defer func() { _ = os.RemoveAll(stage) }()
			src := p.Path
			revision = "local"
			if p.Git != "" {
				gitDir := filepath.Join(stage, "git")
				// Configuration may name local repositories; package manifests are limited to remote ones.
				protocols := "https:ssh:git"
				if !p.Manifest {
					protocols += ":file"
				}
				if _, err := git(
					ctx,
					protocols,
					false,
					"clone",
					"--quiet",
					"--no-checkout",
					"--",
					p.Git,
					gitDir,
				); err != nil {
					return err
				}
				ref := p.Ref
				if ref == "" {
					ref = "HEAD"
				}
				if sourceMatches && !updateAll && !wanted[id] {
					ref = previous.Revision
				}
				out, err := git(
					ctx,
					protocols,
					true,
					"-C",
					gitDir,
					"rev-parse",
					"--verify",
					"--end-of-options",
					ref+"^{commit}",
				)
				if err != nil {
					return fmt.Errorf("cannot resolve revision for package %s", id)
				}
				revision = strings.TrimSpace(string(out))
				if _, err := git(
					ctx,
					protocols,
					true,
					"-C",
					gitDir,
					"checkout",
					"--quiet",
					"--detach",
					revision,
				); err != nil {
					return err
				}
				src = gitDir
			}
			if p.Subdir != "" {
				src, err = securefs.Contained(src, p.Subdir)
				if err != nil {
					return err
				}
			}
			content := filepath.Join(stage, "content")
			if err := securefs.CopyTree(src, content); err != nil {
				return fmt.Errorf("package %s: %w", id, err)
			}
			hash, err = config.TreeHash(content)
			if err != nil {
				return err
			}
			if frozen && (hash != previous.Hash || revision != previous.Revision) {
				return fmt.Errorf("locked package %s content changed", id)
			}
			dest = filepath.Join(cache, "packages", hash)
			if _, err := os.Stat(dest); os.IsNotExist(err) {
				if err := os.Rename(content, dest); err != nil {
					return err
				}
			} else if err != nil {
				return err
			} else {
				existing, err := config.TreeHash(dest)
				if err != nil {
					return fmt.Errorf("cache entry for %s: %w", id, err)
				}
				if existing != hash {
					return fmt.Errorf("corrupt cache entry for %s", id)
				}
			}

		}
		manifest := filepath.Join(dest, "agx.package.yaml")
		d, node, err := config.DecodeManifest(manifest)
		if err != nil {
			return err
		}
		if err := c.AddManifest(d, node, manifest, id); err != nil {
			return err
		}
		deps := []config.ID{}
		for dep := range d.Packages {
			name := config.ID(string(id) + "/" + string(dep))
			// Configuration overrides of a dependency keep their own lockfile.
			if child := c.Packages[name]; child.Manifest {
				child.Lockfile = p.Lockfile
				c.Packages[name] = child
			}
			deps = append(deps, name)
		}
		sort.Slice(deps, func(i, j int) bool { return deps[i] < deps[j] })
		if frozen && config.Digest(deps) != config.Digest(previous.Dependencies) {
			return fmt.Errorf("stale dependencies for %s", id)
		}
		l.Packages[id] = config.LockedPackage{Source: source, Revision: revision, Hash: hash, Dependencies: deps}
		for _, dep := range deps {
			if err := install(dep, append(chain, id)); err != nil {
				return err
			}
		}
		loaded[id] = true
		result.PackageIDs = append(result.PackageIDs, id)
		return nil
	}
	ids := []config.ID{}
	for id := range c.Packages {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		if err := install(id, nil); err != nil {
			return result, err
		}
	}
	for id := range wanted {
		if !loaded[id] {
			return result, fmt.Errorf("unknown package %s", id)
		}
	}
	if err := c.Validate(); err != nil {
		return result, err
	}
	for path, l := range locks {
		for id := range l.Packages {
			if !loaded[id] || c.Packages[id].Lockfile != path {
				delete(l.Packages, id)
			}
		}
		result.Lockfiles = append(result.Lockfiles, path)
	}
	if !frozen {
		if err := writeLocks(locks); err != nil {
			return result, err
		}
	}
	sort.Strings(result.Lockfiles)
	sort.Slice(result.PackageIDs, func(i, j int) bool { return result.PackageIDs[i] < result.PackageIDs[j] })
	return result, nil
}

// writeLocks stages every lockfile before replacing any, so a failure while writing
// leaves all previous locks in place.
func writeLocks(locks map[string]config.Lock) error {
	staged := map[string]string{}
	defer func() {
		for _, tmp := range staged {
			_ = os.Remove(tmp)
		}
	}()
	for path, l := range locks {
		b, err := json.MarshalIndent(l, "", "  ")
		if err != nil {
			return err
		}
		if err := securefs.EnsureDir(filepath.Dir(path)); err != nil {
			return err
		}
		f, err := os.CreateTemp(filepath.Dir(path), ".agx-*")
		if err != nil {
			return err
		}
		staged[path] = f.Name()
		if err = f.Chmod(0o600); err == nil {
			err = securefs.Private(f.Name())
		}
		if err == nil {
			_, err = f.Write(append(b, '\n'))
		}
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	for path, tmp := range staged {
		if err := os.Rename(tmp, path); err != nil {
			return err
		}
		delete(staged, path)
	}
	return nil
}

// git runs with allowed transports restricted. Isolated invocations shape checked-out
// content, so they ignore user/system configuration and attributes that would make package
// hashes depend on the machine; cloning keeps user configuration for credentials and URL rewrites.
func git(ctx context.Context, protocols string, isolated bool, args ...string) ([]byte, error) {
	env := append(
		os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_ALLOW_PROTOCOL="+protocols,
		"GIT_LFS_SKIP_SMUDGE=1",
	)
	if isolated {
		env = append(env, "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_ATTR_NOSYSTEM=1")
		args = append([]string{
			"-c", "core.autocrlf=false",
			"-c", "core.eol=lf",
			"-c", "core.attributesFile=" + os.DevNull,
			"-c", "core.hooksPath=" + os.DevNull,
			"-c", "filter.lfs.smudge=",
			"-c", "filter.lfs.process=",
			"-c", "filter.lfs.required=false",
		}, args...)
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && len(exit.Stderr) > 0 {
			return nil, fmt.Errorf("git package operation failed: %w: %s", err, strings.TrimSpace(string(exit.Stderr)))
		}
		return nil, fmt.Errorf("git package operation failed: %w", err)
	}
	return out, nil
}
