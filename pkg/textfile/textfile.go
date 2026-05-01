package textfile

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

func Read(path string) (*TextFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	style, text, err := Decode(raw)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}

	return &TextFile{
		Path:  path,
		Text:  text,
		Style: style,
		Raw:   raw,
	}, nil
}

func Write(path string, text string, style FileStyle, perm os.FileMode) error {
	raw, err := Encode(text, style)
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}

	return os.WriteFile(path, raw, perm)
}

func WritePreserve(path string, text string) error {
	tf, err := Read(path)
	if err != nil {
		return err
	}

	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	return Write(path, text, tf.Style, info.Mode().Perm())
}

func Append(path string, suffix string) error {
	tf, err := Read(path)
	if err != nil {
		return err
	}

	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	text := tf.Text

	// If the original had a final newline, append after it.
	// If not, keep that style and append directly.
	text += suffix

	return Write(path, text, tf.Style, info.Mode().Perm())
}

func Replace(path string, old string, new string, n int) error {
	tf, err := Read(path)
	if err != nil {
		return err
	}

	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	updated := strings.Replace(tf.Text, old, new, n)

	return Write(path, updated, tf.Style, info.Mode().Perm())
}

func Update(path string, fn func(text string, style FileStyle) (string, error)) error {
	tf, err := Read(path)
	if err != nil {
		return err
	}

	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	updated, err := fn(tf.Text, tf.Style)
	if err != nil {
		return err
	}

	return Write(path, updated, tf.Style, info.Mode().Perm())
}

func Decode(raw []byte) (FileStyle, string, error) {
	style := FileStyle{
		Newline: detectNewline(raw),
	}

	textRaw := raw

	switch {
	case bytes.HasPrefix(raw, []byte{0xEF, 0xBB, 0xBF}):
		style.Encoding = EncodingUTF8BOM
		textRaw = raw[3:]

		if !utf8.Valid(textRaw) {
			return style, "", errors.New("utf-8 BOM present but body is invalid UTF-8")
		}

		text := string(textRaw)
		style.HadFinalNewline = hasFinalNewline(text)
		text = normalizeNewlines(text)
		return style, text, nil

	case bytes.HasPrefix(raw, []byte{0xFF, 0xFE}):
		style.Encoding = EncodingUTF16LEBOM
		decoded, err := decodeWith(unicode.UTF16(unicode.LittleEndian, unicode.ExpectBOM), raw)
		if err != nil {
			return style, "", err
		}
		style.HadFinalNewline = hasFinalNewline(decoded)
		return style, normalizeNewlines(decoded), nil

	case bytes.HasPrefix(raw, []byte{0xFE, 0xFF}):
		style.Encoding = EncodingUTF16BEBOM
		decoded, err := decodeWith(unicode.UTF16(unicode.BigEndian, unicode.ExpectBOM), raw)
		if err != nil {
			return style, "", err
		}
		style.HadFinalNewline = hasFinalNewline(decoded)
		return style, normalizeNewlines(decoded), nil

	case isASCII(raw):
		style.Encoding = EncodingASCII
		text := string(raw)
		style.HadFinalNewline = hasFinalNewline(text)
		return style, normalizeNewlines(text), nil

	case utf8.Valid(raw):
		style.Encoding = EncodingUTF8
		text := string(raw)
		style.HadFinalNewline = hasFinalNewline(text)
		return style, normalizeNewlines(text), nil
	}

	// Japanese game files commonly show up as:
	//
	//   Non-ISO extended-ASCII text, with CRLF line terminators
	//
	// In practice that is often Shift-JIS / Windows-31J / CP932.
	if decoded, ok := tryDecodeJapanese(raw, japanese.ShiftJIS); ok {
		style.Encoding = EncodingShiftJIS
		style.HadFinalNewline = hasFinalNewline(decoded)
		return style, normalizeNewlines(decoded), nil
	}

	if decoded, ok := tryDecodeJapanese(raw, japanese.EUCJP); ok {
		style.Encoding = EncodingEUCJP
		style.HadFinalNewline = hasFinalNewline(decoded)
		return style, normalizeNewlines(decoded), nil
	}

	return style, "", errors.New("unknown text encoding")
}

func Encode(text string, style FileStyle) ([]byte, error) {
	text = normalizeNewlines(text)
	text = applyNewlineStyle(text, style.Newline)

	switch style.Encoding {
	case EncodingASCII:
		if !isASCIIString(text) {
			// ASCII files cannot represent Japanese. Upgrade to UTF-8 rather than corrupting.
			return []byte(text), nil
		}
		return []byte(text), nil

	case EncodingUTF8:
		return []byte(text), nil

	case EncodingUTF8BOM:
		return append([]byte{0xEF, 0xBB, 0xBF}, []byte(text)...), nil

	case EncodingShiftJIS:
		return encodeWith(japanese.ShiftJIS, text)

	case EncodingEUCJP:
		return encodeWith(japanese.EUCJP, text)

	case EncodingUTF16LEBOM:
		enc := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM)
		return encodeWith(enc, text)

	case EncodingUTF16BEBOM:
		enc := unicode.UTF16(unicode.BigEndian, unicode.UseBOM)
		return encodeWith(enc, text)

	default:
		return nil, fmt.Errorf("unsupported encoding: %s", style.Encoding)
	}
}

func detectNewline(raw []byte) NewlineStyle {
	crlf := bytes.Count(raw, []byte("\r\n"))
	lf := bytes.Count(raw, []byte("\n"))
	cr := bytes.Count(raw, []byte("\r")) - crlf

	plainLF := lf - crlf

	if crlf == 0 && plainLF == 0 && cr == 0 {
		return NewlineLF
	}

	switch {
	case crlf >= plainLF && crlf >= cr:
		return NewlineCRLF
	case cr >= plainLF:
		return NewlineCR
	default:
		return NewlineLF
	}
}

func normalizeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}

func applyNewlineStyle(s string, style NewlineStyle) string {
	s = normalizeNewlines(s)

	switch style {
	case NewlineCRLF:
		return strings.ReplaceAll(s, "\n", "\r\n")
	case NewlineCR:
		return strings.ReplaceAll(s, "\n", "\r")
	default:
		return s
	}
}

func hasFinalNewline(s string) bool {
	return strings.HasSuffix(s, "\n") || strings.HasSuffix(s, "\r\n") || strings.HasSuffix(s, "\r")
}

func isASCII(b []byte) bool {
	for _, c := range b {
		if c > 0x7F {
			return false
		}
	}
	return true
}

func isASCIIString(s string) bool {
	for _, r := range s {
		if r > 0x7F {
			return false
		}
	}
	return true
}

func decodeWith(enc encoding.Encoding, raw []byte) (string, error) {
	out, _, err := transform.String(enc.NewDecoder(), string(raw))
	if err != nil {
		return "", err
	}
	return out, nil
}

func encodeWith(enc encoding.Encoding, text string) ([]byte, error) {
	out, _, err := transform.String(enc.NewEncoder(), text)
	if err != nil {
		return nil, err
	}
	return []byte(out), nil
}

func tryDecodeJapanese(raw []byte, enc encoding.Encoding) (string, bool) {
	decoded, err := decodeWith(enc, raw)
	if err != nil {
		return "", false
	}

	// Reject obvious replacement-character-heavy decodes.
	// This is not perfect, but prevents many false positives.
	replacementCount := strings.Count(decoded, "\uFFFD")
	if replacementCount > 0 {
		return "", false
	}

	return decoded, true
}
