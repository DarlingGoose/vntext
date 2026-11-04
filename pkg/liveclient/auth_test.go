package liveclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestExchangeCredentialsAndMintToken(t *testing.T) {
	var mu sync.Mutex
	var form url.Values
	var mintedName, mintedAuth string

	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		mu.Lock()
		form = r.PostForm
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "access-123", "token_type": "Bearer", "expires_in": 3600,
		})
	})
	mux.HandleFunc("/live/tokens", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var body struct {
			Name string `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		mintedName = body.Name
		mintedAuth = r.Header.Get("Authorization")
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token":  map[string]any{"id": "tok-1", "name": body.Name},
			"secret": "kbc_new_secret",
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := DefaultConfig()
	cfg.APIURL = server.URL
	cfg.TokenURL = server.URL + "/token"
	cfg.ClientID = "client"
	cfg.ClientSecret = "shh"
	client := NewClient(cfg)

	access, err := client.ExchangeClientCredentials(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if access != "access-123" {
		t.Fatalf("access token: %q", access)
	}

	mu.Lock()
	if form.Get("grant_type") != "client_credentials" || form.Get("client_id") != "client" || form.Get("client_secret") != "shh" {
		t.Fatalf("unexpected form: %v", form)
	}
	mu.Unlock()

	token, secret, err := client.MintCaptureToken(context.Background(), access)
	if err != nil {
		t.Fatal(err)
	}
	if token.ID != "tok-1" || secret != "kbc_new_secret" {
		t.Fatalf("mint result: %+v %q", token, secret)
	}
	mu.Lock()
	if mintedAuth != "Bearer access-123" {
		t.Fatalf("mint auth: %q", mintedAuth)
	}
	if !strings.Contains(mintedName, "vntext") {
		t.Fatalf("mint name: %q", mintedName)
	}
	mu.Unlock()
	if client.Config().CaptureToken != "kbc_new_secret" || client.Config().CaptureTokenID != "tok-1" {
		t.Fatalf("client config not updated: %+v", client.Config())
	}
}

func TestExchangeCredentialsRejectsBadSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid client credentials"}`))
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.APIURL = server.URL
	cfg.TokenURL = server.URL + "/token"
	cfg.ClientID = "id"
	cfg.ClientSecret = "wrong"
	if _, err := NewClient(cfg).ExchangeClientCredentials(context.Background()); err == nil {
		t.Fatal("expected an error")
	}
}

func TestTokenSourceCachesAndRefreshes(t *testing.T) {
	mints := 0
	server := testAPIServer(t, &mints)
	defer server.Close()

	cfg := DefaultConfig()
	cfg.APIURL = server.URL
	cfg.TokenURL = server.URL + "/token"
	cfg.ClientID = "id"
	cfg.ClientSecret = "secret"

	var saved Config
	client := NewClient(cfg)
	source := NewTokenSource(client, func(c Config) error {
		saved = c
		return nil
	})

	first, err := source.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first == "" {
		t.Fatal("empty token")
	}
	second, err := source.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second != first || mints != 1 {
		t.Fatalf("token should be cached: first=%q second=%q mints=%d", first, second, mints)
	}

	refreshed, err := source.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if refreshed == first {
		t.Fatal("refresh returned the old token")
	}
	if mints != 2 {
		t.Fatalf("mints: got %d want 2", mints)
	}
	if saved.CaptureToken == "" {
		t.Fatal("OnSave was not called")
	}
}

func testAPIServer(t *testing.T, mints *int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "access"})
	})
	mux.HandleFunc("/live/tokens", func(w http.ResponseWriter, r *http.Request) {
		*mints++
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token":  map[string]any{"id": "tok"},
			"secret": "kbc_token_" + string(rune('a'+*mints-1)),
		})
	})
	return httptest.NewServer(mux)
}

func TestProbeCaptureToken(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer kbc_good" {
			http.Error(w, "bad token", http.StatusUnauthorized)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		_ = conn.Close()
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.APIURL = server.URL
	client := NewClient(cfg)

	if err := client.ProbeCaptureToken(context.Background(), "kbc_good"); err != nil {
		t.Fatalf("good token rejected: %v", err)
	}
	if err := client.ProbeCaptureToken(context.Background(), "kbc_bad"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("bad token: got %v, want ErrUnauthorized", err)
	}
}

func TestExchangeFallsBackWhenTokenURLIsWrong(t *testing.T) {
	var rootCalls int
	mux := http.NewServeMux()
	mux.HandleFunc("/api/token", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		rootCalls++
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "access-root"})
	})
	mux.HandleFunc("/live/status", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := DefaultConfig()
	cfg.APIURL = server.URL
	cfg.TokenURL = server.URL + "/api/token" // JPN_LEARNING_URL as commonly written
	cfg.ClientID = "id"
	cfg.ClientSecret = "secret"

	token, err := NewClient(cfg).ExchangeClientCredentials(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if token != "access-root" || rootCalls != 1 {
		t.Fatalf("token=%q rootCalls=%d", token, rootCalls)
	}
}

func TestExchangeDoesNotFallBackOnBadCredentials(t *testing.T) {
	rootCalls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/api/token", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid client credentials"}`))
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		rootCalls++
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "should-not-be-used"})
	})
	mux.HandleFunc("/live/status", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := DefaultConfig()
	cfg.APIURL = server.URL
	cfg.TokenURL = server.URL + "/api/token"
	cfg.ClientID = "id"
	cfg.ClientSecret = "wrong"

	if _, err := NewClient(cfg).ExchangeClientCredentials(context.Background()); err == nil {
		t.Fatal("expected an error")
	}
	if rootCalls != 0 {
		t.Fatalf("fallback should not run on bad credentials; rootCalls=%d", rootCalls)
	}
}

func TestExchangeAcceptsFormEncodedTokenResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mirror the real OAuth handler: the response encoding follows the
		// request's Content-Type.
		if strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
			w.Header().Set("Content-Type", "application/x-www-form-urlencoded")
			_, _ = w.Write([]byte("access_token=form-token&token_type=Bearer&expires_in=3600"))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "json-token"})
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.APIURL = server.URL
	cfg.TokenURL = server.URL + "/token"
	cfg.ClientID = "id"
	cfg.ClientSecret = "secret"

	token, err := NewClient(cfg).ExchangeClientCredentials(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if token != "form-token" {
		t.Fatalf("token: %q", token)
	}
}

func TestExchangeReportsPlainTextError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid client credentials"))
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.APIURL = server.URL
	cfg.TokenURL = server.URL + "/token"
	cfg.ClientID = "id"
	cfg.ClientSecret = "wrong"

	_, err := NewClient(cfg).ExchangeClientCredentials(context.Background())
	if err == nil || !strings.Contains(err.Error(), "invalid client credentials") {
		t.Fatalf("error: %v", err)
	}
}

func TestProbeHandshakeDetectsSilentSocket(t *testing.T) {
	oldWait := handshakeProbeWait
	handshakeProbeWait = 250 * time.Millisecond
	defer func() { handshakeProbeWait = oldWait }()

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		var frame map[string]any
		if err := conn.ReadJSON(&frame); err != nil {
			return
		}
		if frame["type"] != "hello" {
			_ = conn.WriteJSON(map[string]any{"type": "error", "code": "hello_required"})
			return
		}
		// A healthy server rejects the game-less hello before touching the
		// database, which is what makes this probe side-effect free.
		if game, _ := frame["game"].(map[string]any); game == nil || game["name"] == "" {
			_ = conn.WriteJSON(map[string]any{"type": "error", "code": "game_required", "message": "hello needs game.name"})
			return
		}
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.APIURL = server.URL
	client := NewClient(cfg)

	// Valid token, healthy socket.
	if err := client.ProbeHandshake(context.Background(), "kbc_good"); err != nil {
		t.Fatalf("handshake probe failed: %v", err)
	}
}

func TestProbeHandshakeReportsUpgradeThatNeverSpeaks(t *testing.T) {
	oldWait := handshakeProbeWait
	handshakeProbeWait = 250 * time.Millisecond
	defer func() { handshakeProbeWait = oldWait }()

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		// Upgrade, then nothing: what an HTTP-only proxy does.
		time.Sleep(time.Second)
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.APIURL = server.URL
	client := NewClient(cfg)

	err := client.ProbeHandshake(context.Background(), "kbc_good")
	if err == nil {
		t.Fatal("silent socket was reported healthy")
	}
	if !strings.Contains(err.Error(), "never answered") || !strings.Contains(err.Error(), "JPN_LEARNING_WS") {
		t.Fatalf("error does not explain the failure: %v", err)
	}
}
