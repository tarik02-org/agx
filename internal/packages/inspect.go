package packages

import (
	"cmp"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/securefs"
)

// maxInspectedDirectories bounds the directories an inspection lists.
const maxInspectedDirectories = 5000

// Inspection is a Git source checked out at a ref or revision.
type Inspection struct {
	Revision string
	// Hash is the content hash of the subdirectory, as a package pinned to it records it.
	Hash string
	// Directories are the source's directories, relative and /-separated, sorted, without Git
	// metadata; at most maxInspectedDirectories.
	Directories []string
}

// Inspect checks out a Git source the way Install does, without configuring or caching it: url
// is a package's git source, ref and rev its pins, and subdir the directory whose content hash
// is wanted. The checkout is staged under cache and removed afterwards.
func Inspect(ctx context.Context, cache, url, ref, rev, subdir string) (Inspection, error) {
	staging := filepath.Join(cache, "packages")
	if err := os.MkdirAll(staging, 0o700); err != nil {
		return Inspection{}, err
	}
	stage, err := os.MkdirTemp(staging, ".inspect-*")
	if err != nil {
		return Inspection{}, err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	gitDir := filepath.Join(stage, "git")
	revision, err := checkoutGit(ctx, gitDir, url, "https:ssh:git:file", cmp.Or(rev, ref, "HEAD"), rev)
	if err != nil {
		return Inspection{}, err
	}
	src := gitDir
	if subdir != "" {
		if src, err = securefs.Contained(gitDir, subdir); err != nil {
			return Inspection{}, err
		}
	}
	// The hash is taken over a copy, as Install takes it, so the two agree.
	content := filepath.Join(stage, "content")
	if err := securefs.CopyTree(src, content); err != nil {
		return Inspection{}, err
	}
	hash, err := config.TreeHash(content)
	if err != nil {
		return Inspection{}, err
	}
	directories := []string{}
	err = filepath.WalkDir(gitDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() || path == gitDir {
			return nil
		}
		if entry.Name() == ".git" {
			return filepath.SkipDir
		}
		rel, err := filepath.Rel(gitDir, path)
		if err != nil {
			return err
		}
		directories = append(directories, filepath.ToSlash(rel))
		if len(directories) == maxInspectedDirectories {
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return Inspection{}, err
	}
	slices.Sort(directories)
	return Inspection{Revision: revision, Hash: hash, Directories: directories}, nil
}
