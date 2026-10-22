package packages

import (
	"errors"
	"strings"
	"testing"
)

func TestGitMissingFromPath(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := git(t.Context(), "https", false, "version")
	if !errors.Is(err, ErrFetch) || !strings.Contains(err.Error(), "git is not on PATH") {
		t.Fatalf("err = %v", err)
	}
}
