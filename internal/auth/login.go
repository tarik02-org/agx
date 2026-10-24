package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/securefs"
)

var (
	ErrLoginExpired   = errors.New("the login link expired")
	ErrLoginCancelled = errors.New("login cancelled")
)

// Attempt is a pending OAuth authorization shared by every login waiting for the same secret.
type Attempt struct {
	ID         string
	SecretID   config.ID
	URL        string
	Expires    time.Time
	state      string
	definition config.Secret
	oauth      oauth2.Config
	verifier   string
	origin     string
	resource   string
	issuer     string
	requireIss bool
	// done closes once the attempt finishes with err; waiters counts logins that still wait
	// for it. All three are guarded by Manager.mu.
	done    chan struct{}
	err     error
	waiters int
}

type beginCall struct {
	key     string
	origin  string
	done    chan struct{}
	attempt *Attempt
	err     error
	// waiters counts callers that joined while discovery ran; guarded by Manager.mu.
	waiters int
}

type resourceMetadata struct {
	AuthorizationServers []string `json:"authorization_servers"`
	Resource             string   `json:"resource"`
}

// Begin starts an OAuth login, or joins the one already pending for the secret, including
// while its discovery still runs. Every successful call must be paired with Release; the
// attempt is cancelled when its last waiter releases it before it finishes.
func (m *Manager) Begin(ctx context.Context, id config.ID, a config.Secret, serverURL string) (*Attempt, error) {
	if a.Kind != "oauth" {
		return nil, fmt.Errorf("secret %s uses %s, not OAuth", id, a.Kind)
	}
	origin, err := Origin(serverURL)
	if err != nil {
		return nil, err
	}
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
		if p.origin != origin {
			m.mu.Unlock()
			return nil, fmt.Errorf("OAuth login already pending for another server")
		}
		p.waiters++
		m.mu.Unlock()
		return p, nil
	}
	if call := m.begins[id]; call != nil {
		if call.key != k || call.origin != origin {
			m.mu.Unlock()
			return nil, fmt.Errorf("OAuth login already pending for another server")
		}
		call.waiters++
		m.mu.Unlock()
		select {
		case <-call.done:
			return call.attempt, call.err
		case <-ctx.Done():
			m.mu.Lock()
			select {
			case <-call.done:
				m.mu.Unlock()
				if call.err == nil {
					m.Release(call.attempt)
				}
			default:
				call.waiters--
				m.mu.Unlock()
			}
			return nil, ctx.Err()
		}
	}
	call := &beginCall{key: k, origin: origin, done: make(chan struct{})}
	m.begins[id] = call
	m.mu.Unlock()

	// Waiters share this attempt, so one caller cancelling must not fail the rest.
	beginCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
	defer cancel()
	p, err := m.begin(beginCtx, id, a, serverURL, origin)
	m.mu.Lock()
	delete(m.begins, id)
	if err == nil {
		p.waiters += call.waiters
		m.pending[p.state] = p
	}
	call.attempt, call.err = p, err
	close(call.done)
	m.mu.Unlock()
	if err == nil && ctx.Err() != nil {
		m.Release(p)
		return nil, ctx.Err()
	}
	return p, err
}

// Wait blocks until the attempt finishes, expires, or ctx ends, and returns its outcome.
func (m *Manager) Wait(ctx context.Context, p *Attempt) error {
	timer := time.NewTimer(time.Until(p.Expires))
	defer timer.Stop()
	select {
	case <-p.done:
	case <-timer.C:
		m.mu.Lock()
		m.finishLocked(p, ErrLoginExpired)
		m.mu.Unlock()
	case <-ctx.Done():
		return ctx.Err()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return p.err
}

// Release ends one caller's interest in an attempt; the last one cancels it, so its callback
// is then rejected and its pending state freed.
func (m *Manager) Release(p *Attempt) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p.waiters--
	if p.waiters <= 0 {
		m.finishLocked(p, ErrLoginCancelled)
	}
}

// finishLocked must be called with m.mu held; later outcomes of a finished attempt are ignored.
func (m *Manager) finishLocked(p *Attempt, err error) {
	select {
	case <-p.done:
		return
	default:
	}
	if m.pending[p.state] == p {
		delete(m.pending, p.state)
	}
	p.err = err
	close(p.done)
}

// expireLocked must be called with m.mu held.
func (m *Manager) expireLocked(now time.Time) {
	for _, p := range m.pending {
		if now.After(p.Expires) {
			m.finishLocked(p, ErrLoginExpired)
		}
	}
}

// RequireLogin records that serverURL rejected a request without a usable OAuth credential,
// once the server proves it supports OAuth discovery (MCP authorization): it advertises
// protected resource metadata, through the challenge's resource_metadata parameter or the
// well-known location. The metadata location is kept for the login that follows.
func (m *Manager) RequireLogin(ctx context.Context, id config.ID, a config.Secret, serverURL, challenge string) error {
	hint := resourceMetadataURL(challenge)
	resource, err := normalizeResource(serverURL)
	if err != nil {
		return err
	}
	k := key(id, a)
	m.mu.Lock()
	known := m.required[k] && m.hints[resource] == hint
	m.mu.Unlock()
	if !known {
		if _, err := m.protectedResource(ctx, serverURL, hint); err != nil {
			return err
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.required[k] = true
	if hint != "" {
		m.hints[resource] = hint
	} else {
		delete(m.hints, resource)
	}
	return nil
}

// protectedResource fetches RFC 9728 metadata for serverURL from hint, when the server sent
// one in a 401 challenge, then from the well-known locations, path-specific before root.
// Metadata describing another resource is never used (RFC 9728 section 3.3).
func (m *Manager) protectedResource(ctx context.Context, serverURL, hint string) (resourceMetadata, error) {
	requested, err := normalizeResource(serverURL)
	if err != nil {
		return resourceMetadata{}, err
	}
	u, _ := url.Parse(serverURL)
	origin := u.Scheme + "://" + u.Host
	path := strings.TrimRight(u.Path, "/")
	type candidate struct {
		endpoint string
		accepted []string
	}
	candidates := []candidate{}
	if hint != "" {
		candidates = append(candidates, candidate{hint, []string{requested}})
	}
	candidates = append(
		candidates,
		candidate{origin + "/.well-known/oauth-protected-resource" + path, []string{requested}},
	)
	if path != "" {
		// Root metadata describes the origin; servers commonly list their full URL there too.
		root, _ := normalizeResource(origin)
		candidates = append(
			candidates,
			candidate{origin + "/.well-known/oauth-protected-resource", []string{root, requested}},
		)
	}
	failure := errors.New("OAuth resource discovery failed; configure issuer explicitly")
	for _, c := range candidates {
		var metadata resourceMetadata
		if err := m.getJSON(ctx, c.endpoint, &metadata); err != nil {
			continue
		}
		advertised, err := normalizeResource(metadata.Resource)
		switch {
		case err != nil:
			failure = errors.New("protected resource metadata has no valid resource")
		case !slices.Contains(c.accepted, advertised):
			failure = errors.New("protected resource metadata is for a different resource")
		case len(metadata.AuthorizationServers) == 0:
			failure = errors.New("resource does not advertise an authorization server")
		default:
			return metadata, nil
		}
	}
	return resourceMetadata{}, failure
}

// challenge asks the server for the resource_metadata of its 401 challenge, as a client
// without a credential would meet it, for logins before any request saw one. A GET opens no
// MCP session; "" means the server sent none.
func (m *Manager) challenge(ctx context.Context, serverURL string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := m.client.Do(req)
	if err != nil {
		return ""
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		return ""
	}
	return resourceMetadataURL(strings.Join(resp.Header.Values("WWW-Authenticate"), ", "))
}

// NeedsLogin reports whether serverURL asks for an OAuth login the way on-demand OAuth
// recognizes one: it answers a request without a credential with 401 and advertises protected
// resource metadata. Only reaching the server is an error; any other answer means no login.
func NeedsLogin(ctx context.Context, serverURL string) (bool, error) {
	m := New("")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := m.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("cannot reach %s", serverURL)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		return false, nil
	}
	hint := resourceMetadataURL(strings.Join(resp.Header.Values("WWW-Authenticate"), ", "))
	_, err = m.protectedResource(ctx, serverURL, hint)
	return err == nil, nil
}

// resourceMetadataURL returns the resource_metadata parameter of a WWW-Authenticate Bearer
// challenge (RFC 9728 section 5.1), or "".
func resourceMetadataURL(challenge string) string {
	lower := strings.ToLower(challenge)
	for from := 0; ; {
		i := strings.Index(lower[from:], "resource_metadata=")
		if i < 0 {
			return ""
		}
		i += from
		from = i + len("resource_metadata=")
		if i > 0 && !strings.ContainsRune(" ,\t", rune(lower[i-1])) {
			continue
		}
		value := challenge[from:]
		if quoted, ok := strings.CutPrefix(value, `"`); ok {
			end := strings.IndexByte(quoted, '"')
			if end < 0 {
				return ""
			}
			return quoted[:end]
		}
		if end := strings.IndexAny(value, ", \t"); end >= 0 {
			value = value[:end]
		}
		return value
	}
}

func (m *Manager) begin(
	ctx context.Context,
	id config.ID,
	a config.Secret,
	serverURL, origin string,
) (*Attempt, error) {
	issuer := a.Issuer
	resource := a.Resource
	if resource == "" {
		resource = serverURL
	}
	if issuer == "" {
		normalized, _ := normalizeResource(serverURL)
		m.mu.Lock()
		hint := m.hints[normalized]
		m.mu.Unlock()
		if hint == "" {
			hint = m.challenge(ctx, serverURL)
		}
		metadata, err := m.protectedResource(ctx, serverURL, hint)
		if err != nil {
			return nil, err
		}
		issuer = metadata.AuthorizationServers[0]
		if a.Resource == "" {
			resource = metadata.Resource
		}
	}
	if err := secureURL(issuer); err != nil {
		return nil, err
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
			return nil, fmt.Errorf("OAuth authorization server discovery failed")
		}
	}
	if metadata.Issuer != issuer {
		return nil, fmt.Errorf("OAuth issuer mismatch")
	}
	if err := secureURL(metadata.Authorization); err != nil {
		return nil, err
	}
	if err := secureURL(metadata.Token); err != nil {
		return nil, err
	}
	clientID := a.ClientID
	if clientID == "" {
		if err := secureURL(metadata.Registration); err != nil {
			return nil, clientIDRequired(id, a, issuer, m.callback)
		}
		body := map[string]any{
			"client_name":                "agx",
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
			return nil, fmt.Errorf("OAuth registration failed")
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
			return nil, fmt.Errorf("OAuth public-client registration failed")
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
		SecretID:   id,
		state:      state,
		done:       make(chan struct{}),
		waiters:    1,
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
	return p, nil
}

// clientIDRequired explains how to configure a client ID for an authorization server that does
// not support dynamic client registration. Providers that also require a client secret, such as
// GitHub, cannot work with a public client, so a stored token is the alternative.
func clientIDRequired(id config.ID, a config.Secret, issuer, callback string) error {
	where := "secret " + string(id) + ": {kind: oauth, client_id: <client-id>}"
	if a.Owner != "" {
		where = "the auth of MCP server " + string(a.Owner) + ": auth: {kind: oauth, client_id: <client-id>}"
	}
	if a.Source.Path != "" {
		where += fmt.Sprintf(" (defined at %s:%d)", a.Source.Path, a.Source.Line)
	}
	keychain := "auth: keychain"
	if a.Owner == "" {
		keychain = "kind: keychain"
	}
	return fmt.Errorf(
		"OAuth server %s does not register clients automatically, so %s needs a client ID: "+
			"create an OAuth app with the provider, allowing the redirect URI %s, and set its public client ID in %s. "+
			"agx logs in as a public client without a client secret; for a provider that requires one, such as GitHub, "+
			"use a personal access token instead: set %s and store it with agx secret set %s --stdin",
		issuer, id, callback, where, keychain, id,
	)
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

// Callback completes the attempt named by the state parameter. Every outcome finishes the
// attempt, so the waiting login reports it; a state that is unknown, finished, or expired
// changes nothing.
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
	m.expireLocked(time.Now())
	p := m.pending[state]
	if p == nil {
		m.mu.Unlock()
		http.Error(
			w,
			"This login is no longer pending: it finished, expired, or was cancelled. Run agx secret login again.",
			http.StatusBadRequest,
		)
		return
	}
	// The state is single-use, even when the exchange below fails.
	delete(m.pending, state)
	k := key(p.SecretID, p.definition)
	generation := m.generation[k]
	m.mu.Unlock()
	fail := func(status int, err error) {
		m.mu.Lock()
		m.finishLocked(p, err)
		m.mu.Unlock()
		http.Error(w, "Login failed: "+err.Error()+".", status)
	}
	// RFC 9207: reject responses from a mixed-up authorization server.
	if iss := query.Get("iss"); (p.requireIss || iss != "") && iss != p.issuer {
		fail(http.StatusBadRequest, errors.New("OAuth issuer mismatch"))
		return
	}
	if query.Get("error") != "" || query.Get("code") == "" {
		fail(http.StatusBadRequest, errors.New("authorization was not completed"))
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
		fail(http.StatusBadRequest, errors.New("OAuth token exchange failed"))
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
		fail(http.StatusInternalServerError, errors.New("cannot encode credential"))
		return
	}
	lock := m.credentialLock(k)
	lock.Lock()
	defer lock.Unlock()
	m.mu.Lock()
	loggedOut := m.generation[k] != generation
	cancelled := false
	select {
	case <-p.done:
		cancelled = true
	default:
	}
	m.mu.Unlock()
	if cancelled {
		http.Error(w, "Login was cancelled; the credential was not stored.", http.StatusConflict)
		return
	}
	if loggedOut {
		fail(http.StatusConflict, errors.New("logged out while authorizing; run the login again"))
		return
	}
	if err := keychainSet(k, string(data)); err != nil {
		fail(http.StatusServiceUnavailable, fmt.Errorf("the credential was not stored: %w", err))
		return
	}
	m.mu.Lock()
	delete(m.unpersisted, k)
	delete(m.required, k)
	delete(m.refused, k)
	m.finishLocked(p, nil)
	m.mu.Unlock()
	_, _ = fmt.Fprintf(w, "Logged in to %s. You can close this window.\n", p.SecretID)
}
