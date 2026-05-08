package tts

import "testing"

func TestF5BuildArgsUsesJapaneseDefaultsForJapaneseLanguage(t *testing.T) {
	f := NewF5Unchecked(F5Config{})
	speaker := Speaker{
		ID:             "speaker",
		VoiceClipsPath: []string{"ref.wav"},
		ReferenceText:  "こんにちは。",
	}

	args := f.buildArgs("今日はいい天気です。", speaker, NewOptions(WithLanguage("ja")))

	if got := argValue(args, "-m"); got != japaneseF5Model {
		t.Fatalf("model = %q, want %q", got, japaneseF5Model)
	}
	if got := argValue(args, "-p"); got != japaneseF5Checkpoint {
		t.Fatalf("checkpoint = %q, want %q", got, japaneseF5Checkpoint)
	}
	if got := argValue(args, "-v"); got != japaneseF5Vocab {
		t.Fatalf("vocab = %q, want %q", got, japaneseF5Vocab)
	}
}

func TestF5BuildArgsUsesSpeakerJapaneseLanguage(t *testing.T) {
	f := NewF5Unchecked(F5Config{})
	speaker := Speaker{
		ID:             "speaker",
		VoiceClipsPath: []string{"ref.wav"},
		ReferenceText:  "こんにちは。",
		Language:       "japanese",
	}

	args := f.buildArgs("こんにちは。", speaker, NewOptions())

	if got := argValue(args, "-m"); got != japaneseF5Model {
		t.Fatalf("model = %q, want %q", got, japaneseF5Model)
	}
}

func TestF5BuildArgsDoesNotOverrideExplicitJapaneseModelFiles(t *testing.T) {
	f := NewF5Unchecked(F5Config{})
	speaker := Speaker{
		ID:             "speaker",
		VoiceClipsPath: []string{"ref.wav"},
		ReferenceText:  "こんにちは。",
		Language:       "ja-JP",
	}

	args := f.buildArgs("こんにちは。", speaker, NewOptions(
		WithModel("custom-ja"),
		WithCheckpoint("custom.pt"),
		WithVocab("custom.txt"),
	))

	if got := argValue(args, "-m"); got != "custom-ja" {
		t.Fatalf("model = %q, want custom-ja", got)
	}
	if got := argValue(args, "-p"); got != "custom.pt" {
		t.Fatalf("checkpoint = %q, want custom.pt", got)
	}
	if got := argValue(args, "-v"); got != "custom.txt" {
		t.Fatalf("vocab = %q, want custom.txt", got)
	}
}

func TestF5BuildArgsKeepsDefaultForUnspecifiedLanguage(t *testing.T) {
	f := NewF5Unchecked(F5Config{})
	speaker := Speaker{
		ID:             "speaker",
		VoiceClipsPath: []string{"ref.wav"},
		ReferenceText:  "Hello.",
	}

	args := f.buildArgs("Hello.", speaker, NewOptions())

	if got := argValue(args, "-m"); got != defaultF5Model {
		t.Fatalf("model = %q, want %q", got, defaultF5Model)
	}
	if got := argValue(args, "-p"); got != "" {
		t.Fatalf("checkpoint = %q, want empty", got)
	}
	if got := argValue(args, "-v"); got != "" {
		t.Fatalf("vocab = %q, want empty", got)
	}
}

func argValue(args []string, flag string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}
