package securefs

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

// makePrivate leaves a private directory be only when it recognizes its own DACL; otherwise every
// call rewrites the whole tree again.
func TestPrivateRecognizesItsDACL(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "child"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	sid, err := currentSID()
	if err != nil {
		t.Fatal(err)
	}
	if isPrivate(dir, sid) {
		t.Fatal("a fresh temporary directory reads as private")
	}
	if err := makePrivate(dir); err != nil {
		t.Fatal(err)
	}
	if !isPrivate(dir, sid) {
		sd, _ := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		t.Fatalf("makePrivate's own DACL does not read as private: %v", sd)
	}
}
