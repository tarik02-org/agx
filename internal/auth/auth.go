package auth

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/zalando/go-keyring"
	"golang.org/x/oauth2"

	"github.com/tarik02/weft/internal/config"
	"github.com/tarik02/weft/internal/platform"
	"github.com/tarik02/weft/internal/securefs"
)

type Manager struct {
	mu sync.Mutex
	// pending maps OAuth state to attempts; begins holds in-flight discovery per auth ID.
	pending map[string]*Attempt
	begins  map[config.ID]*beginCall
	// locks serialize refresh, login storage and logout per credential key.
	locks map[string]*sync.Mutex
	// unpersisted holds refreshed credentials the OS keychain refused to store.
	unpersisted map[string]credential
	generation  map[string]uint64
	client      *http.Client
	callback    string
}

type Attempt struct {
	ID         string
	AuthID     config.ID
	URL        string
	Expires    time.Time
	definition config.Auth
	oauth      oauth2.Config
	verifier   string
	origin     string
	resource   string
	issuer     string
	requireIss bool
}

type beginCall struct {
	key     string
	origin  string
	done    chan struct{}
	attempt *Attempt
	err     error
}

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
}

func New(callback string) *Manager {
	return &Manager{
		pending:     map[string]*Attempt{},
		begins:      map[config.ID]*beginCall{},
		locks:       map[string]*sync.Mutex{},
		unpersisted: map[string]credential{},
		generation:  map[string]uint64{},
		client: &http.Client{
			Timeout:       20 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		},
		callback: callback,
	}
}

func key(id config.ID, a config.Auth) string {
	a.Source = config.Source{}
	return string(id) + ":" + config.Digest(a)
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
	a config.Auth,
	inputs map[string]string,
	origin string,
) (string, error) {
	switch a.Kind {
	case "environment":
		if value := inputs[a.Variable]; value != "" {
			return value, nil
		}
		return "", fmt.Errorf("missing environment variable %s", a.Variable)
	case "daemon_environment":
		if value := os.Getenv(a.Variable); value != "" {
			return value, nil
		}
		return "", fmt.Errorf("missing daemon environment variable %s", a.Variable)
	case "command":
		return runHelper(ctx, id, a.Command)
	case "keychain":
		value, err := keyring.Get("weft", key(id, a))
		if err != nil {
			return "", fmt.Errorf("keychain secret %s unavailable; use weft auth set", id)
		}
		return value, nil
	case "oauth":
		return m.oauthValue(ctx, id, a, origin)
	default:
		return "", fmt.Errorf("unsupported auth kind")
	}
}

func runHelper(ctx context.Context, id config.ID, command []string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Env = SystemEnvironment()
	platform.IsolateOwned(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("credential helper for %s failed", id)
	}
	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		return "", fmt.Errorf("credential helper for %s failed", id)
	}
	kill, err := platform.Own(cmd)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return "", fmt.Errorf("cannot supervise credential helper for %s", id)
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
		return "", fmt.Errorf("credential helper output failed or exceeded 1 MiB")
	}
	if err := wait(); err != nil {
		return "", fmt.Errorf("credential helper for %s failed", id)
	}

	value := strings.TrimSpace(string(out))
	if value == "" {
		return "", fmt.Errorf("credential helper for %s returned no credential", id)
	}
	return value, nil
}

func (m *Manager) oauthValue(ctx context.Context, id config.ID, a config.Auth, origin string) (string, error) {
	k := key(id, a)
	lock := m.credentialLock(k)
	lock.Lock()
	defer lock.Unlock()

	m.mu.Lock()
	saved, cached := m.unpersisted[k]
	m.mu.Unlock()
	if !cached {
		data, err := keyring.Get("weft", k)
		if err != nil {
			return "", fmt.Errorf("auth %s needs login", id)
		}
		if err := json.Unmarshal([]byte(data), &saved); err != nil || saved.Token == nil {
			return "", fmt.Errorf("invalid stored OAuth credential for %s", id)
		}
	}
	if saved.Origin != origin {
		return "", fmt.Errorf("auth %s is bound to a different server origin", id)
	}
	if saved.Token.Valid() {
		return saved.Token.AccessToken, nil
	}
	if saved.Token.RefreshToken == "" {
		return "", fmt.Errorf("auth %s needs login", id)
	}
	token, err := m.refresh(ctx, saved)
	if err != nil {
		return "", fmt.Errorf("auth %s refresh failed; log in again", id)
	}
	saved.Token = token
	data, err := json.Marshal(saved)
	if err != nil {
		return "", err
	}
	if err := keyring.Set("weft", k, string(data)); err != nil {
		// The refresh token may have rotated; dropping it would force a new login.
		m.mu.Lock()
		m.unpersisted[k] = saved
		m.mu.Unlock()
		slog.Warn(
			"cannot persist refreshed OAuth credential; keeping it in memory until the daemon stops",
			"auth",
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

func (m *Manager) Set(id config.ID, a config.Auth, secret []byte) error {
	if a.Kind != "keychain" {
		return fmt.Errorf("auth set requires a keychain source")
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
	if err := keyring.Set("weft", k, value); err != nil {
		return fmt.Errorf("OS keychain is unavailable: %w", err)
	}
	return nil
}

func (m *Manager) Logout(id config.ID, a config.Auth) error {
	if a.Kind != "keychain" && a.Kind != "oauth" {
		return fmt.Errorf("auth %s uses %s and has no stored credential to remove", id, a.Kind)
	}
	k := key(id, a)
	lock := m.credentialLock(k)
	lock.Lock()
	defer lock.Unlock()
	m.mu.Lock()
	m.generation[k]++
	delete(m.unpersisted, k)
	for state, p := range m.pending {
		if p.AuthID == id {
			delete(m.pending, state)
		}
	}
	m.mu.Unlock()
	if err := keyring.Delete("weft", k); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("OS keychain is unavailable: %w", err)
	}
	return nil
}

func (m *Manager) Status(id config.ID, a config.Auth) Status {
	s := Status{ID: id, State: "unconfigured"}
	k := key(id, a)
	m.mu.Lock()
	for state, p := range m.pending {
		if time.Now().After(p.Expires) {
			delete(m.pending, state)
			continue
		}
		if p.AuthID == id {
			if key(id, p.definition) != k {
				delete(m.pending, state)
				continue
			}
			s.State = "pending"
			s.Expires = p.Expires
			m.mu.Unlock()
			return s
		}
	}
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
	case "keychain", "oauth":
		if inMemory {
			s.State = "ready"
			s.Expires = memory.Token.Expiry
			break
		}
		data, err := keyring.Get("weft", k)
		switch {
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

// Begin shares one pending attempt per auth ID, including concurrent callers
// that arrive while discovery is still running.
func (m *Manager) Begin(ctx context.Context, id config.ID, a config.Auth, serverURL string) (*Attempt, error) {
	if a.Kind != "oauth" {
		return nil, fmt.Errorf("connection does not use OAuth")
	}
	origin, err := Origin(serverURL)
	if err != nil {
		return nil, err
	}
	k := key(id, a)
	m.mu.Lock()
	for state, p := range m.pending {
		if time.Now().After(p.Expires) {
			delete(m.pending, state)
			continue
		}
		if p.AuthID == id {
			if key(id, p.definition) != k {
				delete(m.pending, state)
				continue
			}
			m.mu.Unlock()
			if p.origin != origin {
				return nil, fmt.Errorf("OAuth login already pending for another server")
			}
			return p, nil
		}
	}
	if call := m.begins[id]; call != nil {
		m.mu.Unlock()
		if call.key != k || call.origin != origin {
			return nil, fmt.Errorf("OAuth login already pending for another server")
		}
		select {
		case <-call.done:
			return call.attempt, call.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	call := &beginCall{key: k, origin: origin, done: make(chan struct{})}
	m.begins[id] = call
	m.mu.Unlock()

	// Waiters share this attempt, so one caller cancelling must not fail the rest.
	beginCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
	defer cancel()
	p, state, err := m.begin(beginCtx, id, a, serverURL, origin)
	m.mu.Lock()
	delete(m.begins, id)
	if err == nil {
		m.pending[state] = p
	}
	m.mu.Unlock()
	call.attempt, call.err = p, err
	close(call.done)
	return p, err
}

func (m *Manager) begin(
	ctx context.Context,
	id config.ID,
	a config.Auth,
	serverURL, origin string,
) (*Attempt, string, error) {
	issuer := a.Issuer
	resource := a.Resource
	if resource == "" {
		resource = serverURL
	}
	if issuer == "" {
		var metadata struct {
			AuthorizationServers []string `json:"authorization_servers"`
			Resource             string   `json:"resource"`
		}
		u, _ := url.Parse(serverURL)
		endpoint := origin + "/.well-known/oauth-protected-resource" + strings.TrimRight(u.Path, "/")
		if err := m.getJSON(ctx, endpoint, &metadata); err != nil {
			return nil, "", fmt.Errorf("OAuth resource discovery failed; configure issuer explicitly")
		}
		// RFC 9728 section 3.3: metadata for another resource must not be used.
		advertised, err := normalizeResource(metadata.Resource)
		if err != nil {
			return nil, "", fmt.Errorf("protected resource metadata has no valid resource")
		}
		requested, err := normalizeResource(serverURL)
		if err != nil || advertised != requested {
			return nil, "", fmt.Errorf("protected resource metadata is for a different resource")
		}
		if len(metadata.AuthorizationServers) == 0 {
			return nil, "", fmt.Errorf("resource does not advertise an authorization server")
		}
		issuer = metadata.AuthorizationServers[0]
		if a.Resource == "" {
			resource = metadata.Resource
		}
	}
	if err := secureURL(issuer); err != nil {
		return nil, "", err
	}
	var metadata struct {
		Issuer        string `json:"issuer"`
		Authorization string `json:"authorization_endpoint"`
		Token         string `json:"token_endpoint"`
		Registration  string `json:"registration_endpoint"`
		IssParameter  bool   `json:"authorization_response_iss_parameter_supported"`
	}
	u, _ := url.Parse(issuer)
	discovery := u.Scheme + "://" + u.Host + "/.well-known/oauth-authorization-server" + strings.TrimRight(u.Path, "/")
	if err := m.getJSON(ctx, discovery, &metadata); err != nil {
		if err := m.getJSON(
			ctx,
			strings.TrimRight(issuer, "/")+"/.well-known/openid-configuration",
			&metadata,
		); err != nil {
			return nil, "", fmt.Errorf("OAuth authorization server discovery failed")
		}
	}
	if metadata.Issuer != issuer {
		return nil, "", fmt.Errorf("OAuth issuer mismatch")
	}
	if err := secureURL(metadata.Authorization); err != nil {
		return nil, "", err
	}
	if err := secureURL(metadata.Token); err != nil {
		return nil, "", err
	}
	clientID := a.ClientID
	if clientID == "" {
		if err := secureURL(metadata.Registration); err != nil {
			return nil, "", fmt.Errorf("OAuth server requires a configured public client_id")
		}
		body := map[string]any{
			"client_name":                "weft",
			"redirect_uris":              []string{m.callback},
			"token_endpoint_auth_method": "none",
			"grant_types":                []string{"authorization_code", "refresh_token"},
			"response_types":             []string{"code"},
		}
		data, _ := json.Marshal(body)
		req, _ := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			metadata.Registration,
			strings.NewReader(string(data)),
		)
		req.Header.Set("Content-Type", "application/json")
		resp, err := m.client.Do(req)
		if err != nil {
			return nil, "", fmt.Errorf("OAuth registration failed")
		}
		defer func() { _ = resp.Body.Close() }()
		var registration struct {
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
		}
		if resp.StatusCode >= http.StatusMultipleChoices ||
			json.NewDecoder(io.LimitReader(resp.Body, 1024*1024)).Decode(&registration) != nil ||
			registration.ClientID == "" ||
			registration.ClientSecret != "" {
			return nil, "", fmt.Errorf("OAuth public-client registration failed")
		}
		clientID = registration.ClientID
	}
	verifier := oauth2.GenerateVerifier()
	state := securefs.Token()
	cfg := oauth2.Config{
		ClientID:    clientID,
		RedirectURL: m.callback,
		Scopes:      a.Scopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:   metadata.Authorization,
			TokenURL:  metadata.Token,
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}
	p := &Attempt{
		ID:         securefs.Token(),
		AuthID:     id,
		Expires:    time.Now().Add(5 * time.Minute),
		definition: a,
		oauth:      cfg,
		verifier:   verifier,
		origin:     origin,
		resource:   resource,
		issuer:     metadata.Issuer,
		requireIss: metadata.IssParameter,
	}
	p.URL = cfg.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oauth2.SetAuthURLParam("resource", resource))
	return p, state, nil
}

func normalizeResource(value string) (string, error) {
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return "", fmt.Errorf("invalid resource")
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	defaultPort := (scheme == "https" && port == "443") || (scheme == "http" && port == "80")
	if port != "" && !defaultPort {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return scheme + "://" + host + strings.TrimRight(u.EscapedPath(), "/"), nil
}

func (m *Manager) Callback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	query := r.URL.Query()
	state := query.Get("state")
	m.mu.Lock()
	p := m.pending[state]
	if p == nil || time.Now().After(p.Expires) {
		m.mu.Unlock()
		http.Error(w, "invalid or expired OAuth attempt", http.StatusBadRequest)
		return
	}
	delete(m.pending, state)
	k := key(p.AuthID, p.definition)
	generation := m.generation[k]
	m.mu.Unlock()
	// RFC 9207: reject responses from a mixed-up authorization server.
	if iss := query.Get("iss"); (p.requireIss || iss != "") && iss != p.issuer {
		http.Error(w, "OAuth issuer mismatch", http.StatusBadRequest)
		return
	}
	if query.Get("error") != "" || query.Get("code") == "" {
		http.Error(w, "authorization was not completed", http.StatusBadRequest)
		return
	}
	ctx := context.WithValue(r.Context(), oauth2.HTTPClient, m.client)
	token, err := p.oauth.Exchange(
		ctx,
		query.Get("code"),
		oauth2.VerifierOption(p.verifier),
		oauth2.SetAuthURLParam("resource", p.resource),
	)
	if err != nil {
		http.Error(w, "OAuth token exchange failed", http.StatusBadRequest)
		return
	}
	data, err := json.Marshal(
		credential{
			Token:    token,
			Endpoint: p.oauth.Endpoint,
			ClientID: p.oauth.ClientID,
			Origin:   p.origin,
			Resource: p.resource,
		},
	)
	if err != nil {
		http.Error(w, "cannot encode credential", http.StatusInternalServerError)
		return
	}
	lock := m.credentialLock(k)
	lock.Lock()
	defer lock.Unlock()
	m.mu.Lock()
	loggedOut := m.generation[k] != generation
	m.mu.Unlock()
	if loggedOut {
		http.Error(w, "logged out while authorizing; start the login again", http.StatusConflict)
		return
	}
	if err := keyring.Set("weft", k, string(data)); err != nil {
		http.Error(w, "OS keychain is unavailable; credential was not stored", http.StatusServiceUnavailable)
		return
	}
	m.mu.Lock()
	delete(m.unpersisted, k)
	m.mu.Unlock()
	_, _ = fmt.Fprintln(w, "Authorized. You can close this window.")
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
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1") {
		return nil
	}
	return fmt.Errorf("OAuth endpoints must use HTTPS or loopback HTTP")
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
