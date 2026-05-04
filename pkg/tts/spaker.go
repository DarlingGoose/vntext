package tts

type Speaker struct {
	// Stable ID used by your app.
	// Example: "maya", "rier", "narrator-default"
	ID string

	// Display name.
	// Example: "Maya", "Rier", "Default Narrator"
	Name string

	// F5-TTS needs reference audio and reference text.
	// Prefer one good 5-15 second clip.
	VoiceClipsPath []string
	ReferenceText  string

	// Optional metadata for UI/search.
	TypeOfVoice string
	Descriptors []string
	Language    string
	Gender      string
	Age         string

	// Backend-specific metadata.
	Metadata map[string]string
}

func (s Speaker) BestReferenceAudio() string {
	if len(s.VoiceClipsPath) == 0 {
		return ""
	}
	return s.VoiceClipsPath[0]
}
