package liveclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestBaseURLEndpointPresets(t *testing.T) {
	tests := []struct {
		endpoint string
		want     string
	}{
		{"", RemoteAPI},
		{"remote", RemoteAPI},
		{"local", LocalAPI},
		{"https://example.test/api", "https://example.test/api"},
	}
	for _, test := range tests {
		cfg := DefaultConfig()
		cfg.Endpoint = test.endpoint
		got, err := cfg.BaseURL()
		if err != nil {
			t.Fatalf("%q: %v", test.endpoint, err)
		}
		if got != test.want {
			t.Fatalf("%q: got %q want %q", test.endpoint, got, test.want)
		}
	}

	cfg := DefaultConfig()
	cfg.Endpoint = "ftp://example.test"
	if _, err := cfg.BaseURL(); err == nil {
		t.Fatal("expected an error for an unsupported scheme")
	}
}

func TestEndpointDiscoveryFallsBackToRoot(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/live/status", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	mux.HandleFunc("/live/status", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := DefaultConfig()
	cfg.TokenURL = server.URL + "/api/token" // JPN_LEARNING_URL shape
	client := NewClient(cfg)

	endpoints, err := client.Endpoints(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if endpoints.APIBase != server.URL {
		t.Fatalf("api base: got %q want %q", endpoints.APIBase, server.URL)
	}
	if endpoints.TokenURL != server.URL+"/api/token" {
		t.Fatalf("token url: got %q", endpoints.TokenURL)
	}
	if want := "ws" + server.URL[len("http"):] + "/live/capture"; endpoints.CaptureWS != want {
		t.Fatalf("capture ws: got %q want %q", endpoints.CaptureWS, want)
	}
}

func TestEndpointDiscoveryKeepsWorkingPrefix(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/live/status", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	mux.HandleFunc("/live/status", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := DefaultConfig()
	cfg.TokenURL = server.URL + "/api/token"
	endpoints, err := NewClient(cfg).Endpoints(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if endpoints.APIBase != server.URL+"/api" {
		t.Fatalf("api base: got %q want %q", endpoints.APIBase, server.URL+"/api")
	}
}

func TestConfigSavePermissionsAndRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "live.json")
	cfg := DefaultConfig()
	cfg.Endpoint = "local"
	cfg.CaptureToken = "kbc_secret"
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("mode: got %o want 600", mode)
	}

	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Endpoint != "local" || loaded.CaptureToken != "kbc_secret" {
		t.Fatalf("round trip lost values: %+v", loaded)
	}
	if !loaded.AutoSaveEnabled() {
		t.Fatal("auto-save should default to on")
	}
}

func TestApplyEnv(t *testing.T) {
	env := map[string]string{
		"JPN_LEARNING_URL":           "https://host/api/token",
		"JPN_LEARNING_API_URL":       "https://host",
		"JPN_LEARNING_CLIENT_ID":     "id",
		"JPN_LEARNING_CLIENT_SECRET": "secret",
		"JPN_LEARNING_CAPTURE_TOKEN": "kbc_token",
		"VNTEXT_LIVE_ENDPOINT":       "local",
		"VNTEXT_LIVE_DEVICE":         "PC-1",
		"VNTEXT_LIVE_KIND":           "narration",
	}
	cfg := DefaultConfig().ApplyEnv(func(key string) string { return env[key] })
	if cfg.TokenURL != env["JPN_LEARNING_URL"] || cfg.APIURL != env["JPN_LEARNING_API_URL"] {
		t.Fatalf("urls not applied: %+v", cfg)
	}
	if cfg.ClientID != "id" || cfg.ClientSecret != "secret" || cfg.CaptureToken != "kbc_token" {
		t.Fatalf("credentials not applied: %+v", cfg)
	}
	if cfg.Endpoint != "local" || cfg.DeviceName != "PC-1" || cfg.DefaultKind != "narration" {
		t.Fatalf("options not applied: %+v", cfg)
	}
}

func TestGameKey(t *testing.T) {
	cases := map[string]string{
		"Fate/stay night": "fate-stay-night",
		"Fate stay night": "fate-stay-night",
		"逆転裁判":            "逆転裁判",
		"  ...  ":         "game",
	}
	for input, want := range cases {
		if got := gameKey(input); got != want {
			t.Fatalf("%q: got %q want %q", input, got, want)
		}
	}
}

func TestPresetCaptureSocketWinsOverSavedAPIBase(t *testing.T) {
	// Regression: live.json had saved api_url as the website's /api proxy. That
	// host answers OAuth and status requests, so discovery picked it, and the
	// capture URL derived from it pointed at a proxy that upgrades the socket
	// and then forwards nothing. The preset must keep its own socket URL.
	cfg := DefaultConfig()
	cfg.APIURL = "https://jpn-learning.darlinggoose.com/api"
	cfg.TokenURL = "https://jpn-learning.darlinggoose.com/api/token"
	cfg.CaptureToken = "kbc_token"

	endpoints, err := NewClient(cfg).Endpoints(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if endpoints.CaptureWS != RemoteCaptureWS {
		t.Fatalf("capture socket = %q, want %q", endpoints.CaptureWS, RemoteCaptureWS)
	}
	if endpoints.APIBase != cfg.APIURL {
		t.Fatalf("api base = %q, want the configured proxy %q", endpoints.APIBase, cfg.APIURL)
	}

	local := DefaultConfig()
	local.Endpoint = "local"
	endpoints, err = NewClient(local).Endpoints(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if endpoints.CaptureWS != LocalCaptureWS {
		t.Fatalf("local capture socket = %q, want %q", endpoints.CaptureWS, LocalCaptureWS)
	}
}

func TestCaptureSocketOverrideAndDerivation(t *testing.T) {
	custom := Config{Endpoint: "https://api.example.test", APIURL: "https://api.example.test"}
	ws, err := custom.captureWS(custom.APIURL)
	if err != nil {
		t.Fatal(err)
	}
	if ws != "wss://api.example.test/live/capture" {
		t.Fatalf("derived socket = %q", ws)
	}

	override := custom
	override.WSURL = "wss://sockets.example.test/live/capture"
	ws, err = override.captureWS(override.APIURL)
	if err != nil {
		t.Fatal(err)
	}
	if ws != override.WSURL {
		t.Fatalf("override socket = %q, want %q", ws, override.WSURL)
	}

	// http(s) schemes are rewritten, and a URL that already names the socket
	// does not get the path twice.
	bare := Config{Endpoint: "remote", WSURL: "https://api.example.test"}
	ws, err = bare.captureWS(bare.APIURL)
	if err != nil {
		t.Fatal(err)
	}
	if ws != "wss://api.example.test/live/capture" {
		t.Fatalf("scheme rewritten socket = %q", ws)
	}
}

func TestCaptureSocketEnvOverride(t *testing.T) {
	env := map[string]string{"JPN_LEARNING_WS": "wss://api.example.test/live/capture"}
	cfg := DefaultConfig().ApplyEnv(func(key string) string { return env[key] })
	if cfg.WSURL != env["JPN_LEARNING_WS"] {
		t.Fatalf("ws url = %q", cfg.WSURL)
	}

	// Switching deployment must clear a socket URL that belongs to the old one.
	cfg.Endpoint = "remote"
	cfg = cfg.ApplyEnv(func(key string) string {
		if key == "VNTEXT_LIVE_ENDPOINT" {
			return "local"
		}
		return ""
	})
	if cfg.WSURL != "" {
		t.Fatalf("ws url survived an endpoint switch: %q", cfg.WSURL)
	}
}
