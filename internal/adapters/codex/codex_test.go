package codex_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tarik02-org/agx/internal/adapters/codex"
	"github.com/tarik02-org/agx/internal/config"
)

func TestClassify(t *testing.T) {
	for _, tt := range []struct {
		args []string
		mode string
	}{
		{nil, "interactive"},
		{[]string{"fix the tests"}, "interactive"},
		{[]string{"exec", "hi"}, "exec"},
		{[]string{"e", "hi"}, "exec"},
		{[]string{"review"}, "exec"},
		{[]string{"resume", "id"}, "resume"},
		{[]string{"fork", "id"}, "resume"},
		{[]string{"app-server"}, "app-server"},
		{[]string{"app-server", "--listen", "stdio://"}, "app-server"},
		{[]string{"app-server", "generate-ts"}, "passthrough"},
		{[]string{"login", "status"}, "passthrough"},
		{[]string{"mcp", "list"}, "passthrough"},
		{[]string{"plugin", "add", "x"}, "passthrough"},
		{[]string{"--version"}, "passthrough"},
		{[]string{"-h"}, "passthrough"},
		{[]string{"exec", "--", "--help"}, "exec"},
		{[]string{"-c", "model=x", "exec"}, "exec"},
		{[]string{"-C", "sub", "resume", "id"}, "resume"},
		{[]string{"-m", "exec"}, "interactive"},
		{[]string{"--", "exec"}, "interactive"},
	} {
		if got := (codex.Adapter{}).Classify(tt.args); got != tt.mode {
			t.Errorf("Classify(%q) = %s, want %s", tt.args, got, tt.mode)
		}
	}
}

func TestDirectory(t *testing.T) {
	cwd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cwd, "sub", "deeper"), 0o755); err != nil {
		t.Fatal(err)
	}
	sub, err := config.Canonical(filepath.Join(cwd, "sub"))
	if err != nil {
		t.Fatal(err)
	}
	deeper := filepath.Join(sub, "deeper")
	for _, tt := range []struct {
		args, want []string
		dir        string
	}{
		{[]string{"exec", "hi"}, []string{"exec", "hi"}, ""},
		{[]string{"-C", "sub", "exec"}, []string{"-C", sub, "exec"}, sub},
		{[]string{"--cd=sub", "exec"}, []string{"--cd=" + sub, "exec"}, sub},
		{[]string{"-Csub"}, []string{"-C" + sub}, sub},
		{[]string{"-C", "sub", "--cd", "sub/deeper"}, []string{"-C", deeper, "--cd", deeper}, deeper},
		{[]string{"exec", "--", "-C", "sub"}, []string{"exec", "--", "-C", "sub"}, ""},
	} {
		dir, args, err := (codex.Adapter{}).Directory(tt.args, cwd)
		if err != nil {
			t.Fatal(err)
		}
		want := tt.dir
		if want == "" {
			if want, err = config.Canonical(cwd); err != nil {
				t.Fatal(err)
			}
		}
		if dir != want || !slices.Equal(args, tt.want) {
			t.Errorf("Directory(%q) = %s, %q; want %s, %q", tt.args, dir, args, want, tt.want)
		}
	}
	if _, _, err := (codex.Adapter{}).Directory([]string{"-C"}, cwd); err == nil ||
		!strings.Contains(err.Error(), "-C requires a directory") {
		t.Errorf("trailing -C: %v", err)
	}
}
