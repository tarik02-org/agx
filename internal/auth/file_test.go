package auth_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tarik02-org/agx/internal/auth"
	"github.com/tarik02-org/agx/internal/config"
)

func TestFileSecret(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "token")
	secret := config.Secret{Kind: "file", File: file}
	manager := auth.New("http://127.0.0.1:1/callback", auth.OpenStore(t.TempDir()))

	t.Run("missing", func(t *testing.T) {
		if _, err := auth.LocalValue(
			t.Context(),
			auth.OpenStore(t.TempDir()),
			"token",
			secret,
			nil,
			noEnv,
		); err == nil {
			t.Fatal("a missing file resolved")
		}
		if status := manager.Status("token", secret, nil); status.State != "unavailable" || status.Problem == "" {
			t.Fatalf("status = %+v, want unavailable with a problem", status)
		}
	})

	t.Run("trailing newlines are dropped", func(t *testing.T) {
		if err := os.WriteFile(file, []byte("s3cret\n\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := auth.LocalValue(t.Context(), auth.OpenStore(t.TempDir()), "token", secret, nil, noEnv)
		if err != nil || got != "s3cret" {
			t.Fatalf("value = %q, %v", got, err)
		}
		if state := manager.Status("token", secret, nil).State; state != "ready" {
			t.Fatalf("state = %s, want ready", state)
		}
	})

	t.Run("empty", func(t *testing.T) {
		if err := os.WriteFile(file, []byte("\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := auth.LocalValue(t.Context(), auth.OpenStore(t.TempDir()), "token", secret, nil, noEnv)
		if err == nil || !strings.Contains(err.Error(), "is empty") {
			t.Fatalf("err = %v, want an empty-file error", err)
		}
	})

	t.Run("cannot be set", func(t *testing.T) {
		err := manager.Set("token", secret, []byte("other"))
		if err == nil || !strings.Contains(err.Error(), "write that file instead") {
			t.Fatalf("err = %v", err)
		}
	})
}
