package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/DarlingGoose/vntext/pkg/engine"
	"github.com/DarlingGoose/vntext/pkg/game"
	"github.com/DarlingGoose/vntext/pkg/liveclient"
)

func TestToLiveLine(t *testing.T) {
	line := toLiveLine(engine.Line{
		Text:    "  セイバー、行くぞ。 ",
		Speaker: "士郎",
		Hook:    "0x1f3",
	}, "")
	if line.Text != "セイバー、行くぞ。" || line.Speaker != "士郎" || line.Kind != "" || line.Ref != "0x1f3" {
		t.Fatalf("unexpected line: %+v", line)
	}

	system := toLiveLine(engine.Line{Text: "保存しました", Speaker: "system"}, "dialogue")
	if system.Kind != "system" || system.Speaker != "" || system.Text != "保存しました" {
		t.Fatalf("system line: %+v", system)
	}

	fallback := toLiveLine(engine.Line{Raw: "  raw text  "}, "narration")
	if fallback.Text != "raw text" || fallback.Kind != "narration" {
		t.Fatalf("raw fallback: %+v", fallback)
	}
}

func TestResolveLiveConfigPrecedence(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("JPN_LEARNING_URL", "https://host/api/token")
	t.Setenv("JPN_LEARNING_CAPTURE_TOKEN", "kbc_from_env")
	t.Setenv("VNTEXT_LIVE_ENDPOINT", "local")

	cfg, path, err := resolveLiveConfig(liveFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dir, "vntext", "live.json") {
		t.Fatalf("config path: %q", path)
	}
	if cfg.Endpoint != "local" || cfg.TokenURL != "https://host/api/token" || cfg.CaptureToken != "kbc_from_env" {
		t.Fatalf("env values not applied: %+v", cfg)
	}

	flagged, _, err := resolveLiveConfig(liveFlags{Endpoint: "remote", Token: "kbc_flag"})
	if err != nil {
		t.Fatal(err)
	}
	if flagged.Endpoint != "remote" || flagged.CaptureToken != "kbc_flag" {
		t.Fatalf("flags did not win: %+v", flagged)
	}
}

func TestNewLivePublisherDisabled(t *testing.T) {
	publisher, err := newLivePublisher(t.Context(), &game.Game{Name: "Test"}, RunGameOptions{}, nil)
	if err != nil || publisher != nil {
		t.Fatalf("disabled publisher: %v %v", publisher, err)
	}
}

func TestNewLivePublisherDryRunNeedsNoToken(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	publisher, err := newLivePublisher(t.Context(), &game.Game{Name: "Test"}, RunGameOptions{
		Live: LiveCaptureOptions{Enabled: true, DryRun: true, NoInput: true},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if publisher == nil {
		t.Fatal("expected a dry-run publisher")
	}
	_ = publisher.Close(t.Context())
}

func TestLivePipeCommand(t *testing.T) {
	var mu sync.Mutex
	var gotText string
	var gotHello bool
	byes := make(chan struct{}, 1)

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer kbc_test" {
			http.Error(w, "bad token", http.StatusUnauthorized)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			var frame map[string]any
			if err := conn.ReadJSON(&frame); err != nil {
				return
			}
			switch frame["type"] {
			case "hello":
				mu.Lock()
				gotHello = true
				mu.Unlock()
				_ = conn.WriteJSON(map[string]any{
					"type": "hello_ok", "session_id": "s1", "project_id": "p1",
					"project_name": "Test", "auto_save": true,
				})
			case "line":
				mu.Lock()
				gotText, _ = frame["text"].(string)
				mu.Unlock()
				_ = conn.WriteJSON(map[string]any{"type": "ack", "client_seq": frame["client_seq"]})
			case "bye":
				select {
				case byes <- struct{}{}:
				default:
				}
			}
		}
	}))
	defer server.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("JPN_LEARNING_CAPTURE_TOKEN", "kbc_test")

	command := newLivePipeCommand()
	var stdout, stderr bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(&stderr)
	command.SetIn(strings.NewReader("こんにちは\n「行くぞ」\n"))
	command.SetArgs([]string{"--game", "Test", "--endpoint", server.URL})

	if err := command.Execute(); err != nil {
		t.Fatalf("execute: %v\nstderr: %s", err, stderr.String())
	}

	mu.Lock()
	hello, text := gotHello, gotText
	mu.Unlock()
	if !hello {
		t.Fatal("no hello frame")
	}
	if text != "「行くぞ」" {
		t.Fatalf("last line: %q", text)
	}
	select {
	case <-byes:
	case <-time.After(time.Second):
		t.Fatal("no bye frame")
	}
}

func TestLiveLoginCommandMintsAndCachesToken(t *testing.T) {
	const secret = "kbc_minted_secret"
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "access"})
	})
	mux.HandleFunc("/live/tokens", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access" {
			http.Error(w, "bad auth", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token": map[string]any{"id": "tok-1"}, "secret": secret,
		})
	})
	mux.HandleFunc("/live/status", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sessions": []any{},
			"totals":   map[string]any{"sessions": 0},
		})
	})
	mux.HandleFunc("/live/capture", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+secret {
			http.Error(w, "bad token", http.StatusUnauthorized)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		_ = conn.Close()
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("JPN_LEARNING_URL", server.URL+"/token")
	t.Setenv("JPN_LEARNING_CLIENT_ID", "client")
	t.Setenv("JPN_LEARNING_CLIENT_SECRET", "secret")

	command := newLiveLoginCommand()
	var stdout bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(&stdout)
	command.SetArgs([]string{"--no-input", "--api", server.URL})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), "capture    kbc_minted_s") {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}

	path := filepath.Join(dir, "vntext", "live.json")
	cfg, err := liveclient.LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CaptureToken != secret || cfg.CaptureTokenID != "tok-1" {
		t.Fatalf("token not cached: %+v", cfg)
	}
	if cfg.ClientSecret != "" {
		t.Fatalf("OAuth secret was persisted without --save: %+v", cfg)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode: %o", info.Mode().Perm())
	}
}
