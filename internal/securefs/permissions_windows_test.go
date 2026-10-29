package securefs

import (
	"os"
	"os/user"
	"path/filepath"
	"testing"
)

// Private leaves a private directory be only when it recognizes its own DACL; otherwise every
// call rewrites the whole tree again.
func TestPrivateRecognizesItsDACL(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "child"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	current, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	if isPrivate(dir, current.Uid) {
		t.Fatal("a fresh temporary directory reads as private")
	}
	if err := Private(dir); err != nil {
		t.Fatal(err)
	}
	if !isPrivate(dir, current.Uid) {
		t.Fatal("Private's own DACL does not read as private")
	}
}
