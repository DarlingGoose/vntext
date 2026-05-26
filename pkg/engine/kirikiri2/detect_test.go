package kirikiri2

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DarlingGoose/krkrxp3/pkg/xp3"
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

func TestIsEngineDirectExeUsesEncryptedSiblingXP3(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "Game.exe")
	writeKiriTestFile(t, exe, "")

	writer, err := xp3.CreateWriter(filepath.Join(root, "data.xp3"))
	if err != nil {
		t.Fatalf("create xp3 writer: %v", err)
	}
	if err := writer.Add(startupFileName, scrambleKiriKiriTextMode0(`Scripts.execStorage("system/Initialize.tjs");`), "neko_vol0", 0); err != nil {
		t.Fatalf("add encrypted startup: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close xp3 writer: %v", err)
	}

	eng := New()
	if !eng.IsEngine(exe) {
		t.Fatalf("expected direct exe with encrypted sibling KiriKiri XP3 to be detected")
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

func TestCanExtractProtectedXP3AllowsMarkerOnlyArchives(t *testing.T) {
	check := xp3.Extractability{
		Protected:      true,
		ProtectedPaths: []string{"This is a protected archive.txt"},
	}

	if !canExtractProtectedXP3(check) {
		t.Fatalf("protected marker-only archive should be usable")
	}

	check.Encrypted = true
	if canExtractProtectedXP3(check) {
		t.Fatalf("protected encrypted archive without a matching encryption type should not be usable")
	}
}

func writeKiriTestFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
