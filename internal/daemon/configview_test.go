package daemon

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"connectrpc.com/connect"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
)

func TestCompleteDirectory(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"work", "workshop", ".hidden", "other"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "workfile"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{}
	complete := func(prefix string) []string {
		t.Helper()
		response, err := s.CompleteDirectory(
			t.Context(),
			connect.NewRequest(&v1.CompleteDirectoryRequest{Prefix: prefix}),
		)
		if err != nil {
			t.Fatal(err)
		}
		return response.Msg.Directories
	}
	if got := complete(
		filepath.Join(dir, "wo"),
	); !slices.Equal(
		got,
		[]string{filepath.Join(dir, "work"), filepath.Join(dir, "workshop")},
	) {
		t.Errorf("wo = %v", got)
	}
	if got := complete(
		dir + string(filepath.Separator),
	); len(got) != 3 ||
		slices.Contains(got, filepath.Join(dir, ".hidden")) {
		t.Errorf("all = %v", got)
	}
	if got := complete(filepath.Join(dir, ".h")); !slices.Equal(got, []string{filepath.Join(dir, ".hidden")}) {
		t.Errorf(".h = %v", got)
	}
	if got := complete(filepath.Join(dir, "missing", "x")); len(got) != 0 {
		t.Errorf("missing = %v", got)
	}
	if _, err := s.CompleteDirectory(
		t.Context(),
		connect.NewRequest(&v1.CompleteDirectoryRequest{Prefix: "relative"}),
	); err == nil {
		t.Error("a relative prefix was accepted")
	}
	withFiles, err := s.CompleteDirectory(
		t.Context(),
		connect.NewRequest(&v1.CompleteDirectoryRequest{Prefix: filepath.Join(dir, "wo"), Files: true}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(withFiles.Msg.Files, []string{filepath.Join(dir, "workfile")}) ||
		len(withFiles.Msg.Directories) != 2 {
		t.Errorf("files = %v, directories = %v", withFiles.Msg.Files, withFiles.Msg.Directories)
	}
}

func TestConfigurationFileReadOnly(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "agx.yaml")
	if err := os.WriteFile(plain, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "10-nix.yaml")
	if err := os.Symlink(plain, link); err != nil {
		t.Skip("symlinks unavailable")
	}
	if f := configurationFile(plain); f.ReadOnly {
		t.Errorf("plain file read-only: %+v", f)
	}
	if f := configurationFile(link); !f.ReadOnly || f.LinkTarget != plain {
		t.Errorf("link = %+v", f)
	}
}
