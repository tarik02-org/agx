package shims

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Target is the agx path shims run. Package managers expose agx through a stable link, such as
// ~/.nix-profile/bin/agx or a Homebrew prefix, to a versioned file that an upgrade or Nix garbage
// collection removes. So the path agx was invoked through (argv[0], looked up on PATH when bare)
// wins when it reaches the running executable and is not itself in the Nix store; otherwise the
// resolved executable is used.
func Target(arg0 string) (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(self)
	if err != nil {
		return "", err
	}
	if invoked := invokedPath(arg0); invoked != "" && !InNixStore(invoked) && sameFile(invoked, resolved) {
		return invoked, nil
	}
	return resolved, nil
}

// invokedPath is the absolute path argv[0] names, or "" when it names none.
func invokedPath(arg0 string) string {
	path := arg0
	if filepath.Base(arg0) == arg0 {
		found, err := exec.LookPath(arg0)
		if err != nil {
			return ""
		}
		path = found
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	return abs
}

func sameFile(a, b string) bool {
	infoA, errA := os.Stat(a)
	infoB, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(infoA, infoB)
}

// IsShim reports whether path is a shim: a file in agx's default shim directory, or one the
// manifest of its directory lists. Native executable lookup skips shims, which run agx again.
func IsShim(path string) bool {
	// The shim itself may be a link to agx, so only its directory is resolved.
	dir, base := canonical(filepath.Dir(path)), filepath.Base(path)
	if defaultDir, err := DefaultDir(); err == nil && samePath(canonical(defaultDir), dir) {
		return true
	}
	m, err := readManifest(dir)
	if err != nil {
		// A manifest agx cannot read still marks the directory as agx's.
		_, statErr := os.Stat(filepath.Join(dir, manifestName))
		return statErr == nil
	}
	for _, s := range m.Shims {
		if samePath(s.File, base) {
			return true
		}
	}
	return false
}

// InNixStore reports a path that garbage collection may remove once nothing roots it.
func InNixStore(path string) bool { return strings.HasPrefix(path, "/nix/store/") }

func canonical(path string) string {
	path, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	return path
}

func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// pathIndex is dir's position among the PATH entries, or -1.
func pathIndex(entries []string, dir string) int {
	dir = canonical(dir)
	for i, entry := range entries {
		if entry != "" && samePath(canonical(entry), dir) {
			return i
		}
	}
	return -1
}

// OnPath reports whether dir is a PATH entry.
func OnPath(dir string) bool {
	return pathIndex(filepath.SplitList(os.Getenv("PATH")), dir) >= 0
}

// Collision is another executable named like a shim.
type Collision struct {
	Name string
	Path string
	// First reports that Path is what the name runs: it precedes dir on PATH, or dir is not on it.
	First bool
}

// Collisions finds executables outside dir named like the shims for names, other than agx itself.
func Collisions(dir, target string, names []string) []Collision {
	entries := filepath.SplitList(os.Getenv("PATH"))
	index := pathIndex(entries, dir)
	self, selfErr := os.Stat(target)
	extensions := []string{""}
	if runtime.GOOS == "windows" {
		extensions = strings.Split(os.Getenv("PATHEXT"), ";")
	}
	collisions := []Collision{}
	for _, name := range names {
	search:
		for i, entry := range entries {
			if entry == "" || i == index {
				continue
			}
			for _, ext := range extensions {
				path := filepath.Join(entry, name+ext)
				info, err := os.Stat(path)
				if err != nil || info.IsDir() || selfErr == nil && os.SameFile(self, info) {
					continue
				}
				if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
					continue
				}
				collisions = append(collisions, Collision{Name: name, Path: path, First: index < 0 || i < index})
				break search
			}
		}
	}
	return collisions
}
