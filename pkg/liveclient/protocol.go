// Package liveclient publishes visual-novel text to the Kotoba / JPN Learning
// live capture service.
//
// It is deliberately independent of the rest of vntext: its input is the small
// Line type in this package, and its only third-party dependency is
// github.com/gorilla/websocket. If another tool ever needs the same publisher,
// this package can be moved out as-is.
package liveclient

import "time"

// ProtocolVersion is the capture websocket's wire version.
const ProtocolVersion = 1

const (
	// CloseIdle is the close code the server uses when a socket sent nothing
	// inside its idle window. It is an ordinary disconnect: reconnect + resume.
	CloseIdle = 4408

	defaultPingInterval = 20 * time.Second
	defaultReadTimeout  = 60 * time.Second
	defaultWriteTimeout = 10 * time.Second

	// defaultHandshakeTimeout is the deadline for hello → hello_ok. A healthy
	// server answers in well under a second.
	defaultHandshakeTimeout = 10 * time.Second
	ackDrainTimeout         = 5 * time.Second

	reconnectMin = 500 * time.Millisecond
	reconnectMax = 15 * time.Second

	// Unacknowledged frames kept for replay. The server dedupes by client_seq,
	// so replaying them after a reconnect can never duplicate a line.
	pendingLimit = 64

	// Offer queue. A stalled API drops lines instead of stalling the game.
	queueLimit = 512

	maxLineRunes = 2000
)

// handshakeTimeout is the hello → hello_ok deadline. It is a var so tests can
// shorten it; production always uses defaultHandshakeTimeout.
var handshakeTimeout = defaultHandshakeTimeout

// Line is one piece of captured text.
type Line struct {
	Text    string
	Speaker string
	Kind    string
	At      time.Time
	Ref     string // hook identifier, for debugging; the server ignores it today
}

type helloGame struct {
	Name     string `json:"name"`
	Medium   string `json:"medium"`
	Language string `json:"language"`
}

type helloDevice struct {
	Name    string `json:"name,omitempty"`
	Client  string `json:"client,omitempty"`
	Version string `json:"version,omitempty"`
}

type helloOptions struct {
	AutoSave bool `json:"auto_save"`
}

type helloResume struct {
	SessionID     string `json:"session_id"`
	LastClientSeq int64  `json:"last_client_seq"`
}

type helloFrame struct {
	Type     string       `json:"type"`
	Protocol int          `json:"protocol"`
	Game     helloGame    `json:"game"`
	Device   helloDevice  `json:"device"`
	Options  helloOptions `json:"options"`
	Resume   *helloResume `json:"resume,omitempty"`
}

type lineFrame struct {
	Type      string `json:"type"`
	ClientSeq int64  `json:"client_seq"`
	Text      string `json:"text"`
	Speaker   string `json:"speaker,omitempty"`
	Kind      string `json:"kind,omitempty"`
	At        string `json:"at,omitempty"`
	Ref       string `json:"ref,omitempty"`
}

type simpleFrame struct {
	Type   string `json:"type"`
	Reason string `json:"reason,omitempty"`
}

type controlFrame struct {
	Type   string `json:"type"`
	Action string `json:"action,omitempty"`
}

// serverFrame covers every frame the server can send. Unknown fields are
// ignored so a newer server does not break an older client.
type serverFrame struct {
	Type           string `json:"type"`
	Protocol       int    `json:"protocol"`
	SessionID      string `json:"session_id"`
	ProjectID      string `json:"project_id"`
	ProjectName    string `json:"project_name"`
	ProjectCreated bool   `json:"project_created"`
	SectionID      string `json:"section_id"`
	Seq            int64  `json:"seq"`
	AutoSave       bool   `json:"auto_save"`
	ClientSeq      int64  `json:"client_seq"`
	EntryID        string `json:"entry_id"`
	Deduplicated   bool   `json:"deduplicated"`
	Code           string `json:"code"`
	Message        string `json:"message"`
	Action         string `json:"action"`
	RetryAfterMS   int    `json:"retry_after_ms"`
}
