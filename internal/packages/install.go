package packages

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gofrs/flock"

	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/securefs"
)

// Install marks its errors with these, keeping each error's own message, so callers can tell a
// failed fetch from a lock that no longer matches without parsing messages.
var (
	ErrFetch     = errors.New("package fetch failed")
	ErrStaleLock = errors.New("lockfile does not match")
	ErrUnknown   = errors.New("unknown package")
)

type marked struct {
	err  error
	kind error
}

func (m marked) Error() string { return m.err.Error() }

func (m marked) Unwrap() error { return m.err }

func (m marked) Is(target error) bool { return target == m.kind }

func mark(kind, err error) error { return marked{err: err, kind: kind} }

type Result struct {
	LocalLock  *config.Lock `json:"-"`
	PackageIDs []config.ID  `json:"package_ids"`
	Lockfiles  []string     `json:"lockfiles"`
	// Locks holds the encoded lockfiles by path when Options.Stage leaves them unwritten.
	Locks map[string][]byte `json:"-"`
}

type Options struct {
	State string
	Local *config.Local
	// Frozen permits no lockfile changes.
	Frozen bool
	// Updates advances these packages; UpdateAll advances every one.
	Updates   []config.ID
	UpdateAll bool
	// Stage returns the lockfiles in Result.Locks instead of writing them.
	Stage bool
}

// lockRetry is how often installation retries the lock of another one sharing the cache,
// until ctx ends.
const lockRetry = 100 * time.Millisecond

func Install(ctx context.Context, root, cache string, builtins config.Builtins, opts Options) (Result, error) {
	frozen, updates, updateAll := opts.Frozen, opts.Updates, opts.UpdateAll
	result := Result{PackageIDs: []config.ID{}, Lockfiles: []string{}}
	if err := os.MkdirAll(filepath.Join(cache, "packages"), 0o700); err != nil {
		return result, err
	}
	lock := flock.New(filepath.Join(cache, "install.lock"))
	_, err := lock.TryLockContext(ctx, lockRetry)
	if err != nil {
		return result, err
	}
	defer func() { _ = lock.Unlock() }()
	c, err := config.Load(
		root,
		config.Options{Builtins: builtins, State: opts.State, Local: opts.Local},
	)
	if err != nil {
		return result, err
	}
	wanted := map[config.ID]bool{}
	for _, id := range updates {
		wanted[id] = true
	}
	locks := map[string]config.Lock{}
	loaded := map[config.ID]bool{}
	// fetched holds what fetch found out about each package until it merges.
	type fetchedPackage struct {
		previous       config.LockedPackage
		hash, revision string
		pinned         bool
		lock           config.Lock
		source         config.Package
	}
	fetched := map[config.ID]fetchedPackage{}
	fetch := func(id config.ID, p config.Package) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		pinned := p.Pinned()
		l, ok := locks[p.Lockfile]
		if !ok {
			l, err = c.PackageLock(p)
			switch {
			case err == nil:
				locks[p.Lockfile] = l
			case !os.IsNotExist(err):
				return "", err
			// Pinned packages need no lockfile, so none is created for them.
			case pinned:
			case frozen:
				return "", mark(ErrStaleLock, fmt.Errorf("missing lockfile %s", p.Lockfile))
			default:
				l = config.Lock{Version: 1, Packages: map[config.ID]config.LockedPackage{}}
				locks[p.Lockfile] = l
			}
		}
		previous, locked := l.Packages[id]
		source := p.LockSource()
		sourceMatches := !pinned && locked && config.Digest(previous.Source) == config.Digest(source)
		if frozen && !pinned && !sourceMatches {
			return "", mark(ErrStaleLock, fmt.Errorf("stale lock for %s", id))
		}
		hash, revision, dest := "", "", ""
		reuse := func(candidateHash, candidateRevision string) {
			candidate := filepath.Join(cache, "packages", candidateHash)
			if !config.ValidHash(candidateHash) {
				return
			}
			// Entries appear only by renaming content hashed under that name into place.
			if _, err := os.Stat(candidate); err == nil {
				hash = candidateHash
				revision = candidateRevision
				dest = candidate
			}
		}
		pin := config.PinPath(cache, p)
		switch {
		case p.Hash != "":
			reuse(p.Hash, pinnedCommit(p.Ref))
		case pinned:
			if recorded, err := config.ReadPin(pin); err == nil {
				reuse(recorded, pinnedCommit(p.Ref))
			}
		case p.Git != "" && sourceMatches && !updateAll && !wanted[id]:
			reuse(previous.Hash, previous.Revision)
		}
		if dest == "" {
			stage, err := os.MkdirTemp(filepath.Join(cache, "packages"), ".install-*")
			if err != nil {
				return "", err
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
				ref := cmp.Or(p.Ref, "HEAD")
				if sourceMatches && !updateAll && !wanted[id] {
					ref = previous.Revision
				}
				revision, err = checkoutGit(ctx, gitDir, p.Git, protocols, ref)
				if errors.Is(err, errUnresolved) {
					return "", fmt.Errorf("cannot resolve revision for package %s", id)
				}
				if err != nil {
					return "", err
				}
				src = gitDir
			}
			if p.Subdir != "" {
				src, err = securefs.Contained(src, p.Subdir)
				if err != nil {
					return "", err
				}
			}
			content := filepath.Join(stage, "content")
			if err := securefs.CopyTree(src, content); err != nil {
				return "", fmt.Errorf("package %s: %w", id, err)
			}
			hash, err = config.TreeHash(content)
			if err != nil {
				return "", err
			}
			if p.Hash != "" && hash != p.Hash {
				return "", fmt.Errorf("package %s content hash %s does not match its pinned hash %s", id, hash, p.Hash)
			}
			if frozen && !pinned && (hash != previous.Hash || revision != previous.Revision) {
				return "", mark(ErrStaleLock, fmt.Errorf("locked package %s content changed", id))
			}
			dest = filepath.Join(cache, "packages", hash)
			if _, err := os.Stat(dest); os.IsNotExist(err) {
				if err := os.Rename(content, dest); err != nil {
					return "", err
				}
			} else if err != nil {
				return "", err
			} else {
				existing, err := config.TreeHash(dest)
				if err != nil {
					return "", fmt.Errorf("cache entry for %s: %w", id, err)
				}
				if existing != hash {
					return "", fmt.Errorf("corrupt cache entry for %s", id)
				}
			}
			if pinned && p.Hash == "" {
				if err := config.WritePin(pin, hash); err != nil {
					return "", err
				}
			}
		}
		fetched[id] = fetchedPackage{previous, hash, revision, pinned, l, source}
		return dest, nil
	}
	merged := func(id config.ID, _ config.Package, deps []config.ID) error {
		f := fetched[id]
		if !f.pinned {
			if frozen && config.Digest(deps) != config.Digest(f.previous.Dependencies) {
				return fmt.Errorf("stale dependencies for %s", id)
			}
			f.lock.Packages[id] = config.LockedPackage{
				Source: f.source, Revision: f.revision, Hash: f.hash, Dependencies: deps,
			}
		}
		loaded[id] = true
		result.PackageIDs = append(result.PackageIDs, id)
		return nil
	}
	if err := c.MergePackages(fetch, merged); err != nil {
		return result, err
	}
	for id := range wanted {
		if !loaded[id] {
			return result, mark(ErrUnknown, fmt.Errorf("unknown package %s", id))
		}
	}
	if err := c.Validate(); err != nil {
		return result, err
	}
	for path, l := range locks {
		for id := range l.Packages {
			if !loaded[id] || c.Packages[id].Lockfile != path || c.Packages[id].Pinned() {
				delete(l.Packages, id)
			}
		}
		if path != "" {
			result.Lockfiles = append(result.Lockfiles, path)
		}
	}
	switch {
	case opts.Stage:
		result.Locks = map[string][]byte{}
		for path, l := range locks {
			if path == "" {
				result.LocalLock = &l
				continue
			}
			b, err := EncodeLock(l)
			if err != nil {
				return result, err
			}
			result.Locks[path] = b
		}
	case !frozen:
		if local, ok := locks[""]; ok {
			c.Local.Lock = local
			if _, err := config.WriteLocal(ctx, opts.State, root, c.Local); err != nil {
				return result, err
			}
			delete(locks, "")
		}
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
	staged := map[string]*securefs.Staged{}
	defer func() {
		for _, s := range staged {
			s.Discard()
		}
	}()
	for path, l := range locks {
		b, err := EncodeLock(l)
		if err != nil {
			return err
		}
		s, err := securefs.Stage(path, b)
		if err != nil {
			return err
		}
		staged[path] = s
	}
	for path, s := range staged {
		if err := s.Commit(); err != nil {
			return err
		}
		delete(staged, path)
	}
	return nil
}

// EncodeLock renders a lockfile as Install writes it.
func EncodeLock(l config.Lock) ([]byte, error) {
	b, err := json.MarshalIndent(l, "", "  ")
	return append(b, '\n'), err
}

// pinnedCommit is ref when it is a full commit, the revision a pinned source installs.
func pinnedCommit(ref string) string {
	if config.IsCommit(ref) {
		return ref
	}
	return ""
}

// errUnresolved is a ref or revision the repository does not have.
var errUnresolved = errors.New("cannot resolve revision")

// checkoutGit fetches ref of url, a branch, a tag, HEAD, or a full commit, into a new repository
// at dir, checks it out, and returns the commit. Only that commit is fetched, without history.
func checkoutGit(ctx context.Context, dir, url, protocols, ref string) (string, error) {
	if _, err := git(ctx, protocols, true, "init", "--quiet", dir); err != nil {
		return "", err
	}
	if _, err := git(
		ctx, protocols, false, "-C", dir, "fetch", "--quiet", "--depth", "1", "--end-of-options", url, ref,
	); err != nil {
		var failed gitError
		if errors.As(err, &failed) && (strings.Contains(failed.stderr, "couldn't find remote ref") ||
			strings.Contains(failed.stderr, "not our ref") || strings.Contains(failed.stderr, "unadvertised object")) {
			return "", errUnresolved
		}
		return "", fetchError(url, err)
	}
	out, err := git(ctx, protocols, true, "-C", dir, "rev-parse", "--verify", "FETCH_HEAD^{commit}")
	if err != nil {
		return "", errUnresolved
	}
	revision := strings.TrimSpace(string(out))
	if _, err := git(ctx, protocols, true, "-C", dir, "checkout", "--quiet", "--detach", revision); err != nil {
		return "", err
	}
	return revision, nil
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
		// Failures are classified by git's messages, which translations would change.
		"LC_ALL=C",
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
	if errors.Is(err, exec.ErrNotFound) {
		return nil, mark(ErrFetch, errors.New(
			"git is not on PATH; agx needs git to install Git packages, so install git or add it to PATH "+
				"(the daemon takes PATH from the shell that started it; agx daemon restart picks up a new one)",
		))
	}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && len(exit.Stderr) > 0 {
			return nil, mark(ErrFetch, gitError{err: err, stderr: strings.TrimSpace(string(exit.Stderr))})
		}
		return nil, mark(ErrFetch, fmt.Errorf("git package operation failed: %w", err))
	}
	return out, nil
}

// gitError is a git command that failed with a message on standard error.
type gitError struct {
	err    error
	stderr string
}

func (e gitError) Error() string {
	return fmt.Sprintf("git package operation failed: %v: %s", e.err, e.stderr)
}

func (e gitError) Unwrap() error { return e.err }

// unreachable are what git says, over HTTPS or SSH, about a repository that does not exist or
// that it has no credentials for: hosts such as GitHub answer a private repository as missing.
var unreachable = []string{
	"could not read Username",
	"Repository not found",
	"repository not found",
	"' not found",
	"Authentication failed",
	"Permission denied (publickey",
	"Could not read from remote repository",
}

// fetchError explains a failed fetch of url whose repository is missing or private, keeping git's
// own words as detail.
func fetchError(url string, err error) error {
	var failed gitError
	if !errors.As(err, &failed) {
		return err
	}
	for _, line := range strings.Split(failed.stderr, "\n") {
		for _, phrase := range unreachable {
			if !strings.Contains(line, phrase) {
				continue
			}
			access := "check that an SSH key with access to it is loaded"
			if strings.HasPrefix(url, "https://") {
				access = "set up git credentials for HTTPS (such as gh auth setup-git) or use an SSH URL"
			}
			return mark(ErrFetch, fmt.Errorf(
				"cannot fetch %s: the repository does not exist, or it is private and git has no "+
					"credentials for it; for a private repository, %s (git: %s)",
				url,
				access,
				strings.TrimSpace(line),
			))
		}
	}
	return err
}
