package kirikiri2

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsEngineDirectExeDoesNotUseNestedFilesInParent(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "Goodbye Tired Stars 1.05.exe")
	writeKiriTestFile(t, exe, "")

	otherGame := filepath.Join(root, "other-game")
	if err := os.MkdirAll(otherGame, 0o755); err != nil {
		t.Fatalf("create other game dir: %v", err)
	}
	writeKiriTestFile(t, filepath.Join(otherGame, "data.xp3"), "")
	writeKiriTestFile(t, filepath.Join(otherGame, "startup.tjs"), "")

	eng := New()
	if eng.IsEngine(exe) {
		t.Fatalf("direct exe detection should not use nested files elsewhere in the parent directory")
	}
}

func TestIsEngineDirectExeUsesSiblingKiriKiriFiles(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "Game.exe")
	writeKiriTestFile(t, exe, "")
	sourceDir := filepath.Join(root, "source")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatalf("create source dir: %v", err)
	}
	writeKiriTestFile(t, filepath.Join(sourceDir, "startup.tjs"), "")
	if err := createXP3FromFolder(filepath.Join(root, "data.xp3"), sourceDir, false); err != nil {
		t.Fatalf("create fixture xp3: %v", err)
	}

	eng := New()
	if !eng.IsEngine(exe) {
		t.Fatalf("expected direct exe with sibling KiriKiri files to be detected")
	}
}

func TestIsEngineReturnsFalseWhenXP3CannotExtract(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "Game.exe")
	writeKiriTestFile(t, exe, "")
	writeKiriTestFile(t, filepath.Join(root, "startup.tjs"), "")
	writeKiriTestFile(t, filepath.Join(root, "data.xp3"), "")

	eng := New()
	if eng.IsEngine(exe) {
		t.Fatalf("expected direct exe with invalid XP3 archive not to be detected")
	}
}

func writeKiriTestFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
