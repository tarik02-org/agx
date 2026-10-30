package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/tarik02-org/agx/internal/shims"
)

func daemonExecutable() (string, error) {
	path := os.Getenv("AGX_DAEMON")
	if path == "" {
		self, err := os.Executable()
		if err != nil {
			return "", err
		}
		if origin, ok := shims.Origin(self); ok {
			self = origin
		}
		self, err = filepath.EvalSymlinks(self)
		if err != nil {
			return "", err
		}
		name := "agxd"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		path = filepath.Join(filepath.Dir(self), name)
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("AGX_DAEMON must name an absolute executable path")
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf(
			"agxd executable unavailable at %s; install agx and agxd together or set AGX_DAEMON",
			path,
		)
	}
	return path, nil
}
