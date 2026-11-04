package liveclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Endpoint presets.
const (
	LocalAPI  = "http://127.0.0.1:18080"
	RemoteAPI = "https://jpn-learning-api.darlinggoose.com"

	// The capture socket is a different endpoint from the REST API. The
	// website proxies /api/* as plain HTTP only, so a host can serve the OAuth
	// and token endpoints perfectly and still swallow a websocket upgrade.
	// Presets therefore carry the socket URL explicitly instead of deriving it
	// from the API base.
	LocalCaptureWS  = "ws://127.0.0.1:18080/live/capture"
	RemoteCaptureWS = "wss://jpn-learning-api.darlinggoose.com/live/capture"

	DefaultEndpoint = "remote"
	DefaultKind     = "dialogue"
)

// ErrNotConfigured means there is neither a capture token nor a way to mint
// one. Callers turn this into a one-line instruction, never a stack trace.
var ErrNotConfigured = errors.New("live capture is not configured")

// Config is the on-disk (~/.config/vntext/live.json) and in-memory
// configuration. Secrets live here only when the user chooses to save them;
// environment variables win at runtime.
type Config struct {
	Version int  `json:"version,omitempty"`
	Enabled bool `json:"enabled,omitempty"`

	Endpoint string `json:"endpoint,omitempty"` // "local", "remote", or an API base URL
	APIURL   string `json:"api_url,omitempty"`
	TokenURL string `json:"token_url,omitempty"`
	// WSURL overrides the capture socket. Empty means "use the preset's
	// socket URL, or derive it from a custom API base".
	WSURL string `json:"ws_url,omitempty"`

	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`

	CaptureToken   string `json:"capture_token,omitempty"`
	CaptureTokenID string `json:"capture_token_id,omitempty"`

	DeviceName  string `json:"device_name,omitempty"`
	DefaultKind string `json:"default_kind,omitempty"`

	// AutoSave defaults to true when nil: a captured line should be saved
	// without a second click.
	AutoSave *bool `json:"auto_save,omitempty"`
}

// DefaultConfig returns the built-in defaults.
func DefaultConfig() Config {
	enabled := true
	return Config{
		Version:     1,
		Endpoint:    DefaultEndpoint,
		DefaultKind: DefaultKind,
		AutoSave:    &enabled,
	}
}

// AutoSaveEnabled reports whether lines should become project entries.
func (c Config) AutoSaveEnabled() bool {
	return c.AutoSave == nil || *c.AutoSave
}

// ApplyEnv overlays environment variables. The JPN_LEARNING_* names are the
// same ones the API tooling already uses.
func (c Config) ApplyEnv(getenv func(string) string) Config {
	if getenv == nil {
		getenv = os.Getenv
	}
	value := func(name string) string { return strings.TrimSpace(getenv(name)) }

	if v := value("VNTEXT_LIVE_ENDPOINT"); v != "" {
		// Selecting a deployment also selects its URLs; a stale token URL
		// from the config file must not keep pointing at the other one.
		c.Endpoint = v
		c.APIURL = ""
		c.TokenURL = ""
		c.WSURL = ""
	}
	if v := value("JPN_LEARNING_URL"); v != "" {
		// JPN_LEARNING_URL points at the OAuth token endpoint.
		c.TokenURL = v
	}
	if v := value("JPN_LEARNING_WS"); v != "" {
		// The capture socket, when it is not the preset's. Needed because the
		// website's /api prefix does not forward websockets.
		c.WSURL = v
	}
	if v := value("JPN_LEARNING_API_URL"); v != "" {
		c.APIURL = v
	}
	if v := value("JPN_LEARNING_CLIENT_ID"); v != "" {
		c.ClientID = v
	}
	if v := value("JPN_LEARNING_CLIENT_SECRET"); v != "" {
		c.ClientSecret = v
	}
	if v := value("JPN_LEARNING_CAPTURE_TOKEN"); v != "" {
		c.CaptureToken = v
	}
	if v := value("VNTEXT_LIVE_DEVICE"); v != "" {
		c.DeviceName = v
	}
	if v := value("VNTEXT_LIVE_KIND"); v != "" {
		c.DefaultKind = v
	}
	return c.normalized()
}

func (c Config) normalized() Config {
	c.Endpoint = strings.TrimSpace(c.Endpoint)
	c.APIURL = strings.TrimRight(strings.TrimSpace(c.APIURL), "/")
	c.TokenURL = strings.TrimRight(strings.TrimSpace(c.TokenURL), "/")
	c.WSURL = strings.TrimRight(strings.TrimSpace(c.WSURL), "/")
	c.ClientID = strings.TrimSpace(c.ClientID)
	c.ClientSecret = strings.TrimSpace(c.ClientSecret)
	c.CaptureToken = strings.TrimSpace(c.CaptureToken)
	c.CaptureTokenID = strings.TrimSpace(c.CaptureTokenID)
	c.DeviceName = strings.TrimSpace(c.DeviceName)
	c.DefaultKind = strings.TrimSpace(c.DefaultKind)
	if c.Endpoint == "" {
		c.Endpoint = DefaultEndpoint
	}
	if c.DefaultKind == "" {
		c.DefaultKind = DefaultKind
	}
	if c.Version == 0 {
		c.Version = 1
	}
	return c
}

// BaseURL returns the endpoint flag's value as a URL, expanding the presets.
func (c Config) BaseURL() (string, error) {
	switch strings.ToLower(c.Endpoint) {
	case "", "remote":
		return RemoteAPI, nil
	case "local":
		return LocalAPI, nil
	default:
		raw := strings.TrimRight(c.Endpoint, "/")
		if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
			return "", fmt.Errorf("unknown live endpoint %q: use local, remote, or an http(s) URL", c.Endpoint)
		}
		return raw, nil
	}
}

// apiCandidates lists the API bases to try, most specific first.
func (c Config) apiCandidates() ([]string, error) {
	var candidates []string
	add := func(candidate string) {
		candidate = strings.TrimRight(strings.TrimSpace(candidate), "/")
		if candidate == "" {
			return
		}
		for _, existing := range candidates {
			if strings.EqualFold(existing, candidate) {
				return
			}
		}
		candidates = append(candidates, candidate)
	}

	if c.APIURL != "" {
		add(c.APIURL)
	}
	if c.TokenURL != "" {
		base := strings.TrimSuffix(c.TokenURL, "/token")
		add(base)
		// "/api/token" deployments: the API may be mounted at the root even
		// when the token endpoint sits behind an /api prefix.
		if strings.HasSuffix(base, "/api") {
			add(strings.TrimSuffix(base, "/api"))
		}
	}
	if len(candidates) == 0 {
		base, err := c.BaseURL()
		if err != nil {
			return nil, err
		}
		add(base)
	}
	return candidates, nil
}

// LoadConfig reads a config file. A missing file is not an error: first-run
// setups are expected.
func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()
	path = strings.TrimSpace(path)
	if path == "" {
		return cfg, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read live config: %w", err)
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return DefaultConfig(), fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg.normalized(), nil
}

// Save writes the config with 0600 permissions using an atomic rename.
func (c Config) Save(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("live config path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	persisted := c.normalized()
	// The token endpoint is derived from the API base unless JPN_LEARNING_URL
	// provides it at runtime. Persisting it would pin a deployment and break
	// switching between local and remote with --endpoint.
	persisted.TokenURL = ""
	raw, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		return fmt.Errorf("encode live config: %w", err)
	}
	raw = append(raw, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), ".live-*.json")
	if err != nil {
		return fmt.Errorf("write live config: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod live config: %w", err)
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return fmt.Errorf("write live config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close live config: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("install live config: %w", err)
	}
	return nil
}

// captureWS resolves the capture socket URL. An explicit ws_url / --live-ws
// always wins. Otherwise a preset contributes its own socket URL, and a custom
// API base is asked to derive one.
//
// The preset also wins when the API base is a "/api" proxy path: a host that
// serves the REST API under /api (the website in front of the API) forwards
// plain HTTP only, so deriving wss://host/api/live/capture from it produces a
// socket that upgrades and then never speaks.
func (c Config) captureWS(apiBase string) (string, error) {
	if c.WSURL != "" {
		return wsURL(c.WSURL)
	}
	presetWS := ""
	switch strings.ToLower(strings.TrimSpace(c.Endpoint)) {
	case "local":
		presetWS = LocalCaptureWS
	case "", "remote":
		presetWS = RemoteCaptureWS
	}
	if presetWS != "" {
		preset, err := c.BaseURL()
		if err == nil && (sameHost(apiBase, preset) || isAPIProxyPath(apiBase)) {
			return presetWS, nil
		}
	}
	return wsURL(apiBase)
}

// isAPIProxyPath reports whether a base URL ends in an "/api" prefix, the shape
// of the website's HTTP-only proxy.
func isAPIProxyPath(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	path := strings.TrimRight(parsed.Path, "/")
	return path == "/api" || strings.HasSuffix(path, "/api")
}

// sameHost reports whether two URLs share a host, ignoring the path.
func sameHost(a, b string) bool {
	left, err := url.Parse(a)
	if err != nil {
		return false
	}
	right, err := url.Parse(b)
	if err != nil {
		return false
	}
	if left.Host == "" || right.Host == "" {
		return false
	}
	return strings.EqualFold(left.Host, right.Host)
}

// wsURL converts an http(s) API base into the capture websocket URL.
func wsURL(apiBase string) (string, error) {
	parsed, err := url.Parse(apiBase)
	if err != nil {
		return "", fmt.Errorf("parse API URL: %w", err)
	}
	switch parsed.Scheme {
	case "http", "ws":
		parsed.Scheme = "ws"
	case "https", "wss":
		parsed.Scheme = "wss"
	default:
		return "", fmt.Errorf("unsupported API scheme %q", parsed.Scheme)
	}
	path := strings.TrimRight(parsed.Path, "/")
	// An explicit --live-ws / JPN_LEARNING_WS may already name the socket.
	if !strings.HasSuffix(path, "/live/capture") {
		path += "/live/capture"
	}
	parsed.Path = path
	return parsed.String(), nil
}
