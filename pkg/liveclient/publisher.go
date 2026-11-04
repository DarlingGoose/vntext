package liveclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
)

const clientName = "vntext"

// errClosed is the internal "input finished" signal for the connection loop.
var errClosed = errors.New("publisher closed")

// Stats is a snapshot of what the publisher has done, for `-v` output and
// tests. It is not used for control flow.
type Stats struct {
	Sent       int
	Acked      int
	Dropped    int
	Reconnects int
	LastError  string
}

// PublisherOptions describes one capture stream.
type PublisherOptions struct {
	GameName     string
	DeviceName   string
	Kind         string
	AutoSave     bool
	ResumeFile   string
	ClientName   string
	Version      string
	PingInterval time.Duration

	// DryRun prints frames instead of connecting; useful for debugging a hook.
	DryRun bool
	Out    io.Writer
	Logger *slog.Logger
}

// Publisher owns one live capture stream: a bounded queue of lines, one
// websocket at a time, reconnect/resume, and an ack-before-exit drain.
//
// Offer is safe to call from the game-following goroutine and never blocks.
type Publisher struct {
	client *Client
	tokens *TokenSource
	opts   PublisherOptions
	log    *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc

	lines  chan Line
	closed chan struct{}
	done   chan struct{}

	closeOnce sync.Once

	mu    sync.Mutex
	stats Stats
	fatal error

	// dedupe state
	lastText string
	lastAt   time.Time

	// session state, owned by the run goroutine
	sessionID   string
	projectID   string
	nextSeq     int64
	lastAcked   int64
	pending     []lineFrame
	paused      bool
	resume      *resumeStore
	resumeKey   string
	resumeDirty bool
	lastSave    time.Time
	dropWarned  bool
}

// NewPublisher starts the background stream. It validates options but does not
// touch the network; the first connection happens when the first line arrives.
func NewPublisher(parent context.Context, client *Client, tokens *TokenSource, opts PublisherOptions) (*Publisher, error) {
	if client == nil {
		return nil, errors.New("live client is nil")
	}
	if tokens == nil {
		return nil, errors.New("live token source is nil")
	}
	opts.GameName = strings.TrimSpace(opts.GameName)
	if opts.GameName == "" {
		return nil, errors.New("live capture needs a game name")
	}
	if opts.DeviceName == "" {
		opts.DeviceName = defaultDeviceName()
	}
	if opts.Kind == "" {
		opts.Kind = DefaultKind
	}
	if opts.PingInterval <= 0 {
		opts.PingInterval = defaultPingInterval
	}
	if opts.PingInterval >= defaultReadTimeout {
		return nil, fmt.Errorf("ping interval %s must be below the read timeout %s", opts.PingInterval, defaultReadTimeout)
	}
	if opts.ClientName == "" {
		opts.ClientName = clientName
	}
	if opts.Version == "" {
		opts.Version = buildVersion()
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Out == nil {
		opts.Out = io.Discard
	}

	ctx, cancel := context.WithCancel(parent)
	p := &Publisher{
		client: client,
		tokens: tokens,
		opts:   opts,
		log:    opts.Logger,
		ctx:    ctx,
		cancel: cancel,
		lines:  make(chan Line, queueLimit),
		closed: make(chan struct{}),
		done:   make(chan struct{}),
	}
	p.resumeKey = gameKey(opts.GameName)
	if opts.ResumeFile != "" {
		p.resume = newResumeStore(opts.ResumeFile)
		if entry, ok := p.resume.get(p.resumeKey); ok && time.Since(entry.UpdatedAt) < 12*time.Hour {
			p.sessionID = entry.SessionID
			p.projectID = entry.ProjectID
			p.nextSeq = entry.LastClientSeq
			p.lastAcked = entry.LastClientSeq
			p.log.Info("live: resuming session", "session", entry.SessionID, "last_client_seq", entry.LastClientSeq)
		}
	}

	go p.run()
	return p, nil
}

// Offer queues one line. It returns false for an empty line; a full queue drops
// the oldest queued line so the game is never held up.
func (p *Publisher) Offer(line Line) bool {
	line.Text = strings.TrimSpace(line.Text)
	if line.Text == "" {
		return false
	}
	if utf8.RuneCountInString(line.Text) > maxLineRunes {
		line.Text = string([]rune(line.Text)[:maxLineRunes])
		p.log.Warn("live: line truncated", "limit_runes", maxLineRunes)
	}
	if line.At.IsZero() {
		line.At = time.Now()
	}
	if line.Kind == "" {
		line.Kind = p.opts.Kind
	}

	p.mu.Lock()
	if line.Text == p.lastText && time.Since(p.lastAt) < time.Second {
		p.mu.Unlock()
		return false // a hook re-emitting the same box; the server would dedupe it too
	}
	p.lastText, p.lastAt = line.Text, time.Now()
	p.mu.Unlock()

	select {
	case p.lines <- line:
		return true
	default:
		select {
		case <-p.lines:
		default:
		}
		select {
		case p.lines <- line:
		default:
		}
		p.mu.Lock()
		p.stats.Dropped++
		warn := !p.dropWarned
		p.dropWarned = true
		p.mu.Unlock()
		if warn {
			p.log.Warn("live: queue full; dropping the oldest lines")
		}
		return true
	}
}

// Close stops accepting lines, drains what can be sent, waits for acks, sends
// bye, and waits for the goroutine to finish. It never waits longer than ctx.
func (p *Publisher) Close(ctx context.Context) error {
	p.closeOnce.Do(func() {
		close(p.closed)
	})
	select {
	case <-p.done:
	case <-ctx.Done():
		p.cancel()
		<-p.done
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.fatal
}

// Stats returns a copy of the counters.
func (p *Publisher) Stats() Stats {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stats
}

// Fatal returns the error that stopped publishing, if any. Callers use it to
// print one actionable line; the game itself is never affected.
func (p *Publisher) Fatal() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.fatal
}

func (p *Publisher) setFatal(err error) {
	if err == nil {
		return
	}
	p.mu.Lock()
	if p.fatal == nil {
		p.fatal = err
	}
	p.stats.LastError = err.Error()
	p.mu.Unlock()
}

func (p *Publisher) noteError(err error) {
	p.mu.Lock()
	p.stats.LastError = err.Error()
	p.mu.Unlock()
}

func (p *Publisher) run() {
	defer close(p.done)
	defer p.cancel()

	if p.opts.DryRun {
		p.runDry()
		return
	}

	backoff := reconnectMin
	attempt := 0
	for {
		if p.stopping() && len(p.lines) == 0 {
			p.discardQueue("closed before connecting")
			return
		}
		// Closing right after the last line of a short pipe is normal. Fall
		// through to one connection attempt so those lines can still be sent;
		// stream() drains the queue and says bye.
		if attempt > 0 {
			p.mu.Lock()
			p.stats.Reconnects++
			p.mu.Unlock()
		}
		attempt++
		conn, err := p.connect()
		if err != nil {
			if fatalStop(err) {
				p.setFatal(err)
				p.log.Error("live: capture disabled", "error", err)
				// Keep accepting Offer calls (they are dropped) but stop
				// hammering an API that will never accept us.
				select {
				case <-p.closed:
				case <-p.ctx.Done():
				}
				return
			}
			if p.ctx.Err() != nil || p.stopping() {
				p.discardQueue("closed before connecting")
				return
			}
			p.noteError(err)
			p.log.Warn("live: connect failed", "error", err, "retry_in", backoff)
			if !p.wait(backoff) {
				p.discardQueue("closed while reconnecting")
				return
			}
			backoff = nextBackoff(backoff)
			continue
		}
		backoff = reconnectMin

		err = p.stream(conn)
		_ = conn.Close()
		if errors.Is(err, errClosed) || p.ctx.Err() != nil {
			return
		}
		p.noteError(err)
		p.log.Warn("live: connection ended", "error", err)
		if !p.wait(reconnectMin) {
			p.discardQueue("closed while reconnecting")
			return
		}
	}
}

// connect dials, authenticates and completes the hello handshake. It retries
// once with a freshly minted token when the server answers 401.
func (p *Publisher) connect() (*websocket.Conn, error) {
	endpoints, err := p.client.Endpoints(p.ctx)
	if err != nil {
		return nil, err
	}
	token, err := p.tokens.Token(p.ctx)
	if err != nil {
		return nil, err
	}

	conn, err := p.dial(endpoints.CaptureWS, token)
	if err != nil && errors.Is(err, ErrUnauthorized) && p.canRefreshToken() {
		p.log.Warn("live: capture token rejected; minting a new one")
		if token, refreshErr := p.tokens.Refresh(p.ctx); refreshErr == nil {
			conn, err = p.dial(endpoints.CaptureWS, token)
		}
	}
	if err != nil {
		return nil, err
	}

	reply, err := p.hello(conn, endpoints.CaptureWS)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	// Resend anything the previous connection did not get acknowledged.
	for _, frame := range p.pendingSnapshot() {
		if err := writeJSON(conn, frame); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("resend line %d: %w", frame.ClientSeq, err)
		}
	}

	p.mu.Lock()
	p.sessionID = reply.SessionID
	p.projectID = reply.ProjectID
	if reply.Seq > p.lastAcked {
		// hello_ok.seq is the server's last sequence, not ours; it is only
		// useful when resuming against a session we have no local state for.
		p.resumeDirty = true
	}
	p.mu.Unlock()

	if reply.SessionID != "" {
		fmt.Fprintf(p.opts.Out, "live: session %s → project %s", shortID(reply.SessionID), projectLabel(reply))
		if reply.ProjectCreated {
			fmt.Fprint(p.opts.Out, " (created)")
		}
		if reply.Seq > 0 {
			fmt.Fprintf(p.opts.Out, " · %d lines already", reply.Seq)
		}
		fmt.Fprintln(p.opts.Out)
	}
	if !reply.AutoSave {
		p.log.Warn("live: the session has auto-save off; lines stream but are not saved")
	}
	p.saveResumeIfDue(true)
	return conn, nil
}

func (p *Publisher) dial(captureURL, token string) (*websocket.Conn, error) {
	header := http.Header{"Authorization": []string{"Bearer " + strings.TrimSpace(token)}}
	dialer := websocket.Dialer{HandshakeTimeout: handshakeTimeout}
	conn, resp, err := dialer.DialContext(p.ctx, captureURL, header)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			return nil, fmt.Errorf("%w: create a new token on the live page or run `vntext live login`", ErrUnauthorized)
		}
		if resp != nil {
			return nil, fmt.Errorf("connect to %s: HTTP %d", captureURL, resp.StatusCode)
		}
		return nil, fmt.Errorf("connect to %s: %w", captureURL, err)
	}
	return conn, nil
}

func (p *Publisher) hello(conn *websocket.Conn, captureURL string) (serverFrame, error) {
	p.mu.Lock()
	sessionID := p.sessionID
	lastAcked := p.lastAcked
	p.mu.Unlock()

	frame := helloFrame{
		Type:     "hello",
		Protocol: ProtocolVersion,
		Game:     helloGame{Name: p.opts.GameName, Medium: "game", Language: "ja"},
		Device:   helloDevice{Name: p.opts.DeviceName, Client: p.opts.ClientName, Version: p.opts.Version},
		Options:  helloOptions{AutoSave: p.opts.AutoSave},
	}
	if sessionID != "" {
		frame.Resume = &helloResume{SessionID: sessionID, LastClientSeq: lastAcked}
	}
	if err := writeJSON(conn, frame); err != nil {
		return serverFrame{}, fmt.Errorf("send hello: %w", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(handshakeTimeout))
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				// The upgrade completed and then nothing arrived. A host that
				// proxies the REST API as plain HTTP does exactly this, so the
				// message points at the socket URL rather than the network.
				return serverFrame{}, fmt.Errorf("no hello_ok from %s within %s: the socket opened but the server sent no frame (an HTTP-only /api proxy in front of the API behaves this way; set --live-ws or JPN_LEARNING_WS)", captureURL, handshakeTimeout)
			}
			return serverFrame{}, fmt.Errorf("waiting for hello_ok: %w", err)
		}
		var reply serverFrame
		if err := json.Unmarshal(data, &reply); err != nil {
			continue
		}
		switch reply.Type {
		case "ping":
			if err := writeJSON(conn, simpleFrame{Type: "pong"}); err != nil {
				return serverFrame{}, fmt.Errorf("pong during handshake: %w", err)
			}
		case "hello_ok":
			return reply, nil
		case "error":
			if reply.Code == "unauthorized" || reply.Code == "auth_failed" {
				return serverFrame{}, fmt.Errorf("%w: %s", ErrUnauthorized, reply.Message)
			}
			return serverFrame{}, fmt.Errorf("server refused the connection: %s %s", reply.Code, reply.Message)
		}
	}
}

// stream runs one connection lifetime.
func (p *Publisher) stream(conn *websocket.Conn) error {
	inbound := make(chan serverFrame, 32)
	readErr := make(chan error, 1)
	go func() {
		for {
			_ = conn.SetReadDeadline(time.Now().Add(defaultReadTimeout))
			_, data, err := conn.ReadMessage()
			if err != nil {
				readErr <- err
				return
			}
			var frame serverFrame
			if err := json.Unmarshal(data, &frame); err != nil {
				continue
			}
			select {
			case inbound <- frame:
			default: // a slow consumer drops frames; acks are idempotent
			}
		}
	}()

	ticker := time.NewTicker(p.opts.PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			p.shutdownConnection(conn, inbound)
			return errClosed

		case <-p.closed:
			p.shutdownConnection(conn, inbound)
			return errClosed

		case err := <-readErr:
			if websocket.IsCloseError(err, CloseIdle) {
				return errors.New("the server closed the idle socket (4408); reconnecting")
			}
			return fmt.Errorf("read: %w", err)

		case frame := <-inbound:
			switch frame.Type {
			case "ping":
				if err := writeJSON(conn, simpleFrame{Type: "pong"}); err != nil {
					return fmt.Errorf("pong: %w", err)
				}
			case "ack":
				p.handleAck(frame)
			case "control":
				p.handleControl(frame)
			case "warning":
				p.log.Warn("live: server warning", "code", frame.Code, "message", frame.Message)
			case "error":
				p.log.Error("live: server error", "code", frame.Code, "message", frame.Message)
			}

		case <-ticker.C:
			if err := writeJSON(conn, simpleFrame{Type: "ping"}); err != nil {
				return fmt.Errorf("ping: %w", err)
			}
			p.saveResumeIfDue(false)

		case line, ok := <-p.lines:
			if !ok {
				p.shutdownConnection(conn, inbound)
				return errClosed
			}
			if err := p.sendLine(conn, line); err != nil {
				return err
			}
		}
	}
}

func (p *Publisher) sendLine(conn *websocket.Conn, line Line) error {
	p.mu.Lock()
	if p.paused {
		p.mu.Unlock()
		return nil
	}
	p.nextSeq++
	frame := lineFrame{
		Type:      "line",
		ClientSeq: p.nextSeq,
		Text:      line.Text,
		Speaker:   strings.TrimSpace(line.Speaker),
		Kind:      line.Kind,
		At:        line.At.UTC().Format(time.RFC3339),
		Ref:       line.Ref,
	}
	p.pending = append(p.pending, frame)
	if len(p.pending) > pendingLimit {
		p.pending = p.pending[len(p.pending)-pendingLimit:]
	}
	p.stats.Sent++
	p.mu.Unlock()

	if err := writeJSON(conn, frame); err != nil {
		return fmt.Errorf("send line %d: %w", frame.ClientSeq, err)
	}
	return nil
}

func (p *Publisher) handleAck(frame serverFrame) {
	p.mu.Lock()
	if frame.ClientSeq > p.lastAcked {
		p.lastAcked = frame.ClientSeq
	}
	kept := p.pending[:0]
	for _, pending := range p.pending {
		if pending.ClientSeq > p.lastAcked {
			kept = append(kept, pending)
		}
	}
	p.pending = kept
	p.stats.Acked++
	if frame.Deduplicated && frame.ClientSeq > 0 {
		// A resend after reconnect; nothing is wrong.
		p.log.Debug("live: line already stored", "client_seq", frame.ClientSeq)
	}
	p.resumeDirty = true
	p.mu.Unlock()
}

func (p *Publisher) handleControl(frame serverFrame) {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch frame.Action {
	case "pause":
		if !p.paused {
			p.paused = true
			p.log.Info("live: capture paused from the website")
		}
	case "resume":
		if p.paused {
			p.paused = false
			p.log.Info("live: capture resumed from the website")
		}
	}
}

// shutdownConnection sends what is still queued, waits for outstanding acks,
// then says bye and forgets the resume pointer.
func (p *Publisher) shutdownConnection(conn *websocket.Conn, inbound <-chan serverFrame) {
	// Drain the local queue first; anything unsent before shutdown is worth
	// keeping, and the server acks it in time for the ack drain below.
	for {
		select {
		case line := <-p.lines:
			if err := p.sendLine(conn, line); err != nil {
				p.log.Warn("live: could not send a queued line", "error", err)
				return
			}
		default:
			goto drain
		}
	}
drain:
	p.drainAcks(conn, inbound)
	if err := writeJSON(conn, simpleFrame{Type: "bye", Reason: "vntext exited"}); err != nil {
		p.log.Warn("live: bye was not sent", "error", err)
	}
	if p.resume != nil {
		p.resume.clear(p.resumeKey)
	}
}

func (p *Publisher) drainAcks(conn *websocket.Conn, inbound <-chan serverFrame) {
	deadline := time.After(ackDrainTimeout)
	for {
		if len(p.pendingSnapshot()) == 0 {
			return
		}
		select {
		case frame := <-inbound:
			switch frame.Type {
			case "ping":
				_ = writeJSON(conn, simpleFrame{Type: "pong"})
			case "ack":
				p.handleAck(frame)
			}
		case <-deadline:
			p.log.Warn("live: some lines were not acknowledged before exit", "pending", len(p.pendingSnapshot()))
			return
		}
	}
}

func (p *Publisher) pendingSnapshot() []lineFrame {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]lineFrame(nil), p.pending...)
}

func (p *Publisher) saveResumeIfDue(force bool) {
	p.mu.Lock()
	if p.resume == nil || p.sessionID == "" {
		p.mu.Unlock()
		return
	}
	if !force && (!p.resumeDirty || time.Since(p.lastSave) < 10*time.Second) {
		p.mu.Unlock()
		return
	}
	entry := resumeEntry{
		SessionID:     p.sessionID,
		ProjectID:     p.projectID,
		LastClientSeq: p.lastAcked,
		UpdatedAt:     time.Now(),
	}
	p.resumeDirty = false
	p.lastSave = time.Now()
	p.mu.Unlock()
	p.resume.save(p.resumeKey, entry)
}

func (p *Publisher) wait(d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-p.ctx.Done():
		return false
	case <-p.closed:
		return false
	case <-timer.C:
		return true
	}
}

func (p *Publisher) stopping() bool {
	select {
	case <-p.closed:
		return true
	case <-p.ctx.Done():
		return true
	default:
		return false
	}
}

// discardQueue logs the number of lines that will never be sent.
func (p *Publisher) discardQueue(reason string) {
	count := 0
	for {
		select {
		case <-p.lines:
			count++
		default:
			if count > 0 {
				p.log.Warn("live: discarded queued lines", "count", count, "reason", reason)
			}
			return
		}
	}
}

// runDry prints frames without connecting, for `--live-dry-run`.
func (p *Publisher) runDry() {
	encoder := json.NewEncoder(p.opts.Out)
	fmt.Fprintln(p.opts.Out, `{"type":"hello"}`)
	seq := int64(0)
	for {
		select {
		case <-p.ctx.Done():
			_ = encoder.Encode(simpleFrame{Type: "bye", Reason: "vntext exited"})
			return
		case <-p.closed:
			for {
				select {
				case line := <-p.lines:
					seq++
					_ = encoder.Encode(lineFrame{
						Type:      "line",
						ClientSeq: seq,
						Text:      line.Text,
						Speaker:   line.Speaker,
						Kind:      line.Kind,
						At:        line.At.UTC().Format(time.RFC3339),
						Ref:       line.Ref,
					})
					continue
				default:
				}
				break
			}
			_ = encoder.Encode(simpleFrame{Type: "bye", Reason: "vntext exited"})
			return
		case line := <-p.lines:
			seq++
			_ = encoder.Encode(lineFrame{
				Type:      "line",
				ClientSeq: seq,
				Text:      line.Text,
				Speaker:   line.Speaker,
				Kind:      line.Kind,
				At:        line.At.UTC().Format(time.RFC3339),
				Ref:       line.Ref,
			})
			p.mu.Lock()
			p.stats.Sent++
			p.stats.Acked++
			p.mu.Unlock()
		}
	}
}

func writeJSON(conn *websocket.Conn, value any) error {
	_ = conn.SetWriteDeadline(time.Now().Add(defaultWriteTimeout))
	return conn.WriteJSON(value)
}

func nextBackoff(current time.Duration) time.Duration {
	next := current * 2
	if next > reconnectMax {
		return reconnectMax
	}
	return next
}

// fatalStop is true for errors that retrying cannot fix.
// canRefreshToken reports whether OAuth client credentials are available to
// replace a rejected capture token.
func (p *Publisher) canRefreshToken() bool {
	cfg := p.client.Config()
	return cfg.ClientID != "" && cfg.ClientSecret != ""
}

func fatalStop(err error) bool {
	return errors.Is(err, ErrNotConfigured) || errors.Is(err, ErrUnauthorized)
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func projectLabel(reply serverFrame) string {
	name := strings.TrimSpace(reply.ProjectName)
	if name == "" {
		return shortID(reply.ProjectID)
	}
	return name
}

func defaultDeviceName() string {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		return "vntext"
	}
	return host
}

func buildVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			return info.Main.Version
		}
	}
	return "dev"
}
