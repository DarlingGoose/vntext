package liveclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ErrUnauthorized means the server rejected the credential. The message says
// what to do next, because this is the error a first-time user sees.
var ErrUnauthorized = errors.New("the live capture credential was rejected")

// APIError is a non-2xx response.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("live API returned %d", e.Status)
	}
	return fmt.Sprintf("live API returned %d: %s", e.Status, e.Message)
}

// Endpoints is the resolved set of URLs for one API deployment.
type Endpoints struct {
	APIBase   string
	TokenURL  string
	StatusURL string
	CaptureWS string
}

// Token is a capture token as listed by the API. The secret itself is never
// returned by the list endpoint.
type Token struct {
	ID          string     `json:"id"`
	OwnerUserID string     `json:"owner_user_id,omitempty"`
	Name        string     `json:"name"`
	Prefix      string     `json:"prefix"`
	Scopes      string     `json:"scopes,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at,omitempty"`
}

// StatusSummary mirrors GET /live/status.
type StatusSummary struct {
	Sessions []StatusSession `json:"sessions"`
	Totals   StatusTotals    `json:"totals"`
}

type StatusTotals struct {
	Sessions   int `json:"sessions"`
	Publishing int `json:"publishing"`
	Watching   int `json:"watching"`
}

type StatusSession struct {
	SessionID   string          `json:"session_id"`
	ProjectID   string          `json:"project_id"`
	ProjectName string          `json:"project_name,omitempty"`
	GameName    string          `json:"game_name"`
	Status      string          `json:"status"`
	StartedAt   time.Time       `json:"started_at"`
	LineCount   int64           `json:"line_count"`
	LastSeq     int64           `json:"last_seq"`
	LastLineAt  *time.Time      `json:"last_line_at,omitempty"`
	Publishing  PublisherStatus `json:"publishing"`
	Watching    WatcherStatus   `json:"watching"`
}

type PublisherStatus struct {
	Connected   bool       `json:"connected"`
	DeviceName  string     `json:"device_name,omitempty"`
	Client      string     `json:"client,omitempty"`
	ConnectedAt *time.Time `json:"connected_at,omitempty"`
	LastSeen    *time.Time `json:"last_seen,omitempty"`
	IdleSeconds int        `json:"idle_seconds"`
}

type WatcherStatus struct {
	Count     int  `json:"count"`
	Connected bool `json:"connected"`
}

// Client performs the non-streaming API calls: discovery, OAuth, token
// management and presence.
type Client struct {
	cfg Config
	// HTTP is injectable for tests; nil means a 15 second client.
	HTTP *http.Client
	// Name is used when minting a token ("vntext on <host>").
	Name string

	mu        sync.Mutex
	endpoints *Endpoints
}

func NewClient(cfg Config) *Client {
	return &Client{cfg: cfg.normalized(), Name: defaultTokenName()}
}

func (c *Client) Config() Config {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cfg
}

// SetConfig replaces the runtime config, e.g. after a token was minted or
// pasted interactively.
func (c *Client) SetConfig(cfg Config) { c.setConfig(cfg) }

func (c *Client) setConfig(cfg Config) {
	c.mu.Lock()
	c.cfg = cfg.normalized()
	c.endpoints = nil
	c.mu.Unlock()
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func defaultTokenName() string {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		return "vntext"
	}
	name := "vntext on " + host
	// The API rejects names over 120 runes; a long hostname should not turn
	// first-run setup into an error.
	if runes := []rune(name); len(runes) > 120 {
		name = string(runes[:120])
	}
	return name
}

// Endpoints resolves the API base. When the token URL is path-prefixed (the
// common case is "/api/token"), the base candidates are probed with
// GET /live/status; a 404 means "wrong base", 401/403/200 means "right base".
func (c *Client) Endpoints(ctx context.Context) (Endpoints, error) {
	c.mu.Lock()
	if c.endpoints != nil {
		endpoints := *c.endpoints
		c.mu.Unlock()
		return endpoints, nil
	}
	c.mu.Unlock()

	cfg := c.Config()
	candidates, err := cfg.apiCandidates()
	if err != nil {
		return Endpoints{}, err
	}
	if len(candidates) == 0 {
		return Endpoints{}, fmt.Errorf("no API URL configured")
	}

	base := candidates[0]
	if len(candidates) > 1 {
		chosen, probeErr := c.probeCandidates(ctx, candidates)
		if probeErr != nil {
			return Endpoints{}, probeErr
		}
		if chosen != "" {
			base = chosen
		}
	}

	tokenURL := cfg.TokenURL
	if tokenURL == "" {
		tokenURL = base + "/token"
	}
	captureWS, err := cfg.captureWS(base)
	if err != nil {
		return Endpoints{}, err
	}
	endpoints := Endpoints{
		APIBase:   base,
		TokenURL:  tokenURL,
		StatusURL: base + "/live/status",
		CaptureWS: captureWS,
	}

	c.mu.Lock()
	c.endpoints = &endpoints
	c.mu.Unlock()
	return endpoints, nil
}

func (c *Client) probeCandidates(ctx context.Context, candidates []string) (string, error) {
	var last404 bool
	for _, candidate := range candidates {
		code, err := c.probeStatus(ctx, candidate+"/live/status")
		if err != nil {
			// Network broken: do not guess further. Returning the first
			// candidate gives the actual request a more useful error.
			return candidates[0], nil
		}
		switch code {
		case http.StatusNotFound:
			last404 = true
			continue
		default:
			return candidate, nil
		}
	}
	if last404 {
		return "", fmt.Errorf("could not find the live API under %s (all returned 404); set JPN_LEARNING_API_URL or pass --live-api", strings.Join(candidates, ", "))
	}
	return candidates[0], nil
}

func (c *Client) probeStatus(ctx context.Context, statusURL string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
	if err != nil {
		return 0, err
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return resp.StatusCode, nil
}

// ExchangeClientCredentials turns the configured OAuth client into an access
// token. The API binds the token to the client's owning account, so it can mint
// capture tokens and read presence.
func (c *Client) ExchangeClientCredentials(ctx context.Context) (string, error) {
	cfg := c.Config()
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return "", fmt.Errorf("%w: JPN_LEARNING_CLIENT_ID and JPN_LEARNING_CLIENT_SECRET are required", ErrNotConfigured)
	}
	endpoints, err := c.Endpoints(ctx)
	if err != nil {
		return "", err
	}

	// JPN_LEARNING_URL may carry a path prefix the deployment does not serve
	// (for example "/api/token" on a host that mounts the API at the root).
	// Try the configured URL first, then the token path derived from the
	// discovered API base.
	candidates := []string{endpoints.TokenURL}
	if derived := endpoints.APIBase + "/token"; derived != endpoints.TokenURL {
		candidates = append(candidates, derived)
	}

	var lastErr error
	for index, tokenURL := range candidates {
		accessToken, status, err := c.exchangeAt(ctx, tokenURL, cfg)
		if accessToken != "" {
			return accessToken, nil
		}
		if status == http.StatusNotFound && index < len(candidates)-1 {
			lastErr = err
			continue // try the token URL derived from the discovered base
		}
		if err != nil {
			return "", err
		}
		return "", &APIError{Status: status, Message: "the token endpoint returned no access_token"}
	}
	return "", lastErr
}

// exchangeAt performs one token request. A non-empty token means success; a
// non-zero status with an APIError in statusMessage is returned through err.
func (c *Client) exchangeAt(ctx context.Context, tokenURL string, cfg Config) (string, int, error) {
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("exchange OAuth credentials: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", resp.StatusCode, fmt.Errorf("read token response: %w", err)
	}
	accessToken, message := parseTokenResponse(raw)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && accessToken != "" {
		return accessToken, resp.StatusCode, nil
	}
	if message == "" {
		message = truncateMessage(string(raw))
	}
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/html") {
		message = "the token URL returned an HTML page, not the API; check JPN_LEARNING_URL (it usually ends in /api/token or /token)"
	}
	if message == "" {
		message = "the token endpoint returned no access_token"
	}
	return "", resp.StatusCode, &APIError{Status: resp.StatusCode, Message: message}
}

// parseTokenResponse accepts both JSON and form-encoded bodies. The OAuth
// handler chooses the response encoding from the request's Content-Type, so a
// conventional form-encoded token request gets an
// "access_token=…&token_type=…" body rather than JSON.
func parseTokenResponse(raw []byte) (accessToken, message string) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return "", ""
	}

	if strings.HasPrefix(trimmed, "{") {
		var payload struct {
			AccessToken string `json:"access_token"`
			Error       string `json:"error"`
			Message     string `json:"message"`
		}
		if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
			message = payload.Error
			if message == "" {
				message = payload.Message
			}
			return payload.AccessToken, message
		}
	}

	if values, err := url.ParseQuery(trimmed); err == nil {
		return values.Get("access_token"), values.Get("error")
	}
	return "", ""
}

// MintCaptureToken creates a kbc_… credential for the OAuth client's owner.
// The secret is only returned here; the API keeps just its hash.
func (c *Client) MintCaptureToken(ctx context.Context, accessToken string) (Token, string, error) {
	endpoints, err := c.Endpoints(ctx)
	if err != nil {
		return Token{}, "", err
	}
	body, _ := json.Marshal(map[string]string{"name": c.Name})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoints.APIBase+"/live/tokens", bytes.NewReader(body))
	if err != nil {
		return Token{}, "", err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return Token{}, "", fmt.Errorf("create capture token: %w", err)
	}
	defer resp.Body.Close()

	var payload struct {
		Token  Token  `json:"token"`
		Secret string `json:"secret"`
	}
	if err := decodeJSON(resp, &payload); err != nil {
		return Token{}, "", err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return Token{}, "", fmt.Errorf("%w: the OAuth client cannot create capture tokens (does its account have the user role?)", ErrUnauthorized)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Token{}, "", &APIError{Status: resp.StatusCode, Message: "could not create a capture token"}
	}
	if payload.Secret == "" {
		return Token{}, "", &APIError{Status: resp.StatusCode, Message: "the API returned no token secret"}
	}
	c.mu.Lock()
	c.cfg.CaptureToken = payload.Secret
	c.cfg.CaptureTokenID = payload.Token.ID
	c.mu.Unlock()
	return payload.Token, payload.Secret, nil
}

// Status reads presence for the caller's sessions. It needs a user-bound
// credential: an OAuth access token, not a capture token.
func (c *Client) Status(ctx context.Context, accessToken string) (*StatusSummary, error) {
	endpoints, err := c.Endpoints(ctx)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoints.StatusURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("read live status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("%w: GET /live/status needs an OAuth access token", ErrUnauthorized)
	}
	var summary StatusSummary
	if err := decodeJSON(resp, &summary); err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{Status: resp.StatusCode, Message: "could not read live status"}
	}
	return &summary, nil
}

// ListTokens lists the caller's capture tokens. Requires an OAuth access token.
func (c *Client) ListTokens(ctx context.Context, accessToken string) ([]Token, error) {
	endpoints, err := c.Endpoints(ctx)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoints.APIBase+"/live/tokens", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("list capture tokens: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("%w: listing tokens needs an OAuth access token", ErrUnauthorized)
	}
	var tokens []Token
	if err := decodeJSON(resp, &tokens); err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{Status: resp.StatusCode, Message: "could not list capture tokens"}
	}
	return tokens, nil
}

// RevokeToken revokes one capture token by id.
func (c *Client) RevokeToken(ctx context.Context, accessToken, id string) error {
	endpoints, err := c.Endpoints(ctx)
	if err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("token id is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoints.APIBase+"/live/tokens/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("revoke capture token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("%w: revoking tokens needs an OAuth access token", ErrUnauthorized)
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("token %s not found", id)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{Status: resp.StatusCode, Message: "could not revoke the token"}
	}
	return nil
}

// ProbeCaptureToken checks a kbc_… secret without creating a session. The
// handler validates the token before upgrading, so a successful handshake is
// proof; closing immediately means no hello is sent and nothing is persisted.
func (c *Client) ProbeCaptureToken(ctx context.Context, token string) error {
	endpoints, err := c.Endpoints(ctx)
	if err != nil {
		return err
	}
	dialer := websocket.Dialer{HandshakeTimeout: handshakeTimeout}
	header := http.Header{"Authorization": []string{"Bearer " + strings.TrimSpace(token)}}
	conn, resp, err := dialer.DialContext(ctx, endpoints.CaptureWS, header)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			return fmt.Errorf("%w: check the token, or create a new one on the live page", ErrUnauthorized)
		}
		return fmt.Errorf("connect to %s: %w", endpoints.CaptureWS, err)
	}
	_ = conn.Close()
	return nil
}

// handshakeProbeWait is how long ProbeHandshake waits for the server's reply.
var handshakeProbeWait = 6 * time.Second

// ProbeHandshake opens the capture socket and sends a hello that deliberately
// omits game.name. The server answers with an `error game_required` frame
// before it touches the database, so a healthy endpoint replies in
// milliseconds, no session is created and nothing is written to the account.
//
// This is the only cheap way to tell a working socket from a host that accepts
// the upgrade and then forwards nothing (a plain HTTP proxy in front of the
// API does exactly that).
func (c *Client) ProbeHandshake(ctx context.Context, token string) error {
	endpoints, err := c.Endpoints(ctx)
	if err != nil {
		return err
	}
	header := http.Header{"Authorization": []string{"Bearer " + strings.TrimSpace(token)}}
	dialer := websocket.Dialer{HandshakeTimeout: handshakeTimeout}
	conn, resp, err := dialer.DialContext(ctx, endpoints.CaptureWS, header)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			return fmt.Errorf("%w: check the token, or create a new one on the live page", ErrUnauthorized)
		}
		return fmt.Errorf("connect to %s: %w", endpoints.CaptureWS, err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{"type": "hello", "protocol": ProtocolVersion}); err != nil {
		return fmt.Errorf("send hello to %s: %w", endpoints.CaptureWS, err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(handshakeProbeWait))
	_, data, err := conn.ReadMessage()
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return fmt.Errorf("%s accepted the connection but never answered: the server sent no frame in %s, which usually means an HTTP-only proxy is in front of the API (set --live-ws / JPN_LEARNING_WS to the API's socket URL)", endpoints.CaptureWS, handshakeProbeWait)
		}
		if websocket.IsCloseError(err, websocket.CloseNormalClosure) || errors.Is(err, io.EOF) {
			return fmt.Errorf("%s closed the connection during the handshake: %w", endpoints.CaptureWS, err)
		}
		return fmt.Errorf("handshake with %s: %w", endpoints.CaptureWS, err)
	}
	var frame serverFrame
	if err := json.Unmarshal(data, &frame); err != nil {
		return fmt.Errorf("%s sent %q, which is not a protocol frame", endpoints.CaptureWS, truncateMessage(string(data)))
	}
	if frame.Type == "error" && frame.Code == "game_required" {
		return nil
	}
	return fmt.Errorf("%s answered %s %s to the handshake probe", endpoints.CaptureWS, frame.Type, frame.Code)
}

func decodeJSON(resp *http.Response, out any) error {
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		if resp.StatusCode >= 400 {
			return &APIError{Status: resp.StatusCode, Message: truncateMessage(string(raw))}
		}
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func truncateMessage(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 200 {
		return value[:200] + "…"
	}
	return value
}

// TokenSource returns a usable capture secret, minting and caching one when
// only OAuth client credentials are configured.
type TokenSource struct {
	Client *Client
	// OnSave persists a freshly minted secret. Optional.
	OnSave func(Config) error

	mu    sync.Mutex
	token string
}

func NewTokenSource(client *Client, onSave func(Config) error) *TokenSource {
	return &TokenSource{Client: client, OnSave: onSave}
}

// Set pins a token the caller already has (for example one pasted at a prompt).
func (s *TokenSource) Set(token string) {
	s.mu.Lock()
	s.token = strings.TrimSpace(token)
	s.mu.Unlock()
}

// Token returns the cached secret, the configured secret, or a new one.
func (s *TokenSource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	if s.token != "" {
		token := s.token
		s.mu.Unlock()
		return token, nil
	}
	s.mu.Unlock()

	if token := strings.TrimSpace(s.Client.Config().CaptureToken); token != "" {
		s.mu.Lock()
		s.token = token
		s.mu.Unlock()
		return token, nil
	}
	return s.Refresh(ctx)
}

// Refresh mints a new capture token. Used on first use and after a 401.
func (s *TokenSource) Refresh(ctx context.Context) (string, error) {
	accessToken, err := s.Client.ExchangeClientCredentials(ctx)
	if err != nil {
		return "", err
	}
	_, secret, err := s.Client.MintCaptureToken(ctx, accessToken)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.token = secret
	s.mu.Unlock()

	if s.OnSave != nil {
		if err := s.OnSave(s.Client.Config()); err != nil {
			// A cache failure must not stop capture; the token still works.
			return secret, nil
		}
	}
	return secret, nil
}

// EnsureCaptureToken resolves a secret for non-streaming use (login, doctor).
// It does not prompt; callers own the terminal interaction.
func EnsureCaptureToken(ctx context.Context, client *Client, source *TokenSource) (string, error) {
	token, err := source.Token(ctx)
	if err == nil {
		return token, nil
	}
	if errors.Is(err, ErrNotConfigured) {
		return "", fmt.Errorf("%w: run `vntext live login`, or paste a token with `vntext live login --token kbc_…`", ErrNotConfigured)
	}
	return "", err
}
