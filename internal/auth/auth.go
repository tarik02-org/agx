package auth

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/zalando/go-keyring"
	"golang.org/x/oauth2"

	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/platform"
)

type Manager struct {
	mu sync.Mutex
	// pending maps OAuth state to attempts; begins holds in-flight discovery per secret ID.
	pending map[string]*Attempt
	begins  map[config.ID]*beginCall
	// locks serialize refresh, login storage and logout per credential key.
	locks map[string]*sync.Mutex
	// unpersisted holds refreshed credentials the OS keychain refused to store.
	unpersisted map[string]credential
	generation  map[string]uint64
	// required marks credential keys whose server asked for login; hints maps a normalized
	// server URL to the protected resource metadata URL its 401 challenge named.
	required map[string]bool
	hints    map[string]string
	client   *http.Client
	callback string
}

// ErrNoCredential reports that no OAuth credential is stored for a secret.
var ErrNoCredential = errors.New("no stored credential")

type credential struct {
	Token    *oauth2.Token   `json:"token"`
	Endpoint oauth2.Endpoint `json:"endpoint"`
	ClientID string          `json:"client_id"`
	Origin   string          `json:"origin"`
	Resource string          `json:"resource"`
}

type Status struct {
	ID      config.ID
	State   string
	Expires time.Time
	// Problem explains an unavailable state; it never contains a secret value.
	Problem string
}

func New(callback string) *Manager {
	return &Manager{
		pending:     map[string]*Attempt{},
		begins:      map[config.ID]*beginCall{},
		locks:       map[string]*sync.Mutex{},
		unpersisted: map[string]credential{},
		generation:  map[string]uint64{},
		required:    map[string]bool{},
		hints:       map[string]string{},
		client: &http.Client{
			Timeout:       20 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		},
		callback: callback,
	}
}

// key names a secret's keychain item. It covers the definition, so editing a secret starts
// over, and the owner, so an inline secret never shares an item with a top-level one; whether
// OAuth is implicit or written as auth: oauth does not matter.
func key(id config.ID, s config.Secret) string {
	s.Source = config.Source{}
	s.Implicit = false
	return string(id) + ":" + config.Digest(s)
}

func (m *Manager) credentialLock(k string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	lock := m.locks[k]
	if lock == nil {
		lock = &sync.Mutex{}
		m.locks[k] = lock
	}
	return lock
}

func (m *Manager) Value(
	ctx context.Context,
	id config.ID,
	s config.Secret,
	inputs map[string]string,
	origin string,
) (string, error) {
	switch s.Kind {
	case "daemon_environment":
		if value := os.Getenv(s.Variable); value != "" {
			return value, nil
		}
		return "", fmt.Errorf("secret %s: daemon environment variable %s is not set", id, s.Variable)
	case "oauth":
		return m.oauthValue(ctx, id, s, origin)
	}
	return LocalValue(ctx, id, s, func(name string) (string, bool) {
		value, ok := inputs[name]
		return value, ok
	})
}

// LocalValue resolves secrets that need no daemon state, for commands that run without the
// daemon. env looks up the caller's environment.
func LocalValue(
	ctx context.Context,
	id config.ID,
	s config.Secret,
	env func(string) (string, bool),
) (string, error) {
	switch s.Kind {
	case "environment":
		if value, ok := env(s.Variable); ok && value != "" {
			return value, nil
		}
		return "", fmt.Errorf("secret %s: environment variable %s is not set", id, s.Variable)
	case "command":
		return runHelper(ctx, id, s.Command)
	case "sops":
		return sopsValue(ctx, id, s)
	case "keychain":
		value, err := keyring.Get("agx", key(id, s))
		if err != nil {
			return "", fmt.Errorf("keychain secret %s is unavailable; store it with agx secret set %s --stdin", id, id)
		}
		return value, nil
	case "daemon_environment":
		return "", fmt.Errorf("secret %s reads the daemon environment, which needs a managed launch", id)
	case "oauth":
		return "", fmt.Errorf("OAuth secret %s authenticates only HTTP MCP servers", id)
	}
	return "", fmt.Errorf("secret %s has unsupported kind %q", id, s.Kind)
}

func runHelper(ctx context.Context, id config.ID, command []string) (string, error) {
	out, _, err := runBounded(ctx, command, SystemEnvironment(), "")
	switch {
	case errors.Is(err, errOutputLimit):
		return "", fmt.Errorf("credential helper output failed or exceeded 1 MiB")
	case errors.Is(err, errSupervise):
		return "", fmt.Errorf("cannot supervise credential helper for %s", id)
	case err != nil:
		return "", fmt.Errorf("credential helper for %s failed", id)
	}
	value := strings.TrimSpace(string(out))
	if value == "" {
		return "", fmt.Errorf("credential helper for %s returned no credential", id)
	}
	return value, nil
}

var (
	errOutputLimit = errors.New("output failed or exceeded 1 MiB")
	errSupervise   = errors.New("cannot supervise process")
)

// runBounded runs a credential process for at most 20 seconds, keeping up to 1 MiB of stdout
// and the start of stderr; it kills the process and its descendants when either bound is hit.
func runBounded(ctx context.Context, command, env []string, dir string) ([]byte, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Env = env
	cmd.Dir = dir
	stderr := &limitedBuffer{limit: 16 * 1024}
	cmd.Stderr = stderr
	// A descendant holding stderr open must not keep Wait from returning.
	cmd.WaitDelay = time.Second
	platform.IsolateOwned(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		return nil, nil, err
	}
	kill, err := platform.Own(cmd)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, nil, errSupervise
	}
	kill = sync.OnceFunc(kill)
	// On Unix the group ID may be recycled once the leader is reaped, so the
	// group is only killed while the helper is still unreaped.
	var reapMu sync.Mutex
	reaped := false
	killGroup := func() {
		reapMu.Lock()
		defer reapMu.Unlock()
		if !reaped {
			kill()
		}
	}
	wait := func() error {
		err := cmd.Wait()
		reapMu.Lock()
		reaped = true
		reapMu.Unlock()
		if runtime.GOOS == "windows" {
			// Closes the job handle, terminating any remaining descendants.
			kill()
		}
		return err
	}
	stop := context.AfterFunc(ctx, killGroup)
	defer stop()
	out, readErr := io.ReadAll(io.LimitReader(stdout, 1024*1024+1))
	if readErr != nil || len(out) > 1024*1024 {
		killGroup()
		_ = wait()
		return nil, stderr.Bytes(), errOutputLimit
	}
	if err := wait(); err != nil {
		if ctx.Err() != nil {
			return nil, stderr.Bytes(), fmt.Errorf("timed out: %w", ctx.Err())
		}
		return nil, stderr.Bytes(), err
	}
	return out, stderr.Bytes(), nil
}

// limitedBuffer keeps the first limit bytes written and discards the rest without failing the
// writer, so a chatty process is not killed by a closed pipe.
type limitedBuffer struct {
	mu    sync.Mutex
	limit int
	data  []byte
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if room := b.limit - len(b.data); room > 0 {
		b.data = append(b.data, p[:min(room, len(p))]...)
	}
	return len(p), nil
}

func (b *limitedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.data)
}

func (m *Manager) oauthValue(ctx context.Context, id config.ID, a config.Secret, origin string) (string, error) {
	k := key(id, a)
	lock := m.credentialLock(k)
	lock.Lock()
	defer lock.Unlock()

	m.mu.Lock()
	saved, cached := m.unpersisted[k]
	m.mu.Unlock()
	if !cached {
		data, err := keyring.Get("agx", k)
		if errors.Is(err, keyring.ErrNotFound) {
			return "", fmt.Errorf("secret %s needs login; run agx secret login %s: %w", id, id, ErrNoCredential)
		}
		if err != nil {
			return "", fmt.Errorf("secret %s: OS keychain is unavailable", id)
		}
		if err := json.Unmarshal([]byte(data), &saved); err != nil || saved.Token == nil {
			return "", fmt.Errorf("invalid stored OAuth credential for %s", id)
		}
	}
	if saved.Origin != origin {
		return "", fmt.Errorf("secret %s is logged in to another server origin; run agx secret login %s", id, id)
	}
	if saved.Token.Valid() {
		return saved.Token.AccessToken, nil
	}
	if saved.Token.RefreshToken == "" {
		return "", fmt.Errorf("secret %s needs login; run agx secret login %s", id, id)
	}
	token, err := m.refresh(ctx, saved)
	if err != nil {
		return "", fmt.Errorf("secret %s refresh failed; run agx secret login %s", id, id)
	}
	saved.Token = token
	data, err := json.Marshal(saved)
	if err != nil {
		return "", err
	}
	if err := keyring.Set("agx", k, string(data)); err != nil {
		// The refresh token may have rotated; dropping it would force a new login.
		m.mu.Lock()
		m.unpersisted[k] = saved
		m.mu.Unlock()
		slog.Warn(
			"cannot persist refreshed OAuth credential; keeping it in memory until the daemon stops",
			"secret",
			id,
			"error",
			err,
		)
		return token.AccessToken, nil
	}
	m.mu.Lock()
	delete(m.unpersisted, k)
	m.mu.Unlock()
	return token.AccessToken, nil
}

// refresh is done by hand because oauth2.TokenSource cannot send the RFC 8707
// resource parameter.
func (m *Manager) refresh(ctx context.Context, saved credential) (*oauth2.Token, error) {
	if err := secureURL(saved.Endpoint.TokenURL); err != nil {
		return nil, err
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {saved.Token.RefreshToken},
		"client_id":     {saved.ClientID},
	}
	if saved.Resource != "" {
		form.Set("resource", saved.Resource)
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		saved.Endpoint.TokenURL,
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint returned HTTP %d", resp.StatusCode)
	}
	var body struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1024*1024)).
		Decode(&body); err != nil ||
		body.AccessToken == "" {
		return nil, fmt.Errorf("invalid token response")
	}
	token := &oauth2.Token{AccessToken: body.AccessToken, TokenType: body.TokenType, RefreshToken: body.RefreshToken}
	if token.RefreshToken == "" {
		token.RefreshToken = saved.Token.RefreshToken
	}
	if body.ExpiresIn > 0 {
		token.Expiry = time.Now().Add(time.Duration(body.ExpiresIn) * time.Second)
	}
	return token, nil
}

func (m *Manager) Set(id config.ID, a config.Secret, secret []byte) error {
	if a.Kind == "sops" {
		return fmt.Errorf("secret %s is encrypted in %s; edit it with sops %s", id, a.File, a.File)
	}
	if a.Kind != "keychain" {
		return fmt.Errorf("secret %s is not keychain-backed; only keychain secrets can be set", id)
	}
	if len(secret) > 1024*1024 {
		return fmt.Errorf("secret must contain 1 to 1048576 bytes")
	}
	value := strings.TrimRight(string(secret), "\r\n")
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("secret must not be empty")
	}
	k := key(id, a)
	lock := m.credentialLock(k)
	lock.Lock()
	defer lock.Unlock()
	if err := keyring.Set("agx", k, value); err != nil {
		return fmt.Errorf("OS keychain is unavailable: %w", err)
	}
	return nil
}

func (m *Manager) Logout(id config.ID, a config.Secret) error {
	if a.Kind != "keychain" && a.Kind != "oauth" {
		return fmt.Errorf("secret %s uses %s and has no stored credential to remove", id, a.Kind)
	}
	k := key(id, a)
	lock := m.credentialLock(k)
	lock.Lock()
	defer lock.Unlock()
	m.mu.Lock()
	m.generation[k]++
	delete(m.unpersisted, k)
	for _, p := range m.pending {
		if p.SecretID == id {
			m.finishLocked(p, errors.New("logged out while authorizing"))
		}
	}
	m.mu.Unlock()
	if err := keyring.Delete("agx", k); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("OS keychain is unavailable: %w", err)
	}
	return nil
}

func (m *Manager) Status(id config.ID, a config.Secret) Status {
	s := Status{ID: id, State: "unconfigured"}
	k := key(id, a)
	m.mu.Lock()
	m.expireLocked(time.Now())
	for _, p := range m.pending {
		if p.SecretID != id {
			continue
		}
		if key(id, p.definition) != k {
			m.finishLocked(p, errors.New("the secret's configuration changed; run the login again"))
			continue
		}
		s.State = "pending"
		s.Expires = p.Expires
		m.mu.Unlock()
		return s
	}
	required := m.required[k]
	memory, inMemory := m.unpersisted[k]
	m.mu.Unlock()
	switch a.Kind {
	case "environment":
		s.State = "unconfigured"
	case "daemon_environment":
		if os.Getenv(a.Variable) != "" {
			s.State = "ready"
		}
	case "command":
		s.State = "ready"
	case "sops":
		// Decrypting proves the key is available; the cache makes repeated checks cheap.
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if _, err := sopsValue(ctx, id, a); err != nil {
			s.State = "unavailable"
			s.Problem = err.Error()
		} else {
			s.State = "ready"
		}
	case "keychain", "oauth":
		if inMemory {
			s.State = "ready"
			s.Expires = memory.Token.Expiry
			break
		}
		data, err := keyring.Get("agx", k)
		switch {
		case errors.Is(err, keyring.ErrNotFound) && a.Implicit && !required:
			// On-demand OAuth needs no login until its server asks for one.
		case errors.Is(err, keyring.ErrNotFound):
			s.State = "needs_login"
		case err != nil:
			s.State = "unavailable"
		default:
			s.State = "ready"
			if a.Kind == "oauth" {
				var c credential
				if json.Unmarshal([]byte(data), &c) != nil || c.Token == nil {
					s.State = "unavailable"
				} else {
					s.Expires = c.Token.Expiry
					if !c.Token.Valid() && c.Token.RefreshToken == "" {
						s.State = "needs_login"
					}
				}
			}
		}
	}
	return s
}

func (m *Manager) getJSON(ctx context.Context, endpoint string, out any) error {
	if err := secureURL(endpoint); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("discovery request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("discovery returned HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1024*1024)).Decode(out)
}

func Origin(value string) (string, error) {
	u, err := url.Parse(value)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("invalid server URL")
	}
	return strings.ToLower(u.Scheme + "://" + u.Host), nil
}

func secureURL(value string) error {
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || u.User != nil {
		return fmt.Errorf("invalid OAuth URL")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("OAuth endpoints must use http or https")
	}
	return nil
}

func SystemEnvironment() []string {
	env := []string{}
	for _, name := range []string{
		"PATH", "HOME", "USERPROFILE", "SystemRoot", "COMSPEC", "PATHEXT",
		"TMPDIR", "TEMP", "TMP", "LANG", "LC_ALL", "SSL_CERT_FILE", "SSL_CERT_DIR",
	} {
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}
	return env
}

func Verifier(value string) [32]byte         { return sha256.Sum256([]byte(value)) }
func Match(hash [32]byte, value string) bool { return hash == Verifier(value) }
