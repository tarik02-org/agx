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

	"github.com/zalando/go-keyring"

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
		got, err := auth.LocalValue(t.Context(), auth.OpenStore(t.TempDir()), "gh", secret, nil, noEnv)
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

	got, err := auth.LocalValue(
		t.Context(),
		auth.OpenStore(t.TempDir()),
		"env",
		config.Secret{Kind: "sops", File: file},
		nil,
		noEnv,
	)
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
	manager := auth.New("http://127.0.0.1:1/callback", auth.OpenStore(t.TempDir()))
	value := func() string {
		t.Helper()
		got, err := manager.Value(t.Context(), "token", secret, nil, nil, "")
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
	if _, err := auth.LocalValue(t.Context(), auth.OpenStore(t.TempDir()), "token", secret, nil, noEnv); err != nil {
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
		_, err := auth.LocalValue(t.Context(), auth.OpenStore(t.TempDir()), "token", secret, nil, noEnv)
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
			_, err := auth.LocalValue(t.Context(), auth.OpenStore(t.TempDir()), "token", tc.secret, nil, noEnv)
			want := "secret token: sops cannot decrypt " + tc.secret.File + ": " + tc.reason
			if err == nil || err.Error() != want {
				t.Fatalf("err = %v, want %s", err, want)
			}
			status := auth.New("http://127.0.0.1:1/callback", auth.OpenStore(t.TempDir())).
				Status("token", tc.secret, nil)
			if status.State != "unavailable" || status.Problem != want {
				t.Fatalf("status = %+v", status)
			}
		})
	}
}

func TestSopsCannotBeSet(t *testing.T) {
	secret := config.Secret{Kind: "sops", File: "/config/secrets.yaml"}
	err := auth.New("http://127.0.0.1:1/callback", auth.OpenStore(t.TempDir())).Set("token", secret, []byte("value"))
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
	got, err := auth.LocalValue(ctx, auth.OpenStore(t.TempDir()), "github", secret, nil, noEnv)
	if err != nil {
		t.Fatal(err)
	}
	if got != "age-token" {
		t.Fatalf("value = %q", got)
	}

	// The same file decrypts with the identity stored as a keychain secret, while the
	// inherited key file points nowhere.
	identity, err := os.ReadFile(keys)
	if err != nil {
		t.Fatal(err)
	}
	keyring.MockInit()
	age := config.Secret{Kind: "keychain"}
	manager := auth.New("http://127.0.0.1:1/callback", auth.OpenStore(t.TempDir()))
	if err := manager.Set("age", age, identity); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SOPS_AGE_KEY_FILE", filepath.Join(dir, "missing.txt"))
	secret.AgeKey = config.Value{Secret: "age"}
	got, err = manager.Value(ctx, "github", secret, map[config.ID]config.Secret{"age": age}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "age-token" {
		t.Fatalf("value with age_key = %q", got)
	}
}

func TestSopsAgeKey(t *testing.T) {
	log := fakeSops(t)
	keyring.MockInit()
	file := filepath.Join(t.TempDir(), "secrets.json")
	writeFile(t, file, `{"token": "value"}`)
	// Inherited age settings must not reach sops once age_key is configured.
	t.Setenv("SOPS_AGE_KEY", "AGE-SECRET-KEY-INHERITED")
	t.Setenv("SOPS_AGE_KEY_FILE", filepath.Join(t.TempDir(), "keys.txt"))
	t.Setenv("SOPS_AGE_KEY_CMD", "false")
	keyFile := filepath.Join(t.TempDir(), "age.txt")
	writeFile(t, keyFile, "# created: today\nAGE-SECRET-KEY-FROM-FILE\n")

	for _, tc := range []struct {
		name, want string
		ageKey     config.Value
		secrets    map[config.ID]config.Secret
	}{
		{"!env", "AGE-SECRET-KEY-ENV", config.Value{Env: "AGE_IDENTITY"}, nil},
		{"!file", "# created: today\nAGE-SECRET-KEY-FROM-FILE", config.Value{File: keyFile}, nil},
		{
			"!secret environment",
			"AGE-SECRET-KEY-ENV",
			config.Value{Secret: "age"},
			map[config.ID]config.Secret{"age": {Kind: "environment", Variable: "AGE_IDENTITY"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SOPS_FAKE_AGE_KEY", tc.want)
			secret := config.Secret{Kind: "sops", File: file, Key: config.SecretKey{"token"}, AgeKey: tc.ageKey}
			env := func(name string) (string, bool) {
				if name == "AGE_IDENTITY" {
					return "AGE-SECRET-KEY-ENV", true
				}
				return "", false
			}
			if got, err := auth.LocalValue(t.Context(), auth.OpenStore(t.TempDir()),
				"token",
				secret,
				tc.secrets,
				env,
			); err != nil ||
				got != "value" {
				t.Fatalf("local value = %q, %v", got, err)
			}
			inputs := map[string]string{"AGE_IDENTITY": "AGE-SECRET-KEY-ENV"}
			manager := auth.New("http://127.0.0.1:1/callback", auth.OpenStore(t.TempDir()))
			if got, err := manager.Value(
				t.Context(),
				"token",
				secret,
				tc.secrets,
				inputs,
				"",
			); err != nil ||
				got != "value" {
				t.Fatalf("daemon value = %q, %v", got, err)
			}
		})
	}

	t.Run("!secret keychain", func(t *testing.T) {
		t.Setenv("SOPS_FAKE_AGE_KEY", "AGE-SECRET-KEY-KEYCHAIN")
		age := config.Secret{Kind: "keychain"}
		secrets := map[config.ID]config.Secret{"age": age}
		secret := config.Secret{
			Kind:   "sops",
			File:   file,
			Key:    config.SecretKey{"token"},
			AgeKey: config.Value{Secret: "age"},
		}
		manager := auth.New("http://127.0.0.1:1/callback", auth.OpenStore(t.TempDir()))

		status := manager.Status("token", secret, secrets)
		if status.State != "unavailable" || !strings.Contains(status.Problem, "keychain secret age is not stored") {
			t.Fatalf("status without the age key = %+v", status)
		}
		if err := manager.Set("age", age, []byte("AGE-SECRET-KEY-KEYCHAIN\n")); err != nil {
			t.Fatal(err)
		}
		if status := manager.Status("token", secret, secrets); status.State != "ready" {
			t.Fatalf("status with the age key = %+v", status)
		}
	})

	t.Run("caller environment", func(t *testing.T) {
		secret := config.Secret{Kind: "sops", File: file, AgeKey: config.Value{Env: "AGE_IDENTITY"}}
		status := auth.New("http://127.0.0.1:1/callback", auth.OpenStore(t.TempDir())).Status("token", secret, nil)
		if status.State != "unconfigured" {
			t.Fatalf("an age key from the caller's environment resolves at launch: %+v", status)
		}
	})

	t.Run("cache follows the key", func(t *testing.T) {
		cached := filepath.Join(t.TempDir(), "cached.json")
		writeFile(t, cached, `{"token": "value"}`)
		secret := config.Secret{Kind: "sops", File: cached, AgeKey: config.Value{Env: "AGE_IDENTITY"}}
		decrypt := func(key string) error {
			env := func(string) (string, bool) { return key, true }
			_, err := auth.LocalValue(t.Context(), auth.OpenStore(t.TempDir()), "token", secret, nil, env)
			return err
		}
		before := len(runs(t, log))
		t.Setenv("SOPS_FAKE_AGE_KEY", "AGE-SECRET-KEY-ONE")
		for range 2 {
			if err := decrypt("AGE-SECRET-KEY-ONE"); err != nil {
				t.Fatal(err)
			}
		}
		if n := len(runs(t, log)) - before; n != 1 {
			t.Fatalf("sops ran %d times; an unchanged key and file decrypt once", n)
		}
		if err := decrypt("AGE-SECRET-KEY-TWO"); err == nil {
			t.Fatal("a changed key was served from the cache instead of decrypting again")
		}
		if n := len(runs(t, log)) - before; n != 2 {
			t.Fatalf("sops ran %d times, want 2", n)
		}
	})
}
