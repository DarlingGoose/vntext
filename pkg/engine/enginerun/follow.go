package enginerun

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/DarlingGoose/vntext/pkg/engine"
	"github.com/DarlingGoose/vntext/pkg/game"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

const defaultFollowPoll = 250 * time.Millisecond

func FollowGameText(ctx context.Context, g *game.Game, opts ...engine.FollowGameOptions) (chan engine.Line, error) {
	if g == nil {
		return nil, errors.New("game is nil")
	}
	path := strings.TrimSpace(g.TextHookLogFile)
	if path == "" {
		return nil, errors.New("game text hook log file is empty")
	}

	cfg := mergeFollowOptions(opts)
	out := make(chan engine.Line, 64)

	go func() {
		defer close(out)
		if cfg.History {
			emitHistory(ctx, out, path, cfg)
		}
		followFile(ctx, out, path, cfg)
	}()

	return out, nil
}

func mergeFollowOptions(opts []engine.FollowGameOptions) engine.FollowGameOptions {
	var cfg engine.FollowGameOptions
	for _, opt := range opts {
		if opt.MaxLines > 0 {
			cfg.MaxLines = opt.MaxLines
		}
		if opt.History {
			cfg.History = true
		}
		cfg.Filters = append(cfg.Filters, opt.Filters...)
	}
	return cfg
}

func emitHistory(ctx context.Context, out chan<- engine.Line, path string, cfg engine.FollowGameOptions) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, decodeFollowLine(sc.Bytes()))
	}

	if cfg.MaxLines > 0 && len(lines) > cfg.MaxLines {
		lines = lines[len(lines)-cfg.MaxLines:]
	}

	for _, raw := range lines {
		if !emitLine(ctx, out, raw, cfg.Filters) {
			return
		}
	}
}

func followFile(ctx context.Context, out chan<- engine.Line, path string, cfg engine.FollowGameOptions) {
	f, err := waitOpen(ctx, path)
	if err != nil {
		return
	}
	defer f.Close()

	_, _ = f.Seek(0, io.SeekEnd)
	reader := bufio.NewReader(f)

	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 && !emitLine(ctx, out, decodeFollowLine([]byte(line)), cfg.Filters) {
			return
		}
		if err == nil {
			continue
		}
		if !errors.Is(err, io.EOF) {
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(defaultFollowPoll):
		}
	}
}

func decodeFollowLine(raw []byte) string {
	raw = bytes.TrimRight(raw, "\r\n")

	if utf8.Valid(raw) {
		return string(raw)
	}

	decoded, _, err := transform.String(japanese.ShiftJIS.NewDecoder(), string(raw))
	if err == nil && !strings.ContainsRune(decoded, utf8.RuneError) {
		return decoded
	}

	return string(raw)
}

func waitOpen(ctx context.Context, path string) (*os.File, error) {
	for {
		f, err := os.Open(path)
		if err == nil {
			return f, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(defaultFollowPoll):
		}
	}
}

func emitLine(ctx context.Context, out chan<- engine.Line, raw string, filters []func(*engine.Line) *engine.Line) bool {
	line, err := engine.ParseLogLine(raw)
	if err != nil {
		line = &engine.Line{Raw: raw}
	}
	for _, filter := range filters {
		if filter != nil {
			line = filter(line)
		}
		if line == nil {
			return true
		}
	}

	select {
	case <-ctx.Done():
		return false
	case out <- *line:
		return true
	}
}

func UnsupportedFile(g *game.Game, file string) (*engine.EngineFileInfo, error) {
	return nil, fmt.Errorf("engine file lookup is not supported for %q", file)
}
