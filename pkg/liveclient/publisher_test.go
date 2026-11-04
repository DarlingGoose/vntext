package liveclient

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type captureServer struct {
	t         *testing.T
	mu        sync.Mutex
	hellos    []map[string]any
	lines     []map[string]any
	byes      int
	conns     int
	dropFirst bool
	firstAck  chan struct{}
}

func newCaptureServer(t *testing.T) (*captureServer, *httptest.Server) {
	t.Helper()
	capture := &captureServer{t: t, firstAck: make(chan struct{}, 8)}
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	mux := http.NewServeMux()
	mux.HandleFunc("/live/capture", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer kbc_test" {
			http.Error(w, "bad token", http.StatusUnauthorized)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		capture.mu.Lock()
		capture.conns++
		number := capture.conns
		capture.mu.Unlock()

		for {
			var frame map[string]any
			if err := conn.ReadJSON(&frame); err != nil {
				return
			}
			switch frame["type"] {
			case "hello":
				capture.mu.Lock()
				capture.hellos = append(capture.hellos, frame)
				capture.mu.Unlock()
				_ = conn.WriteJSON(map[string]any{
					"type": "hello_ok", "protocol": 1, "session_id": "session-1",
					"project_id": "project-1", "project_name": "Test Game",
					"seq": 0, "auto_save": true, "server_time": time.Now().UTC().Format(time.RFC3339),
				})
			case "line":
				capture.mu.Lock()
				capture.lines = append(capture.lines, frame)
				seq := len(capture.lines)
				capture.mu.Unlock()
				_ = conn.WriteJSON(map[string]any{
					"type": "ack", "seq": seq, "client_seq": frame["client_seq"],
				})
				select {
				case capture.firstAck <- struct{}{}:
				default:
				}
				if number == 1 && capture.dropFirst {
					// Give the client time to process the ack, then drop the
					// connection so it has to resume.
					time.Sleep(50 * time.Millisecond)
					_ = conn.Close()
					return
				}
			case "bye":
				capture.mu.Lock()
				capture.byes++
				capture.mu.Unlock()
			}
		}
	})
	return capture, httptest.NewServer(mux)
}

func testPublisher(t *testing.T, serverURL, resumeFile string) (*Publisher, *Client) {
	t.Helper()
	cfg := DefaultConfig()
	cfg.APIURL = serverURL
	cfg.CaptureToken = "kbc_test"
	client := NewClient(cfg)
	source := NewTokenSource(client, nil)
	source.Set("kbc_test")

	publisher, err := NewPublisher(context.Background(), client, source, PublisherOptions{
		GameName:     "Test Game",
		DeviceName:   "PC-1",
		Kind:         "dialogue",
		AutoSave:     true,
		ResumeFile:   resumeFile,
		Out:          io.Discard,
		PingInterval: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	return publisher, client
}

func TestPublisherHandshakeLineAndBye(t *testing.T) {
	capture, server := newCaptureServer(t)
	defer server.Close()

	publisher, _ := testPublisher(t, server.URL, filepath.Join(t.TempDir(), "sessions.json"))
	publisher.Offer(Line{Text: "セイバー、行くぞ。", Speaker: "士郎", Kind: "dialogue"})

	select {
	case <-capture.firstAck:
	case <-time.After(3 * time.Second):
		t.Fatal("line was not acknowledged")
	}

	closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := publisher.Close(closeCtx); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		capture.mu.Lock()
		byes := capture.byes
		capture.mu.Unlock()
		if byes > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	capture.mu.Lock()
	defer capture.mu.Unlock()
	if len(capture.hellos) == 0 {
		t.Fatal("no hello frame")
	}
	hello := capture.hellos[0]
	game := hello["game"].(map[string]any)
	if game["name"] != "Test Game" || game["medium"] != "game" || game["language"] != "ja" {
		t.Fatalf("hello game: %+v", game)
	}
	device := hello["device"].(map[string]any)
	if device["name"] != "PC-1" {
		t.Fatalf("hello device: %+v", device)
	}
	if len(capture.lines) == 0 || capture.lines[0]["text"] != "セイバー、行くぞ。" {
		t.Fatalf("lines: %+v", capture.lines)
	}
	if capture.lines[0]["speaker"] != "士郎" {
		t.Fatalf("speaker: %+v", capture.lines[0])
	}
	if capture.byes == 0 {
		t.Fatal("bye was not sent")
	}
}

func TestPublisherResumesAfterDisconnect(t *testing.T) {
	capture, server := newCaptureServer(t)
	capture.dropFirst = true
	defer server.Close()

	resumeFile := filepath.Join(t.TempDir(), "sessions.json")
	publisher, _ := testPublisher(t, server.URL, resumeFile)
	publisher.Offer(Line{Text: "first", Kind: "dialogue"})

	deadline := time.After(5 * time.Second)
	for {
		capture.mu.Lock()
		hellos := len(capture.hellos)
		capture.mu.Unlock()
		if hellos >= 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("client did not reconnect (hellos=%d)", hellos)
		case <-time.After(20 * time.Millisecond):
		}
	}

	closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = publisher.Close(closeCtx)

	capture.mu.Lock()
	defer capture.mu.Unlock()
	second := capture.hellos[1]
	resume, ok := second["resume"].(map[string]any)
	if !ok {
		t.Fatalf("second hello has no resume: %+v", second)
	}
	if resume["session_id"] != "session-1" {
		t.Fatalf("resume session: %+v", resume)
	}
	if seq, _ := resume["last_client_seq"].(float64); seq != 1 {
		t.Fatalf("resume last_client_seq: %+v", resume)
	}
}

func TestOfferNeverBlocksWhenDisconnected(t *testing.T) {
	cfg := DefaultConfig()
	cfg.APIURL = "http://127.0.0.1:1" // nothing listens here
	cfg.CaptureToken = "kbc_test"
	client := NewClient(cfg)
	source := NewTokenSource(client, nil)
	source.Set("kbc_test")

	publisher, err := NewPublisher(context.Background(), client, source, PublisherOptions{
		GameName: "Test", Out: io.Discard, PingInterval: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	for i := 0; i < queueLimit+100; i++ {
		publisher.Offer(Line{Text: "line " + strings.Repeat("x", i%20)})
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Offer blocked for %s", elapsed)
	}
	if publisher.Stats().Dropped == 0 {
		t.Fatal("expected dropped lines once the queue is full")
	}

	closeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := publisher.Close(closeCtx); err != nil {
		t.Fatal(err)
	}
}

func TestPublisherDryRun(t *testing.T) {
	var out bytes.Buffer
	cfg := DefaultConfig()
	client := NewClient(cfg)
	source := NewTokenSource(client, nil)

	publisher, err := NewPublisher(context.Background(), client, source, PublisherOptions{
		GameName: "Test", Out: &out, DryRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	publisher.Offer(Line{Text: "dry run line"})

	closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := publisher.Close(closeCtx); err != nil {
		t.Fatal(err)
	}

	decoder := json.NewDecoder(bytes.NewReader(out.Bytes()))
	var sawLine bool
	for {
		var frame map[string]any
		if err := decoder.Decode(&frame); err != nil {
			break
		}
		if frame["type"] == "line" && frame["text"] == "dry run line" {
			sawLine = true
		}
	}
	if !sawLine {
		t.Fatalf("dry run did not print the line:\n%s", out.String())
	}
}

func TestResumeStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	store := newResumeStore(path)
	store.save("game", resumeEntry{SessionID: "s1", LastClientSeq: 4, UpdatedAt: time.Now()})

	reloaded := newResumeStore(path)
	entry, ok := reloaded.get("game")
	if !ok || entry.SessionID != "s1" || entry.LastClientSeq != 4 {
		t.Fatalf("resume entry: %+v ok=%v", entry, ok)
	}

	reloaded.clear("game")
	if _, ok := newResumeStore(path).get("game"); ok {
		t.Fatal("clear did not remove the entry")
	}
}

func TestPublisherNamesTheSocketWhenHelloNeverArrives(t *testing.T) {
	oldTimeout := handshakeTimeout
	handshakeTimeout = 300 * time.Millisecond
	defer func() { handshakeTimeout = oldTimeout }()

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		// Accept the upgrade and stay silent.
		time.Sleep(2 * time.Second)
	}))
	defer server.Close()

	publisher, _ := testPublisher(t, server.URL, filepath.Join(t.TempDir(), "sessions.json"))
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = publisher.Close(ctx)
	}()

	publisher.Offer(Line{Text: "テスト", Kind: "dialogue"})

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if stats := publisher.Stats(); stats.LastError != "" {
			if !strings.Contains(stats.LastError, "/live/capture") {
				t.Fatalf("error does not name the socket: %q", stats.LastError)
			}
			if !strings.Contains(stats.LastError, "no hello_ok") || !strings.Contains(stats.LastError, "--live-ws") {
				t.Fatalf("error does not explain the failure: %q", stats.LastError)
			}
			if publisher.Fatal() != nil {
				t.Fatalf("a silent socket should be retried, not fatal: %v", publisher.Fatal())
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("no connection attempt was recorded")
}
