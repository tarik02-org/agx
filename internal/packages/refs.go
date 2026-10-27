package packages

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"unicode"
)

// Ref is a branch or tag of a Git repository, by the name a package's ref takes.
type Ref struct {
	Name string
	Tag  bool
	// Commit is the commit the ref points to; an annotated tag's is the commit it tags.
	Commit string
	// Default is set for the branch the repository's HEAD names.
	Default bool
}

// RemoteRefs lists a repository's branches and tags without fetching it. git is a package's git
// source: a remote URL, or a local repository, which configuration may name. The default branch
// comes first, then the other branches by name, then tags, newest version first.
func RemoteRefs(ctx context.Context, url string) ([]Ref, error) {
	out, err := git(
		ctx,
		"https:ssh:git:file",
		false,
		"ls-remote", "--symref", "--end-of-options", url, "HEAD", "refs/heads/*", "refs/tags/*",
	)
	if err != nil {
		return nil, err
	}
	return parseRemoteRefs(string(out)), nil
}

// parseRemoteRefs reads git ls-remote --symref output.
func parseRemoteRefs(out string) []Ref {
	var head string
	byName := map[string]*Ref{}
	var refs []*Ref
	for line := range strings.Lines(out) {
		object, name, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if !ok {
			continue
		}
		if target, symbolic := strings.CutPrefix(object, "ref: "); symbolic {
			if name == "HEAD" {
				head = strings.TrimPrefix(target, "refs/heads/")
			}
			continue
		}
		branch, isBranch := strings.CutPrefix(name, "refs/heads/")
		tag, isTag := strings.CutPrefix(name, "refs/tags/")
		peeled := false
		if isTag {
			tag, peeled = strings.CutSuffix(tag, "^{}")
		}
		key := name
		if peeled {
			key = "refs/tags/" + tag
		}
		switch {
		case isBranch:
			ref := &Ref{Name: branch, Commit: object}
			byName[key], refs = ref, append(refs, ref)
		case isTag && byName[key] != nil:
			// An annotated tag's peeled line names the commit it tags.
			if peeled {
				byName[key].Commit = object
			}
		case isTag:
			ref := &Ref{Name: tag, Tag: true, Commit: object}
			byName[key], refs = ref, append(refs, ref)
		}
	}
	listed := make([]Ref, 0, len(refs))
	for _, ref := range refs {
		ref.Default = !ref.Tag && ref.Name == head
		listed = append(listed, *ref)
	}
	slices.SortStableFunc(listed, func(a, b Ref) int {
		switch {
		case a.Default != b.Default:
			return boolOrder(a.Default)
		case a.Tag != b.Tag:
			return boolOrder(!a.Tag)
		case a.Tag:
			return naturalCompare(b.Name, a.Name)
		default:
			return cmp.Compare(a.Name, b.Name)
		}
	})
	return listed
}

func boolOrder(first bool) int {
	if first {
		return -1
	}
	return 1
}

// naturalCompare orders names with their digit runs compared as numbers, so v1.10 follows v1.9.
func naturalCompare(a, b string) int {
	for a != "" && b != "" {
		aDigits, bDigits := unicode.IsDigit(rune(a[0])), unicode.IsDigit(rune(b[0]))
		if aDigits != bDigits {
			return cmp.Compare(a[0], b[0])
		}
		aRun, bRun := run(a, aDigits), run(b, bDigits)
		var order int
		if aDigits {
			aNumber, bNumber := strings.TrimLeft(aRun, "0"), strings.TrimLeft(bRun, "0")
			order = cmp.Or(cmp.Compare(len(aNumber), len(bNumber)), cmp.Compare(aNumber, bNumber))
		} else {
			order = cmp.Compare(aRun, bRun)
		}
		if order != 0 {
			return order
		}
		a, b = a[len(aRun):], b[len(bRun):]
	}
	return cmp.Compare(len(a), len(b))
}

// run is the leading run of digits, or of other characters, in s.
func run(s string, digits bool) string {
	end := strings.IndexFunc(s, func(r rune) bool { return unicode.IsDigit(r) != digits })
	if end == -1 {
		return s
	}
	return s[:end]
}
