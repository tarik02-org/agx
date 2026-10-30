package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"

	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/securefs"
)

var (
	ErrLoginExpired   = errors.New("the login link expired")
	ErrLoginCancelled = errors.New("login cancelled")
)

// Attempt is one pending OAuth authorization.
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
	done       chan struct{}
	err        error
	cancel     context.CancelFunc
	exchanging bool
}

// Begin starts a login and cancels the previous attempt for the same secret.
func (m *Manager) Begin(ctx context.Context, id config.ID, a config.Secret, serverURL string) (*Attempt, error) {
	if a.Kind != "oauth" {
		return nil, fmt.Errorf("secret %s uses %s, not OAuth", id, a.Kind)
	}
	origin, err := Origin(serverURL)
	if err != nil {
		return nil, err
	}
	beginCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	p := &Attempt{
		ID: securefs.Token(), SecretID: id, state: securefs.Token(), definition: a,
		origin: origin, Expires: time.Now().Add(5 * time.Minute), done: make(chan struct{}), cancel: cancel,
	}
	m.resetMu.RLock()
	m.mu.Lock()
	m.expireLocked(time.Now())
	for _, old := range m.pending {
		if old.SecretID == id {
			m.finishLocked(old, ErrLoginCancelled)
		}
	}
	m.pending[p.state] = p
	m.mu.Unlock()
	m.resetMu.RUnlock()
	prepared, err := m.begin(beginCtx, id, a, serverURL, origin)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pending[p.state] != p {
		return nil, p.err
	}
	if err != nil {
		m.finishLocked(p, err)
		return nil, err
	}
	prepared.ID, prepared.state, prepared.done, prepared.cancel = p.ID, p.state, p.done, cancel
	prepared.URL = prepared.oauth.AuthCodeURL(
		p.state,
		oauth2.S256ChallengeOption(prepared.verifier),
		oauth2.SetAuthURLParam("resource", prepared.resource),
	)
	*p = *prepared
	return p, nil
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

// Cancel ends the attempt and rejects later callbacks.
func (m *Manager) Cancel(p *Attempt) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.finishLocked(p, ErrLoginCancelled)
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
	if p.cancel != nil {
		p.cancel()
	}
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
		if _, err := protectedResource(ctx, serverURL, hint); err != nil {
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
func protectedResource(
	ctx context.Context,
	serverURL, hint string,
) (oauthex.ProtectedResourceMetadata, error) {
	requested, err := normalizeResource(serverURL)
	if err != nil {
		return oauthex.ProtectedResourceMetadata{}, err
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
		var metadata oauthex.ProtectedResourceMetadata
		if err := getJSON(ctx, c.endpoint, &metadata); err != nil {
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
	return oauthex.ProtectedResourceMetadata{}, failure
}

// challenge requests serverURL without a credential, as on-demand OAuth meets it, and returns
// whether it answered 401 and the resource_metadata of its challenge, "" when it sent none. A GET
// opens no MCP session.
func challenge(ctx context.Context, serverURL string) (unauthorized bool, hint string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL, nil)
	if err != nil {
		return false, "", err
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := client.Do(req)
	if err != nil {
		return false, "", fmt.Errorf("cannot reach %s", serverURL)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		return false, "", nil
	}
	return true, resourceMetadataURL(resp.Header.Values("WWW-Authenticate")...), nil
}

// NeedsLogin reports whether serverURL asks for an OAuth login the way on-demand OAuth
// recognizes one: it answers a request without a credential with 401 and advertises protected
// resource metadata. Only reaching the server is an error; any other answer means no login.
func NeedsLogin(ctx context.Context, serverURL string) (bool, error) {
	unauthorized, hint, err := challenge(ctx, serverURL)
	if err != nil || !unauthorized {
		return false, err
	}
	_, err = protectedResource(ctx, serverURL, hint)
	return err == nil, nil
}

// resourceMetadataURL returns the resource_metadata parameter of a WWW-Authenticate challenge
// (RFC 9728 section 5.1), or "".
func resourceMetadataURL(headers ...string) string {
	challenges, _ := oauthex.ParseWWWAuthenticate(headers)
	for _, c := range challenges {
		if hint := c.Params["resource_metadata"]; hint != "" {
			return hint
		}
	}
	return ""
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
			_, hint, _ = challenge(ctx, serverURL)
		}
		metadata, err := protectedResource(ctx, serverURL, hint)
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
	metadata, err := m.authorizationServer(ctx, issuer)
	if err != nil {
		return nil, err
	}
	if err := secureURL(metadata.AuthorizationEndpoint); err != nil {
		return nil, err
	}
	if err := secureURL(metadata.TokenEndpoint); err != nil {
		return nil, err
	}
	clientID := a.ClientID
	if clientID == "" {
		if err := secureURL(metadata.RegistrationEndpoint); err != nil {
			return nil, clientIDRequired(id, a, issuer, m.callback)
		}
		registration, err := oauthex.RegisterClient(
			ctx,
			metadata.RegistrationEndpoint,
			&oauthex.ClientRegistrationMetadata{
				ClientName:              "agx",
				RedirectURIs:            []string{m.callback},
				TokenEndpointAuthMethod: "none",
				GrantTypes:              []string{"authorization_code", "refresh_token"},
				ResponseTypes:           []string{"code"},
			},
			client,
		)
		if err != nil || registration.ClientSecret != "" {
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
			AuthURL:   metadata.AuthorizationEndpoint,
			TokenURL:  metadata.TokenEndpoint,
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}
	p := &Attempt{
		ID:         securefs.Token(),
		SecretID:   id,
		state:      state,
		done:       make(chan struct{}),
		Expires:    time.Now().Add(5 * time.Minute),
		definition: a,
		oauth:      cfg,
		verifier:   verifier,
		origin:     origin,
		resource:   resource,
		issuer:     metadata.Issuer,
		requireIss: metadata.AuthorizationResponseIssParameterSupported,
	}
	p.URL = cfg.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oauth2.SetAuthURLParam("resource", resource))
	return p, nil
}

// authorizationServer fetches RFC 8414 metadata for issuer from the locations the MCP
// authorization spec lists: OAuth with path insertion, then OpenID Connect with path insertion and
// appending. oauthex.GetAuthServerMeta would refuse servers that omit
// code_challenge_methods_supported and servers on private network addresses.
func (m *Manager) authorizationServer(ctx context.Context, issuer string) (oauthex.AuthServerMeta, error) {
	u, _ := url.Parse(issuer)
	root, path := u.Scheme+"://"+u.Host, strings.TrimRight(u.Path, "/")
	for _, endpoint := range slices.Compact([]string{
		root + "/.well-known/oauth-authorization-server" + path,
		root + "/.well-known/openid-configuration" + path,
		root + path + "/.well-known/openid-configuration",
	}) {
		var metadata oauthex.AuthServerMeta
		if getJSON(ctx, endpoint, &metadata) != nil {
			continue
		}
		if metadata.Issuer != issuer {
			return metadata, fmt.Errorf("OAuth issuer mismatch")
		}
		return metadata, nil
	}
	return oauthex.AuthServerMeta{}, fmt.Errorf("OAuth authorization server discovery failed")
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
			"use a personal access token instead: set %s and store it with agx secret set %s",
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
	if p == nil || p.exchanging {
		m.mu.Unlock()
		http.Error(
			w,
			"This login is no longer pending: it finished, expired, or was cancelled. Run agx secret login again.",
			http.StatusBadRequest,
		)
		return
	}
	// Keep it pending while exchanging so a newer login or logout can cancel it.
	p.exchanging = true
	k := key(p.SecretID, p.definition)
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
	ctx := context.WithValue(r.Context(), oauth2.HTTPClient, client)
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
		},
	)
	if err != nil {
		fail(http.StatusInternalServerError, errors.New("cannot encode credential"))
		return
	}
	m.resetMu.RLock()
	defer m.resetMu.RUnlock()
	lock := m.credentialLock(k)
	lock.Lock()
	defer lock.Unlock()
	m.mu.Lock()
	cancelled := false
	select {
	case <-p.done:
		cancelled = true
	default:
	}
	if cancelled {
		m.mu.Unlock()
		http.Error(w, "Login was cancelled; the credential was not stored.", http.StatusConflict)
		return
	}
	if err := m.store.Set(k, string(data)); err != nil {
		m.mu.Unlock()
		fail(http.StatusServiceUnavailable, fmt.Errorf("the credential was not stored: %w", err))
		return
	}
	delete(m.required, k)
	delete(m.refused, k)
	m.finishLocked(p, nil)
	m.mu.Unlock()
	_, _ = fmt.Fprintf(w, "Logged in to %s. You can close this window.\n", p.SecretID)
}
