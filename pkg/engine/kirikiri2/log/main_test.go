package main

import "testing"

func TestNormalizeLogMessageInfersRepeatedSpeakerPrefix(t *testing.T) {
	tests := []struct {
		name        string
		in          string
		wantSpeaker string
		wantText    string
	}{
		{
			name:        "kiria",
			in:          "キリアキリア「「話が違う……か」 キリア",
			wantSpeaker: "キリア",
			wantText:    "「話が違う……か」",
		},
		{
			name:        "dino",
			in:          "ディノディノ「「閣下、それでは話が違います」 ディノ",
			wantSpeaker: "ディノ",
			wantText:    "「閣下、それでは話が違います」",
		},
		{
			name:        "unknown",
			in:          "？？？？「「………………」",
			wantSpeaker: "？？？？",
			wantText:    "「………………」",
		},
		{
			name:        "parenthetical aside",
			in:          "ディノディノ（（糞が……）",
			wantSpeaker: "ディノ",
			wantText:    "（糞が……）",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSpeaker, gotText := normalizeLogMessage("", tt.in)
			if gotSpeaker != tt.wantSpeaker || gotText != tt.wantText {
				t.Fatalf(
					"normalizeLogMessage() = speaker %q text %q, want speaker %q text %q",
					gotSpeaker,
					gotText,
					tt.wantSpeaker,
					tt.wantText,
				)
			}
		})
	}
}

func TestNormalizeLogMessageInlineSpeakerOverridesStaleSpeaker(t *testing.T) {
	gotSpeaker, gotText := normalizeLogMessage("ディノ", "キリアキリア「「話が違う……か」")
	if gotSpeaker != "キリア" || gotText != "「話が違う……か」" {
		t.Fatalf("normalizeLogMessage() = speaker %q text %q", gotSpeaker, gotText)
	}
}

func TestNormalizeLogMessageStripsLeadingUnsetNoise(t *testing.T) {
	in := `\n\n(未設定)\n(未設定)\n(未設定)人類が宇宙へと広がりはじめた時代。`
	gotSpeaker, gotText := normalizeLogMessage("", in)
	if gotSpeaker != "" || gotText != "人類が宇宙へと広がりはじめた時代。" {
		t.Fatalf("normalizeLogMessage() = speaker %q text %q", gotSpeaker, gotText)
	}
}

func TestNormalizeLogMessageStripsKnownSpeakerArtifacts(t *testing.T) {
	gotSpeaker, gotText := normalizeLogMessage("キリア", "キリアキリア「「話が違う……か」 キリア")
	if gotSpeaker != "キリア" || gotText != "「話が違う……か」" {
		t.Fatalf("normalizeLogMessage() = speaker %q text %q", gotSpeaker, gotText)
	}
}

func TestNormalizeLogMessageLeavesPlainNarrationWithoutSpeaker(t *testing.T) {
	gotSpeaker, gotText := normalizeLogMessage("", "話が違う……か")
	if gotSpeaker != "" || gotText != "話が違う……か" {
		t.Fatalf("normalizeLogMessage() = speaker %q text %q", gotSpeaker, gotText)
	}
}

func TestFormatLogLineIncludesVoice(t *testing.T) {
	got := formatLogLine("2026-05-04T12:00:00-07:00", "ディノ", "voice/dino_001.ogg", "それでは話が違う！")
	want := "[2026-05-04T12:00:00-07:00][speaker:ディノ][voice:voice/dino_001.ogg]: それでは話が違う！\n"
	if got != want {
		t.Fatalf("formatLogLine() = %q, want %q", got, want)
	}
}
