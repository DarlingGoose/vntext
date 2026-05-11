package engine

import (
	"errors"
	"strings"
	"time"
)

var ErrInvalidLogLine = errors.New("invalid log line")

func ParseLogLine(line string) (*Line, error) {
	var out Line
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, ErrInvalidLogLine
	}
	if !strings.HasPrefix(line, "[") {
		return &Line{
			Time:    time.Now(),
			RawTime: "",
			Speaker: "",
			Text:    line,
		}, nil
	}
	// First bracket group is always the timestamp.
	rawTime, rest, ok := consumeBracket(line)
	if !ok {
		return nil, ErrInvalidLogLine
	}

	out.RawTime = rawTime

	t, err := parseLooseTime(rawTime)
	if err != nil {
		return nil, err
	}
	out.Time = t

	rest = strings.TrimSpace(rest)

	// Optional metadata groups:
	// [speaker:Alice][other:meta]: text
	for strings.HasPrefix(rest, "[") {
		meta, next, ok := consumeBracket(rest)
		if !ok {
			break
		}

		key, val, found := strings.Cut(meta, ":")
		if found {
			switch strings.ToLower(strings.TrimSpace(key)) {
			case "speaker":
				out.Speaker = strings.TrimSpace(val)
			case "voice":
				out.Voice = strings.TrimSpace(val)
			}
		} else {
			switch strings.ToLower(strings.TrimSpace(meta)) {
			case "system", "new session":
				out.Speaker = strings.TrimSpace(meta)
			}
		}

		rest = strings.TrimSpace(next)
	}

	// Optional ":" after timestamp/metadata.
	rest = strings.TrimSpace(strings.TrimPrefix(rest, ":"))
	out.Text = strings.TrimRight(rest, "\r\n")

	return &out, nil
}

func consumeBracket(s string) (inside string, rest string, ok bool) {
	if !strings.HasPrefix(s, "[") {
		return "", s, false
	}

	end := strings.IndexByte(s, ']')
	if end < 0 {
		return "", s, false
	}

	return s[1:end], s[end+1:], true
}

func parseLooseTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)

	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,

		// JS Date().toISOString()
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z",

		// Common local-ish formats.
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05.999999999 -0700",
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05 -0700",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006/01/02 15:04:05",
		"01/02/2006 15:04:05",
		"2006-01-02",
	}

	var lastErr error
	for _, layout := range layouts {
		t, err := time.Parse(layout, s)
		if err == nil {
			return t, nil
		}
		lastErr = err
	}

	return time.Time{}, lastErr
}
