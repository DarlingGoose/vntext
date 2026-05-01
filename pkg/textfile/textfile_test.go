package textfile

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

func TestDecode_Table(t *testing.T) {
	tests := []struct {
		name        string
		raw         []byte
		wantText    string
		wantEnc     EncodingName
		wantNewline NewlineStyle
		wantFinalNL bool
	}{
		{
			name:        "ascii crlf",
			raw:         []byte("line 1\r\nline 2\r\n"),
			wantText:    "line 1\nline 2\n",
			wantEnc:     EncodingASCII,
			wantNewline: NewlineCRLF,
			wantFinalNL: true,
		},
		{
			name:        "ascii lf no final newline",
			raw:         []byte("line 1\nline 2"),
			wantText:    "line 1\nline 2",
			wantEnc:     EncodingASCII,
			wantNewline: NewlineLF,
			wantFinalNL: false,
		},
		{
			name:        "utf8 japanese lf",
			raw:         []byte("こんにちは\n世界\n"),
			wantText:    "こんにちは\n世界\n",
			wantEnc:     EncodingUTF8,
			wantNewline: NewlineLF,
			wantFinalNL: true,
		},
		{
			name:        "utf8 bom crlf",
			raw:         append([]byte{0xEF, 0xBB, 0xBF}, []byte("hello\r\nworld\r\n")...),
			wantText:    "hello\nworld\n",
			wantEnc:     EncodingUTF8BOM,
			wantNewline: NewlineCRLF,
			wantFinalNL: true,
		},
		{
			name:        "shift jis japanese crlf",
			raw:         mustEncodeShiftJIS(t, "はい\r\nいいえ\r\n"),
			wantText:    "はい\nいいえ\n",
			wantEnc:     EncodingShiftJIS,
			wantNewline: NewlineCRLF,
			wantFinalNL: true,
		},
		{
			name:        "shift jis japanese no final newline",
			raw:         mustEncodeShiftJIS(t, "グラフィック設定\r\nあとで変更可"),
			wantText:    "グラフィック設定\nあとで変更可",
			wantEnc:     EncodingShiftJIS,
			wantNewline: NewlineCRLF,
			wantFinalNL: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			style, gotText, err := Decode(tt.raw)
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}

			if gotText != tt.wantText {
				t.Fatalf("text mismatch\nwant: %q\n got: %q", tt.wantText, gotText)
			}

			if style.Encoding != tt.wantEnc {
				t.Fatalf("encoding mismatch: want %q, got %q", tt.wantEnc, style.Encoding)
			}

			if style.Newline != tt.wantNewline {
				t.Fatalf("newline mismatch: want %q, got %q", tt.wantNewline, style.Newline)
			}

			if style.HadFinalNewline != tt.wantFinalNL {
				t.Fatalf("HadFinalNewline mismatch: want %v, got %v", tt.wantFinalNL, style.HadFinalNewline)
			}
		})
	}
}

func TestEncode_Table(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		style   FileStyle
		wantRaw []byte
	}{
		{
			name: "ascii crlf",
			text: "line 1\nline 2\n",
			style: FileStyle{
				Encoding: EncodingASCII,
				Newline:  NewlineCRLF,
			},
			wantRaw: []byte("line 1\r\nline 2\r\n"),
		},
		{
			name: "utf8 lf",
			text: "こんにちは\n世界\n",
			style: FileStyle{
				Encoding: EncodingUTF8,
				Newline:  NewlineLF,
			},
			wantRaw: []byte("こんにちは\n世界\n"),
		},
		{
			name: "utf8 bom crlf",
			text: "hello\nworld\n",
			style: FileStyle{
				Encoding: EncodingUTF8BOM,
				Newline:  NewlineCRLF,
			},
			wantRaw: append([]byte{0xEF, 0xBB, 0xBF}, []byte("hello\r\nworld\r\n")...),
		},
		{
			name: "shift jis crlf",
			text: "はい\nいいえ\n",
			style: FileStyle{
				Encoding: EncodingShiftJIS,
				Newline:  NewlineCRLF,
			},
			wantRaw: mustEncodeShiftJIS(t, "はい\r\nいいえ\r\n"),
		},
		{
			name: "ascii with japanese upgrades to utf8 bytes",
			text: "hello\nこんにちは\n",
			style: FileStyle{
				Encoding: EncodingASCII,
				Newline:  NewlineLF,
			},
			wantRaw: []byte("hello\nこんにちは\n"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRaw, err := Encode(tt.text, tt.style)
			if err != nil {
				t.Fatalf("Encode() error = %v", err)
			}

			if !bytes.Equal(gotRaw, tt.wantRaw) {
				t.Fatalf("raw mismatch\nwant: % x\n got: % x", tt.wantRaw, gotRaw)
			}
		})
	}
}

func TestRoundTripDecodeEncode_Table(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
	}{
		{
			name: "ascii crlf",
			raw:  []byte("Scripts.execStorage(\"system/Initialize.tjs\");\r\n"),
		},
		{
			name: "utf8 bom crlf",
			raw: append(
				[]byte{0xEF, 0xBB, 0xBF},
				[]byte("Scripts.execStorage(\"system/Initialize.tjs\");\r\n")...,
			),
		},
		{
			name: "shift jis crlf",
			raw:  mustEncodeShiftJIS(t, "System.inform(\"はい\");\r\nSystem.inform(\"いいえ\");\r\n"),
		},
		{
			name: "shift jis no final newline",
			raw:  mustEncodeShiftJIS(t, "System.inform(\"はい\");\r\nSystem.inform(\"いいえ\");"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			style, text, err := Decode(tt.raw)
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}

			gotRaw, err := Encode(text, style)
			if err != nil {
				t.Fatalf("Encode() error = %v", err)
			}

			if !bytes.Equal(gotRaw, tt.raw) {
				t.Fatalf("round trip mismatch\nwant: % x\n got: % x", tt.raw, gotRaw)
			}
		})
	}
}

func TestReadWritePreserve_Table(t *testing.T) {
	tests := []struct {
		name       string
		initialRaw []byte
		updateText string
		wantEnc    EncodingName
		wantNL     NewlineStyle
		wantRaw    []byte
	}{
		{
			name:       "ascii crlf remains ascii crlf",
			initialRaw: []byte("line 1\r\nline 2\r\n"),
			updateText: "line 1\nline 2\nline 3\n",
			wantEnc:    EncodingASCII,
			wantNL:     NewlineCRLF,
			wantRaw:    []byte("line 1\r\nline 2\r\nline 3\r\n"),
		},
		{
			name:       "shift jis crlf remains shift jis crlf",
			initialRaw: mustEncodeShiftJIS(t, "はい\r\nいいえ\r\n"),
			updateText: "はい\nいいえ\nキャンセル\n",
			wantEnc:    EncodingShiftJIS,
			wantNL:     NewlineCRLF,
			wantRaw:    mustEncodeShiftJIS(t, "はい\r\nいいえ\r\nキャンセル\r\n"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "script.tjs")

			if err := os.WriteFile(path, tt.initialRaw, 0644); err != nil {
				t.Fatal(err)
			}

			tf, err := Read(path)
			if err != nil {
				t.Fatalf("Read() error = %v", err)
			}

			if tf.Style.Encoding != tt.wantEnc {
				t.Fatalf("encoding mismatch: want %q, got %q", tt.wantEnc, tf.Style.Encoding)
			}

			if tf.Style.Newline != tt.wantNL {
				t.Fatalf("newline mismatch: want %q, got %q", tt.wantNL, tf.Style.Newline)
			}

			if err := Write(path, tt.updateText, tf.Style, 0644); err != nil {
				t.Fatalf("Write() error = %v", err)
			}

			gotRaw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(gotRaw, tt.wantRaw) {
				t.Fatalf("raw mismatch\nwant: % x\n got: % x", tt.wantRaw, gotRaw)
			}
		})
	}
}

func TestAppend_Table(t *testing.T) {
	tests := []struct {
		name       string
		initialRaw []byte
		suffix     string
		wantRaw    []byte
	}{
		{
			name:       "append ascii crlf",
			initialRaw: []byte("line 1\r\n"),
			suffix:     "line 2\n",
			wantRaw:    []byte("line 1\r\nline 2\r\n"),
		},
		{
			name:       "append shift jis crlf",
			initialRaw: mustEncodeShiftJIS(t, "はい\r\n"),
			suffix:     "いいえ\n",
			wantRaw:    mustEncodeShiftJIS(t, "はい\r\nいいえ\r\n"),
		},
		{
			name:       "append ascii no final newline",
			initialRaw: []byte("line 1"),
			suffix:     "\nline 2\n",
			wantRaw:    []byte("line 1\nline 2\n"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "script.tjs")

			if err := os.WriteFile(path, tt.initialRaw, 0644); err != nil {
				t.Fatal(err)
			}

			if err := Append(path, tt.suffix); err != nil {
				t.Fatalf("Append() error = %v", err)
			}

			gotRaw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(gotRaw, tt.wantRaw) {
				t.Fatalf("raw mismatch\nwant: % x\n got: % x", tt.wantRaw, gotRaw)
			}
		})
	}
}

func TestReplace_Table(t *testing.T) {
	tests := []struct {
		name       string
		initialRaw []byte
		old        string
		new        string
		n          int
		wantRaw    []byte
	}{
		{
			name:       "replace ascii crlf",
			initialRaw: []byte("hello\r\nworld\r\n"),
			old:        "world",
			new:        "kirikiri",
			n:          1,
			wantRaw:    []byte("hello\r\nkirikiri\r\n"),
		},
		{
			name:       "replace shift jis crlf",
			initialRaw: mustEncodeShiftJIS(t, "はい\r\nいいえ\r\n"),
			old:        "いいえ",
			new:        "キャンセル",
			n:          1,
			wantRaw:    mustEncodeShiftJIS(t, "はい\r\nキャンセル\r\n"),
		},
		{
			name:       "replace only one occurrence",
			initialRaw: []byte("x\r\nx\r\nx\r\n"),
			old:        "x",
			new:        "y",
			n:          1,
			wantRaw:    []byte("y\r\nx\r\nx\r\n"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "script.tjs")

			if err := os.WriteFile(path, tt.initialRaw, 0644); err != nil {
				t.Fatal(err)
			}

			if err := Replace(path, tt.old, tt.new, tt.n); err != nil {
				t.Fatalf("Replace() error = %v", err)
			}

			gotRaw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(gotRaw, tt.wantRaw) {
				t.Fatalf("raw mismatch\nwant: % x\n got: % x", tt.wantRaw, gotRaw)
			}
		})
	}
}

func TestUpdate_Table(t *testing.T) {
	tests := []struct {
		name       string
		initialRaw []byte
		update     func(string, FileStyle) (string, error)
		wantRaw    []byte
	}{
		{
			name:       "insert startup hook ascii crlf",
			initialRaw: []byte("Scripts.execStorage(\"system/Initialize.tjs\");\r\n"),
			update: func(s string, style FileStyle) (string, error) {
				line := `Scripts.execStorage("text_logger.tjs");`

				if strings.Contains(s, line) {
					return s, nil
				}

				if !strings.HasSuffix(s, "\n") {
					s += "\n"
				}

				s += line + "\n"
				return s, nil
			},
			wantRaw: []byte(
				"Scripts.execStorage(\"system/Initialize.tjs\");\r\n" +
					"Scripts.execStorage(\"text_logger.tjs\");\r\n",
			),
		},
		{
			name:       "insert startup hook shift jis crlf",
			initialRaw: mustEncodeShiftJIS(t, "System.inform(\"開始\");\r\n"),
			update: func(s string, style FileStyle) (string, error) {
				line := `System.inform("ログ開始");`

				if strings.Contains(s, line) {
					return s, nil
				}

				if !strings.HasSuffix(s, "\n") {
					s += "\n"
				}

				s += line + "\n"
				return s, nil
			},
			wantRaw: mustEncodeShiftJIS(t, "System.inform(\"開始\");\r\nSystem.inform(\"ログ開始\");\r\n"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "startup.tjs")

			if err := os.WriteFile(path, tt.initialRaw, 0644); err != nil {
				t.Fatal(err)
			}

			if err := Update(path, tt.update); err != nil {
				t.Fatalf("Update() error = %v", err)
			}

			gotRaw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(gotRaw, tt.wantRaw) {
				t.Fatalf("raw mismatch\nwant: % x\n got: % x", tt.wantRaw, gotRaw)
			}
		})
	}
}

func TestDetectNewline_Table(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want NewlineStyle
	}{
		{
			name: "crlf",
			raw:  []byte("a\r\nb\r\n"),
			want: NewlineCRLF,
		},
		{
			name: "lf",
			raw:  []byte("a\nb\n"),
			want: NewlineLF,
		},
		{
			name: "cr",
			raw:  []byte("a\rb\r"),
			want: NewlineCR,
		},
		{
			name: "mixed prefers most common crlf",
			raw:  []byte("a\r\nb\r\nc\n"),
			want: NewlineCRLF,
		},
		{
			name: "mixed prefers lf",
			raw:  []byte("a\nb\nc\r\n"),
			want: NewlineLF,
		},
		{
			name: "no newline defaults lf",
			raw:  []byte("abc"),
			want: NewlineLF,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectNewline(tt.raw)
			if got != tt.want {
				t.Fatalf("detectNewline() = %q, want %q", got, tt.want)
			}
		})
	}
}

func mustEncodeShiftJIS(t *testing.T, s string) []byte {
	t.Helper()

	out, _, err := transform.String(japanese.ShiftJIS.NewEncoder(), s)
	if err != nil {
		t.Fatalf("encode shift jis: %v", err)
	}

	return []byte(out)
}
