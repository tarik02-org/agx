package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
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
	resetMu sync.RWMutex
	mu      sync.Mutex
	// pending maps OAuth state to the current attempts.
	pending map[string]*Attempt
	// locks serialize refresh, login storage and logout per credential key.
	locks map[string]*sync.Mutex
	// required marks credential keys whose server asked for login; hints maps a normalized
	// server URL to the protected resource metadata URL its 401 challenge named.
	required map[string]bool
	hints    map[string]string
	// refused maps credential keys whose refresh token the authorization server refused to the
	// reason, until a new login or logout; refreshing again could not succeed.
	refused map[string]string
	// helpers keeps command secrets' values by credential key for helperTTL, since every launch
	// and MCP server start resolves them.
	helpers  map[string]helperValue
	callback string
	store    *Store
}

// helperTTL is how long a command secret's value is reused before its command runs again.
const helperTTL = time.Minute

type helperValue struct {
	value   string
	expires time.Time
}

var (
	// ErrNoCredential reports that no OAuth credential is stored for a secret.
	ErrNoCredential = errors.New("no stored credential")
	// ErrRefreshFailed reports an OAuth refresh that failed for a reason a retry may outlast,
	// such as an unreachable authorization server; every other OAuth failure needs a login.
	ErrRefreshFailed = errors.New("refresh failed")
)

type credential struct {
	Token    *oauth2.Token   `json:"token"`
	Endpoint oauth2.Endpoint `json:"endpoint"`
	ClientID string          `json:"client_id"`
	Origin   string          `json:"origin"`
}

type Status struct {
	ID      config.ID
	State   string
	Expires time.Time
	// Refreshable reports an OAuth login whose access token is renewed when it is next used
	// after Expires.
	Refreshable bool
	// Problem explains an unavailable state or a refused refresh; it never contains a secret
	// value.
	Problem string
}

// New returns a manager whose OAuth logins return to callback and whose stored secrets live in
// store.
func New(callback string, store *Store) *Manager {
	return &Manager{
		store:    store,
		pending:  map[string]*Attempt{},
		locks:    map[string]*sync.Mutex{},
		required: map[string]bool{},
		hints:    map[string]string{},
		refused:  map[string]string{},
		helpers:  map[string]helperValue{},
		callback: callback,
	}
}

// client makes every OAuth and discovery request; it does not follow redirects.
var client = &http.Client{
	Timeout:       20 * time.Second,
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
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

// Value resolves secret s, named id. secrets resolves the !secret reference of a sops secret's
// age key, and inputs holds the caller's environment.
func (m *Manager) Value(
	ctx context.Context,
	id config.ID,
	s config.Secret,
	secrets map[config.ID]config.Secret,
	inputs map[string]string,
	origin string,
) (string, error) {
	env := func(name string) (string, bool) {
		value, ok := inputs[name]
		return value, ok
	}
	switch s.Kind {
	case "oauth":
		return m.oauthValue(ctx, id, s, origin)
	case "sops":
		return sopsValue(ctx, id, s, env, func(ref config.ID) (string, error) {
			return m.Value(ctx, ref, secrets[ref], secrets, inputs, "")
		})
	case "command":
		return m.helperValue(ctx, id, s)
	}
	return LocalValue(ctx, m.store, id, s, secrets, env)
}

func (m *Manager) helperValue(ctx context.Context, id config.ID, s config.Secret) (string, error) {
	k := key(id, s)
	m.mu.Lock()
	cached, ok := m.helpers[k]
	m.mu.Unlock()
	if ok && time.Now().Before(cached.expires) {
		return cached.value, nil
	}
	value, err := runHelper(ctx, id, s.Command)
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for k, v := range m.helpers {
		if !now.Before(v.expires) {
			delete(m.helpers, k)
		}
	}
	m.helpers[k] = helperValue{value: value, expires: now.Add(helperTTL)}
	return value, nil
}

// ForgetHelpers drops the command secrets' values, so that their commands run again.
func (m *Manager) ForgetHelpers() {
	m.mu.Lock()
	defer m.mu.Unlock()
	clear(m.helpers)
}

// LocalValue resolves secrets that need no daemon state, for commands that run without the
// daemon. secrets resolves the !secret reference of a sops secret's age key, and env looks up
// the caller's environment.
func LocalValue(
	ctx context.Context,
	store *Store,
	id config.ID,
	s config.Secret,
	secrets map[config.ID]config.Secret,
	env func(string) (string, bool),
) (string, error) {
	switch s.Kind {
	case "command":
		return runHelper(ctx, id, s.Command)
	case "sops":
		return sopsValue(ctx, id, s, env, func(ref config.ID) (string, error) {
			return LocalValue(ctx, store, ref, secrets[ref], secrets, env)
		})
	case "keychain":
		value, err := store.Get(key(id, s))
		if errors.Is(err, errNotStored) {
			return "", fmt.Errorf("keychain secret %s is not stored; store it with agx secret set %s", id, id)
		} else if err != nil {
			return "", fmt.Errorf("keychain secret %s: %w", id, err)
		}
		return value, nil
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
	case err != nil:
		return "", fmt.Errorf("credential helper for %s failed", id)
	}
	value := strings.TrimSpace(string(out))
	if value == "" {
		return "", fmt.Errorf("credential helper for %s returned no credential", id)
	}
	return value, nil
}

var errOutputLimit = errors.New("output failed or exceeded 1 MiB")

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
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	group, err := platform.Start(cmd)
	if err != nil {
		_ = stdout.Close()
		return nil, nil, err
	}
	stop := context.AfterFunc(ctx, group.Kill)
	defer stop()
	out, readErr := io.ReadAll(io.LimitReader(stdout, 1024*1024+1))
	if readErr != nil || len(out) > 1024*1024 {
		group.Kill()
		_ = group.Wait()
		return nil, stderr.Bytes(), errOutputLimit
	}
	if err := group.Wait(); err != nil {
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
	return m.oauthToken(ctx, id, a, origin, "")
}

// Refresh renews the access token after the server rejected it as rejected, whatever its
// recorded expiry: servers revoke tokens early, and a token response without expires_in never
// expires on its own. A stored token that already differs from rejected, renewed by a concurrent
// request or a new login, is returned without refreshing again.
func (m *Manager) Refresh(ctx context.Context, id config.ID, a config.Secret, origin, rejected string) (string, error) {
	if a.Kind != "oauth" {
		return "", fmt.Errorf("secret %s uses %s, not OAuth", id, a.Kind)
	}
	return m.oauthToken(ctx, id, a, origin, rejected)
}

// oauthToken returns the stored access token, refreshing it when it expired or equals rejected.
func (m *Manager) oauthToken(
	ctx context.Context,
	id config.ID,
	a config.Secret,
	origin, rejected string,
) (string, error) {
	k := key(id, a)
	m.resetMu.RLock()
	defer m.resetMu.RUnlock()
	lock := m.credentialLock(k)
	lock.Lock()
	defer lock.Unlock()

	saved, err := m.stored(k, id)
	if err != nil {
		return "", err
	}
	if saved.Origin != origin {
		return "", fmt.Errorf("secret %s is logged in to another server origin; run agx secret login %s", id, id)
	}
	if saved.Token.Valid() && (rejected == "" || saved.Token.AccessToken != rejected) {
		return saved.Token.AccessToken, nil
	}
	if saved.Token.RefreshToken == "" {
		return "", fmt.Errorf("secret %s expired and has no refresh token; run agx secret login %s", id, id)
	}
	m.mu.Lock()
	refused, wasRefused := m.refused[k]
	m.mu.Unlock()
	if wasRefused {
		return "", fmt.Errorf("secret %s cannot be refreshed: %s; run agx secret login %s", id, refused, id)
	}
	token, err := m.renew(ctx, k, id, saved)
	if err != nil {
		return "", err
	}
	return token.AccessToken, nil
}

// Rotate renews OAuth secret a, named id, with its refresh token now, whether or not its access
// token expired, and returns the new token's expiry, zero when the server did not say. Unlike a
// renewal on use, it tries a refresh token the server refused before, in case it accepts it now.
func (m *Manager) Rotate(ctx context.Context, id config.ID, a config.Secret) (time.Time, error) {
	if a.Kind != "oauth" {
		return time.Time{}, fmt.Errorf("secret %s uses %s; only OAuth logins can be rotated", id, a.Kind)
	}
	k := key(id, a)
	m.resetMu.RLock()
	defer m.resetMu.RUnlock()
	lock := m.credentialLock(k)
	lock.Lock()
	defer lock.Unlock()

	saved, err := m.stored(k, id)
	if err != nil {
		return time.Time{}, err
	}
	if saved.Token.RefreshToken == "" {
		return time.Time{}, fmt.Errorf("secret %s has no refresh token; run agx secret login %s", id, id)
	}
	token, err := m.renew(ctx, k, id, saved)
	if err != nil {
		return time.Time{}, err
	}
	return token.Expiry, nil
}

// stored reads the OAuth credential under key k; callers hold its lock.
func (m *Manager) stored(k string, id config.ID) (credential, error) {
	var saved credential
	data, err := m.store.Get(k)
	if errors.Is(err, errNotStored) {
		return credential{}, fmt.Errorf("secret %s needs login; run agx secret login %s: %w", id, id, ErrNoCredential)
	}
	if err != nil {
		return credential{}, fmt.Errorf("secret %s: %w", id, err)
	}
	if err := json.Unmarshal([]byte(data), &saved); err != nil || saved.Token == nil {
		return credential{}, fmt.Errorf("invalid stored OAuth credential for %s", id)
	}
	return saved, nil
}

// refreshTimeout bounds an OAuth refresh, which no caller can cancel.
const refreshTimeout = 30 * time.Second

// renew refreshes saved and stores the result under key k; callers hold the credential's lock.
// A refused refresh is recorded, so renewals on use stop sending the refresh token.
func (m *Manager) renew(ctx context.Context, k string, id config.ID, saved credential) (*oauth2.Token, error) {
	// Authorization servers that rotate refresh tokens invalidate the one sent as soon as they
	// answer, so a refresh cut off by its caller, such as a harness cancelling its MCP request,
	// would lose the only valid refresh token. It finishes and is stored whatever the caller does.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), refreshTimeout)
	defer cancel()
	token, refusal, err := m.refresh(ctx, saved)
	if err != nil {
		slog.Warn("OAuth refresh failed", "secret", id, "refused", refusal, "error", err)
		if refusal {
			m.mu.Lock()
			m.refused[k] = err.Error()
			m.mu.Unlock()
			return nil, fmt.Errorf("secret %s cannot be refreshed: %w; run agx secret login %s", id, err, id)
		}
		return nil, fmt.Errorf("secret %s: %w: %w", id, ErrRefreshFailed, err)
	}
	saved.Token = token
	data, err := json.Marshal(saved)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	delete(m.refused, k)
	m.mu.Unlock()
	if err := m.store.Set(k, string(data)); err != nil {
		return nil, fmt.Errorf("cannot persist refreshed OAuth credential for %s: %w", id, err)
	}
	return token, nil
}

func (m *Manager) refresh(ctx context.Context, saved credential) (*oauth2.Token, bool, error) {
	if err := secureURL(saved.Endpoint.TokenURL); err != nil {
		return nil, true, err
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, client)
	cfg := oauth2.Config{ClientID: saved.ClientID, Endpoint: saved.Endpoint}
	// Rotate and early server rejection force a refresh even before the saved expiry.
	token := *saved.Token
	token.Expiry = time.Now().Add(-time.Hour)
	refreshed, err := cfg.TokenSource(ctx, &token).Token()
	if err == nil {
		return refreshed, false, nil
	}
	var failure *oauth2.RetrieveError
	if errors.As(err, &failure) {
		status := failure.Response.StatusCode
		message := fmt.Sprintf("token endpoint returned HTTP %d", status)
		if failure.ErrorCode != "" {
			message += fmt.Sprintf(" (%q)", failure.ErrorCode)
		}
		return nil, status == http.StatusBadRequest || status == http.StatusUnauthorized, errors.New(message)
	}
	return nil, false, fmt.Errorf("cannot reach the token endpoint: %w", err)
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
	m.resetMu.RLock()
	defer m.resetMu.RUnlock()
	lock := m.credentialLock(k)
	lock.Lock()
	defer lock.Unlock()
	if err := m.store.Set(k, value); err != nil {
		return fmt.Errorf("secret %s was not stored: %w", id, err)
	}
	return nil
}

func (m *Manager) Logout(id config.ID, a config.Secret) error {
	if a.Kind != "keychain" && a.Kind != "oauth" {
		return fmt.Errorf("secret %s uses %s and has no stored credential to remove", id, a.Kind)
	}
	k := key(id, a)
	m.resetMu.RLock()
	defer m.resetMu.RUnlock()
	lock := m.credentialLock(k)
	lock.Lock()
	defer lock.Unlock()
	m.mu.Lock()
	delete(m.refused, k)
	for _, p := range m.pending {
		if p.SecretID == id {
			m.finishLocked(p, errors.New("logged out while authorizing"))
		}
	}
	m.mu.Unlock()
	if err := m.store.Delete(k); err != nil {
		return fmt.Errorf("secret %s was not removed: %w", id, err)
	}
	return nil
}

// Reset logs out all stored credentials, including definitions no longer configured.
func (m *Manager) Reset() error {
	m.resetMu.Lock()
	defer m.resetMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.pending {
		m.finishLocked(p, errors.New("logged out while authorizing"))
	}
	clear(m.refused)
	clear(m.required)
	return m.store.Reset()
}

// Status reports whether secret a, named id, is usable; secrets resolves the !secret reference
// of a sops secret's age key.
func (m *Manager) Status(id config.ID, a config.Secret, secrets map[config.ID]config.Secret) Status {
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
	refused, wasRefused := m.refused[k]
	m.mu.Unlock()
	switch a.Kind {
	case "command":
		s.State = "ready"
	case "sops":
		// An age key from the caller's environment exists only at launch.
		if config.Variable(secrets, a.AgeKey) != "" {
			s.State = "unconfigured"
			break
		}
		// Decrypting proves the key is available; the cache makes repeated checks cheap.
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if _, err := m.Value(ctx, id, a, secrets, nil, ""); err != nil {
			s.State = "unavailable"
			s.Problem = err.Error()
		} else {
			s.State = "ready"
		}
	case "keychain", "oauth":
		if wasRefused {
			s.State = "needs_login"
			s.Problem = "secret " + string(id) + " cannot be refreshed: " + refused
			break
		}
		data, err := m.store.Get(k)
		switch {
		case errors.Is(err, errNotStored) && a.Implicit && !required:
			// On-demand OAuth needs no login until its server asks for one.
		case errors.Is(err, errNotStored):
			s.State = "needs_login"
		case err != nil:
			s.State = "unavailable"
			s.Problem = err.Error()
		default:
			s.State = "ready"
			if a.Kind == "oauth" {
				var c credential
				if json.Unmarshal([]byte(data), &c) != nil || c.Token == nil {
					s.State = "unavailable"
				} else {
					s.Expires = c.Token.Expiry
					s.Refreshable = c.Token.RefreshToken != ""
					if !c.Token.Valid() && c.Token.RefreshToken == "" {
						s.State = "needs_login"
					}
				}
			}
		}
	}
	return s
}

func getJSON(ctx context.Context, endpoint string, out any) error {
	if err := secureURL(endpoint); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
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
	if u.Scheme == "https" || u.Scheme == "http" && loopback(u.Hostname()) {
		return nil
	}
	return fmt.Errorf("OAuth endpoints must use https, or http on a loopback address")
}

func loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
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

// keychainTimeout bounds an OS keychain call, which blocks while the keychain is locked or shows
// an access prompt, so callers get an explanation before their own deadline instead of a timeout.
var keychainTimeout = 10 * time.Second

// keychainAdvice tells how to proceed after an OS keychain failure.
const keychainAdvice = "unlock the OS keychain (or start its secret service) and retry, " +
	"or use a secret of kind command or sops instead"

func keychainGet(k string) (string, error) {
	return keychainCall(func() (string, error) { return keyring.Get("agx", k) })
}

func keychainSet(k, value string) error {
	_, err := keychainCall(func() (struct{}, error) { return struct{}{}, keyring.Set("agx", k, value) })
	return err
}

// keychainCall runs op with keychainTimeout. A call that times out keeps running in the
// background, as keychain APIs cannot be cancelled; its result is discarded.
func keychainCall[T any](op func() (T, error)) (T, error) {
	type result struct {
		value T
		err   error
	}
	done := make(chan result, 1)
	go func() {
		value, err := op()
		done <- result{value, err}
	}()
	timer := time.NewTimer(keychainTimeout)
	defer timer.Stop()
	select {
	case r := <-done:
		if r.err != nil && !errors.Is(r.err, keyring.ErrNotFound) {
			return r.value, fmt.Errorf("cannot use the OS keychain: %w; %s", r.err, keychainAdvice)
		}
		return r.value, r.err
	case <-timer.C:
		var zero T
		return zero, fmt.Errorf(
			"the OS keychain did not answer within %s; it may be locked or waiting for an access prompt: %s",
			keychainTimeout, keychainAdvice,
		)
	}
}
