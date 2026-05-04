package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	character := flag.String("c", "", "character/speaker name")
	voice := flag.String("v", "", "voice/audio file")
	flag.Parse()
	exe, err := os.Executable()
	if err != nil {
		exe = "."
	}

	dir := filepath.Dir(exe)
	logPath := filepath.Join(dir, "vntext.log")

	msg := strings.Join(flag.Args(), " ")

	// If no args were passed, try stdin.
	if strings.TrimSpace(msg) == "" {
		if b, err := os.ReadFile("/dev/stdin"); err == nil {
			msg = string(b)
		}
	}

	if strings.TrimSpace(msg) == "" {
		msg = "(empty)"
	}

	speaker, msg := normalizeLogMessage(*character, msg)
	voiceFile := strings.TrimSpace(*voice)
	prefix := time.Now().Format(time.RFC3339)

	line := formatLogLine(prefix, speaker, voiceFile, msg)

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		os.Exit(1)
	}
	defer f.Close()

	if _, err := f.WriteString(line); err != nil {
		os.Exit(1)
	}
}

func formatLogLine(timestamp, speaker, voiceFile, msg string) string {
	header := "[" + timestamp + "]"
	if strings.TrimSpace(speaker) != "" {
		header += "[speaker:" + strings.TrimSpace(speaker) + "]"
	}
	if strings.TrimSpace(voiceFile) != "" {
		header += "[voice:" + strings.TrimSpace(voiceFile) + "]"
	}

	return fmt.Sprintf("%s: %s\n", header, msg)
}

func normalizeLogMessage(character, msg string) (string, string) {
	speaker := strings.TrimSpace(character)
	text := stripLeadingNoise(strings.TrimSpace(msg))

	if inferred, cleaned, ok := inferRepeatedSpeakerPrefix(text); ok {
		return inferred, cleaned
	}

	if speaker != "" {
		return speaker, stripSpeakerArtifacts(text, speaker)
	}

	return "", text
}

func stripLeadingNoise(text string) string {
	text = strings.TrimSpace(text)

	for {
		changed := false

		for strings.HasPrefix(text, `\n`) {
			text = strings.TrimSpace(strings.TrimPrefix(text, `\n`))
			changed = true
		}

		if strings.HasPrefix(text, "(未設定)") {
			text = strings.TrimSpace(strings.TrimPrefix(text, "(未設定)"))
			changed = true
		}

		if !changed {
			return text
		}
	}
}

func inferRepeatedSpeakerPrefix(text string) (string, string, bool) {
	idx := firstDialogueOpenerIndex(text)
	if idx <= 0 {
		return "", text, false
	}

	prefix := strings.TrimSpace(text[:idx])
	if isUnknownSpeakerPrefix(prefix) {
		return prefix, stripSpeakerArtifacts(text, prefix), true
	}

	runes := []rune(prefix)
	if len(runes) == 0 || len(runes)%2 != 0 {
		return "", text, false
	}

	half := len(runes) / 2
	first := string(runes[:half])
	second := string(runes[half:])
	if first == "" || first != second {
		return "", text, false
	}

	return first, stripSpeakerArtifacts(text, first), true
}

func firstDialogueOpenerIndex(text string) int {
	idx := strings.Index(text, "「")
	parenIdx := strings.Index(text, "（")
	switch {
	case idx < 0:
		return parenIdx
	case parenIdx >= 0 && parenIdx < idx:
		return parenIdx
	default:
		return idx
	}
}

func isUnknownSpeakerPrefix(prefix string) bool {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return false
	}

	for _, r := range prefix {
		if r != '？' && r != '?' {
			return false
		}
	}

	return true
}

func stripSpeakerArtifacts(text, speaker string) string {
	text = strings.TrimSpace(text)
	speaker = strings.TrimSpace(speaker)
	if text == "" || speaker == "" {
		return text
	}

	switch {
	case strings.HasPrefix(text, speaker+speaker+"「"):
		text = text[len(speaker+speaker):]
	case strings.HasPrefix(text, speaker+speaker+"（"):
		text = text[len(speaker+speaker):]
	case strings.HasPrefix(text, speaker+"「"):
		text = text[len(speaker):]
	case strings.HasPrefix(text, speaker+"（"):
		text = text[len(speaker):]
	}

	for strings.HasPrefix(text, "「「") {
		text = "「" + strings.TrimPrefix(text, "「「")
	}
	for strings.HasPrefix(text, "（（") {
		text = "（" + strings.TrimPrefix(text, "（（")
	}

	text = stripTrailingSpeakerName(text, speaker)

	return strings.TrimSpace(text)
}

func stripTrailingSpeakerName(text, speaker string) string {
	trimmed := strings.TrimSpace(text)
	if !strings.HasSuffix(trimmed, speaker) {
		return trimmed
	}

	withoutSpeaker := strings.TrimSpace(strings.TrimSuffix(trimmed, speaker))
	if strings.HasSuffix(withoutSpeaker, "」") {
		return withoutSpeaker
	}

	return trimmed
}
