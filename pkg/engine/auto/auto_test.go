package auto

import (
	"os"
	"path/filepath"
	"testing"
)

//func TestSelectEngineMapsRPGMakerOldDetection(t *testing.T) {
//	root := t.TempDir()
//	if err := os.MkdirAll(filepath.Join(root, "System"), 0o755); err != nil {
//		t.Fatalf("create system dir: %v", err)
//	}
//	writeAutoTestFile(t, filepath.Join(root, "Game.exe"), "")
//	writeAutoTestFile(t, filepath.Join(root, "Game.ini"), "")
//	writeAutoTestFile(t, filepath.Join(root, "Game.rgss3a"), "")
//	writeAutoTestFile(t, filepath.Join(root, "System", "RGSS301.dll"), "")
//
//	eng, err := SelectEngine(root)
//	if err != nil {
//		t.Fatalf("SelectEngine(%q) returned error: %v", root, err)
//	}
//	if eng.Name() != "rpgmaker-xp-vx-ace" {
//		t.Fatalf("expected rpgmaker-xp-vx-ace engine, got %q", eng.Name())
//	}
//}

func TestSelectEngineDoesNotUseBroadDetectorForExeParent(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "Goodbye Tired Stars 1.05.exe")
	writeAutoTestFile(t, exe, "")

	extracted := filepath.Join(root, "Goodbye Tired Stars 1.05")
	if err := os.MkdirAll(filepath.Join(extracted, "System"), 0o755); err != nil {
		t.Fatalf("create system dir: %v", err)
	}
	writeAutoTestFile(t, filepath.Join(extracted, "Game.exe"), "")
	writeAutoTestFile(t, filepath.Join(extracted, "Game.ini"), "")
	writeAutoTestFile(t, filepath.Join(extracted, "Game.rgss3a"), "")
	writeAutoTestFile(t, filepath.Join(extracted, "System", "RGSS301.dll"), "")

	if _, err := SelectEngine(exe); err == nil {
		t.Fatalf("SelectEngine should not detect an exe from an extracted sibling directory")
	}
}

func writeAutoTestFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
