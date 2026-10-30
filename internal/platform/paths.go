package platform

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/adrg/xdg"
)

// Paths locates agx's files. Runtime holds discovery, lock, and log files under the cache;
// State holds data worth keeping when the cache is wiped.
type Paths struct {
	Config, Cache, Runtime, State string
	canonicalConfig               string
}

func DefaultPaths() (Paths, error) {
	p := Paths{Config: filepath.Join(xdg.ConfigHome, "agx", "agx.yaml"), Cache: defaultCache()}
	// These explicit overrides also allow isolated, bounded verification runs.
	if value := os.Getenv("AGX_CONFIG"); value != "" {
		p.Config = value
	}
	if value := os.Getenv("AGX_CACHE"); value != "" {
		p.Cache = value
	}
	for _, value := range []string{p.Config, p.Cache} {
		if !filepath.IsAbs(value) {
			return Paths{}, fmt.Errorf(
				"configuration and cache paths must be absolute; check AGX_CONFIG, AGX_CACHE, XDG_CONFIG_HOME, and XDG_CACHE_HOME",
			)
		}
	}
	return p, nil
}

func defaultCache() string { return filepath.Join(xdg.CacheHome, "agx") }

// Derive sets Runtime and State from Cache. State is AGX_STATE when set; a cache other than the
// default keeps its state beside it, so a run isolated by AGX_CACHE or --cache stays isolated.
// It also spells Config as the filesystem does, so that a daemon and its callers, comparing
// configuration roots, do not tell apart spellings of one file (case, or a Windows short name).
func (p *Paths) Derive() error {
	cache, err := filepath.Abs(p.Cache)
	if err != nil {
		return err
	}
	p.Cache = cache
	// Canonicalization walks the filesystem; repeated derivation of one root need not.
	if p.Config != p.canonicalConfig {
		p.Config, err = Canonical(p.Config)
		if err != nil {
			return err
		}
		p.canonicalConfig = p.Config
	}
	hash := sha256.Sum256([]byte(p.Config))
	p.Runtime = filepath.Join(p.Cache, "runtime", hex.EncodeToString(hash[:6]))
	switch value := os.Getenv("AGX_STATE"); {
	case value != "":
		if !filepath.IsAbs(value) {
			return fmt.Errorf("AGX_STATE must be an absolute path")
		}
		p.State = value
	case filepath.Clean(p.Cache) != filepath.Clean(defaultCache()):
		p.State = filepath.Join(p.Cache, "state")
	default:
		p.State = filepath.Join(xdg.StateHome, "agx")
	}
	return nil
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
			real = ActualCase(real)
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

// PathCandidates yields PATH positions and executable candidates in search order.
func PathCandidates(name string) iter.Seq2[int, string] {
	return func(yield func(int, string) bool) {
		extensions := []string{""}
		if runtime.GOOS == "windows" {
			extensions = strings.Split(os.Getenv("PATHEXT"), ";")
		}
		for i, dir := range filepath.SplitList(os.Getenv("PATH")) {
			if dir == "" {
				continue
			}
			for _, ext := range extensions {
				if !yield(i, filepath.Join(dir, name+ext)) {
					return
				}
			}
		}
	}
}
