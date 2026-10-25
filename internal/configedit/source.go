package configedit

import (
	"context"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/config/edit"
	"github.com/tarik02-org/agx/internal/packages"
)

// GitHub is the base of GitHub's owner/repo sources.
const GitHub = "https://github.com"

// Source is what skill add fetches: a local path, or a Git repository.
type Source struct {
	// Local is the path as given; the caller resolves it.
	Local                  string
	Git, Ref, Subdir, Name string
}

var (
	githubShorthand = regexp.MustCompile(`^([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+?)(?:\.git)?(?:@([^/]+))?(?:/(.+))?$`)
	scpLike         = regexp.MustCompile(`^[^/:]+@[^/:]+:`)
	invalidIDRunes  = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
)

// ParseSource reads a path, a Git URL, or GitHub's owner/repo[@ref][/path]. With local, a
// relative path without ./ is a path only when it exists, so owner/repo reaches GitHub; without
// it, paths are refused.
func ParseSource(arg, githubBase string, local bool) (Source, error) {
	isGit := strings.Contains(arg, "://") || strings.HasSuffix(arg, ".git") || scpLike.MatchString(arg)
	if config.IsPath(arg) && !isGit {
		if !local {
			return Source{}, errorf(InvalidRequest, "%s is a path; expected a Git URL or owner/repo[@ref][/path]", arg)
		}
		return Source{Local: arg}, nil
	}
	if !isGit {
		if local {
			if _, err := os.Stat(arg); err == nil {
				return Source{Local: arg}, nil
			}
		}
		m := githubShorthand.FindStringSubmatch(arg)
		if m == nil {
			if !local {
				return Source{}, errorf(InvalidRequest, "%s is not a Git URL or owner/repo[@ref][/path]", arg)
			}
			return Source{}, errorf(InvalidRequest, "%s is not a path, a Git URL, or owner/repo[@ref][/path]", arg)
		}
		git := strings.TrimRight(githubBase, "/") + "/" + m[1] + "/" + m[2] + ".git"
		return Source{
			Git:    git,
			Ref:    m[3],
			Subdir: m[4],
			Name:   config.RepositoryName(git),
		}, nil
	}
	return Source{Git: arg, Name: config.RepositoryName(arg)}, nil
}

// PackageID is the package a Git source is declared as: an existing package with the same
// source, else packageID, else one derived from the repository name.
func PackageID(c *config.Config, source Source, packageID string) (config.ID, error) {
	id := config.ID(packageID)
	for _, existing := range slices.Sorted(maps.Keys(c.Packages)) {
		p := c.Packages[existing]
		if !p.Anonymous && p.Git == source.Git && p.Ref == source.Ref && p.Subdir == source.Subdir &&
			(packageID == "" || existing == id) {
			return existing, nil
		}
	}
	if id == "" {
		id = config.ID(strings.Trim(invalidIDRunes.ReplaceAllString(source.Name, "-"), "-._"))
		if id == "" {
			return "", errorf(InvalidRequest, "cannot derive a package ID from %s; pass --package", source.Git)
		}
	}
	return id, nil
}

// DeclarePackage declares a Git source as package id in file, unless the package exists with the
// same source. It reports whether it declared one.
func (s *Session) DeclarePackage(id config.ID, source Source, file string) (bool, error) {
	if p, exists := s.Config.Packages[id]; exists {
		if p.Git != source.Git || p.Ref != source.Ref || p.Subdir != source.Subdir {
			return false, errorf(
				Conflict,
				"package %s already exists with another source; pass --package to choose another ID",
				id,
			)
		}
		return false, nil
	}
	definition := edit.Mapping("git", source.Git)
	if source.Ref != "" {
		edit.Set(definition, "ref", edit.Scalar(source.Ref))
	}
	if source.Subdir != "" {
		edit.Set(definition, "subdir", edit.Scalar(source.Subdir))
	}
	f, err := s.File(file)
	if err != nil {
		return false, err
	}
	if f.Definition(Packages, string(id)) != nil {
		return false, errorf(
			Conflict,
			"package %s is already defined in %s; pass --package to choose another ID",
			id,
			file,
		)
	}
	f.Define(Packages, string(id), definition)
	return true, nil
}

// Install installs the edited configuration's packages into the cache, keeping the lockfiles it
// would write in the session. Definitions in file, when it is outside the configuration, are
// installed on their own, as planning checks them.
func (s *Session) Install(ctx context.Context, file string) error {
	overlay, err := s.Overlay()
	if err != nil {
		return err
	}
	result, err := packages.Install(ctx, s.loadRoot(file), s.Paths.Cache, s.Builtins, packages.Options{
		Overlay: overlay,
		Stage:   true,
	})
	if err != nil {
		return err
	}
	s.Locks = result.Locks
	return nil
}

// Load loads the edited configuration, or file on its own when it is outside the configuration.
func (s *Session) Load(file string) (*config.Config, error) {
	overlay, err := s.Overlay()
	if err != nil {
		return nil, err
	}
	return config.Load(s.loadRoot(file), config.Options{Builtins: s.Builtins, Cache: s.Paths.Cache, Overlay: overlay})
}

func (s *Session) loadRoot(file string) string {
	if file != s.Root && !slices.Contains(s.Config.Files, file) {
		return file
	}
	return s.Paths.Config
}
