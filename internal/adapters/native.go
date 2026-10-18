package adapters

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/shims"
)

// FindExecutable locates a native executable, skipping agx itself and shims, so a harness-named
// link to agx cannot recurse. On Windows it accepts only real executables: Go cannot pass arguments to a batch
// file without cmd.exe reparsing them.
func FindExecutable(name, configured string) (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	selfInfo, err := os.Stat(self)
	if err != nil {
		return "", err
	}
	candidates := []string{}
	if configured != "" {
		candidates = append(candidates, configured)
	} else {
		extensions := []string{""}
		if runtime.GOOS == "windows" {
			extensions = strings.Split(os.Getenv("PATHEXT"), ";")
		}
		for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
			if dir == "" {
				continue
			}
			for _, ext := range extensions {
				candidates = append(candidates, filepath.Join(dir, name+ext))
			}
		}
	}
	script := false
	for _, p := range candidates {
		info, err := os.Stat(p)
		if err != nil || info.IsDir() || os.SameFile(selfInfo, info) {
			continue
		}
		// A configured path is the user's explicit choice; the wrapper's guard catches a shim there.
		if configured == "" && shims.IsShim(p) {
			continue
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
			continue
		}
		if runtime.GOOS == "windows" {
			ext := strings.ToLower(filepath.Ext(p))
			if ext == ".cmd" || ext == ".bat" {
				script = true
				continue
			}
		}
		return filepath.Abs(p)
	}
	if script {
		return "", fmt.Errorf("native %s must be an .exe executable; configure its absolute executable path", name)
	}
	return "", fmt.Errorf("native %s executable not found; configure harnesses.%s.executable", name, name)
}

// DirectoryFlag locates one working-directory flag value in native arguments: args[Index] holds
// Prefix followed by the directory.
type DirectoryFlag struct {
	Index  int
	Prefix string
}

// ResolveDirectory makes dir absolute against cwd and canonical, and returns args with every flag
// rewritten to it, so the harness does not apply a relative change a second time.
func ResolveDirectory(args []string, cwd, dir string, flags []DirectoryFlag) (string, []string, error) {
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(cwd, dir)
	}
	dir, err := config.Canonical(dir)
	if err != nil {
		return "", nil, err
	}
	rewritten := append([]string(nil), args...)
	for _, f := range flags {
		rewritten[f.Index] = f.Prefix + dir
	}
	return dir, rewritten, nil
}

// Probe reports a help or version probe before any "--": --help, -h, --version, -V, or one of the
// harness's extra flags.
func Probe(args []string, extra ...string) bool {
	for _, a := range args {
		if a == "--" {
			break
		}
		if slices.Contains([]string{"--help", "-h", "--version", "-V"}, a) || slices.Contains(extra, a) {
			return true
		}
	}
	return false
}
