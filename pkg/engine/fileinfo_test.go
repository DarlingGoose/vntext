package engine

import "testing"

func TestNewEngineFileInfo(t *testing.T) {
	info := NewEngineFileInfo("/game/audio/voice/dino001.ogg", []byte("audio"))

	if info.Name != "dino001.ogg" {
		t.Fatalf("Name = %q, want dino001.ogg", info.Name)
	}
	if string(info.Data) != "audio" {
		t.Fatalf("Data = %q, want audio", string(info.Data))
	}
	if info.Ext != ".ogg" {
		t.Fatalf("Ext = %q, want .ogg", info.Ext)
	}
	if info.MediaType != "audio/ogg" {
		t.Fatalf("MediaType = %q, want audio/ogg", info.MediaType)
	}
}
