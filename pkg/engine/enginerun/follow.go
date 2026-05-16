package enginerun

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/DarlingGoose/vntext/pkg/engine"
	"github.com/DarlingGoose/vntext/pkg/game"
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
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	for sc.Scan() {
		lines = append(lines, sc.Text())
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
	var offset int64

	// If History=false, start at end once the file exists.
	f, err := waitOpen(ctx, path)
	if err != nil {
		return
	}

	if !cfg.History {
		if st, statErr := f.Stat(); statErr == nil {
			offset = st.Size()
		}
	}

	_ = f.Close()

	ticker := time.NewTicker(defaultFollowPoll)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			next, err := readNewLines(ctx, out, path, offset, cfg)
			if err != nil {
				// File might not exist yet, or the game may recreate it.
				// Keep following unless context is cancelled.
				continue
			}
			offset = next
		}
	}
}

func readNewLines(
	ctx context.Context,
	out chan<- engine.Line,
	path string,
	offset int64,
	cfg engine.FollowGameOptions,
) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return offset, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return offset, err
	}

	// Handle truncate/rotation.
	if st.Size() < offset {
		offset = 0
	}

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return offset, err
	}

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	for sc.Scan() {
		raw := strings.TrimRight(sc.Text(), "\r\n")
		if !emitLine(ctx, out, raw, cfg.Filters) {
			pos, _ := f.Seek(0, io.SeekCurrent)
			return pos, ctx.Err()
		}
	}

	if err := sc.Err(); err != nil {
		return offset, err
	}

	pos, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return offset, err
	}

	return pos, nil
}

func waitOpen(ctx context.Context, path string) (*os.File, error) {
	ticker := time.NewTicker(defaultFollowPoll)
	defer ticker.Stop()

	for {
		f, err := os.Open(path)
		if err == nil {
			return f, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func emitLine(
	ctx context.Context,
	out chan<- engine.Line,
	raw string,
	filters []func(*engine.Line) *engine.Line,
) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}

	line, err := engine.ParseLogLine(raw)
	if err != nil {
		line = &engine.Line{
			Raw:  raw,
			Hook: "raw",
			Text: raw,
			Time: time.Now(),
		}
	}

	for _, filter := range filters {
		if filter == nil {
			continue
		}

		line = filter(line)
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
