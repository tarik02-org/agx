package auth

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/tarik02-org/agx/internal/config"
)

// sopsEnvironmentPrefixes extend the credential-helper baseline with what sops needs to find a
// decryption key: its own settings (SOPS_AGE_KEY_FILE and friends), the default age key
// location under XDG_CONFIG_HOME, the GnuPG home and agent, the credential chains of the cloud
// KMS providers and Vault, and proxies to reach them. The rest of the environment stays out, as
// for command secrets.
var sopsEnvironmentPrefixes = []string{
	"SOPS_", "AGE_", "XDG_CONFIG_HOME", "XDG_RUNTIME_DIR",
	"GNUPGHOME", "GPG_AGENT_INFO", "GPG_TTY",
	"AWS_", "GOOGLE_", "CLOUDSDK_", "AZURE_", "VAULT_",
	"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "no_proxy",
}

func sopsEnvironment() []string {
	env := SystemEnvironment()
	for _, pair := range os.Environ() {
		name, _, _ := strings.Cut(pair, "=")
		for _, prefix := range sopsEnvironmentPrefixes {
			if strings.HasPrefix(name, prefix) {
				env = append(env, pair)
				break
			}
		}
	}
	return env
}

type sopsEntry struct {
	modified time.Time
	size     int64
	value    string
}

// sopsCache keeps decrypted values in memory, keyed by file and key, until the file's
// modification time or size changes. Values are never written to disk.
var sopsCache = struct {
	sync.Mutex
	entries map[string]sopsEntry
}{entries: map[string]sopsEntry{}}

func sopsValue(ctx context.Context, id config.ID, s config.Secret) (string, error) {
	info, err := os.Stat(s.File)
	if err != nil {
		return "", fmt.Errorf("secret %s: cannot read sops file %s", id, s.File)
	}
	cacheKey := s.File + "\x00" + strings.Join(s.Key, "\x00")
	sopsCache.Lock()
	entry, ok := sopsCache.entries[cacheKey]
	sopsCache.Unlock()
	if ok && entry.modified.Equal(info.ModTime()) && entry.size == info.Size() {
		return entry.value, nil
	}

	executable, err := exec.LookPath("sops")
	if err != nil {
		return "", fmt.Errorf("secret %s: sops executable not found on PATH; install sops to decrypt %s", id, s.File)
	}
	command := []string{executable, "decrypt"}
	if len(s.Key) > 0 {
		command = append(command, "--extract", s.Key.Extract())
	}
	// "--" keeps a file name starting with a dash from reading as a flag.
	command = append(command, "--", s.File)
	out, stderr, err := runBounded(ctx, command, sopsEnvironment(), filepath.Dir(s.File))
	if err != nil {
		reason := firstLine(stderr)
		if reason == "" {
			reason = err.Error()
		}
		return "", fmt.Errorf("secret %s: sops cannot decrypt %s: %s", id, s.File, reason)
	}
	value := strings.TrimSuffix(string(out), "\n")
	value = strings.TrimSuffix(value, "\r")
	if value == "" {
		return "", fmt.Errorf("secret %s: sops decrypted an empty value from %s", id, s.File)
	}
	// The stat taken before decrypting keys the entry, so an edit during decryption is caught
	// by the next lookup.
	sopsCache.Lock()
	sopsCache.entries[cacheKey] = sopsEntry{modified: info.ModTime(), size: info.Size(), value: value}
	sopsCache.Unlock()
	return value, nil
}

// firstLine returns the first non-empty line of a process's stderr, stripped of control
// characters and shortened, to explain a failure without echoing more than sops chose to say.
func firstLine(stderr []byte) string {
	for line := range strings.Lines(string(stderr)) {
		line = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return -1
			}
			return r
		}, line)
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if runes := []rune(line); len(runes) > 200 {
			line = string(runes[:200]) + "…"
		}
		return line
	}
	return ""
}
