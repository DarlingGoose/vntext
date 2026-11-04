package liveclient

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
)

// resumeEntry remembers the session a game was streaming into, so a restarted
// vntext continues the same project section instead of creating a new one.
type resumeEntry struct {
	SessionID     string    `json:"session_id"`
	ProjectID     string    `json:"project_id,omitempty"`
	LastClientSeq int64     `json:"last_client_seq"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type resumeFile struct {
	Games map[string]resumeEntry `json:"games"`
}

type resumeStore struct {
	path string

	mu   sync.Mutex
	data resumeFile
}

func newResumeStore(path string) *resumeStore {
	store := &resumeStore{path: strings.TrimSpace(path)}
	store.data = resumeFile{Games: map[string]resumeEntry{}}
	if store.path == "" {
		return store
	}
	raw, err := os.ReadFile(store.path)
	if err != nil {
		return store
	}
	var file resumeFile
	if err := json.Unmarshal(raw, &file); err != nil || file.Games == nil {
		return store
	}
	store.data = file
	return store
}

func (s *resumeStore) get(key string) (resumeEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.data.Games[key]
	return entry, ok
}

func (s *resumeStore) save(key string, entry resumeEntry) {
	if s.path == "" {
		return
	}
	s.mu.Lock()
	if s.data.Games == nil {
		s.data.Games = map[string]resumeEntry{}
	}
	s.data.Games[key] = entry
	raw, err := json.MarshalIndent(s.data, "", "  ")
	s.mu.Unlock()
	if err != nil {
		return
	}
	raw = append(raw, '\n')
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".sessions-*.json")
	if err != nil {
		return
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return
	}
	if err := tmp.Close(); err != nil {
		return
	}
	_ = os.Rename(name, s.path)
}

func (s *resumeStore) clear(key string) {
	if s.path == "" {
		return
	}
	s.mu.Lock()
	delete(s.data.Games, key)
	raw, err := json.MarshalIndent(s.data, "", "  ")
	s.mu.Unlock()
	if err != nil {
		return
	}
	_ = os.WriteFile(s.path, append(raw, '\n'), 0o600)
}

// gameKey is a stable, filesystem-safe key for a game name.
func gameKey(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var builder strings.Builder
	lastDash := false
	for _, r := range name {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			builder.WriteRune(r)
			lastDash = false
		case r == '\'' || r == '’':
			// "Fate/stay night" and "Fate-stay night" should match.
		default:
			if !lastDash && builder.Len() > 0 {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}
	key := strings.Trim(builder.String(), "-")
	if key == "" {
		return "game"
	}
	return key
}
