package auth_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/tarik02-org/agx/internal/auth"
	"github.com/tarik02-org/agx/internal/config"
)

// fakeSops builds testdata/fakesops as the only sops on PATH and returns the file its runs are
// logged to.
func fakeSops(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	name := "sops"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	build := exec.CommandContext(t.Context(), "go", "build", "-o", filepath.Join(dir, name), "./testdata/fakesops")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building fake sops: %v\n%s", err, out)
	}
	t.Setenv("PATH", dir)
	log := filepath.Join(t.TempDir(), "sops.log")
	t.Setenv("SOPS_FAKE_LOG", log)
	return log
}

func runs(t *testing.T, log string) []string {
	t.Helper()
	data, err := os.ReadFile(log)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func noEnv(string) (string, bool) { return "", false }

func TestSopsExtract(t *testing.T) {
	log := fakeSops(t)
	file := filepath.Join(t.TempDir(), "secrets.yaml")
	writeFile(t, file, `{"github": {"token": "gh-token\n"}, "a.b": {"c": "dotted"}}`)

	for _, tc := range []struct {
		key   config.SecretKey
		want  string
		quote string
	}{
		{config.SecretKey{"github", "token"}, "gh-token", `["github"]["token"]`},
		{config.SecretKey{"a.b", "c"}, "dotted", `["a.b"]["c"]`},
	} {
		secret := config.Secret{Kind: "sops", File: file, Key: tc.key}
		got, err := auth.LocalValue(t.Context(), "gh", secret, noEnv)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("value = %q, want %q", got, tc.want)
		}
		all := runs(t, log)
		if want := "decrypt --extract " + tc.quote + " -- " + file; all[len(all)-1] != want {
			t.Errorf("sops ran with %q, want %q", all[len(all)-1], want)
		}
	}
}

func TestSopsWholeFile(t *testing.T) {
	fakeSops(t)
	file := filepath.Join(t.TempDir(), "app.env")
	writeFile(t, file, "A=1\nB=2\n\n")

	got, err := auth.LocalValue(t.Context(), "env", config.Secret{Kind: "sops", File: file}, noEnv)
	if err != nil {
		t.Fatal(err)
	}
	if got != "A=1\nB=2\n" {
		t.Errorf("value = %q; only one trailing newline is trimmed", got)
	}
}

func TestSopsCache(t *testing.T) {
	log := fakeSops(t)
	file := filepath.Join(t.TempDir(), "secrets.json")
	writeFile(t, file, `{"token": "one"}`)
	secret := config.Secret{Kind: "sops", File: file, Key: config.SecretKey{"token"}}
	manager := auth.New("http://127.0.0.1:1/callback")
	value := func() string {
		t.Helper()
		got, err := manager.Value(t.Context(), "token", secret, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	for range 2 {
		if got := value(); got != "one" {
			t.Fatalf("value = %q", got)
		}
	}
	if n := len(runs(t, log)); n != 1 {
		t.Fatalf("sops ran %d times; an unchanged file is decrypted once", n)
	}

	writeFile(t, file, `{"token": "second"}`)
	if got := value(); got != "second" {
		t.Fatalf("after a size change: %q", got)
	}

	// Same size, new modification time.
	writeFile(t, file, `{"token": "third!"}`)
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(file, later, later); err != nil {
		t.Fatal(err)
	}
	if got := value(); got != "third!" {
		t.Fatalf("after a modification time change: %q", got)
	}
	if n := len(runs(t, log)); n != 3 {
		t.Fatalf("sops ran %d times, want 3", n)
	}
}

func TestSopsEnvironment(t *testing.T) {
	fakeSops(t)
	file := filepath.Join(t.TempDir(), "secrets.json")
	writeFile(t, file, `{"token": "value"}`)
	t.Setenv("UNRELATED_TOKEN", "leak")
	t.Setenv("SOPS_FAKE_FORBIDDEN", "UNRELATED_TOKEN")

	secret := config.Secret{Kind: "sops", File: file, Key: config.SecretKey{"token"}}
	if _, err := auth.LocalValue(t.Context(), "token", secret, noEnv); err != nil {
		t.Fatalf("SOPS_ variables pass and others stay out: %v", err)
	}
}

func TestSopsFailures(t *testing.T) {
	file := filepath.Join(t.TempDir(), "secrets.json")
	writeFile(t, file, `{"token": "value"}`)
	locked := filepath.Join(t.TempDir(), "locked.json")
	writeFile(t, locked, "FAIL")

	t.Run("missing executable", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		secret := config.Secret{Kind: "sops", File: file, Key: config.SecretKey{"token"}}
		_, err := auth.LocalValue(t.Context(), "token", secret, noEnv)
		if err == nil || !strings.Contains(err.Error(), "sops executable not found") {
			t.Fatalf("err = %v", err)
		}
	})

	fakeSops(t)
	for _, tc := range []struct {
		name   string
		secret config.Secret
		reason string
	}{
		{
			"decrypt failure",
			config.Secret{Kind: "sops", File: locked, Key: config.SecretKey{"token"}},
			"Error getting data key: 0 successful groups required, got 0",
		},
		{
			"missing key",
			config.Secret{Kind: "sops", File: file, Key: config.SecretKey{"missing"}},
			`error: component ["missing"] not found`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := auth.LocalValue(t.Context(), "token", tc.secret, noEnv)
			want := "secret token: sops cannot decrypt " + tc.secret.File + ": " + tc.reason
			if err == nil || err.Error() != want {
				t.Fatalf("err = %v, want %s", err, want)
			}
			status := auth.New("http://127.0.0.1:1/callback").Status("token", tc.secret)
			if status.State != "unavailable" || status.Problem != want {
				t.Fatalf("status = %+v", status)
			}
		})
	}
}

func TestSopsCannotBeSet(t *testing.T) {
	secret := config.Secret{Kind: "sops", File: "/config/secrets.yaml"}
	err := auth.New("http://127.0.0.1:1/callback").Set("token", secret, []byte("value"))
	if err == nil || !strings.Contains(err.Error(), "edit it with sops /config/secrets.yaml") {
		t.Fatalf("err = %v", err)
	}
}

// TestSopsAge decrypts a file encrypted with a real age key; it runs only where sops and
// age-keygen are installed.
func TestSopsAge(t *testing.T) {
	for _, tool := range []string{"sops", "age-keygen"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not installed", tool)
		}
	}
	dir := t.TempDir()
	keys := filepath.Join(dir, "keys.txt")
	if out, err := exec.CommandContext(t.Context(), "age-keygen", "-o", keys).CombinedOutput(); err != nil {
		t.Fatalf("age-keygen: %v\n%s", err, out)
	}
	recipient, err := exec.CommandContext(t.Context(), "age-keygen", "-y", keys).Output()
	if err != nil {
		t.Fatal(err)
	}
	plain := filepath.Join(dir, "plain.yaml")
	writeFile(t, plain, "github:\n  token: age-token\n")
	encrypt := exec.CommandContext(
		t.Context(), "sops", "encrypt", "--age", strings.TrimSpace(string(recipient)), plain,
	)
	encrypted, err := encrypt.Output()
	if err != nil {
		t.Fatalf("sops encrypt: %v", err)
	}
	file := filepath.Join(dir, "secrets.yaml")
	writeFile(t, file, string(encrypted))
	if strings.Contains(string(encrypted), "age-token") {
		t.Fatal("sops did not encrypt the value")
	}
	t.Setenv("SOPS_AGE_KEY_FILE", keys)

	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	secret := config.Secret{Kind: "sops", File: file, Key: config.SecretKey{"github", "token"}}
	got, err := auth.LocalValue(ctx, "github", secret, noEnv)
	if err != nil {
		t.Fatal(err)
	}
	if got != "age-token" {
		t.Fatalf("value = %q", got)
	}
}
