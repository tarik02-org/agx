package adapters

import (
	"os"
	"path/filepath"

	"github.com/tarik02-org/agx/internal/config"
)

// MaterializeSkill deploys portable content for a harness that requires local files.
func MaterializeSkill(skill config.Skill, dest string) (string, error) {
	tree, version, err := skill.ReadContent()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return "", err
	}
	for _, directory := range tree.Directories {
		if err := os.MkdirAll(filepath.Join(dest, filepath.FromSlash(directory)), 0o700); err != nil {
			return "", err
		}
	}
	for _, file := range tree.Files {
		mode := os.FileMode(0o600)
		if file.Executable {
			mode = 0o700
		}
		w, err := os.OpenFile(
			filepath.Join(dest, filepath.FromSlash(file.Path)), os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode,
		)
		if err != nil {
			return "", err
		}
		_, err = w.Write(file.Content)
		if closeErr := w.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return "", err
		}
	}
	return version, nil
}
