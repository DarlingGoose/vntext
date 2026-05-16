package enginerun

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DarlingGoose/vntext/pkg/engine"
	"github.com/DarlingGoose/vntext/pkg/game"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

func TestDecodeFollowLineShiftJIS(t *testing.T) {
	wantText := "フェリアギルで見つけた記録を続けていた二つの森の。"
	raw := mustEncodeShiftJIS(t, "[2026-05-16T10:16:05]: "+wantText+"\r\n")

	got := decodeFollowLine(raw)
	line, err := engine.ParseLogLine(got)
	if err != nil {
		t.Fatal(err)
	}

	if line.Text != wantText {
		t.Fatalf("Text = %q, want %q", line.Text, wantText)
	}
}

func TestDecodeFollowLineUTF8(t *testing.T) {
	wantText := "これはUTF-8の行です。"
	raw := []byte("[2026-05-16T10:23:14]: " + wantText + "\n")

	got := decodeFollowLine(raw)
	line, err := engine.ParseLogLine(got)
	if err != nil {
		t.Fatal(err)
	}

	if line.Text != wantText {
		t.Fatalf("Text = %q, want %q", line.Text, wantText)
	}
}

func TestDecodeFollowLineInvalidBytesFallsBack(t *testing.T) {
	got := decodeFollowLine([]byte("[2026-05-16T10:23:14]: \xff\xff\n"))
	if got == "" {
		t.Fatal("decodeFollowLine returned an empty string")
	}

	line, err := engine.ParseLogLine(got)
	if err != nil {
		t.Fatal(err)
	}
	if line.Text == "" {
		t.Fatal("ParseLogLine returned empty text for fallback line")
	}
}

func TestFollowGameTextStartsAtFileByteEnd(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vntext.log")
	oldText := "この履歴行は出さない。"
	newText := "新しい行だけを出す。"

	if err := os.WriteFile(path, mustEncodeShiftJIS(t, "[2026-05-16T10:23:14]: "+oldText+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lines, err := FollowGameText(ctx, &game.Game{TextHookLogFile: path})
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(defaultFollowPoll + 50*time.Millisecond)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(mustEncodeShiftJIS(t, "[2026-05-16T10:23:15]: "+newText+"\n")); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-lines:
		if got.Text != newText {
			t.Fatalf("Text = %q, want %q", got.Text, newText)
		}
		if strings.Contains(got.Text, oldText) {
			t.Fatalf("follow emitted history text despite starting at byte EOF: %q", got.Text)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for followed line")
	}
}

func mustEncodeShiftJIS(t *testing.T, s string) []byte {
	t.Helper()

	out, _, err := transform.String(japanese.ShiftJIS.NewEncoder(), s)
	if err != nil {
		t.Fatal(err)
	}
	return []byte(out)
}
